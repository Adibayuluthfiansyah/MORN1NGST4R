#!/usr/bin/env bash
# S4: inherited file descriptors.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S4
job=$(newjob s4)
out=$(timeout 60 python3 "$FIX/fds_test.py" "$PROBE" "$PROBE_SHA" "$job" "$SB_PY" 2>&1); echo "$out"
ok=1
has 'raw_bwrap_leaked_fds=[1-9]' "$out" || ok=0
has 'python_close_fds_leaked=0' "$out" || ok=0
has 'production_launcher_leaked=0' "$out" || ok=0
echo "-- also: the launcher's own seccomp/memfd/info pipe fds do not reach the tool (see FDS_ABOVE_2 0 above and the 'fds' self-test check)"
if [ $ok = 1 ]; then verdict S4 PASS "bwrap 0.13.1 does NOT close inherited fds (raw leak observed, host path visible through /proc/self/fd); Python close_fds=True + explicit pass_fds closes them, production launcher shows 0 fds above 2. Go exec.Cmd ExtraFiles approach NOT TESTED"
else verdict S4 FAIL "see output"; fi
