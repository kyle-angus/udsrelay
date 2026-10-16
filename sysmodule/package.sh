#!/usr/bin/env bash
# Packages the sysmodule setup as a zip whose contents go straight onto a console's SD card:
#
#   luma/sysmodules/0004013000F00D02.cxi   the udsrelay sysmodule
#   luma/sysmodules/sm.cxi                 Luma's sm with the redirect patch
#   luma/sysmodules/pm.cxi                 Luma's pm, which starts the sysmodule at boot
#   udsrelay/shim.ini                      settings for the chosen mode
#
# Unzip it and copy the luma and udsrelay folders onto the root of the SD card, MERGING them
# with the folders already there (Windows Explorer and FTP clients merge; in macOS Finder hold
# Option while dropping and choose Merge, never Replace, which deletes the existing luma folder
# with Luma's settings). Luma's "Enable loading external FIRMs and modules" option must be on
# (hold Select while powering on), and the plugin loader off.
#
#   sysmodule/package.sh relay --relay 203.0.113.7:28300 [--lobby-file FILE] [options]
#   sysmodule/package.sh trace [options]
#
# relay   play over the relay; needs the relay address and the lobby key. The key is read from
#         --lobby-file, else $UDSRELAY_LOBBY, else asked for (not echoed), so it stays out of
#         shell history. The zip contains the key: share it only with the people in the lobby.
# trace   forward and log real local wireless, for recording a game on two consoles side by side
#         (sysmodule/RUNBOOK.md, "Trace mode"); no relay or key needed.
#
# Options:
#   --payload-max N   bytes of each frame to log (default 64; -1 logs whole frames)
#   --no-build        package sysmodule/out as it is instead of building first
#   -o FILE           output zip (default sysmodule/out/udsrelay-sd-<mode>.zip)
#   --force           overwrite an existing output zip
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$HERE/out"
TITLE_CXI="0004013000F00D02.cxi"

die() { echo "package: $*" >&2; exit 1; }
usage() { sed -n '2,29p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

mode="${1:-}"
[[ "$mode" == relay || "$mode" == trace ]] || usage
shift

relay="" lobby_file="" payload_max=64 build=1 output="" force=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --relay) relay="${2:?--relay needs host:port}"; shift 2 ;;
    --lobby-file) lobby_file="${2:?--lobby-file needs a path}"; shift 2 ;;
    --payload-max) payload_max="${2:?--payload-max needs a number}"; shift 2 ;;
    --no-build) build=0; shift ;;
    -o) output="${2:?-o needs a path}"; shift 2 ;;
    --force) force=1; shift ;;
    -h|--help) usage ;;
    *) die "unknown option $1 (see --help)" ;;
  esac
done

[[ "$payload_max" =~ ^-?[0-9]+$ ]] || die "--payload-max must be a number, got $payload_max"
output="${output:-$OUT_DIR/udsrelay-sd-$mode.zip}"
[[ "$output" == *.zip ]] || die "output must end in .zip: $output"
if [[ -e "$output" && $force -eq 0 ]]; then
  die "$output exists; pass --force to replace it"
fi

lobby=""
if [[ "$mode" == relay ]]; then
  [[ -n "$relay" ]] || die "relay mode needs --relay host:port"
  # Same rule as shim/core's config parser: a host, a colon, a port from 1 to 65535.
  [[ "$relay" =~ ^[^:[:space:]]+:([0-9]+)$ ]] || die "--relay must be host:port, got $relay"
  port="${BASH_REMATCH[1]}"
  (( port >= 1 && port <= 65535 )) || die "relay port out of range: $port"

  if [[ -n "$lobby_file" ]]; then
    [[ -r "$lobby_file" ]] || die "cannot read $lobby_file"
    lobby="$(head -n 1 "$lobby_file")"
  elif [[ -n "${UDSRELAY_LOBBY:-}" ]]; then
    lobby="$UDSRELAY_LOBBY"
  elif [[ -t 0 ]]; then
    read -r -s -p "Lobby key: " lobby
    echo
  fi
  lobby="${lobby//$'\r'/}"
  [[ -n "$lobby" ]] || die "relay mode needs the lobby key (--lobby-file, \$UDSRELAY_LOBBY, or type it when asked)"
  (( ${#lobby} <= 64 )) || die "lobby key is longer than 64 characters"
  [[ "$lobby" != *[[:space:]#]* ]] || die "lobby key must not contain spaces or #"
fi

if [[ $build -eq 1 ]]; then
  echo "building..."
  build_log="$(mktemp "${TMPDIR:-/tmp}/udsrelay-build.XXXXXX")"
  if ! "$HERE/build.sh" >"$build_log" 2>&1; then
    cat "$build_log" >&2
    rm -f "$build_log"
    die "build failed"
  fi
  rm -f "$build_log"
fi
for f in "$TITLE_CXI" sm.cxi pm.cxi; do
  [[ -f "$OUT_DIR/$f" ]] || die "missing $OUT_DIR/$f; run without --no-build"
done

stage="$(mktemp -d "${TMPDIR:-/tmp}/udsrelay-sd.XXXXXX")"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/luma/sysmodules" "$stage/udsrelay"
cp "$OUT_DIR/$TITLE_CXI" "$OUT_DIR/sm.cxi" "$OUT_DIR/pm.cxi" "$stage/luma/sysmodules/"

version="$(sed -n 's/^#define VERSION "\(.*\)"$/\1/p' "$HERE/source/main.c")"
{
  echo "# udsrelay sysmodule $version settings, written by sysmodule/package.sh on $(date -u +%Y-%m-%dT%H:%MZ)."
  if [[ "$mode" == relay ]]; then
    echo "# Relay mode: games' local wireless goes over the relay. Keep this file private: it holds"
    echo "# the lobby key."
    echo "mode = relay"
    echo "relay = $relay"
    echo "lobby = $lobby"
  else
    echo "# Trace mode: real local wireless, forwarded and logged (consoles side by side)."
    echo "mode = trace"
  fi
  echo
  echo "# Bytes of each frame to log; -1 logs whole frames."
  echo "payload_max = $payload_max"
} > "$stage/udsrelay/shim.ini"
chmod 600 "$stage/udsrelay/shim.ini"

mkdir -p "$(dirname "$output")"
rm -f "$output"
(cd "$stage" && zip -qrX "$output" luma udsrelay)
chmod 600 "$output"

echo "wrote $output (sysmodule $version, mode $mode)"
unzip -l "$output" | sed -n '4,$p' | sed '$d' | sed '$d' | awk '{print "  " $4}'
echo
if [[ "$mode" == relay ]]; then expect="serving uds:rly and ndm:rly"; else expect="in trace mode"; fi
echo "On each console: unzip, then copy luma/ and udsrelay/ onto the SD card root, MERGING with the"
echo "folders already there (in macOS Finder hold Option and choose Merge; Replace would delete the"
echo "existing luma folder). Luma's \"Enable loading external FIRMs and modules\" must be on and the"
echo "plugin loader off. After a reboot, the new sdmc:/udsrelay/logs/sysmodule-*.log should say \"$expect\"."
if [[ "$mode" == relay ]]; then
  echo "The zip contains the lobby key; share it only with people in the lobby."
fi
