#!/usr/bin/env python3
"""S11 helper: do the bytes that were hashed equal the bytes that run?

  toctou.py PROBE_A PROBE_B WORKDIR

PROBE_A is the "reviewed" tool whose SHA-256 is pinned. PROBE_B has different bytes and plays the
attacker's replacement. The probe prints the SHA-256 of /proc/self/exe, so what actually ran is observable.
The swap is performed deterministically BETWEEN the hash and the exec, which is the worst case of the race.
"""
import hashlib
import os
import re
import shutil
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import sandbox  # noqa: E402

A, B, WORK = sys.argv[1:4]
BWRAP = "/usr/bin/bwrap"
BASE = [BWRAP, "--unshare-all", "--die-with-parent", "--new-session", "--proc", "/proc", "--dev", "/dev"]


def sha_of(path):
    return hashlib.sha256(open(path, "rb").read()).hexdigest()


def ran_sha(out):
    m = re.search(r"SHA256_SELF ([0-9a-f]{64})", out)
    return m.group(1) if m else None


def fresh(name):
    p = os.path.join(WORK, name)
    shutil.copyfile(A, p)
    os.chmod(p, 0o755)
    return p


def swap_rename(path):
    tmp = path + ".evil"
    shutil.copyfile(B, tmp)
    os.chmod(tmp, 0o755)
    os.replace(tmp, path)


def swap_inplace(path):
    with open(path, "r+b") as f:
        f.write(open(B, "rb").read())
        f.truncate()


pinned = sha_of(A)
evil = sha_of(B)
print(f"pinned sha256(A) = {pinned}\nattacker sha256(B) = {evil}\n")
results = {}


def report(key, label, ran):
    guaranteed = ran == pinned
    verdict = "bytes run == bytes hashed" if guaranteed else ("SWAP SUCCEEDED (ran attacker bytes)" if ran == evil else f"unexpected: {ran}")
    print(f"  {label}: ran sha256={ran}\n      -> {verdict}")
    results[key] = guaranteed


print("(a) hash the PATH, then exec the PATH (bwrap --ro-bind PATH)")
for kind, swap in (("rename", swap_rename), ("in-place", swap_inplace)):
    p = fresh(f"a-{kind}")
    assert sha_of(p) == pinned          # check passes ...
    swap(p)                             # ... attacker swaps between check and use
    r = subprocess.run(BASE + ["--ro-bind", p, "/tool/x", "--", "/tool/x", "sha"], capture_output=True, text=True, env={})
    report(f"a_{kind}", f"swap by {kind}", ran_sha(r.stdout))

print("\n(b) open the file ONCE, hash from the fd, then bwrap --ro-bind-data FD (plain file fd, no sealing)")
for kind, swap in (("rename", swap_rename), ("in-place", swap_inplace)):
    p = fresh(f"b-{kind}")
    fd = os.open(p, os.O_RDONLY)
    h = hashlib.sha256(os.read(fd, 1 << 26)).hexdigest()
    assert h == pinned
    swap(p)
    os.lseek(fd, 0, os.SEEK_SET)
    os.set_inheritable(fd, True)
    r = subprocess.run(BASE + ["--perms", "0555", "--ro-bind-data", str(fd), "/tool/x", "--", "/tool/x", "sha"],
                       capture_output=True, text=True, env={}, close_fds=True, pass_fds=(fd,))
    os.close(fd)
    report(f"b_{kind}", f"swap by {kind}", ran_sha(r.stdout))

print("\n(c) production launcher: open once, hash the bytes read, copy THOSE bytes into a sealed memfd, --ro-bind-data")
orig = sandbox.build_command
for kind, swap in (("rename", swap_rename), ("in-place", swap_inplace)):
    p = fresh(f"c-{kind}")
    job = os.path.join(WORK, f"job-c-{kind}")
    os.makedirs(job, mode=0o700, exist_ok=True)

    def hooked(*a, _p=p, _swap=swap, **k):
        _swap(_p)                       # runs after sealed_tool_memfd() hashed+copied, before the exec
        return orig(*a, **k)
    sandbox.build_command = hooked
    try:
        r = sandbox.run(job, p, pinned, ["sha"])
    finally:
        sandbox.build_command = orig
    print(f"  (on-disk file after swap hashes to {'attacker' if sha_of(p) == evil else 'pinned'} bytes)")
    report(f"c_{kind}", f"swap by {kind}", ran_sha(r["stdout"]))

print("\n(c2) production launcher refuses a binary whose bytes do not match the pin")
p = fresh("c2")
swap_inplace(p)
try:
    sandbox.run(os.path.join(WORK, "job-c-rename"), p, pinned, ["sha"])
    print("  NOT refused")
    results["c2_refuse"] = False
except sandbox.Refuse as e:
    print(f"  refused: layer={e.layer}: {e.detail[:90]}...")
    results["c2_refuse"] = True

print("\n(d) memfd inherited into bwrap and exec'd as /proc/self/fd/N inside the sandbox (no --ro-bind-data)")
p = fresh("d")
mfd = os.memfd_create("tool", os.MFD_ALLOW_SEALING)
os.write(mfd, open(p, "rb").read())
os.set_inheritable(mfd, True)
swap_rename(p)
r = subprocess.run(BASE + ["--", f"/proc/self/fd/{mfd}", "sha"], capture_output=True, text=True, env={}, close_fds=True, pass_fds=(mfd,))
print(f"  exec via /proc/self/fd/{mfd}: rc={r.returncode} stderr={r.stderr.strip()[:100]!r}")
report("d_memfd_procfd", "memfd exec", ran_sha(r.stdout))
r = subprocess.run(BASE + ["--", f"/proc/self/fd/{mfd}", "fds"], capture_output=True, text=True, env={}, close_fds=True, pass_fds=(mfd,))
print("  fds visible to the tool in this variant:", " | ".join(l for l in r.stdout.splitlines() if l.startswith("FD") and not l.startswith(("FD 0", "FD 1", "FD 2"))) or "none")
os.close(mfd)

print("\nSUMMARY " + " ".join(f"{k}={'GUARANTEED' if v else 'NOT_GUARANTEED'}" for k, v in results.items()))
