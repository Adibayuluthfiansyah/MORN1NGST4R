# Sandbox feasibility spike: unprivileged Linux process sandbox for tool execution

Status: spike result, not a design decision. Nothing here changes `security.md`, `architecture.md` or any ADR.

Question: can an unprivileged Linux process sandbox (Bubblewrap and rootless namespaces plus extra controls) meet the isolation requirements of `security.md` section 10 for running untrusted security-tool processes?

Everything below was executed on one host. Only what was observed is reported. Verdict vocabulary: PASS, FAIL, NOT TESTED, BLOCKED.

## Host

| Item | Observed |
| --- | --- |
| Run date/time | 2026-11-15 22:44 +07:00 (host clock; `date -Is`) |
| Kernel | `Linux 7.2.11-zen1-1-zen #1 ZEN SMP PREEMPT_DYNAMIC Fri, 16 Oct 2026 00:28:42 +0000 x86_64` |
| Distribution | Omarchy (`ID=omarchy`, `ID_LIKE=arch`) |
| bubblewrap | `bubblewrap 0.13.1`, `/usr/bin/bwrap`, mode `-rwxr-xr-x root root` (not setuid) |
| User | uid 1000, no sudo used |
| LSMs | `capability,landlock,lockdown,yama,bpf` (no AppArmor, no SELinux) |
| cgroup | `cgroup2fs`; user manager delegates `cpu memory pids` (not `io`) |
| Python / Go / gcc | Python 3.15.2 (mise), go 1.27.3, gcc (static libc available) |
| Resources | 8 CPUs, 7.7 GB RAM, zram swap |

## What was built

All paths are under `scripts/spikes/sandbox/`.

| File | Purpose |
| --- | --- |
| `sandbox.py` | The production-like launcher, called from bash as `python3 sandbox.py run ...`. `run` executes one tool; `self-test` is the start-up self-test. |
| `gen_seccomp.py` | Hand-assembled classic-BPF seccomp filter generator (x86_64 only, no dependencies). Also installs a filter and execs a command, used to simulate denied namespaces. |
| `fixtures/probe.c` | Static C probe that runs inside the sandbox and prints observations (status, env, pids, net, syscalls, fds, limits, Landlock, sha256 of itself). Two builds with different bytes (`A`, `B`) are used for the swap tests. |
| `fixtures/host_fixtures.py` | Host-side loopback TCP (v4, v6), UDP, abstract Unix socket and POSIX shm fixtures that count connections. |
| `fixtures/pty_run.py`, `fds_test.py`, `toctou.py`, `overhead.py`, `runrss.c`, `gotool.go` | Helpers for S3, S4, S11, S14 and a static Go "tool". |
| `lib.sh`, `checks/s01.sh` ... `s14.sh`, `run-all.sh` | One script per question and the driver that prints the verdict table. |

Note on shell: the sandbox launcher is Python, not bash, because passing a sealed memfd and a seccomp program as inherited file descriptors needs `os.memfd_create`, `pass_fds` and `close_fds`. Every script is invoked as `bash script.sh`.

### Argument set under test (as implemented in `sandbox.py`, `build_command`)

```text
systemd-run --user --scope --quiet --collect -p MemoryMax=<n>M -p MemorySwapMax=0 -p TasksMax=<n> --
  /usr/bin/env -i /usr/bin/bwrap
    --unshare-user --unshare-ipc --unshare-pid --unshare-net --unshare-uts --unshare-cgroup
    --disable-userns --die-with-parent --new-session --cap-drop ALL --clearenv
    --proc /proc --dev /dev
    --perms 0555 --ro-bind-data <memfd> /tool/<name>        # pinned tool bytes
    --bind <jobdir> /job --remount-ro /dev --remount-ro /   # only writable path is /job
    --chdir /job --seccomp <fd> --info-fd <fd>
    --setenv PATH /tool  --setenv HOME /job/home  ... (allow-list)
    -- /tool/<name> <args>
```

Launcher-side layers (not expressible as bwrap arguments): bwrap is started with an empty environment; `close_fds=True` with explicit `pass_fds`; rlimits CPU, FSIZE, CORE, NOFILE, AS; wall-clock timeout; stdout/stderr size cap; SHA-256 pin check; sealed memfd copy of the tool.

There is no flag, environment variable or mode in `sandbox.py` that disables any layer.

Two corrections were needed while building the spike. Both were found by the checks failing first, and both are covered in the sections below: the default bwrap root tmpfs and `/dev` tmpfs were writable (fixed with `--remount-ro`), and `--clearenv` alone leaked the parent environment (fixed by starting bwrap with an empty environment).

## Deviations and disclosures

- Early in the session I created three scratch files outside `/tmp/ms-spike-sandbox-*` (`/tmp/ms-probe.sh`, `/tmp/ms-probe2.sh`, `/tmp/ms-fix.py`). They were read-only probes or a one-off patch script. I deleted them afterwards.
- My first host probe listed the names of the entries in `/run/user/1000` (`ls`), without reading any file. One `journalctl --user -n 5` read the last user-journal lines while diagnosing the OOM exit code. No secret store, `~/.ssh`, `~/.gnupg` or `~/.config` was opened.
- `systemd-run --user --scope` creates transient user scopes (auto-collected). That is the mechanism S9 was asked to test; it uses the user manager's private socket, not the session bus.
- Checks use host-side loopback listeners and stand-in processes only. The RFC 5737 address 192.0.2.1 was contacted only from inside a network namespace with no route.

## Reproducibility

```bash
bash scripts/spikes/sandbox/run-all.sh                       # all 14 checks, prints the verdict table
bash scripts/spikes/sandbox/run-all.sh --only S2,S5          # subset
bash scripts/spikes/sandbox/run-all.sh --logs /tmp/ms-spike-sandbox-logs   # keep per-check logs
bash scripts/spikes/sandbox/checks/s09.sh                    # a single check, full evidence
python3 scripts/spikes/sandbox/sandbox.py self-test --probe <path to a static probe binary>
```

Expected runtime on this host: 24 to 26 seconds for `run-all.sh` (two full runs: 24 s and 26 s). The slowest check is S9 (about 11 s, it includes a 2 s CPU-limit run and a 6 s sleep that holds a scope open). The script needs gcc with static libc, python3, bubblewrap, `systemd --user`, util-linux and `ipcmk`. It builds its own fixtures into `/tmp/ms-spike-sandbox-XXXXXX`, removes that directory and any process that references it on exit, and is safe to re-run. It needs no root, no sysctl change and no package install.

Safety properties that the scripts enforce: network probes to the RFC 5737 address 192.0.2.1 run only inside a network namespace whose only interface is `lo` (the probe skips them otherwise, see `only_loopback()` in `probe.c`); the probe is never run un-sandboxed with the `net` command; no D-Bus message is sent (see S9 for the one `systemd-run` socket note); only dummy marker strings are used as secrets.

## S1. Unprivileged user and network namespaces

Commands (`checks/s01.sh`):

```text
unshare -Ur sh -c 'echo uid_in_ns=$(id -u); cat /proc/self/uid_map'
unshare -Urn sh -c 'echo uid_in_ns=$(id -u); echo ifaces=$(tail -n +3 /proc/net/dev | cut -d: -f1 ...)'
bwrap --unshare-user --ro-bind <probe> /p -- /p true
bwrap --unshare-user --unshare-ipc --unshare-pid --unshare-net --unshare-uts --unshare-cgroup --disable-userns --ro-bind <probe> /p -- /p true
```

Observed:

```text
uid_in_ns=0 / "         0       1000          1"        (unshare -Ur)
uid_in_ns=0 / ifaces=lo                                 (unshare -Urn)
bwrap --unshare-user: rc=0
bwrap all six + --disable-userns: rc=0
```

Kernel facts read:

| Sysctl or file | Value |
| --- | --- |
| `/proc/sys/user/max_user_namespaces` | 30251 |
| `/proc/sys/user/max_net_namespaces`, `max_pid_namespaces` | 30251, 30251 |
| `/proc/sys/kernel/unprivileged_userns_clone` | 1 |
| `/proc/sys/kernel/apparmor_restrict_unprivileged_userns` | file does not exist (no AppArmor) |
| `/proc/sys/kernel/unprivileged_bpf_disabled` | 2 |
| `/proc/sys/kernel/io_uring_disabled` | 0 (io_uring is enabled for unprivileged users on this host) |
| `/proc/sys/vm/memfd_noexec` | 0 |
| `/sys/kernel/security/lsm` | `capability,landlock,lockdown,yama,bpf` |

Verdict: **PASS**.

Limits: single host, uid 1000, no hardening profile. Because `io_uring_disabled` is 0, the seccomp filter (S6) is the only thing that blocks io_uring inside the sandbox.

## S2. Isolation observed from inside the production-like sandbox

Script: `checks/s02.sh`. A dummy marker `MS_SPIKE_MARKER_ENV=MSMARK-dummy-<pid>` is exported in the launching shell. A stand-in "worker" (`sleep 300`), an "evidence" directory and an "other job" directory exist on the host. Host fixtures listen on 127.0.0.1, `[::1]`, UDP loopback, an abstract socket and POSIX shm.

### Environment

Baseline, same arguments but `--clearenv` only, launched from the shell that holds the marker:

```text
ENV_COUNT 1
PID 1 (bwrap) environ: readable bytes=11772 vars=250 marker_found=1
MARKER_HITS 1
```

`--clearenv` clears bwrap's live environment but not the original environment block of the bwrap init process, which stays readable as `/proc/1/environ` inside the sandbox. Fix: start bwrap with an empty environment (`env -i`, or `subprocess` with `env={}`):

```text
env -i /usr/bin/bwrap ... env MSMARK  ->  MARKER_HITS 0
```

Production launcher:

```text
ENV PATH=/tool, HOME=/job/home, TMPDIR=/job/tmp, XDG_CONFIG_HOME, XDG_CACHE_HOME, XDG_DATA_HOME, XDG_STATE_HOME, LANG=C, PWD=/job
ENV_COUNT 9           (8 allow-listed + PWD that bwrap --chdir adds)
PID 1 (bwrap) environ: readable bytes=0 vars=0 marker_found=0
PID 2 (probeA) environ: readable bytes=199 vars=9 marker_found=0
MARKER_HITS 0
```

### PID namespace and process secrets

```text
VISIBLE_PIDS 1 2          SELF_PID 2
HOSTPID <launcher> kill(0): ESRCH     open(environ): ENOENT    open(mem): ENOENT
HOSTPID <worker>   kill(0): ESRCH     open(environ): ENOENT    open(mem): ENOENT
PEEK 1 environ: opened, read=0     PEEK 1 mem: EACCES          (bwrap init, empty env)
PEEK 2 environ/mem: readable                                    (the tool's own process)
```

### /proc, /sys, run dir, D-Bus, evidence, other job dir

```text
MNT ... /proc rw,nosuid,nodev,noexec,relatime - proc proc rw        (fresh proc mount)
NS pid pid:[4026532747]   host pid ns: pid:[4026531836]
STAT /sys: ENOENT   STAT /sys/class: ENOENT
```

`stat()` of each of these returned ENOENT inside the sandbox: `/run`, `/run/user`, `/run/user/1000`, `/run/user/1000/bus`, `/run/user/1000/wayland-1`, `/run/user/1000/keyring`, `/tmp/.X11-unix`, `/home`, the home directory and its `.ssh`, `.gnupg`, `.config`, `/etc/passwd`, `/usr`, `/bin`, `/var`, `/root`, `/sys`, the evidence stand-in and its file, the other-job stand-in and its file, `/dev/dri`, `/dev/nvidia0`, `/dev/kvm`, `/dev/snd`, `/dev/input`, `/dev/fuse`. Only `stat()` was issued, and only inside the sandbox where those paths do not exist. `/dev/null`, `zero`, `full`, `random`, `urandom`, `tty`, `shm`, `pts` and `ptmx` exist (the mount table shows bind mounts for the first six and a devpts mount); the device nodes listed above do not.

### Writes

```text
WRITE /rootwrite: EROFS    /tool/x: EROFS   /tmp/x: ENOENT   /etc/x: ENOENT   <evidence>/x: ENOENT
WRITE <other job>/x: ENOENT   /proc/x: ENOENT   /dev/x: EROFS   /job/ok.txt: OK (wrote 1)
```

Before `--remount-ro /` and `--remount-ro /dev` were added, `/rootwrite` and `/dev/x` succeeded: bwrap's root and `/dev` are writable tmpfs mounts by default (memory-backed, so writes would also count against the memory limit). A side effect of read-only `/dev`: `/dev/shm` is read-only too, so tools that need POSIX shared memory (for example headless browsers) need a deliberate `--tmpfs /dev/shm`.

### Network

```text
NET tcp 127.0.0.1 (host fixture): ECONNREFUSED after 0ms
NET tcp [::1] (host fixture): ECONNREFUSED after 0ms
NET tcp 192.0.2.1:80 (RFC5737): ENETUNREACH after 0ms
NET udp 192.0.2.1: sendto ENETUNREACH after 0ms
NET udp 127.0.0.1: sendto OK(into the sandbox-private loopback; host listener must stay at 0)
NETDEV: lo      ROUTE_LINES (1 = header only, i.e. no routes): 1
host fixture connections caused by sandboxed process: 0
control: host connects to the same fixtures OK (tcp4, tcp6)
```

Loopback inside the sandbox is the sandbox's own `lo`; "connection refused" there means nothing listened in that namespace, and the host-side fixture counter stayed at 0.

### --die-with-parent

```text
launcher pid=...; sandboxed tool processes before kill: 1
sandboxed tool processes after kill -9 of launcher: 0
```

Verdict: **PASS**.

Limits: the sandbox has no network at all. Real tools need a controlled egress path (see "Needed before production"). `PEEK 2 environ` shows the tool can read its own environment, as expected.

## S3. `--new-session` and TIOCSTI

Script: `checks/s03.sh`. A PTY created by the script (`pty.fork`) is the only terminal involved.

```text
dev.tty.legacy_tiocsti = 0
A. no sandbox:            TIOCSTI ioctl(0,...): ret=-1 errno=EIO
B. bwrap, no --new-session: TIOCSTI: EIO;   OPEN /dev/tty: OK (controlling terminal reachable);  WRITE /dev/tty: 61
C. bwrap --new-session:     TIOCSTI: EIO;   OPEN /dev/tty: ENXIO;  tcgetsid(0)=-1
D. B + production filter:   TIOCSTI: EPERM (filter), /dev/tty still reachable
E. sandbox.py (stdin /dev/null): TIOCSTI: EPERM, OPEN /dev/tty: ENXIO
every run: injected_marker_seen_on_pty=no
```

The kernel on this host blocks TIOCSTI for everybody (`legacy_tiocsti=0`, EIO). Injection therefore could not be demonstrated, with or without `--new-session`; I cannot flip the sysctl without root. On this host `--new-session` is belt-and-braces for TIOCSTI. What was demonstrated: without `--new-session` the sandboxed process reaches the controlling terminal through `/dev/tty` and writes to it (terminal escape sequences would be possible); with `--new-session` there is no controlling terminal; and the seccomp filter independently denies the TIOCSTI and TIOCLINUX ioctls.

Verdict: **PASS**.

Limits: behaviour with `legacy_tiocsti=1` is NOT TESTED. The bubblewrap README states that `--new-session` is needed unless TIOCSTI is filtered by seccomp (CVE-2017-5226), so both are kept.

## S4. Inherited file descriptors

Script: `checks/s04.sh`, helper `fixtures/fds_test.py`. The parent opens fds 3 to 8 on dummy marker files and fd 9 as a socket, then lists `/proc/self/fd` inside the sandbox.

```text
1. raw bwrap, parent fds inheritable, close_fds=False
   FD 3 -> /tmp/ms-spike-sandbox-.../marker3.txt ... FD 8 -> .../marker8.txt   FD 9 -> socket:[...]   FDS_ABOVE_2 7
2. raw bwrap, Python close_fds=True (default)                                    FDS_ABOVE_2 0
3. sandbox.py started by a parent that leaked fds 3..9 into it                   FDS_ABOVE_2 0
RESULT raw_bwrap_leaked_fds=7 python_close_fds_leaked=0 production_launcher_leaked=0
RESULT_HOSTPATH_VISIBLE_THROUGH_LEAK=yes
```

bwrap 0.13.1 does not close inherited descriptors and passes them to the tool, and `/proc/self/fd` shows the host path behind each. The mitigation that worked is in the launcher: `subprocess.Popen(..., close_fds=True, pass_fds=(tool_fd, seccomp_fd, info_fd))`. None of the three descriptors the launcher hands to bwrap (tool memfd, seccomp program, info pipe) reached the tool (the self-test check `fds` reports `open_fds_above_2=0`). Also observed in S11: a memfd executed via `/proc/self/fd/N` leaves that fd open in the tool.

Verdict: **PASS**.

Limits: the Go `exec.Cmd` approach (`ExtraFiles` only) was NOT TESTED; the Go worker must be tested separately. The guarantee depends on the launcher, not on bwrap.

## S5. Capabilities and privilege

Script: `checks/s05.sh`.

```text
Uid:    1000 1000 1000 1000
CapInh/CapPrm/CapEff/CapBnd/CapAmb: 0000000000000000   (host process: CapBnd 000001ffffffffff)
NoNewPrivs: 1      Seccomp: 2      Seccomp_filters: 1      NSpid: 2
SYS setuid(0) ret=-1 errno=EINVAL     setresuid(0,0,0): EINVAL     setgid(0): EINVAL      (uid 0 is unmapped)
SYS capset(all caps): EPERM     prctl(AMBIENT_RAISE,NET_ADMIN): EPERM     prctl(NO_NEW_PRIVS get): 1
PROCWRITE /proc/sys/kernel/hostname, randomize_va_space, /proc/sysrq-trigger, vm/drop_caches, net/ipv4/ip_forward: open(O_WRONLY) EACCES
SYS sethostname: EPERM     chown(/job,0,0): EINVAL
```

Real setuid binaries from a read-only `/usr` (`--ro-bind /usr /usr --ro-bind /usr/lib /lib64`), run as children of the probe:

```text
id        -> uid=1000 gid=1000 groups=1000,65534          (a normal dynamic tool still runs)
sudo -n id -> sudo: The "no new privileges" flag is set, which prevents sudo from running as root.
su -c id   -> su: user root does not exist or the user entry does not contain all the required fields
mount --bind /job /job -> mount: /job: must be superuser to use mount.
```

Verdict: **PASS**.

Limits: `su` failed because `/etc` is absent, not because of `no_new_privs`; that line is weak evidence. `sudo` and `mount` are the meaningful results.

## S6. Seccomp filter

Script: `checks/s06.sh`. `gen_seccomp.py --profile production` emits 49 classic-BPF instructions (392 bytes) passed with `--seccomp FD`. Design:

- Load `seccomp_data.arch`; anything other than `AUDIT_ARCH_X86_64` returns `SECCOMP_RET_KILL_PROCESS`.
- Syscall numbers with the x32 bit (`0x40000000`) return EPERM.
- Deny with EPERM: `ptrace`, `bpf`, `io_uring_setup/enter/register`, `keyctl`, `add_key`, `request_key`, `perf_event_open`, `mount`, `umount2`, `pivot_root`, `chroot`, `open_tree`, `move_mount`, `fsopen`, `fsconfig`, `fsmount`, `fspick`, `mount_setattr`, `setns`, `process_vm_readv/writev`, `userfaultfd`, `kexec_load`, `kexec_file_load`, `init_module`, `finit_module`, `delete_module`, `reboot`, `swapon`, `swapoff`.
- `unshare` and `clone` with `CLONE_NEWUSER` in `args[0]` return EPERM. `clone3` returns ENOSYS (its flags are inside a user-memory struct and cannot be inspected, so the only safe answer is to make libc fall back to `clone`).
- `ioctl` with request `TIOCSTI` or `TIOCLINUX` returns EPERM.
- Everything else is allowed.

Evidence that the filter, not just dropped capabilities, causes the denials: baseline B runs the same probe under `unshare -Urm` (which keeps capabilities inside its namespace) without and with the filter:

```text
without filter: ptrace(TRACEME) ret=0; mount(tmpfs) ret=0; umount2 ret=0; fsopen ret=3; open_tree ret=4; unshare(CLONE_NEWUSER) ret=0; clone(CLONE_NEWUSER) ret=<pid>; process_vm_readv ret=0
with filter:    every one of those: ret=-1 errno=EPERM
```

Inside the production sandbox (23 listed or extra syscalls plus `unshare(CLONE_NEWNET)`):

```text
ptrace(TRACEME) bpf io_uring_setup io_uring_enter io_uring_register keyctl add_key request_key perf_event_open mount umount2
fsopen open_tree move_mount mount_setattr pivot_root unshare(CLONE_NEWUSER) clone(CLONE_NEWUSER) setns process_vm_readv
userfaultfd kexec_load init_module                          -> ret=-1 errno=EPERM
clone3(size=0)                                               -> ret=-1 errno=ENOSYS
CONTROL getpid, openat(/proc/self), clock_gettime, socketpair, fork+wait -> all succeed
```

`bwrap --disable-userns` alone (no seccomp) also stops `unshare(CLONE_NEWUSER)` and `clone(CLONE_NEWUSER)` (EPERM); both are kept.

Without any filter, a bwrap sandbox with all namespaces and dropped caps still allowed `unshare(CLONE_NEWUSER)`, `clone(CLONE_NEWUSER)`, `ptrace(TRACEME)`, `open_tree` and `process_vm_readv`, and let `bpf` and `io_uring_setup` reach the kernel (EINVAL/EFAULT instead of EPERM).

32-bit and x32 entry points (`probe compat`):

```text
without filter: int 0x80 getpid returned 3 (works);   x32 getpid ret=2 (works)
production:     int 0x80: process killed by signal 31 (SIGSYS);   x32 getpid: ret=-1 errno=EPERM
```

Normal tools under the filter: dynamic glibc `/usr/bin/id`, `/usr/bin/python3` with 8 threads and a subprocess, and a static Go binary (goroutines, file write, loopback dial) all ran.

Verdict: **PASS**.

Architecture assumptions and fragility:

- x86_64 only. Syscall numbers are hard-coded from `arch/x86/entry/syscalls/syscall_64.tbl`; arm64 needs a different table and a different audit arch. Not tested on any other architecture.
- On this kernel the 32-bit `int 0x80` and the x32 ABI both work when unfiltered, so the arch check and the x32 bit test are required, not optional.
- A deny-list is open-ended: any new syscall that matters (the `open_tree`, `fsopen`, `mount_setattr` family appeared after the original mount call) is allowed until added. An allow-list is stronger and costs compatibility testing per tool.
- `clone3` is answered with ENOSYS. I did not test whether answering EPERM would break glibc `pthread_create` or `posix_spawn`; ENOSYS is what the observed working tools rely on.
- The filter does not inspect `clone3` flags, so namespace creation through `clone3` is blocked only because `clone3` is blocked as a whole (and by `--disable-userns`).
- The kernel here has `io_uring_disabled=0`; the filter is the only control for io_uring.

## S7. ptrace and Yama

Script: `checks/s07.sh`. `kernel.yama.ptrace_scope = 1`.

```text
unsandboxed process -> same-uid non-descendant host process: PTRACE_ATTACH EPERM, PTRACE_SEIZE EPERM   (Yama)
bwrap --unshare-user only (host pids visible), no seccomp:   EPERM, EPERM
full-namespace bwrap, no seccomp: attach to pid 1 (its init, an ancestor): EPERM; attach to the host pid: ESRCH (not visible)
production sandbox: attach to host pid: EPERM (filter answers before the pid lookup); attach to pid 1: EPERM; SEIZE: EPERM
production sandbox, pids command: HOSTPID <pid> kill(0): ESRCH, open(environ): ENOENT, open(mem): ENOENT
host stand-in process still running and untouched
```

Verdict: **PASS**.

Limits: with `ptrace_scope=0` Yama would not protect pid 1 of the sandbox (the tool is a descendant of that init). The filter denies ptrace anyway, but that case was NOT TESTED because it needs a sysctl change.

## S8. Abstract Unix sockets and shared memory

Script: `checks/s08.sh`. Host side: abstract listener `@ms-spike-abs-<pid>`, POSIX shm `/dev/shm/ms-spike-shm-<pid>`, and a SysV segment from `ipcmk -M 4096`.

```text
control (host process): ABSTRACT connect: CONNECTED;  SHM shm_open: OPENED
contrast: bwrap --unshare-user --unshare-pid --unshare-ipc (no --unshare-net): ABSTRACT connect: CONNECTED (reachable!)
production sandbox: ABSTRACT connect(@ms-spike-abs-<pid>): ECONNREFUSED
                    SHM shm_open(/ms-spike-shm-<pid>): ENOENT      SHM /dev/shm entries: 0
host listener count: after control=1, after contrast=2, after production sandbox=2 (unchanged)
SysV: host /proc/sysvipc/shm data rows 1; rows visible inside sandbox: 0   (segment removed with ipcrm afterwards)
```

Abstract sockets belong to the network namespace, so `--unshare-net` is what hides them; `--unshare-ipc` alone does not.

Verdict: **PASS**.

## S9. Resource limits

Script: `checks/s09.sh`.

### (a) rlimits

| Limit | Command | Observed |
| --- | --- | --- |
| RLIMIT_AS | `sandbox.py run --as-mb 256 ... -- memhog 512` | `MEMHOG mmap failed at 255 MB: ENOMEM` |
| RLIMIT_CPU | `--cpu-sec 2 ... -- cpuspin 20` | `exit 152` (128 + SIGXCPU), `wall_ms 2132` |
| RLIMIT_FSIZE | `--fsize-mb 1 ... -- fsize 8` | `FSIZE write failed after 1048576 bytes: EFBIG` |
| RLIMIT_FSIZE, many files | `--fsize-mb 1 ... -- fsize 1 8` | `FSIZE_MANY wrote 8388608 bytes in 8 files`: bounds one file, not total disk use |
| RLIMIT_NPROC | `prlimit --nproc=32 -- bwrap ... spawn 100` | `bwrap: Creating new namespace failed: Resource temporarily unavailable` (the uid runs 124 tasks); without sandbox: `created=0 first_error=EAGAIN` |
| Output cap | `--max-output-mb 1 ... -- flood 16` | `"truncated": true`, tool killed |
| Wall clock | `--timeout 2 ... -- sleep 30` | `"timed_out": true`, `wall_ms 2000`, no process left |

RLIMIT_NPROC is **not usable** as a per-job limit: it counts every task of the uid, so a low value stops bwrap itself from starting, and a value high enough to start would not limit a sandbox. The process count therefore relies on cgroup `pids.max`.

### (b) cgroup v2

```text
own cgroup: /user.slice/user-1000.slice/user@1000.service/session.slice/tmux-spawn-....scope
/sys/fs/cgroup/user.slice/user-1000.slice/user@1000.service
  cgroup.controllers=cpu memory pids   cgroup.subtree_control=cpu memory pids   writable-by-us=yes
root cgroup.controllers=cpuset cpu io memory hugetlb pids rdma misc dmem
```

Limits set through `systemd-run --user --scope -p MemoryMax=64M -p MemorySwapMax=0 -p TasksMax=32 --collect`, read from outside while a sandbox ran:

```text
sandboxed tool cgroup: /user.slice/user-1000.slice/user@1000.service/app.slice/run-p354635-i361136.scope
  cgroup.controllers = memory pids    memory.max = 67108864    memory.swap.max = 0    pids.max = 32
```

Behaviour:

```text
memhog 200 (limit 64 MB): MEMHOG touched 16/32/48 MB, then killed; launcher exit -15 (kernel OOM kill inside the scope,
                          then systemd stops the scope with SIGTERM). An exploratory run of the probe directly under `systemd-run` (no bwrap) ended with exit 137.
spawn 100 (limit 32 tasks): SPAWN requested=100 created=29 first_error=EAGAIN
```

Per-property probe through `systemd-run --user --scope -p ...`:

```text
MemoryMax=64M            -> memory.max=67108864          usable
TasksMax=32              -> pids.max=32                  usable
CPUQuota=50%             -> controllers=[cpu memory pids] cpu.max=50000 100000   usable
IOWeight=50              -> io.weight=<absent> io.max=<absent>    systemd exit 0, limit NOT applied
IOReadBandwidthMax=...   -> io.weight=<absent> io.max=<absent>    systemd exit 0, limit NOT applied
```

Usable without root on this host: **memory, pids, cpu** (cgroup v2, delegated to `user@1000.service`). **io** is not delegated and `systemd-run` accepts IO properties without applying them, so a launcher must read the control file back rather than trust the exit code.

`systemd-run --user` connected to the user manager's private socket (`strace -e connect`: `connect(3, {sun_path="/run/user/1000/systemd/private"}, 33) = 0`). It did not use the session bus. `sandbox.py` hands `systemd-run` only `XDG_RUNTIME_DIR`, never `DBUS_SESSION_BUS_ADDRESS`. One `journalctl --user -n 5` was read while diagnosing the OOM exit code.

If a required limit cannot be applied the operation must not run. That is implemented and tested in S12 (simulation 4 and 5: the tool does not start).

Verdict: **PASS** for the four limits named in the requirement (memory, process count, CPU time, output size), with these recorded gaps: RLIMIT_NPROC unusable; no io limit; no per-job disk quota (FSIZE is per file).

## S10. Landlock

Script: `checks/s10.sh`. bwrap 0.13.1 has no Landlock option (`bwrap --help | grep -ci landlock` = 0), so a helper inside the sandbox must apply it.

```text
LANDLOCK ABI: 10                        (landlock_create_ruleset(NULL,0,LANDLOCK_CREATE_RULESET_VERSION); Python ctypes cross-check: 10)
control, no ruleset: write /tmp/ms-spike-sandbox-ll-outside.txt: OK
ruleset on a host process (writes only under one dir; scoped = abstract unix socket + signal):
  LL write inside:  OK
  LL write outside: EACCES
  LL read /proc/self/status: OK                       (reads were not handled by the ruleset)
  LL abstract connect(@ms-spike-abs-<pid>) to host-side listener: EPERM
  LL kill(<pid>,0) to unconfined same-uid process:     EPERM
same ruleset inside the production sandbox (seccomp allows the landlock syscalls):
  write inside /job/ll-ok: OK;   write outside /job/denied.txt: EACCES;   kill(1,0): EPERM
```

ABI number: 10 as returned by this kernel. From the kernel documentation fetched for S13: scoping (abstract Unix socket, signal) needs ABI 6 or later; TCP network rules need ABI 4; UDP rules ABI 10; capability and namespace rules are listed for ABI 12, which this host does not have. Network rules were not exercised here.

Verdict: **PASS**.

Limits: Landlock is not part of the argument set under test; this only shows it is available and works inside the sandbox. Using it needs a small in-sandbox launcher that restricts itself and then execs the tool, and a fallback decision for hosts with a lower ABI.

## S11. Exact verified-byte execution

Script: `checks/s11.sh`, helper `fixtures/toctou.py`. Tool A is pinned (SHA-256 `dca4fad5...`). Tool B (`5344535c...`) is the attacker's replacement. The probe prints the SHA-256 of `/proc/self/exe`, so what ran is observable. The swap is done deterministically between the hash and the exec.

| Approach | Swap by rename | Swap by in-place overwrite |
| --- | --- | --- |
| (a) hash the path, then `bwrap --ro-bind PATH` | ran B: **swap succeeded** | ran B: **swap succeeded** |
| (b) open once, hash from the fd, `--ro-bind-data FD` with a plain file fd | ran A (guaranteed) | ran B: **swap succeeded** (the open fd sees the modified inode) |
| (c) open once, hash the bytes read, copy those bytes into a sealed memfd (`F_SEAL_SEAL|SHRINK|GROW|WRITE`), `--perms 0555 --ro-bind-data MEMFD` (this is `sandbox.py`) | ran A (guaranteed) | ran A (guaranteed) |
| (d) memfd inherited, exec `/proc/self/fd/N` inside bwrap | ran A | not separately tested (memfd is not on disk) |

Also observed:

- (c2) `sandbox.py` refuses a binary whose bytes do not match the pin: `REFUSED: layer=tool pin: sha256 mismatch`, exit 3.
- (d) works in bwrap 0.13.1, but the memfd stays open in the tool: `FD 6 -> /memfd:tool (deleted)`, `FDS_ABOVE_2 1`. That conflicts with the "close every inherited descriptor" requirement.
- `--ro-bind-data` with `--perms 0555` produces an executable read-only file in bwrap 0.13.1; the production runs use it.
- `fexecve` outside bwrap: NOT TESTED.

Only (c) guarantees that the executed bytes equal the hashed bytes. Verdict: **PASS** (for approach c).

Limits: the pin covers the tool binary only. A dynamically linked tool loads libraries from the host (`--ro-bind /usr`), and those are not hashed. Static tools, or a pinned and hashed library set, are required. The bwrap binary itself is trusted from the distribution package, not pinned.

## S12. Self-test and fail-closed behaviour

Script: `checks/s12.sh`. Implementation: `python3 sandbox.py self-test --probe <binary>`. It runs, in order: bwrap present and at least 0.8; one minimal bwrap per namespace kind (user, ipc, pid, net, uts, cgroup); the full production argument set with an in-sandbox probe that checks capabilities, no_new_privs, seccomp mode, open fds, an environment canary, pid namespace, fresh proc, filesystem, syscall filter and network; then behavioural checks that memory, process count, CPU time, file size and output size limits stop a hog. Any failure exits 3 with `REFUSED: layer=<name>: <detail>`.

On this host:

```text
CHECK caps PASS CapInh=0 CapPrm=0 CapEff=0 CapBnd=0 CapAmb=0
CHECK no_new_privs PASS NoNewPrivs=1     CHECK seccomp PASS Seccomp=2     CHECK fds PASS open_fds_above_2=0
CHECK env_canary PASS canary_hits=0      CHECK pid_ns PASS visible_pids=2 self=2     CHECK fresh_proc PASS
CHECK filesystem PASS sys_absent=1 run_absent=1 home_absent=1 root_ro=1 job_writable=1
CHECK syscall_filter PASS bpf=EPERM io_uring=EPERM keyctl=EPERM perf=EPERM mount=EPERM ptrace=EPERM unshare_userns=EPERM
CHECK network PASS ifaces=lo connect(192.0.2.1)=ENETUNREACH
SELFTEST PASS: bubblewrap; namespaces(...); production-argument run: ...; limits: memory,pids,cpu,fsize,output      [exit 0, about 1.3 s]
```

Simulated failures (all exit 3, tool not started):

| # | Simulation | How | Message |
| --- | --- | --- | --- |
| 1 | bwrap missing | `env PATH=/nonexistent python3 sandbox.py self-test ...` | `REFUSED: layer=bubblewrap: bwrap executable not found` |
| 2 | user namespaces denied | `gen_seccomp.py --profile deny-userns --exec -- python3 sandbox.py self-test ...` (filter returns EPERM for `clone`/`unshare` with `CLONE_NEWUSER`) | `REFUSED: layer=unprivileged user namespaces: bwrap: No permissions to create a new namespace ...` |
| 3 | network namespace denied | same wrapper with `--profile deny-newnet` (`CLONE_NEWNET`) | `REFUSED: layer=net namespace: bwrap: No permissions to create a new namespace ...` |
| 4 | cgroup limits unavailable | `env XDG_RUNTIME_DIR=<empty dir> python3 sandbox.py self-test ...` (user manager unreachable) | `REFUSED: layer=cgroup v2 limits (memory/pids via systemd user scope): Failed to connect to user scope bus via local transport` |
| 5 | systemd-run missing | `env PATH=/nonexistent python3 sandbox.py self-test ... --bwrap /usr/bin/bwrap` | `REFUSED: layer=cgroup v2 limits ...: systemd-run or env not found` |
| 6 | wrong pin | `sandbox.py run --tool probeB --sha256 <hash of A>` | `REFUSED: layer=tool pin: sha256 mismatch` |

The refusal also applies to real runs: with the user manager unreachable, `sandbox.py run ... -- write /job/ran.txt` exits 3 and `ran.txt` does not exist.

Verdict: **PASS**.

Limits: the self-test limit checks use small limits (64 MB, 32 tasks) through the same code path, not the production values. It runs at start-up; nothing re-runs it per job except the argument construction, which is the same code. The "unwritable cgroup path" case suggested in the brief was replaced by the unreachable user manager (simulation 4), because this launcher uses `systemd-run` and never writes the cgroup tree itself. A launcher that writes the tree directly under `session.slice` would contradict the systemd delegation guidance fetched in S13 (do not create cgroups below cgroups that systemd manages without `Delegate=`). Other failure modes were not simulated: kernel without seccomp, Landlock absent (not in the argument set), `--disable-userns` unsupported by an older bwrap (covered only by the version check).

## S13. Other distributions

Only this host was tested. Every other distribution is **NOT TESTED**. Fetch date for every source below: 2026-11-15 (host clock). `run-all.sh` also prints the read-only facts to check on another host (`checks/s13.sh`).

| Claim | Source | Result |
| --- | --- | --- |
| Ubuntu 24.04 restricts unprivileged user namespaces with AppArmor: the kernel plus the `apparmor` package restrict them for unprivileged unconfined programs; a default profile allows creating the namespace but denies capabilities inside it; opt-out by `echo 0 > /proc/sys/kernel/apparmor_restrict_unprivileged_userns` or a file in `/etc/sysctl.d` | [Ubuntu 24.04 LTS release notes](https://discourse.ubuntu.com/t/ubuntu-24-04-lts-noble-numbat-release-notes/39890) | Fetched. The text describes the restriction as active and gives the opt-out. It does not print the sysctl's default value. |
| The sysctl is named `kernel.apparmor_restrict_unprivileged_userns`, comes from Ubuntu kernel patches, and was specified to default to 1 through `/usr/lib/sysctl.d/10-apparmor.conf` in the `apparmor` package | [Spec: unprivileged user namespace restrictions via AppArmor in Ubuntu 23.10](https://discourse.ubuntu.com/t/spec-unprivileged-user-namespace-restrictions-via-apparmor-in-ubuntu-23-10/37626) (status "Pending Review", created 2023-05-24) | Fetched. It is a 23.10 proposal, not a statement about 24.04. |
| In 23.10 the feature was opt-in at release and planned to become default later | [Ubuntu blog, 23.10](https://ubuntu.com/blog/ubuntu-23-10-restricted-unprivileged-user-namespaces) | Fetched. |

**Ubuntu 24.04 default for `kernel.apparmor_restrict_unprivileged_userns`: not confirmed by a primary source.** The release notes and the spec together imply the value is 1 on 24.04, and community issue trackers say so (search snippets only, for example [anthropic-experimental/sandbox-runtime issue 74](https://github.com/anthropic-experimental/sandbox-runtime/issues/74); not fetched in full, secondary). No fetched Ubuntu document states "24.04 default = 1" in so many words, and I did not test a 24.04 host. Treat the value as unverified until read from a real host with `sysctl kernel.apparmor_restrict_unprivileged_userns`. What the release notes do state: an unprivileged, unconfined program may create the user namespace but is denied capabilities inside it. A community issue (search snippet only) reports that this stops bwrap from setting up the loopback interface. If that holds, a deployment on Ubuntu needs an AppArmor profile that allows `userns` for the launcher or the bwrap binary. This was not tested.

Other documentation read:

| Topic | Source | What it says (paraphrased) |
| --- | --- | --- |
| bubblewrap | [README](https://raw.githubusercontent.com/containers/bubblewrap/main/README.md) | The setuid mode was removed, so unprivileged user namespaces are required. bwrap sets `PR_SET_NO_NEW_PRIVS`. Protection depends entirely on the arguments the caller passes. `--new-session` is needed unless TIOCSTI is filtered by seccomp (CVE-2017-5226). A D-Bus socket bound into the sandbox can be used to run commands through systemd. |
| Yama | [kernel docs](https://docs.kernel.org/admin-guide/LSM/Yama.html) | `ptrace_scope` mode 1 allows ptrace from a parent to a child, or with `CAP_SYS_PTRACE`. |
| Landlock | [kernel docs](https://docs.kernel.org/userspace-api/landlock.html) | ABI progression and feature list used in S10 (scoping from ABI 6, UDP rules ABI 10, capability and namespace rules ABI 12). |
| cgroup delegation | [systemd CGROUP_DELEGATION](https://systemd.io/CGROUP_DELEGATION/) | Delegation applies to scope and service units with `Delegate=`; do not create cgroups below cgroups systemd manages without it; do not write attributes of cgroups systemd created. |
| RHEL | search snippets of Red Hat CVE pages, for example [CVE-2021-22555](https://access.redhat.com/security/cve/cve-2021-22555) | Mitigation advice for RHEL 7.4/8 is to set `user.max_user_namespaces` to 0, which would disable rootless sandboxes. Snippets only; pages not fetched. |
| Debian | none fetched directly (the Debian wiki URL I tried returned 404). The Ubuntu spec above states that Ubuntu, Debian and Arch carry a downstream sysctl patch to disable unprivileged user namespaces (`kernel.unprivileged_userns_clone`). | Secondary. |
| `user_namespaces(7)` | [man7.org](https://man7.org/linux/man-pages/man7/user_namespaces.7.html) | The fetch returned no match for `max_user_namespaces`; nothing was learned from it. |
| Fedora | no primary document found in this spike | NOT TESTED, no claim made. |

What must be checked on each other distribution: `user.max_user_namespaces`, `kernel.unprivileged_userns_clone` (Debian family), `kernel.apparmor_restrict_unprivileged_userns` and whether an AppArmor profile is required (Ubuntu), SELinux policy for user namespaces (Fedora, RHEL), delegated cgroup v2 controllers for `user@UID.service`, `systemd --user` availability for headless service accounts (a dedicated service account needs lingering enabled, not tested), Landlock ABI, `ptrace_scope`, `legacy_tiocsti`, bwrap packaging and version, and architecture (the filter is x86_64 only).

Verdict: **NOT TESTED** (other distributions).

## S14. Overhead

Script: `checks/s14.sh`, helper `fixtures/overhead.py`. 20 runs per row, tool is `probe true`. Wall time and max RSS are measured by a tiny C parent (`runrss.c`) with `wait4`, because `ru_maxrss` of a child also contains the pre-exec image of the parent and a Python parent would hide the real number.

| Variant | Median wall | Median max RSS (largest process in the tree) |
| --- | --- | --- |
| A: tool directly, no sandbox | 0.3 ms | 870 KB |
| B: bwrap only, production namespace and seccomp flags, no cgroup scope, no Python | 3.0 ms | 2208 KB |
| C: `sandbox.run()` called in-process from a long-lived Python worker (memfd, hash, `systemd-run` scope, bwrap) | 22.3 ms | not measured per sandbox (the Python parent holds 21.8 MB VmHWM) |
| D: fresh `python3 sandbox.py run` per tool | 59.7 ms | 20628 KB (the Python launcher) |
| E: `python3 -c pass` alone | 9.2 ms | 12188 KB |

Four runs of this check during the spike gave in-process medians of 17.8, 18.8, 12.4 and 22.3 ms and fresh-CLI medians of 51.9, 50.8, 54.2 and 59.7 ms, so the figures vary by about a factor of two with host load. The gap between B (about 2 to 3 ms) and C (12 to 22 ms) covers the `systemd-run` scope creation plus the Python-side work (hashing, memfd, seccomp program, process start); the two were not measured separately.

Verdict: **PASS** (measured; no threshold was set).

Limits: the numbers cover start-up only for a static tool that exits immediately. The memory cost of the sandbox's own tmpfs and namespaces was not isolated from the tool's.

## Summary

| Question | Verdict | One line |
| --- | --- | --- |
| S1 | PASS | User, net, ipc, pid, uts, cgroup namespaces and `--disable-userns` work unprivileged. |
| S2 | PASS | Env, worker pid, `/proc`, `/sys`, run dir, D-Bus, evidence, other job dir, writes, network and die-with-parent as required, after two fixes (empty launch environment, read-only root and `/dev`). |
| S3 | PASS | Kernel blocks TIOCSTI globally here; `--new-session` removes `/dev/tty`; filter also denies TIOCSTI. Injection itself not demonstrable on this host. |
| S4 | PASS | bwrap leaks inherited fds; the launcher closes them. Go `ExtraFiles` not tested. |
| S5 | PASS | All capability sets zero, no_new_privs 1, seccomp 2, no escalation. |
| S6 | PASS | 23 syscalls denied with EPERM, `clone3` ENOSYS, x32 EPERM, int 0x80 killed. x86_64 only. |
| S7 | PASS | Host process invisible; ptrace denied by filter. `ptrace_scope=0` not tested. |
| S8 | PASS | Abstract socket, POSIX shm and SysV shm unreachable. |
| S9 | PASS | Memory, pids, cpu (cgroup), CPU time, FSIZE, AS, output cap, timeout enforced. RLIMIT_NPROC unusable, no io controller, no disk quota. |
| S10 | PASS | Landlock ABI 10; fs restriction and scoping work, also inside the sandbox. Not part of bwrap arguments. |
| S11 | PASS | Only the sealed-memfd + `--ro-bind-data` approach guarantees bytes run = bytes hashed. Path-based hashing fails. |
| S12 | PASS | Self-test passes; six simulated failures all refuse with exit 3 and name the layer. |
| S13 | NOT TESTED | Only this host. Ubuntu 24.04 default sysctl value not confirmed by a primary source. |
| S14 | PASS | 12 to 22 ms in-process, 51 to 60 ms fresh CLI. |

## What this means

Proven on this host (Omarchy, kernel 7.2.11, bwrap 0.13.1, uid 1000, no root):

- An unprivileged bwrap sandbox with the full requested argument set starts, and the isolation properties in S2, S5, S7 and S8 hold.
- The self-test refuses to run tools when namespaces, bwrap, the cgroup limit mechanism or the tool pin are unavailable, and a real run refuses the same way.
- Memory, process-count, CPU-time and output limits are enforceable without root. Landlock ABI 10 is available as an extra layer.
- A sealed memfd copy of the tool, bound with `--ro-bind-data`, runs exactly the hashed bytes.

Failed or unusable (observed):

- `--clearenv` alone leaks the launcher environment through `/proc/1/environ`. Launching bwrap with an empty environment fixes it.
- bwrap passes inherited descriptors into the sandbox. Only the launcher can prevent that.
- bwrap's root and `/dev` tmpfs are writable by default. `--remount-ro` is required.
- RLIMIT_NPROC cannot serve as a per-job process limit.
- Hash-then-exec by path fails the TOCTOU test; a plain open fd fails against in-place modification.
- No io limit (controller not delegated, and `systemd-run` ignores IO properties silently) and no total disk quota for the job directory.

Not tested:

- Any distribution other than this host, including Ubuntu 24.04 with AppArmor restriction, Debian, Fedora, RHEL.
- Architectures other than x86_64.
- `legacy_tiocsti=1`, `ptrace_scope=0`, `io_uring_disabled` other than 0.
- Go `exec.Cmd` descriptor hygiene; `fexecve`; Landlock network rules; sandboxing of a real scanner such as nuclei, httpx or subfinder.
- Headless service accounts (`systemd --user` and cgroup delegation without a login session).
- Per-tool argument schemas and secret delivery through an owner-only file or inherited fd (the spike covers environment and fd hygiene only).

Blocked: nothing was blocked on this host.

Needed before production:

1. A controlled network egress path. The sandbox has no network by design; `security.md` section 2 requires every tool to reach the network only through an enforcement path that evaluates resolved addresses and redirects. Candidates (a proxy reached over a Unix socket bound into the sandbox, or a userspace network stack in the namespace) were not evaluated.
2. A disk quota for the job directory (size-limited tmpfs or project quota) and a decision on the io limit gap.
3. A decision on dynamically linked tools: pin and hash the library set, or accept static binaries only. Extra `--ro-bind` mounts in the spike are unrestricted and must become a reviewed allow-list.
4. A Go implementation of the launcher with the same descriptor, environment and memfd discipline, tested the way S4 and S11 test the Python one. The Python launcher is a spike, not the design.
5. An allow-list review of the seccomp profile per tool (the deny-list is open-ended), a plan for other architectures, and a regression test that fails when the filter or any bwrap argument changes.
6. An AppArmor profile or equivalent for Ubuntu 24.04 and later, then re-run `run-all.sh` on every supported distribution.
7. Audit events for sandbox start, refusal and denial, with the audit-write-failure rule from `security.md` section 13 (not implemented in the spike).
8. A decision on bwrap itself: distribution package trust versus a pinned, hash-verified bwrap executed from a descriptor.
9. A per-job re-check or a periodic self-test, not only start-up, because the host configuration can change while the worker runs.
10. An ADR for the sandbox mechanism once the above are decided. This document is evidence for it, not the decision.
