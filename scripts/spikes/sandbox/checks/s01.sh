#!/usr/bin/env bash
# S1: are unprivileged user and network namespaces available?
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S1
echo "== S1 kernel facts"
show uname -srvm
for f in /proc/sys/user/max_user_namespaces /proc/sys/user/max_net_namespaces /proc/sys/user/max_pid_namespaces \
         /proc/sys/kernel/unprivileged_userns_clone /proc/sys/kernel/apparmor_restrict_unprivileged_userns \
         /proc/sys/kernel/unprivileged_bpf_disabled /proc/sys/kernel/io_uring_disabled /proc/sys/vm/memfd_noexec; do
  printf '%s = %s\n' "$f" "$(cat "$f" 2>&1 | head -1)"
done
printf 'LSM = %s\n' "$(cat /sys/kernel/security/lsm 2>&1)"
printf 'cgroup fs = %s\n' "$(stat -f -c %T /sys/fs/cgroup)"
printf 'bubblewrap = %s (%s)\n' "$(bwrap --version)" "$(command -v bwrap)"
ls -l "$(command -v bwrap)"
echo "== S1 namespace creation as uid $(id -u)"
r1=$(timeout 10 unshare -Ur sh -c 'echo uid_in_ns=$(id -u); cat /proc/self/uid_map' 2>&1); echo "$r1"; rc1=$?
r2=$(timeout 10 unshare -Urn sh -c 'echo uid_in_ns=$(id -u); echo ifaces=$(tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d " " | tr "\n" " ")' 2>&1); echo "$r2"
r3=$(timeout 10 bwrap --unshare-user --ro-bind "$PROBE" /p -- /p true 2>&1; echo "rc=$?"); echo "bwrap --unshare-user: $r3"
r4=$(timeout 10 bwrap --unshare-user --unshare-ipc --unshare-pid --unshare-net --unshare-uts --unshare-cgroup --disable-userns --ro-bind "$PROBE" /p -- /p true 2>&1; echo "rc=$?"); echo "bwrap all six + --disable-userns: $r4"
ok=1
has 'uid_in_ns=0' "$r1" || ok=0
has 'uid_in_ns=0' "$r2" || ok=0
has 'rc=0' "$r3" || ok=0
has 'rc=0' "$r4" || ok=0
if [ $ok = 1 ]; then verdict S1 PASS "unshare -Ur, -Urn, bwrap --unshare-user and the six-namespace + --disable-userns set all work as uid $(id -u); no AppArmor userns restriction sysctl present"
else verdict S1 FAIL "a namespace creation failed, see output above"; fi
