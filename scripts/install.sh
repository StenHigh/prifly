#!/bin/sh
# The first install trusts the official GitHub HTTPS release asset. It writes
# only the selected user directory and intentionally does not edit shell files.
set -eu

release_base='https://github.com/StenHigh/prifly/releases/latest/download'
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
