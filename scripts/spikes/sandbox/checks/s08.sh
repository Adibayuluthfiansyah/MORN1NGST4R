#!/usr/bin/env bash
# S8: abstract Unix sockets, POSIX shared memory, SysV shared memory.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S8
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
bg python3 "$FIX/host_fixtures.py" "$RUNDIR/fx8.json"
for _ in $(seq 1 50); do [ -s "$RUNDIR/fx8.json" ] && break; sleep 0.1; done
ABS=$(python3 -c "import json;print(json.load(open('$RUNDIR/fx8.json'))['abstract'])")
SHM=$(python3 -c "import json;print(json.load(open('$RUNDIR/fx8.json'))['shm'])")
echo "host-side: abstract listener @$ABS ; POSIX shm /dev/shm/$SHM"
: > "$RUNDIR/fx8.json.counts"

echo; echo "== S8.1 control: an unsandboxed process on the host reaches both"
h=$("$PROBE" abstract "$ABS" 2>&1; "$PROBE" shm "/$SHM" 2>&1 | /usr/bin/grep -E "shm_open"); echo "$h"
has 'ABSTRACT connect\(@.*\): CONNECTED' "$h" || fail "control: abstract not reachable from host"
has 'shm_open\(/ms-spike-shm-[0-9]+\): OPENED' "$h" || fail "control: shm not reachable from host"
sleep 0.3; n0=$(wc -l < "$RUNDIR/fx8.json.counts"); echo "host listener saw $n0 connection(s) from the control"

echo; echo "== S8.2 contrast: bwrap that unshares user+pid+ipc but NOT net: abstract socket is still reachable"
c=$(bwrap --unshare-user --unshare-pid --unshare-ipc --ro-bind "$PROBE" /tool/probe --proc /proc --dev /dev -- /tool/probe abstract "$ABS" 2>&1); echo "$c"
has 'CONNECTED' "$c" || echo "  (note: not reachable even without netns)"
sleep 0.3; n1=$(wc -l < "$RUNDIR/fx8.json.counts")

echo; echo "== S8.3 production sandbox"
a=$(sbx s8a abstract "$ABS" 2>&1); echo "$a" | /usr/bin/grep ABSTRACT
s=$(sbx s8b shm "/$SHM" 2>&1); echo "$s" | /usr/bin/grep SHM
has "ABSTRACT connect\(@$ABS\): ECONNREFUSED" "$a" || fail "abstract socket reachable or unexpected error"
has "SHM shm_open\(/$SHM\): (ENOENT|EROFS|EACCES)" "$s" || fail "shm reachable"
has 'SHM /dev/shm entries: 0' "$s" || fail "host shm entries visible"
sleep 0.3; n2=$(wc -l < "$RUNDIR/fx8.json.counts")
echo "host listener connection count: after control=$n0 after contrast=$n1 after production sandbox=$n2"
[ "$n2" = "$n1" ] || fail "production sandbox reached the host listener"

echo; echo "== S8.4 SysV shared memory (ipc namespace)"
if command -v ipcmk >/dev/null; then
  idline=$(ipcmk -M 4096); echo "host: $idline"
  id=$(echo "$idline" | /usr/bin/grep -Eo '[0-9]+$')
  hs=$(cat /proc/sysvipc/shm | /usr/bin/grep -c "^ *[0-9]"); echo "host /proc/sysvipc/shm data rows: $hs"
  v=$(sbx s8c cat /proc/sysvipc/shm 2>&1); echo "$v" | /usr/bin/grep CAT | cut -c1-120
  rows=$(echo "$v" | /usr/bin/grep -c 'CAT /proc/sysvipc/shm: *[0-9]')
  echo "rows visible inside sandbox: $rows"
  ipcrm -m "$id"; echo "host segment removed"
  [ "$rows" = 0 ] || fail "SysV segment visible"
  sysv="SysV shm segment invisible in new ipc ns"
else sysv="SysV test NOT TESTED (ipcmk missing)"; fi
if [ $ok = 1 ]; then verdict S8 PASS "host abstract listener and POSIX shm unreachable from the sandbox ($sysv). Abstract socket isolation comes from --unshare-net; without it the socket WAS reachable (contrast run)"
else verdict S8 FAIL "see '!!' lines"; fi
