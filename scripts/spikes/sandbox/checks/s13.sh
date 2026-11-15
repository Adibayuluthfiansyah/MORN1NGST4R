#!/usr/bin/env bash
# S13: other distributions. Only THIS host can be tested. This script prints the facts that must be
# checked on any other distribution and always reports NOT TESTED for distributions other than the host.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
. /etc/os-release 2>/dev/null
echo "this host: ID=${ID:-?} ID_LIKE=${ID_LIKE:-?} PRETTY_NAME=${PRETTY_NAME:-?} kernel=$(uname -r)"
echo "== facts that decide whether bubblewrap + this argument set can work on a distribution (read-only checks)"
for f in /proc/sys/user/max_user_namespaces /proc/sys/kernel/unprivileged_userns_clone \
         /proc/sys/kernel/apparmor_restrict_unprivileged_userns /proc/sys/kernel/apparmor_restrict_unprivileged_unconfined \
         /proc/sys/kernel/yama/ptrace_scope /proc/sys/dev/tty/legacy_tiocsti /proc/sys/kernel/unprivileged_bpf_disabled \
         /proc/sys/kernel/io_uring_disabled /sys/kernel/security/lsm /sys/fs/cgroup/cgroup.controllers; do
  printf '  %-62s %s\n' "$f" "$(cat "$f" 2>&1 | head -1 | cut -c1-80)"
done
printf '  %-62s %s\n' "delegated controllers (user@UID.service)" "$(cat /sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/cgroup.controllers 2>&1 | head -1)"
printf '  %-62s %s\n' "bwrap setuid?" "$(stat -c %A "$(command -v bwrap)" 2>&1)"
echo "== what must be verified on each other distribution (see docs/security/feasibility/sandbox-spike.md section S13 for sources)"
cat <<'EOF'
  Ubuntu 24.04+  : kernel.apparmor_restrict_unprivileged_userns (an AppArmor profile with 'userns,' for bwrap is needed); re-run run-all.sh
  Debian         : kernel.unprivileged_userns_clone, user.max_user_namespaces, bwrap packaging
  Fedora/RHEL    : user.max_user_namespaces (RHEL hardening guides set it to 0), SELinux policy for user namespaces, cgroup v2 delegation
  Every distro   : cgroup v2 delegation of memory+pids to user@UID.service, systemd --user running, Landlock ABI, ptrace_scope, legacy_tiocsti
EOF
verdict S13 "NOT TESTED" "only this host (${ID:-unknown}) was exercised; other distributions are NOT TESTED. Documentation lookups are listed in the report"
