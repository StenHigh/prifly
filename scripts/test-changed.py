#!/usr/bin/env python3
"""Run the tests an edit can affect, for use while developing.

A package whose Go files changed runs with every package that imports it,
directly or not. Some tests open files by path instead of importing them --
schemas, examples, the glossary -- and choosing by name misses exactly those,
so a changed file outside a package is mapped to the packages whose tests read
it. A change the map cannot reason about (build files, CI, go.mod) runs all.

This is the everyday check. It never replaces the full gate before a commit is
released: `make ci-check`, then `make qualify`.
"""

import argparse
import json
import subprocess
import sys

MODULE = "github.com/stenhigh/prifly"

# Directory prefix -> packages whose tests read files under it by path.
READ_BY_PATH = {
    "schemas/": ["./internal/runtime", "./internal/flow", "./cmd/prifly"],
    "examples/": ["./internal/runtime", "./internal/flow", "./cmd/prifly"],
    "test/fixtures/": ["./internal/runtime", "./internal/flow", "./cmd/prifly"],
    "openspec/specs/": ["./internal/runtime"],
}
# A change here decides how everything is built or checked.
RUN_ALL = ("Makefile", "go.mod", "go.sum", ".github/", "scripts/")
# Read by no test.
IGNORED = ("openspec/changes/", "README", "AGENTS.md", "CLAUDE.md", "CONTEXT_STATE.md", ".ai-factory/", ".claude/", ".agents/", "assets/", "docs/")


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout


def changed_files(base):
    merge_base = run("git", "merge-base", base, "HEAD").strip()
    names = set(run("git", "diff", "--name-only", merge_base).split())
    names |= set(run("git", "ls-files", "--others", "--exclude-standard").split())
    return sorted(names)


def packages(go="go"):
    """Every package of the module with the packages its code and tests import."""
    out = run(go, "list", "-json", "./...")
    decoder, index, result = json.JSONDecoder(), 0, {}
    while index < len(out):
        while index < len(out) and out[index].isspace():
            index += 1
        if index >= len(out):
            break
        pkg, index = decoder.raw_decode(out, index)
        imports = set(pkg.get("Imports", [])) | set(pkg.get("TestImports", [])) | set(pkg.get("XTestImports", []))
        result[pkg["ImportPath"]] = {"dir": pkg["Dir"], "imports": imports}
    return result


def choose(files, graph, root, report=lambda *_: None):
    """The go test command and extra steps for these changed files."""
    by_dir = {info["dir"][len(root) + 1:]: path for path, info in graph.items()}
    selected, reasons, extra, everything = set(), {}, [], None

    for name in files:
        if name.startswith(RUN_ALL) or name in RUN_ALL:
            everything = name
            continue
        if name.startswith(IGNORED):
            continue
        directory = name.rsplit("/", 1)[0] if "/" in name else ""
        if directory in by_dir:
            selected.add(by_dir[directory])
            reasons.setdefault(by_dir[directory], name)
            if name.endswith("_test.go"):
                selected.add(MODULE + "/internal/testguard")
                reasons.setdefault(MODULE + "/internal/testguard", name)
            if name.startswith("cmd/prifly/monitor") and not name.endswith(".go"):
                extra.append(["node", "cmd/prifly/monitor_ui_test.cjs"])
            continue
        matched = False
        for prefix, targets in READ_BY_PATH.items():
            if name.startswith(prefix):
                matched = True
                for target in targets:
                    path = MODULE + target[1:]
                    selected.add(path)
                    reasons.setdefault(path, name)
                if prefix == "schemas/":
                    extra.append(["make", "schemas-check"])
        if not matched:
            everything = everything or name

    if everything:
        report(f"test-changed: {everything} decides how everything is built or checked -> all packages")
        command = ["go", "test", "./..."]
    else:
        # Everything that imports a selected package, directly or through others.
        closure = set(selected)
        grew = True
        while grew:
            grew = False
            for path, info in graph.items():
                if path not in closure and info["imports"] & closure:
                    closure.add(path)
                    reasons.setdefault(path, "imports " + next(iter(info["imports"] & closure)))
                    grew = True
        for path in sorted(closure):
            report(f"  {path[len(MODULE) + 1:] or '.'}  <- {reasons.get(path, '')}")
        if not closure and not extra:
            report("test-changed: the changes are read by no test")
            return None, []
        command = ["go", "test"] + ["./" + path[len(MODULE) + 1:] for path in sorted(closure)] if closure else []

    return command, extra


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--base", default="origin/main")
    parser.add_argument("--go", default="go")
    parser.add_argument("--dry-run", action="store_true", help="print the selection without running it")
    args = parser.parse_args()

    root = run("git", "rev-parse", "--show-toplevel").strip()
    files = changed_files(args.base)
    print(f"test-changed: {len(files)} changed files against {args.base}")
    if not files:
        print("test-changed: nothing changed, nothing to run")
        return 0

    graph = packages(args.go)
    command, extra = choose(files, graph, root, report=print)
    if command is None and not extra:
        return 0
    seen = []
    for step in ([command] if command else []) + extra:
        if step in seen:
            continue
        seen.append(step)
        if step[0] == "go":
            step = [args.go] + step[1:]
        print("test-changed: " + " ".join(step))
        if not args.dry_run:
            code = subprocess.run(step).returncode
            if code:
                return code
    return 0


if __name__ == "__main__":
    sys.exit(main())
