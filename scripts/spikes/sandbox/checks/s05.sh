#!/usr/bin/env bash
# S5: capabilities, no_new_privs, seccomp status, privilege escalation attempts.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S5
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
echo "== S5.1 /proc/self/status inside the production sandbox"
st=$(sbx s5a status 2>&1); echo "$st"
for k in CapInh CapPrm CapEff CapBnd CapAmb; do has "^$k:[[:space:]]+0000000000000000$" "$st" || fail "$k not zero"; done
has '^NoNewPrivs:[[:space:]]+1$' "$st" || fail "NoNewPrivs != 1"
has '^Seccomp:[[:space:]]+2$' "$st" || fail "Seccomp != 2"
echo "-- for contrast, the same host user outside the sandbox:"
grep -E '^(CapEff|CapBnd|NoNewPrivs|Seccomp)' /proc/self/status
echo; echo "== S5.2 escalation attempts from inside"
pv=$(sbx s5b priv 2>&1); echo "$pv"
has 'SYS setuid\(0\) +ret=-1 errno=(EPERM|EINVAL)' "$pv" || fail "setuid(0) did not fail"
has 'SYS setresuid\(0,0,0\) +ret=-1' "$pv" || fail "setresuid"
has 'SYS capset\(all caps\) +ret=-1 errno=EPERM' "$pv" || fail "capset did not fail"
has 'SYS prctl\(AMBIENT_RAISE,NET_ADMIN\) +ret=-1' "$pv" || fail "ambient raise"
has 'SYS prctl\(NO_NEW_PRIVS get\) +ret=1' "$pv" || fail "nnp"
if has 'PROCWRITE .*WRITABLE' "$pv"; then fail "a /proc/sys file was writable"; fi
has 'PROCWRITE /proc/sys/kernel/hostname: open\(O_WRONLY\) (EACCES|EROFS|EPERM)' "$pv" || fail "/proc/sys/kernel/hostname open"
has 'SYS sethostname\(x\) +ret=-1 errno=EPERM' "$pv" || fail "sethostname"
echo; echo "== S5.3 real setuid binaries from a read-only /usr (extra --ro-bind /usr, /usr/lib as /lib64)"
SBX_OPTS=(--ro-bind /usr /usr --ro-bind /usr/lib /lib64)
echo '$ id (dynamic tool via ro /usr: normal tool still runs)'; sbx s5c exec /usr/bin/id 2>&1
echo '$ sudo -n id (setuid root binary present on the host)'
sd=$(sbx s5c exec /usr/bin/sudo -n id 2>&1); echo "$sd"
echo '$ su -c id'
su_out=$(sbx s5c exec /usr/bin/su -c id 2>&1); echo "$su_out"
echo '$ mount --bind /job /job (setuid mount helper)'
mt=$(sbx s5c exec /usr/bin/mount --bind /job /job 2>&1); echo "$mt"
SBX_OPTS=()
has 'uid=0' "$sd" && fail "sudo gave root"
has 'uid=0' "$su_out" && fail "su gave root"
has "(no new privileges|not allowed|nosuid|not permitted|must be superuser|only root)" "$sd$mt" || fail "no explicit refusal text from sudo/mount (see output)"
if [ $ok = 1 ]; then verdict S5 PASS "all five capability sets zero, NoNewPrivs 1, Seccomp 2; setuid(0)/capset/ambient raise/sethostname/ /proc/sys writes fail; sudo, su, mount (setuid on host) gain nothing"
else verdict S5 FAIL "see '!!' lines"; fi
