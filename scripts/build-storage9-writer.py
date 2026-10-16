#!/usr/bin/env python3
"""Build the independent writer fixture against the last storage-9 baseline."""
import argparse
import hashlib
import io
import json
import pathlib
import subprocess
import tarfile
import tempfile

BASELINE = "6b4bf5bc50a59abd8e0b821c4d7bc0972c004b18"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    root = pathlib.Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    archive = subprocess.check_output(
        ["git", "archive", BASELINE, "go.mod", "go.sum", "internal/local", "internal/purity"],
        cwd=root,
    )
    with tempfile.TemporaryDirectory(prefix="prifly-storage9-writer-") as directory:
        source = pathlib.Path(directory)
        with tarfile.open(fileobj=io.BytesIO(archive)) as bundle:
            bundle.extractall(source, filter="data")
        target = source / "cmd/storage9-writer"
        target.mkdir(parents=True)
        (target / "main.go").write_bytes((root / "test/fixtures/storage9-writer/main.go").read_bytes())
        subprocess.run([args.go, "build", "-o", str(output), "./cmd/storage9-writer"], cwd=source, check=True)
    print(json.dumps({"storage_version": 9, "source_commit": BASELINE, "binary": str(output),
                      "sha256": hashlib.sha256(output.read_bytes()).hexdigest()}))


if __name__ == "__main__":
    main()
