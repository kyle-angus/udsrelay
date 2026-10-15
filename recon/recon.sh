#!/usr/bin/env bash
# Phase 1 recon helper: run two patched Azahar instances side by side and turn their logs
# into a UDS command trace. See recon/RUNBOOK.md.
#
#   recon/recon.sh setup                  create recon/run/{host,client}/user
#   recon/recon.sh launch host|client     start an instance with its own user dir
#   recon/recon.sh collect <session>      copy both logs and write timeline + summary
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN="$ROOT/recon/run"
TRACES="$ROOT/recon/traces"
INSTANCES=(host client)

# Path to the patched Azahar executable (not the .app bundle).
AZAHAR="${AZAHAR:-}"

die() { echo "recon: $*" >&2; exit 1; }

cmd_setup() {
  for i in "${INSTANCES[@]}"; do
    mkdir -p "$RUN/$i/user"
    echo "created $RUN/$i/user"
  done
  echo
  echo "Next: launch each instance once, set a distinct username in System settings,"
  echo "and copy any keys/sysdata your setup needs into each user dir (RUNBOOK step 3)."
}

cmd_launch() {
  local inst="${1:-}"
  [[ " ${INSTANCES[*]} " == *" $inst "* ]] || die "launch needs one of: ${INSTANCES[*]}"
  [[ -n "$AZAHAR" ]] || die "set AZAHAR to the patched binary, e.g. AZAHAR=~/src/azahar/build/bin/Release/azahar.app/Contents/MacOS/azahar"
  [[ -x "$AZAHAR" ]] || die "AZAHAR is not executable: $AZAHAR"
  [[ -d "$RUN/$inst/user" ]] || die "run '$0 setup' first"
  # Azahar uses ./user as its user dir when it exists in the working directory, which gives
  # each instance its own config, saves and log/azahar_log.txt. On macOS it first changes to
  # the directory containing its .app bundle, so each instance runs its own copy of the bundle,
  # refreshed on every launch to pick up rebuilds.
  local exe="$AZAHAR"
  if [[ "$AZAHAR" == *.app/Contents/MacOS/* ]]; then
    local bundle="${AZAHAR%/Contents/MacOS/*}"
    local name="$(basename "$bundle")"
    rsync -a --delete "$bundle/" "$RUN/$inst/$name/"
    exe="$RUN/$inst/$name/${AZAHAR#"$bundle"/}"
  fi
  cd "$RUN/$inst"
  echo "launching $inst in $PWD (UDSTRACE_PORTS=${UDSTRACE_PORTS:-default}, UDSTRACE_PAYLOAD_MAX=${UDSTRACE_PAYLOAD_MAX:-512})"
  exec "$exe"
}

cmd_collect() {
  local session="${1:-}"
  [[ "$session" =~ ^[A-Za-z0-9._-]+$ ]] || die "collect needs a session name like trade-01"
  local out="$TRACES/$session"
  [[ ! -e "$out" ]] || die "$out already exists"
  mkdir -p "$out"
  local args=()
  for i in "${INSTANCES[@]}"; do
    local log="$RUN/$i/user/log/azahar_log.txt"
    [[ -f "$log" ]] || die "no log for $i at $log"
    cp "$log" "$out/$i.log"
    args+=("$i=$out/$i.log")
  done
  (cd "$ROOT" && go run ./cmd/udstrace timeline "${args[@]}" > "$out/timeline.txt")
  (cd "$ROOT" && go run ./cmd/udstrace timeline -v "${args[@]}" > "$out/timeline-verbose.txt")
  (cd "$ROOT" && go run ./cmd/udstrace summary -title "$session" -o "$out/summary.md" "${args[@]}")
  echo "wrote $out/{host.log,client.log,timeline.txt,timeline-verbose.txt,summary.md}"
}

case "${1:-}" in
  setup) cmd_setup ;;
  launch) shift; cmd_launch "$@" ;;
  collect) shift; cmd_collect "$@" ;;
  *) sed -n '2,8p' "$0"; exit 2 ;;
esac
