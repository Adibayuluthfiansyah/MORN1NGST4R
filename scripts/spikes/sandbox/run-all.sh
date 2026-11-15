#!/usr/bin/env bash
# Re-runs every sandbox feasibility check (S1..S14) and prints a verdict table.
#
#   bash run-all.sh                 run everything (about 1.5-2 minutes)
#   bash run-all.sh --only S2,S5    run a subset
#   bash run-all.sh --logs DIR      keep the per-check logs in DIR (default: discarded with the scratch dir)
#
# Properties: idempotent (fresh scratch dir each run), bounded (each check under `timeout`), light
# (small limits, no unbounded loops), local only (loopback and the RFC 5737 address 192.0.2.1 from
# inside a network namespace that has no route), and it removes everything it creates under
# /tmp/ms-spike-sandbox-*. It needs no root. Requirements: bash, gcc (static libc), python3, bubblewrap,
# systemd --user, util-linux (unshare, ipcmk). Linux x86_64 only.
set -u
export PYTHONDONTWRITEBYTECODE=1
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ONLY=""; KEEP_LOGS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="$2"; shift 2;;
    --logs) KEEP_LOGS="$2"; shift 2;;
    *) echo "usage: run-all.sh [--only S1,S2] [--logs DIR]" >&2; exit 2;;
  esac
done

export MS_SCRATCH; MS_SCRATCH=$(mktemp -d /tmp/ms-spike-sandbox-XXXXXX); chmod 700 "$MS_SCRATCH"
LOGS="$MS_SCRATCH/logs"; mkdir -p "$LOGS"
cleanup() {
  if [ -n "$KEEP_LOGS" ]; then mkdir -p "$KEEP_LOGS" && cp -f "$LOGS"/*.log "$KEEP_LOGS"/ 2>/dev/null; fi
  pkill -u "$(id -u)" -f "$MS_SCRATCH" 2>/dev/null
  rm -rf "$MS_SCRATCH"
}
trap cleanup EXIT

start=$(date +%s)
echo "MORN1NGST4R sandbox feasibility spike: $(date -Is)  host: $(uname -srm)  bwrap: $(bwrap --version 2>&1)"
declare -A V T
order=()
for n in 01 02 03 04 05 06 07 08 09 10 11 12 13 14; do
  id="S${n#0}"
  if [ -n "$ONLY" ] && ! echo ",$ONLY," | /usr/bin/grep -q ",$id,"; then continue; fi
  order+=("$id")
  printf '  running %-4s ' "$id"; t0=$(date +%s)
  timeout 400 bash "$HERE/checks/s$n.sh" > "$LOGS/s$n.log" 2>&1
  rc=$?
  line=$(/usr/bin/grep -E '^VERDICT\|' "$LOGS/s$n.log" | tail -1)
  if [ -z "$line" ]; then V[$id]="BLOCKED"; T[$id]="check produced no verdict (exit $rc), see log"; else
    V[$id]=$(echo "$line" | cut -d'|' -f3); T[$id]=$(echo "$line" | cut -d'|' -f4-)
  fi
  case "${V[$id]}" in PASS|FAIL|"NOT TESTED"|BLOCKED) ;; *) V[$id]="BLOCKED";; esac
  echo "${V[$id]} ($(( $(date +%s) - t0 ))s)"
done

echo; printf '%-5s %-11s %s\n' "ID" "VERDICT" "SUMMARY"; printf '%-5s %-11s %s\n' "-----" "-----------" "-------"
fails=0
for id in "${order[@]}"; do
  printf '%-5s %-11s %s\n' "$id" "${V[$id]}" "$(echo "${T[$id]}" | cut -c1-170)"
  [ "${V[$id]}" = FAIL ] && fails=$((fails+1))
done
stray=$(pgrep -u "$(id -u)" -f "$MS_SCRATCH" | /usr/bin/grep -v "^$$\$" | wc -l)
echo; echo "total runtime: $(( $(date +%s) - start ))s; failing checks: $fails; processes still referencing the scratch dir before cleanup: $stray"
[ "$fails" = 0 ]
