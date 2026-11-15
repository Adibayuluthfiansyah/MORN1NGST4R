#!/usr/bin/env python3
"""S4 helper: open fds 3..8 on dummy marker files plus a socket (fd 9), then start sandboxes and list
/proc/self/fd inside them.   fds_test.py PROBE PROBE_SHA256 JOB_DIR SANDBOX_PY"""
import os
import re
import socket
import subprocess
import sys
import tempfile

probe, sha, job, sandbox_py = sys.argv[1:5]
tmp = tempfile.mkdtemp(prefix="ms-spike-sandbox-fds-", dir=os.path.dirname(job))
fds = []
for i in range(3, 9):
    p = os.path.join(tmp, f"marker{i}.txt")
    with open(p, "w") as f:
        f.write(f"MS_SPIKE_FD_MARKER_{i}\n")
    fds.append(os.open(p, os.O_RDONLY))
a, b = socket.socketpair()
print("parent fds:", fds, "socket fd:", a.fileno())
inherit = fds + [a.fileno()]
for fd in inherit:
    os.set_inheritable(fd, True)


def count(out):
    return int(re.search(r"FDS_ABOVE_2 (\d+)", out).group(1))


def show(label, out):
    lines = [l for l in out.splitlines() if l.startswith("FD")]
    print(f"== {label}\n  " + "\n  ".join(lines))


bw = ["/usr/bin/bwrap", "--unshare-all", "--die-with-parent", "--new-session", "--ro-bind", probe, "/tool/probe",
      "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/job", "--", "/tool/probe", "fds"]
r1 = subprocess.run(bw, capture_output=True, text=True, env={}, close_fds=False)
show("1. raw bwrap, parent fds inheritable, close_fds=False (what a careless launcher does)", r1.stdout)
r2 = subprocess.run(bw, capture_output=True, text=True, env={}, close_fds=True)
show("2. raw bwrap, Python close_fds=True (default)", r2.stdout)
# 3: the production launcher is itself started with the inheritable fds still open
r3 = subprocess.run([sys.executable, sandbox_py, "run", "--job", job, "--tool", probe, "--sha256", sha, "--", "fds"],
                    capture_output=True, text=True, close_fds=False)
show("3. sandbox.py started by a parent that leaked fds 3..9 into it", r3.stdout)
leak_raw, ok_py, ok_prod = count(r1.stdout), count(r2.stdout), count(r3.stdout)
print(f"RESULT raw_bwrap_leaked_fds={leak_raw} python_close_fds_leaked={ok_py} production_launcher_leaked={ok_prod}")
print("RESULT_HOSTPATH_VISIBLE_THROUGH_LEAK=" + ("yes" if "marker3.txt" in r1.stdout else "no"))
for fd in inherit:
    os.close(fd)
for f in os.listdir(tmp):
    os.unlink(os.path.join(tmp, f))
os.rmdir(tmp)
