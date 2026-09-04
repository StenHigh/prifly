#!/bin/sh
# A local stand-in for one GitHub release: an archive, and the manifest naming
# the bytes it must have. The installer must accept the first and refuse the
# second, which differs from what the manifest published.
set -eu
root=$(mktemp -d "${TMPDIR:-/tmp}/prifly-install-test.XXXXXX")
trap 'rm -rf "$root"' 0
release="$root/release"
mkdir -p "$release" "$root/bin" "$root/build"
printf '#!/bin/sh\necho fixture\n' > "$root/build/prifly"
chmod 755 "$root/build/prifly"
case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo skip; exit 0 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo skip; exit 0 ;; esac
archive="prifly-$os-$arch.tar.gz"
tar -czf "$release/$archive" -C "$root/build" prifly
# The manifest records the bare hex digest, exactly as release.Build writes it.
digest=$(shasum -a 256 "$release/$archive" | cut -d' ' -f1)
printf '{"schema_version":"prifly-release-manifest/1","version":"9.9.9","stable":true,"assets":[{"os":"%s","arch":"%s","archive":"%s","binary":"prifly","sha256":"%s"}]}\n' \
  "$os" "$arch" "$archive" "$digest" > "$release/release-manifest.json"

PRIFLY_RELEASE_BASE="file://$release" PRIFLY_INSTALL_DIR="$root/bin" sh scripts/install.sh >/dev/null
test -x "$root/bin/prifly" || { echo "the matching archive was not installed"; exit 1; }

# Now publish different bytes under the same name; the digest no longer matches.
printf '#!/bin/sh\necho tampered\n' > "$root/build/prifly"
tar -czf "$release/$archive" -C "$root/build" prifly
rm -f "$root/bin/prifly"
if PRIFLY_RELEASE_BASE="file://$release" PRIFLY_INSTALL_DIR="$root/bin" sh scripts/install.sh >/dev/null 2>"$root/error"; then
  echo "a tampered archive was installed"
  exit 1
fi
grep -q "does not match the digest" "$root/error" || { echo "refusal did not name the digest: $(cat "$root/error")"; exit 1; }
test ! -e "$root/bin/prifly" || { echo "a refused install still wrote the binary"; exit 1; }
echo "install verification: passed"
