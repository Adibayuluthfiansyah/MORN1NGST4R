#!/usr/bin/env python3
"""S14 helper: wall time and max RSS of starting the sandbox (median of N runs).
  overhead.py PROBE PROBE_SHA256 WORKDIR [N=20]"""
import os
import resource
import statistics
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import gen_seccomp  # noqa: E402
import sandbox  # noqa: E402

probe, sha, work = sys.argv[1:4]
N = int(sys.argv[4]) if len(sys.argv) > 4 else 20
job = os.path.join(work, "job-overhead")
os.makedirs(job, mode=0o700, exist_ok=True)


RUNRSS = os.path.join(work, "runrss")
subprocess.run(["gcc", "-O2", "-o", RUNRSS, os.path.join(HERE, "runrss.c")], check=True)


def run_wait4(cmd, **kw):
    """Run `cmd` under the tiny runrss parent so ru_maxrss is not polluted by this big Python process."""
    r = subprocess.run([RUNRSS] + cmd, capture_output=True, text=True, stdin=subprocess.DEVNULL, **kw)
    us, rss, code = (int(x) for x in r.stdout.split())
    assert code == 0, f"non-zero exit {code} for {cmd[:3]}"
    return us / 1000.0, rss


def summarize(label, samples):
    walls = [s[0] for s in samples]
    rss = [s[1] for s in samples]
    print(f"{label:<62} wall median {statistics.median(walls):7.1f} ms (min {min(walls):6.1f}, max {max(walls):6.1f})   max RSS median {statistics.median(rss):7.0f} KB")


# A: no sandbox
summarize("A  probe true, no sandbox", [run_wait4([probe, "true"], env={}) for _ in range(N)])

# B: bwrap with the production namespace/mount/seccomp flags, no launcher, no cgroup scope
prog = gen_seccomp.build("production")
bpf = os.path.join(work, "prod.bpf")
open(bpf, "wb").write(prog)
bw = ["/usr/bin/bwrap", "--unshare-user", "--unshare-ipc", "--unshare-pid", "--unshare-net", "--unshare-uts", "--unshare-cgroup",
      "--disable-userns", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--proc", "/proc", "--dev", "/dev",
      "--ro-bind", probe, "/tool/probe", "--bind", job, "/job", "--remount-ro", "/dev", "--remount-ro", "/", "--chdir", "/job",
      "--seccomp", "9", "--", "/tool/probe", "true"]
samples = []
for _ in range(N):
    fd = os.open(bpf, os.O_RDONLY)
    os.dup2(fd, 9)
    os.close(fd)
    os.set_inheritable(9, True)
    samples.append(run_wait4(bw, env={}, close_fds=False))
    os.close(9)
summarize("B  bwrap only (production flags + seccomp, no cgroup scope, no Python)", samples)

# C: sandbox.run() called in-process (what a long-lived worker would do): per-run wall, children RSS
walls = []
for _ in range(N):
    t0 = time.perf_counter()
    r = sandbox.run(job, probe, sha, ["true"])
    walls.append((time.perf_counter() - t0) * 1000)
    assert r["exit"] == 0
hwm = [l for l in open("/proc/self/status") if l.startswith("VmHWM")][0].split()[1]
summarize("C  sandbox.run() in-process (memfd+hash+systemd-run scope+bwrap)", [(w, int(hwm)) for w in walls])
print(f"   (C wall is per call inside a long-lived Python process; its 'RSS' column is that Python process's own VmHWM = {hwm} KB, not the sandbox's)")

# D: python sandbox.py run as a fresh process (interpreter start-up included)
cmd = [sys.executable, os.path.join(os.path.dirname(HERE), "sandbox.py"), "run", "--job", job, "--tool", probe, "--sha256", sha, "--", "true"]
summarize("D  python3 sandbox.py run ... (fresh interpreter each time)", [run_wait4(cmd, env={"XDG_RUNTIME_DIR": os.environ.get("XDG_RUNTIME_DIR", ""), "PYTHONDONTWRITEBYTECODE": "1"}) for _ in range(N)])
summarize("E  python3 -c pass (interpreter start-up alone)", [run_wait4([sys.executable, "-c", "pass"], env={}) for _ in range(N)])
print(f"(N={N} per row; A,B,D,E: wall and max RSS measured by the tiny runrss parent via wait4; RSS = largest single process in the tree, KB)")
