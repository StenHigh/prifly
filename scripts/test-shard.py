#!/usr/bin/env python3
"""Split one package's tests into shards for parallel CI jobs.

Each test lands in exactly one shard, chosen by a stable hash of its name, so a
shard's content does not move when an unrelated test is added. `--check` proves
the split covers the package: a test that fell out of every shard would be a
check silently skipped, and a green run would say nothing about it.
"""

import argparse
import hashlib
import re
import subprocess
import sys


def tests(package, go):
    out = subprocess.run([go, "test", "-list", ".", package], check=True, capture_output=True, text=True).stdout
    names = [line for line in out.split() if re.fullmatch(r"(Test|Fuzz|Example)\w*", line)]
    if not names:
        sys.exit(f"test-shard: {package} lists no tests; a split of nothing checks nothing")
    return sorted(set(names))


def shard_of(name, shards):
    return int(hashlib.sha256(name.encode()).hexdigest(), 16) % shards


def pattern(names):
    return "^(" + "|".join(re.escape(name) for name in names) + ")$"


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--package", default="./internal/runtime")
    parser.add_argument("--go", default="go")
    parser.add_argument("--shards", type=int, required=True)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--shard", type=int, help="print the -run pattern of this shard (0-based)")
    group.add_argument("--check", action="store_true", help="prove the shards cover every test exactly once")
    args = parser.parse_args()
    if args.shards < 1:
        sys.exit("test-shard: --shards must be at least 1")

    names = tests(args.package, args.go)
    buckets = [[] for _ in range(args.shards)]
    for name in names:
        buckets[shard_of(name, args.shards)].append(name)

    if args.check:
        covered = [name for bucket in buckets for name in bucket]
        matched = {name for index, bucket in enumerate(buckets) for name in names if bucket and re.fullmatch(pattern(bucket), name)}
        if sorted(covered) != names or matched != set(names):
            missing = sorted(set(names) - matched)
            sys.exit(f"test-shard: {len(missing)} tests of {args.package} fall in no shard: {', '.join(missing[:10])}")
        sizes = ", ".join(str(len(bucket)) for bucket in buckets)
        print(f"test-shard: {len(names)} tests of {args.package} in {args.shards} shards ({sizes}), each exactly once")
        return 0

    if not 0 <= args.shard < args.shards:
        sys.exit(f"test-shard: --shard must be in 0..{args.shards - 1}")
    bucket = buckets[args.shard]
    # An empty shard would match every test with an empty pattern; say so instead.
    print(pattern(bucket) if bucket else "^$")
    return 0


if __name__ == "__main__":
    sys.exit(main())
