#!/usr/bin/env python3
"""Keep the tagged-release publication contract explicit in GitHub Actions."""

from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / ".github" / "workflows"


def job_section(text: str, name: str) -> str:
    """Return the YAML block of one top-level job, up to the next job key."""
    marker = f"\n  {name}:\n"
    start = text.index(marker) + len(marker)
    lines = text[start:].split("\n")
    body = []
    for line in lines:
        if line and not line.startswith("   ") and line.startswith("  ") and line.rstrip().endswith(":"):
            break
        if line and not line.startswith(" "):
            break
        body.append(line)
    return "\n".join(body)


def main() -> int:
    release = (WORKFLOWS / "release.yml").read_text(encoding="utf-8")
    verify = (WORKFLOWS / "verify.yml").read_text(encoding="utf-8")
    required = (
        'tags: ["v[0-9]+.[0-9]+.[0-9]+"]',
        "permissions:\n  contents: read",
        "build-linux-amd64:",
        "build-darwin-arm64:",
        "runs-on: macos-14",
        'test "$(uname -s)" = Darwin',
        'test "$(uname -m)" = arm64',
        "CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build",
        "CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build",
        "release-inputs/prifly-linux-amd64",
        "release-inputs/prifly-darwin-arm64",
        "--asset linux/amd64=release-inputs/prifly-linux-amd64",
        "--asset darwin/arm64=release-inputs/prifly-darwin-arm64",
        "--installer scripts/install.sh",
        "prifly-linux-amd64.tar.gz",
        "prifly-darwin-arm64.tar.gz",
        "release-manifest.json",
        "release-manifest.sig",
        "release-manifest.jcs.sig",
        'test -n "${PRIFLY_RELEASE_SIGNING_KEY:-}"',
        'test -n "${PRIFLY_RELEASE_PUBLIC_KEY:-}"',
        "secrets.PRIFLY_RELEASE_SIGNING_KEY",
        "vars.PRIFLY_RELEASE_PUBLIC_KEY",
        "environment: release",
        "GH_TOKEN: ${{ github.token }}",
        'gh release create "$GITHUB_REF_NAME" --verify-tag',
    )
    missing = [item for item in required if item not in release]
    if missing:
        print("release CI contract is incomplete:", *missing, sep="\n- ", file=sys.stderr)
        return 1
    if release.count("contents: write") != 1:
        print("exactly one job may hold contents: write", file=sys.stderr)
        return 1
    publish = job_section(release, "release")
    publish_required = (
        "needs: [build-linux-amd64, build-darwin-arm64]",
        "environment: release",
        "contents: write",
        "test -f release-inputs/prifly-linux-amd64",
        "test -f release-inputs/prifly-darwin-arm64",
        "gh release create",
    )
    missing = [item for item in publish_required if item not in publish]
    if missing:
        print("release publication job is incomplete:", *missing, sep="\n- ", file=sys.stderr)
        return 1
    for name in ("build-linux-amd64", "build-darwin-arm64"):
        if "secrets." in job_section(release, name):
            print(f"{name} must build without the signing secret", file=sys.stderr)
            return 1
    if 'tags-ignore: ["**"]' not in verify or "make ci-check" not in verify or "make e2e" not in verify:
        print("verify workflow must run the product gates for branches and skip release tags", file=sys.stderr)
        return 1
    installer = (ROOT / "scripts" / "install.sh").read_text(encoding="utf-8")
    # A first install trusts HTTPS to GitHub and nothing else, so the one thing
    # it can still check is that the archive it received is the archive this
    # release published.
    installer_required = (
        "release-manifest.json",
        "does not match the digest this release published",
        "sha256",
    )
    missing = [item for item in installer_required if item not in installer]
    if missing:
        print("installer does not verify the release archive:", *missing, sep="\n- ", file=sys.stderr)
        return 1
    print("release CI contract is present")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
