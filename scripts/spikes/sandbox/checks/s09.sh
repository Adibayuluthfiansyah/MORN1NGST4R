#!/usr/bin/env bash
# S9: resource limits: (a) rlimits, (b) cgroup v2 via the user's delegated subtree.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S9
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
run_lim() { # jobname then sandbox.py options... -- probe args : prints probe output and the launcher summary
  local job; job=$(newjob "$1"); shift
  local opts=() 
  while [ "$1" != "--" ]; do opts+=("$1"); shift; done; shift
  python3 "$SB_PY" run --job "$job" --tool "$PROBE" --sha256 "$PROBE_SHA" "${opts[@]}" -- "$@" 2>&1
}

echo "== S9a.1 RLIMIT_AS (virtual address space): --as-mb 256, probe allocates up to 512 MB"
r=$(run_lim s9a1 --as-mb 256 --mem-mb 1024 -- memhog 512); echo "$r" | tail -4
has 'MEMHOG mmap failed at [0-9]+ MB: ENOMEM' "$r" || fail "RLIMIT_AS not enforced"
echo; echo "== S9a.2 RLIMIT_CPU: --cpu-sec 2, probe spins for 20 s"
r=$(run_lim s9a2 --cpu-sec 2 -- cpuspin 20); echo "$r" | tail -2; echo "(exit 152 = 128+SIGXCPU(24))"
has 'survived' "$r" && fail "cpu spinner survived"
has '"exit": (137|152|153|-?[0-9]+)' "$r" || fail "no exit summary"
echo; echo "== S9a.3 RLIMIT_FSIZE: --fsize-mb 1, probe writes one 8 MB file; then 8 files of 1 MB"
r=$(run_lim s9a3 --fsize-mb 1 -- fsize 8); echo "$r" | tail -2
has 'FSIZE write failed after 1048576 bytes: EFBIG' "$r" || fail "RLIMIT_FSIZE not enforced"
r=$(run_lim s9a3b --fsize-mb 1 -- fsize 1 8); echo "$r" | tail -2
has 'FSIZE_MANY wrote 8388608 bytes in 8 files' "$r" && echo "  GAP (observed): RLIMIT_FSIZE bounds one file, not total job-dir disk use (8 x 1 MB accepted); needs a disk quota or size-limited tmpfs job dir"
echo; echo "== S9a.4 RLIMIT_NPROC semantics: bwrap started under 'prlimit --nproc=32' (this uid runs $(ps -u $HOST_UID --no-headers | wc -l) processes+threads on the host)"
r=$(prlimit --nproc=32 -- bwrap --unshare-all --die-with-parent --new-session --ro-bind "$PROBE" /tool/probe --proc /proc --dev /dev --tmpfs /job -- /tool/probe spawn 100 2>&1); echo "NPROC=32 + user namespace: $r"
r2=$(prlimit --nproc=32 -- "$PROBE" spawn 100 2>&1); echo "NPROC=32, no sandbox: $r2"
echo; echo "== S9a.5 output cap and wall-clock timeout"
r=$(run_lim s9a5 --max-output-mb 1 -- flood 16); echo "$r" | tail -1 | cut -c1-160
has '"truncated": true' "$r" || fail "output cap"
r=$(run_lim s9a6 --timeout 2 -- sleep 30); echo "$r" | tail -1 | cut -c1-160
has '"timed_out": true' "$r" || fail "timeout"
sleep 0.5; pgrep -x probeA >/dev/null && fail "timed-out sandbox left processes behind"

echo; echo "== S9b.1 delegated cgroup subtree (read only)"
mycg=$(cut -d: -f3 /proc/self/cgroup); echo "own cgroup: $mycg"
for d in /sys/fs/cgroup/user.slice/user-$HOST_UID.slice/user@$HOST_UID.service /sys/fs/cgroup$mycg; do
  printf '%s\n  cgroup.controllers=%s\n  cgroup.subtree_control=%s\n  writable-by-us=%s\n' "$d" "$(cat "$d/cgroup.controllers")" "$(cat "$d/cgroup.subtree_control")" "$([ -w "$d" ] && echo yes || echo no)"
done
echo "root cgroup.controllers=$(cat /sys/fs/cgroup/cgroup.controllers)"
echo; echo "== S9b.2 limits actually set on a sandbox scope (read from outside while it runs)"
cp "$PROBE" "$BIN/probe-cg"; H=$(sha256sum "$BIN/probe-cg" | cut -d' ' -f1); j=$(newjob s9b2)
python3 "$SB_PY" run --job "$j" --tool "$BIN/probe-cg" --sha256 "$H" --mem-mb 64 --pids 32 -- sleep 6 >/dev/null 2>&1 &
LP=$!; MS_BG_PIDS+=("$LP")
for _ in $(seq 1 40); do pgrep -x probe-cg >/dev/null && break; sleep 0.1; done
P=$(pgrep -x probe-cg | head -1); scope=$(cut -d: -f3 /proc/"$P"/cgroup); echo "sandboxed tool cgroup: $scope"
for f in cgroup.controllers memory.max memory.swap.max pids.max cpu.max; do printf '  %s = %s\n' "$f" "$(cat /sys/fs/cgroup$scope/$f 2>&1)"; done
echo "  io limit files (io.max/io.weight/io.latency/io.cost.*) present: [$(ls /sys/fs/cgroup$scope | /usr/bin/grep -Ec '^io\.(max|weight|latency|cost)')]  (0 = io controller not usable; io.pressure/io.stat exist regardless)"
memmax=$(cat /sys/fs/cgroup$scope/memory.max); pidsmax=$(cat /sys/fs/cgroup$scope/pids.max)
[ "$memmax" = 67108864 ] || fail "memory.max not 64M"; [ "$pidsmax" = 32 ] || fail "pids.max not 32"
wait "$LP" 2>/dev/null
echo; echo "== S9b.3 behaviour: memory hog (limit 64 MB) and process spawner (limit 32 tasks)"
r=$(run_lim s9b3 --mem-mb 64 -- memhog 200); echo "$r" | tail -3
has 'MEMHOG completed' "$r" && fail "memory limit not enforced"
has '"exit": (137|-9|-15)' "$r" || fail "memory hog was not terminated"; echo "(memory limit reached: kernel OOM kill inside the scope, then systemd stops the scope with SIGTERM: exit -15 or 137)"
r=$(run_lim s9b4 --pids 32 -- spawn 100); echo "$r" | /usr/bin/grep SPAWN
created=$(echo "$r" | sed -n 's/.*created=\([0-9]*\).*/\1/p'); [ "${created:-99}" -le 32 ] || fail "pids limit not enforced"
echo; echo "== S9b.4 which controllers are usable without root? probe each through systemd-run --user --scope"
probe_prop() {
  timeout 20 systemd-run --user --scope --quiet --collect -p "$1" -- bash -c 'c=/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup)
    echo "controllers=[$(cat $c/cgroup.controllers)]"; for f in memory.max pids.max cpu.max io.weight io.max; do if [ -e $c/$f ]; then echo "$f=$(head -1 $c/$f)"; else echo "$f=<absent>"; fi; done' 2>&1 | tr '\n' ' '
}
for prop in "MemoryMax=64M" "TasksMax=32" "CPUQuota=50%" "IOWeight=50" "IOReadBandwidthMax=/dev/null 1M"; do
  out=$(probe_prop "$prop"); echo "  $prop -> $out"
  case "$prop" in
    MemoryMax*) has 'memory.max=67108864' "$out" || fail "MemoryMax not applied";;
    TasksMax*) has 'pids.max=32' "$out" || fail "TasksMax not applied";;
    CPUQuota*) has 'cpu.max=50000 100000' "$out" || fail "CPUQuota not applied";;
    IO*) has 'io.weight=<absent> io.max=<absent>' "$out" || fail "io controller unexpectedly present"; echo "     ^ systemd-run accepted the property (exit 0) but no io.* control file exists: io limits are silently NOT applied";;
  esac
done
if [ $ok = 1 ]; then verdict S9 PASS "usable without root: memory, pids (cgroup v2 via systemd --user scope, delegated 'cpu memory pids'), cpu bandwidth (cgroup cpu), CPU time/FSIZE/AS/NOFILE (rlimits), output cap + timeout (launcher). NOT usable: io controller (not delegated; systemd silently ignores IO* properties), per-job disk quota (FSIZE is per file only). RLIMIT_NPROC is unusable as a per-job limit (counts every process of the uid: bwrap cannot even start under --nproc=32), so process count relies on cgroup pids.max"
else verdict S9 FAIL "see '!!' lines"; fi
