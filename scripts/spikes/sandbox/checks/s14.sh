#!/usr/bin/env bash
# S14: sandbox start-up overhead (median of 20 runs).
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S14
w="$MS_SCRATCH/s14"; mkdir -p "$w"; chmod 700 "$w"
out=$(timeout 150 python3 "$FIX/overhead.py" "$PROBE" "$PROBE_SHA" "$w" 20 2>&1); echo "$out"
if [ "$(echo "$out" | /usr/bin/grep -c 'wall median')" = 5 ]; then
  d=$(echo "$out" | /usr/bin/grep '^D ' | sed -E 's/.*wall median +([0-9.]+) ms.*RSS median +([0-9]+) KB/\1 ms, \2 KB/')
  c=$(echo "$out" | /usr/bin/grep '^C ' | sed -E 's/.*wall median +([0-9.]+) ms.*/\1 ms/')
  verdict S14 PASS "median of 20: in-process sandbox.run() $c; fresh 'python3 sandbox.py run' $d (includes interpreter start-up)"
else verdict S14 FAIL "measurement did not complete"; fi
