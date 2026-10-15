#!/usr/bin/env bash
# Builds udsrelay-shim.3gx. devkitPro's make rules break on paths with spaces, so this copies
# shim/core and shim/plugin to a space-free staging directory, builds there, and copies the
# result back to shim/plugin/udsrelay-shim.3gx.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHIM="$(dirname "$HERE")"
STAGE="${UDSRELAY_STAGE:-/tmp/udsrelay-shim-build}"

case "$STAGE" in
  *" "*) echo "build: UDSRELAY_STAGE must not contain spaces: $STAGE" >&2; exit 1 ;;
esac
: "${DEVKITPRO:?set DEVKITPRO (for example /opt/devkitpro)}"
: "${DEVKITARM:?set DEVKITARM (for example /opt/devkitpro/devkitARM)}"
command -v 3gxtool >/dev/null || { echo "build: 3gxtool not on PATH" >&2; exit 1; }
[[ -d "$DEVKITPRO/libctrpf/include" ]] || { echo "build: libctrpf not installed in $DEVKITPRO/libctrpf" >&2; exit 1; }

mkdir -p "$STAGE"
rsync -a --delete --exclude build --exclude '*.3gx' --exclude '*.elf' "$SHIM/core/" "$STAGE/core/"
rsync -a --delete --exclude build --exclude '*.3gx' --exclude '*.elf' "$SHIM/plugin/" "$STAGE/plugin/"
make -C "$STAGE/plugin" "$@"
cp "$STAGE/plugin/udsrelay-shim.3gx" "$HERE/udsrelay-shim.3gx"
echo "built $HERE/udsrelay-shim.3gx"
