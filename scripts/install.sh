#!/bin/sh
# The first install trusts the official GitHub HTTPS release asset. It writes
# only the selected user directory and intentionally does not edit shell files.
set -eu

# The release assets are read over HTTPS from GitHub, which is the whole trust
# boundary of a first install: nothing here is signed by a key this machine
# already has. PRIFLY_RELEASE_BASE exists so this script can be tested against a
# local directory; pointing it elsewhere is the caller's own decision.
release_base=${PRIFLY_RELEASE_BASE:-'https://github.com/StenHigh/prifly/releases/latest/download'}
destination=${PRIFLY_INSTALL_DIR:-"${HOME:?HOME is required}/.local/bin"}

case "$(uname -s)" in
  Darwin) target_os=darwin ;;
  Linux) target_os=linux ;;
  *) echo "Pri-Fly has no release asset for $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) target_arch=arm64 ;;
  x86_64|amd64) target_arch=amd64 ;;
  *) echo "Pri-Fly has no release asset for $(uname -m)" >&2; exit 1 ;;
esac

archive="prifly-${target_os}-${target_arch}.tar.gz"
temporary=$(mktemp -d "${TMPDIR:-/tmp}/prifly-install.XXXXXX")
staged_binary="$destination/.prifly-new.$$"
staged_receipt="$destination/.prifly-receipt-new.$$"
cleanup() { rm -rf "$temporary"; rm -f "$staged_binary" "$staged_receipt"; }
trap cleanup 0 HUP INT TERM

mkdir -p "$destination"
if [ ! -d "$destination" ] || [ ! -w "$destination" ]; then
  echo "Pri-Fly cannot write to $destination" >&2
  exit 1
fi

curl --fail --location --silent --show-error "$release_base/$archive" -o "$temporary/$archive"

# The manifest of the same release names the bytes each asset must have. It is
# fetched over the same HTTPS connection, so it proves nothing about the origin
# of the release; what it does catch is an archive that is not the one this
# release published - a truncated download, a stale mirror, a swapped asset.
curl --fail --location --silent --show-error "$release_base/release-manifest.json" -o "$temporary/release-manifest.json"
# The digest is read without a JSON parser this machine may not have: the
# manifest is one line, each asset is one object, so splitting on the opening
# brace puts every asset on its own line whatever order its keys are written in.
expected=$(tr -d ' \n' < "$temporary/release-manifest.json" | tr '{' '\n' |
  grep '"archive":"'"$archive"'"' |
  sed -n 's/.*"sha256":"\([0-9a-f]\{64\}\)".*/\1/p' | head -1)
case "$expected" in
  ????????????????????????????????????????????????????????????????) ;;
  *) echo "Pri-Fly release manifest does not name $archive" >&2; exit 1 ;;
esac
if command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$temporary/$archive" | cut -d' ' -f1)
elif command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary/$archive" | cut -d' ' -f1)
else
  echo "Pri-Fly install needs shasum or sha256sum to verify the download" >&2
  exit 1
fi
if [ "$actual" != "$expected" ]; then
  echo "Pri-Fly release archive does not match the digest this release published" >&2
  exit 1
fi

members=$(tar -tzf "$temporary/$archive")
if [ "$members" != prifly ]; then
  echo "Pri-Fly release archive has an unsafe layout" >&2
  exit 1
fi
tar -xOzf "$temporary/$archive" prifly > "$temporary/prifly"
if [ ! -s "$temporary/prifly" ]; then
  echo "Pri-Fly release archive has no executable" >&2
  exit 1
fi
chmod 755 "$temporary/prifly"
cp "$temporary/prifly" "$staged_binary"
chmod 755 "$staged_binary"
printf '%s\n' '{"schema_version":"prifly-managed-install/1","binary":"prifly","channel":"stable"}' > "$staged_receipt"
mv -f "$staged_receipt" "$destination/.prifly-installation.json"
mv -f "$staged_binary" "$destination/prifly"

echo "Pri-Fly installed to $destination/prifly"
case ":$PATH:" in
  *":$destination:"*) ;;
  *) echo "Add $destination to PATH, then run: prifly --help" ;;
esac
