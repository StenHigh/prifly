#!/usr/bin/env python3
"""Package a private local build with exact provenance. Packaging is not qualification."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
LICENSES = {
    "github.com/cyberphone/json-canonicalization": "Apache-2.0",
    "github.com/mattn/go-sqlite3": "MIT",
    "github.com/santhosh-tekuri/jsonschema/v6": "Apache-2.0",
    "go.yaml.in/yaml/v3": "MIT AND Apache-2.0",
    "golang.org/x/text": "BSD-3-Clause",
}


def digest(content):
    return hashlib.sha256(content).hexdigest()


def run(argv, **kwargs):
    return subprocess.check_output([str(x) for x in argv], cwd=ROOT, **kwargs)


def objects(content):
    decoder = json.JSONDecoder()
    rest = content.decode()
    while rest.strip():
        value, end = decoder.raw_decode(rest.lstrip())
        yield value
        rest = rest.lstrip()[end:]


def source_manifest(root):
    source = {}
    for directory in ("cmd", "internal", "examples", "test"):
        for path in sorted((root / directory).rglob("*")):
            if path.is_file() and path.suffix in (".go", ".json"):
                source[str(path.relative_to(root))] = digest(path.read_bytes())
    for name in ("go.mod", "go.sum", "Makefile", "scripts/release.py", "scripts/check-schema.py"):
        source[name] = digest((root / name).read_bytes())
    return source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default=str(ROOT / ".tools/go/bin/go"))
    parser.add_argument("--binary", default="bin/prifly")
    args = parser.parse_args()
    source = source_manifest(ROOT)
    binary = (ROOT / args.binary).resolve()
    content = binary.read_bytes()
    identity = json.loads(run([binary, "version", "--json"]))
    run([args.go, "mod", "verify"])
    goenv = json.loads(run([args.go, "env", "-json", "GOVERSION", "GOROOT", "GOOS", "GOARCH", "CGO_ENABLED", "CC"]))
    if (identity["os"], identity["arch"]) != (goenv["GOOS"], goenv["GOARCH"]):
        raise SystemExit("This command packages a native build, not an untested cross-compile")
    name = f"prifly-{identity['version']}-{identity['os']}-{identity['arch']}-{digest(content)[:12]}"
    output = ROOT / "dist" / name
    if output.exists():
        raise SystemExit(f"Refusing to overwrite {output}; retain the previous evidence")

    # Compare two clean invocations with the same flags, not a claimed portable
    # reproducibility guarantee across different C compilers or operating systems.
    with tempfile.TemporaryDirectory(prefix="prifly-release-") as scratch:
        rebuilt = Path(scratch) / "prifly"
        subprocess.run([args.go, "build", "-trimpath", "-buildvcs=false", "-o", str(rebuilt), "./cmd/prifly"], cwd=ROOT, check=True)
        if rebuilt.read_bytes() != content:
            raise SystemExit("Binary does not match the current source/toolchain build; rerun make build")
        installation = Path(scratch) / "empty-install"
        bare_environment = {"PATH": "/prifly-no-external-tools"}
        run([binary, "init", installation, "--json"], env=bare_environment)
        doctor = json.loads(run([binary, "--project", installation, "doctor", "--json"], env=bare_environment))

    build_info = run([args.go, "version", "-m", binary]).decode().splitlines()
    modules = []
    for line in build_info:
        parts = line.split()
        if parts and parts[0] == "=>":
            raise SystemExit("Review replacement modules before packaging; their provenance is not supported by this manifest")
        if parts and parts[0] == "dep":
            modules.append(parts[1])
    metadata = list(objects(run([args.go, "list", "-m", "-json", *modules])))
    if source_manifest(ROOT) != source:
        raise SystemExit("Source changed during the build; freeze edits and rebuild before packaging")
    source_bytes = json.dumps(source, sort_keys=True, separators=(",", ":")).encode()
    revision, dirty = None, None
    if shutil.which("git"):
        discovery = subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=ROOT, capture_output=True, text=True)
        if discovery.returncode == 0 and Path(discovery.stdout.strip()).resolve() == ROOT:
            head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True)
            if head.returncode == 0:
                revision = head.stdout.strip()
                dirty = bool(run(["git", "status", "--porcelain"]))

    output.mkdir(parents=True)
    (output / "prifly").write_bytes(content)
    (output / "prifly").chmod(0o755)
    packaged_source = output / "source"
    packaged_source.mkdir()
    for name_in_source in ("README.md", "SECURITY.md", "AGENTS.md", "assets", "go.mod", "go.sum", "Makefile", "cmd", "internal", "scripts", "examples", "test", "schemas", "openspec"):
        original = ROOT / name_in_source
        if original.is_symlink() or original.is_dir() and any(p.is_symlink() for p in original.rglob("*")):
            raise SystemExit(f"Refusing to follow source symlinks while packaging: {name_in_source}")
        if original.is_dir():
            shutil.copytree(original, packaged_source / name_in_source, ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
        else:
            shutil.copy2(original, packaged_source / name_in_source)
    if source_manifest(ROOT) != source or source_manifest(packaged_source) != source:
        raise SystemExit("Source changed during packaging; do not publish this incomplete directory")
    notices = output / "third-party"
    notices.mkdir()
    inventory = []
    for module in metadata:
        path = module["Path"]
        if path not in LICENSES:
            raise SystemExit(f"Review the license before packaging a new dependency: {path}")
        directory = Path(module["Dir"])
        target = notices / path.replace("/", "_")
        target.mkdir()
        files = []
        for license_file in sorted(directory.iterdir()):
            if license_file.is_file() and license_file.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE", "PATENTS")):
                shutil.copy2(license_file, target / license_file.name)
                files.append(str((target / license_file.name).relative_to(output)))
        if not files:
            raise SystemExit(f"Missing license text for {path}")
        inventory.append({"module": path, "version": module["Version"], "checksum": module.get("Sum"), "license": LICENSES[path], "notices": files})
        if path == "github.com/mattn/go-sqlite3":
            amalgamation = (directory / "sqlite3-binding.c").read_text()
            start = amalgamation.index("** 2001 September 15")
            end = amalgamation.index("** Internal interface definitions for SQLite.", start)
            (notices / "sqlite-dedication.txt").write_text(amalgamation[start:end])
    for filename in ("LICENSE", "PATENTS"):
        shutil.copy2(Path(goenv["GOROOT"]) / filename, notices / ("Go-" + filename))

    manifest = {
        "schema_version": "private-build-manifest/1",
        "release_status": "candidate; consult matching qualification evidence",
        "binary": {"path": "prifly", "sha256": digest(content), "size_bytes": len(content), **identity},
        "source": {"directory": "source", "git_revision": revision, "dirty": dirty, "input_manifest_sha256": digest(source_bytes), "files": source},
        "build": {"go": goenv["GOVERSION"], "os": goenv["GOOS"], "arch": goenv["GOARCH"], "cgo_enabled": goenv["CGO_ENABLED"], "compiler": run([goenv["CC"], "--version"]).decode().splitlines()[0], "host_system": platform.system(), "host_release": platform.release(), "flags": ["-trimpath", "-buildvcs=false"], "same_environment_rebuild_identical": True},
        "go_build_info": build_info[1:],
        "linked_modules": inventory,
        "module_cache_verified": True,
        "standard_library_license": "BSD-3-Clause; Go-LICENSE and Go-PATENTS included",
        "sqlite_notice": "Public-domain dedication in third-party/sqlite-dedication.txt",
        "project_license": "not declared; private development build, no public license grant inferred",
        "doctor": doctor,
        "empty_install_environment": bare_environment,
    }
    (output / "build-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    files = {str(p.relative_to(output)): digest(p.read_bytes()) for p in sorted(output.rglob("*")) if p.is_file()}
    (output / "SHA256SUMS").write_text("".join(f"{checksum}  {path}\n" for path, checksum in files.items()))
    archive = output.with_name(output.name + ".tar.gz")
    with archive.open("xb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as compressed, tarfile.open(fileobj=compressed, mode="w") as bundle:
        for path in sorted(output.rglob("*")):
            if not path.is_file():
                continue
            data = path.read_bytes()
            info = tarfile.TarInfo(str(Path(output.name) / path.relative_to(output)))
            info.size, info.mtime, info.mode = len(data), 0, 0o755 if path.stat().st_mode & 0o111 else 0o644
            bundle.addfile(info, io.BytesIO(data))
    archive.with_name(archive.name + ".sha256").write_text(f"{digest(archive.read_bytes())}  {archive.name}\n")
    print(json.dumps({"directory": str(output), "archive": str(archive), "binary_sha256": digest(content), "source_digest": digest(source_bytes)}, indent=2))


if __name__ == "__main__":
    main()
