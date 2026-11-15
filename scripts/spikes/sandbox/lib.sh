#!/usr/bin/env bash
# Common helpers for the MORN1NGST4R sandbox feasibility spike. Source this file; do not execute it.
# Everything it creates lives in $MS_SCRATCH (/tmp/ms-spike-sandbox-*) and is removed on exit.

export PYTHONDONTWRITEBYTECODE=1
SPIKE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SB_PY="$SPIKE_DIR/sandbox.py"
GEN_PY="$SPIKE_DIR/gen_seccomp.py"
FIX="$SPIKE_DIR/fixtures"
MS_OWN_SCRATCH=0
if [ -z "${MS_SCRATCH:-}" ]; then
  MS_SCRATCH=$(mktemp -d /tmp/ms-spike-sandbox-XXXXXX)
  MS_OWN_SCRATCH=1
fi
export MS_SCRATCH
BIN="$MS_SCRATCH/bin"; JOBS="$MS_SCRATCH/jobs"; RUNDIR="$MS_SCRATCH/run"
mkdir -p "$BIN" "$JOBS" "$RUNDIR"
chmod 700 "$MS_SCRATCH" "$JOBS"
MS_BG_PIDS=()
HOST_UID=$(id -u)

ms_cleanup() {
  local p
  for p in "${MS_BG_PIDS[@]:-}"; do [ -n "$p" ] && kill "$p" 2>/dev/null; done
  for p in "${MS_BG_PIDS[@]:-}"; do [ -n "$p" ] && wait "$p" 2>/dev/null; done
  if [ "$MS_OWN_SCRATCH" = 1 ]; then rm -rf "$MS_SCRATCH"; fi
}
trap ms_cleanup EXIT

bg() { "$@" & MS_BG_PIDS+=("$!"); LAST_BG=$!; }

build_probes() {
  # Two static builds that differ in content (needed for the swap tests).
  local id
  for id in A B; do
    [ -x "$BIN/probe$id" ] || gcc -static -O2 -DPROBE_ID="\"$id\"" -o "$BIN/probe$id" "$FIX/probe.c" 2>/dev/null || return 1
  done
  PROBE="$BIN/probeA"; PROBE_SHA=$(sha256sum "$PROBE" | cut -d' ' -f1)
}

newjob() { local d="$JOBS/$1"; mkdir -p "$d"; chmod 700 "$d"; echo "$d"; }

# sbx JOBNAME ARGS... : run probeA in the production-like sandbox with the given probe arguments.
sbx() {
  local job; job=$(newjob "$1"); shift
  python3 "$SB_PY" run --job "$job" --tool "$PROBE" --sha256 "$PROBE_SHA" ${SBX_OPTS[@]+"${SBX_OPTS[@]}"} -- "$@"
}
SBX_OPTS=()

# show CMD... : print the command (as run) and its output.
show() { printf '$ %s\n' "$*"; "$@" 2>&1; local rc=$?; printf '[exit %s]\n' "$rc"; return 0; }

# verdict ID PASS|FAIL|NOT TESTED|BLOCKED "text"
verdict() {
  printf 'VERDICT|%s|%s|%s\n' "$1" "$2" "$3"
}

# expect_line "regex" <<< text  -> 0 when a line matches (uses grep -E, not the user's rg alias)
has() { printf '%s\n' "$2" | /usr/bin/grep -Eq -- "$1"; }

# raw bubblewrap with deliberately incomplete arguments, used as a "before" baseline.
raw_bwrap() { bwrap --unshare-all --die-with-parent --ro-bind "$PROBE" /tool/probe --proc /proc --dev /dev --tmpfs /job --chdir /job "$@"; }

need_probes() { build_probes || { echo "cannot build probes (gcc -static)"; verdict "${1:-S?}" BLOCKED "gcc -static unavailable"; exit 0; }; }
