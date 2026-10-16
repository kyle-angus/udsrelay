#!/usr/bin/env bash
# Builds the Phase 6 sysmodule setup into sysmodule/out/, ready for sdmc:/luma/sysmodules/:
#
#   0004013000F00D02.cxi  the udsrelay sysmodule (shim/core's emulator as a background module)
#   sm.cxi                Luma3DS v13.4's sm with luma/sm-redirect.patch: applications asking for
#                         nwm::UDS or ndm:u get uds:rly or ndm:rly while the sysmodule runs
#   pm.cxi                Luma3DS v13.4's pm with luma/pm-autolaunch.patch: starts the sysmodule
#                         at boot
#
# devkitPro's make rules break on spaces in paths, so everything is staged in a space-free
# directory first. Luma3DS is cloned there at the tag the consoles run.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(dirname "$HERE")"
STAGE="${UDSRELAY_STAGE:-/tmp/udsrelay-sysmodule-build}"
LUMA_TAG="v13.4"

case "$STAGE" in
  *" "*) echo "build: UDSRELAY_STAGE must not contain spaces: $STAGE" >&2; exit 1 ;;
esac
: "${DEVKITPRO:?set DEVKITPRO (for example /opt/devkitpro)}"
: "${DEVKITARM:?set DEVKITARM (for example /opt/devkitpro/devkitARM)}"
export PATH="$DEVKITPRO/tools/bin:$DEVKITARM/bin:$PATH"
if ! command -v makerom >/dev/null && [[ -x "$HOME/src/Project_CTR/makerom/bin/makerom" ]]; then
  export PATH="$HOME/src/Project_CTR/makerom/bin:$PATH"
fi
command -v makerom >/dev/null || { echo "build: makerom not on PATH (see sysmodule/README.md)" >&2; exit 1; }

mkdir -p "$STAGE" "$HERE/out"
rsync -a --delete --exclude build "$REPO/shim/core/" "$STAGE/core/"
rsync -a --delete --exclude build --exclude out --exclude luma "$HERE/" "$STAGE/sysmodule/"

# Luma's sm and pm, patched.
if [[ ! -d "$STAGE/luma/.git" ]]; then
  git clone --quiet --depth 1 -b "$LUMA_TAG" https://github.com/LumaTeam/Luma3DS.git "$STAGE/luma"
fi
git -C "$STAGE/luma" checkout --quiet -- sysmodules/sm sysmodules/pm
git -C "$STAGE/luma" apply "$HERE/luma/sm-redirect.patch" "$HERE/luma/pm-autolaunch.patch"

make -C "$STAGE/sysmodule" "$@"
make -C "$STAGE/luma/sysmodules/sm"
make -C "$STAGE/luma/sysmodules/pm"

cp "$STAGE/sysmodule/udsrelay.cxi" "$HERE/out/0004013000F00D02.cxi"
cp "$STAGE/luma/sysmodules/sm/sm.cxi" "$HERE/out/sm.cxi"
cp "$STAGE/luma/sysmodules/pm/pm.cxi" "$HERE/out/pm.cxi"
ls -l "$HERE/out"
