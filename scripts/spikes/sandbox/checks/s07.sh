#!/usr/bin/env bash
# S7: ptrace and Yama.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S7
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
echo "kernel.yama.ptrace_scope = $(cat /proc/sys/kernel/yama/ptrace_scope)"
bg sleep 300; HP=$LAST_BG
echo "host stand-in process (same uid) pid=$HP"

echo; echo "== S7.1 unsandboxed process tries to attach to a same-uid, non-descendant process (Yama alone)"
a=$("$PROBE" ptrace "$HP" 2>&1); echo "$a"
has 'PTRACE_ATTACH [0-9]+: ret=-1 errno=EPERM' "$a" || fail "Yama scope did not block the unsandboxed attach (scope != 1?)"
echo; echo "== S7.2 bwrap with only --unshare-user (host pids visible), no seccomp"
b=$(bwrap --unshare-user --ro-bind "$PROBE" /tool/probe --proc /proc -- /tool/probe ptrace "$HP" 2>&1); echo "$b"
echo; echo "== S7.3 full-namespace bwrap, no seccomp: attach to its own pid 1 (bwrap init = ancestor) and to the host pid"
c=$(raw_bwrap --new-session -- /tool/probe ptrace 1 2>&1); echo "$c"
c2=$(raw_bwrap --new-session -- /tool/probe ptrace "$HP" 2>&1); echo "$c2"
has 'PTRACE_ATTACH 1: ret=-1 errno=EPERM' "$c" || echo "  (note: attach to pid 1 not blocked by Yama here)"
echo; echo "== S7.4 production sandbox"
d1=$(sbx s7a ptrace "$HP" 2>&1); echo "$d1"
d2=$(sbx s7b ptrace 1 2>&1); echo "$d2"
d3=$(sbx s7c pids "$HP" 2>&1); echo "$d3" | /usr/bin/grep HOSTPID
has "HOSTPID $HP kill\(0\): ESRCH" "$d3" || fail "host process visible in sandbox pid namespace"
has 'PTRACE_ATTACH [0-9]+: ret=-1 errno=EPERM' "$d1" || fail "ptrace on host pid not denied (filter answers before pid lookup)"
has 'PTRACE_ATTACH 1: ret=-1 errno=EPERM' "$d2" || fail "filter did not deny ptrace on pid 1"
has 'PTRACE_SEIZE  1: ret=-1 errno=EPERM' "$d2" || fail "filter did not deny PTRACE_SEIZE"
kill -0 "$HP" && echo "host stand-in still running and untouched"
if [ $ok = 1 ]; then verdict S7 PASS "ptrace_scope=$(cat /proc/sys/kernel/yama/ptrace_scope): host process invisible (ESRCH) in the sandbox; ptrace denied by the seccomp filter (EPERM) independent of Yama. Behaviour with ptrace_scope=0 NOT TESTED (needs sysctl change)"
else verdict S7 FAIL "see '!!' lines"; fi
