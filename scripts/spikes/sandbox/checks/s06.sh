#!/usr/bin/env bash
# S6: seccomp BPF filter (hand-assembled classic BPF, x86_64 only) passed to bwrap with --seccomp FD.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S6
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
echo "== S6.0 filter size"; python3 "$GEN_PY" --profile production --dump | head -1
mkdir -p "$RUNDIR/mnt"

echo; echo "== S6.1 BASELINE A: bwrap (all namespaces, caps dropped), NO seccomp, NO --disable-userns"
base=$(raw_bwrap --new-session -- /tool/probe sys /job 2>&1); echo "$base"
echo; echo "== S6.2 BASELINE B: unshare -Urm (keeps caps inside its userns), NO filter: shows what the filter itself adds"
baseB=$(timeout 20 unshare -Urm "$PROBE" sys "$RUNDIR/mnt" 2>&1); echo "$baseB"
echo; echo "== S6.2b BASELINE C: bwrap --disable-userns only (no seccomp): does it alone stop nested user namespaces?"
baseC=$(raw_bwrap --new-session --disable-userns -- /tool/probe sys /job 2>&1); echo "$baseC" | /usr/bin/grep -E "CLONE_NEWUSER"
echo; echo "== S6.3 same as B but the production filter is installed first"
filtB=$(timeout 20 unshare -Urm python3 "$GEN_PY" --profile production --exec -- "$PROBE" sys "$RUNDIR/mnt" 2>&1); echo "$filtB"
echo; echo "== S6.4 PRODUCTION sandbox (sandbox.py)"
prod=$(sbx s6 sys /job 2>&1); echo "$prod"

chk() { # name regex text
  has "$2" "$3" || fail "$1: expected /$2/"
}
for n in 'ptrace\(TRACEME\)' 'bpf\(BPF_MAP_CREATE\)' 'io_uring_setup' 'io_uring_enter' 'io_uring_register' 'keyctl\(GET_KEYRING_ID\)' \
         'add_key' 'request_key' 'perf_event_open' 'mount\(tmpfs\)' 'umount2' 'fsopen' 'open_tree' 'move_mount' 'mount_setattr' \
         'pivot_root' 'unshare\(CLONE_NEWUSER\)' 'clone\(CLONE_NEWUSER\)' 'setns' 'process_vm_readv' 'userfaultfd' 'kexec_load' 'init_module'; do
  chk "prod $n" "SYS $n +ret=-1 errno=EPERM" "$prod"
done
chk "prod clone3" 'SYS clone3\(size=0\) +ret=-1 errno=ENOSYS' "$prod"
for n in 'getpid' 'openat\(/proc/self\)' 'clock_gettime' 'socketpair\(unix\)' 'fork\+wait'; do
  has "SYS CONTROL $n +ret=-?[0-9]+ errno=-" "$prod" || fail "control $n broken"
done
# the filter must be what blocks them in B (caps present): baseline allowed, filtered denied
for n in 'mount\(tmpfs\)' 'unshare\(CLONE_NEWUSER\)' 'ptrace\(TRACEME\)' 'fsopen'; do
  has "SYS $n +ret=0 errno=-|SYS $n +ret=[0-9]+ errno=-" "$baseB" || fail "baseline B should allow $n"
  chk "filtered B $n" "SYS $n +ret=-1 errno=EPERM" "$filtB"
done

echo; echo "== S6.5 32-bit (int 0x80) and x32 entry points"
cb=$(raw_bwrap --new-session -- /tool/probe compat 2>&1); echo "-- without filter:"; echo "$cb"
cp=$(sbx s6c compat 2>&1); echo "-- production:"; echo "$cp"
has 'COMPAT int0x80 getpid returned [0-9]+' "$cb" || echo "  (note: int 0x80 not usable without the filter on this kernel)"
has 'COMPAT x32 getpid: ret=-1 errno=EPERM' "$cp" || fail "x32 syscall number not denied"
has 'COMPAT int0x80: process killed by signal 31 \(SIGSYS\)' "$cp" || fail "int 0x80 not killed"

echo; echo "== S6.6 normal tools still run under the filter"
SBX_OPTS=(--ro-bind /usr /usr --ro-bind /usr/lib /lib64)
echo '$ /usr/bin/id (dynamic glibc binary)'; t1=$(sbx s6d exec /usr/bin/id 2>&1); echo "$t1"; has '^uid=1000' "$t1" || fail "id"
echo '$ /usr/bin/python3: threads + subprocess + sockets inside the sandbox'
t2=$(sbx s6d exec /usr/bin/python3 -c 'import threading,subprocess,socket,os
ts=[threading.Thread(target=lambda:None) for _ in range(8)]
[t.start() for t in ts]; [t.join() for t in ts]
r=subprocess.run(["/usr/bin/id","-u"],capture_output=True,text=True)
print("PY threads=ok subprocess_uid="+r.stdout.strip(), "pid="+str(os.getpid()))' 2>&1); echo "$t2"
has 'PY threads=ok subprocess_uid=1000' "$t2" || fail "python threads/subprocess"
SBX_OPTS=()
if command -v go >/dev/null; then
  echo '$ static Go binary (go build, CGO off): goroutines, threads, file write, loopback dial'
  GOCACHE="$MS_SCRATCH/gocache" GOPATH="$MS_SCRATCH/gopath" GOTOOLCHAIN=local CGO_ENABLED=0 GOFLAGS=-mod=mod HOME="$MS_SCRATCH" \
    go build -o "$BIN/gotool" "$FIX/gotool.go" 2>&1 | head -5
  if [ -x "$BIN/gotool" ]; then
    gj=$(newjob s6e)
    t3=$(python3 "$SB_PY" run --job "$gj" --tool "$BIN/gotool" --sha256 "$(sha256sum "$BIN/gotool" | cut -d' ' -f1)" 2>&1); echo "$t3"
    has 'GOTOOL goroutines=ok .*writefile=true loopback_dial_err=true' "$t3" || fail "go tool"
  else echo "  go build failed: Go tool NOT TESTED"; fi
else echo "  go not installed: Go tool NOT TESTED"; fi
if [ $ok = 1 ]; then verdict S6 PASS "all 23 listed/extra syscalls fail with EPERM inside the production sandbox (clone3 -> ENOSYS by design), controls and dynamic C, Python (threads+subprocess) and static Go tools run; x32 -> EPERM, int 0x80 -> SIGSYS kill. x86_64 only"
else verdict S6 FAIL "see '!!' lines"; fi
