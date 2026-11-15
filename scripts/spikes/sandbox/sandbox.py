#!/usr/bin/env python3
"""Production-like unprivileged sandbox launcher (feasibility spike, Linux x86_64 only).

  sandbox.py run --job DIR --tool PATH --sha256 HEX [limits] [--env K=V ...] -- TOOL_ARGS...
  sandbox.py self-test --probe PATH

Layers applied to every run (there is no flag to disable any of them):
  bubblewrap: user, ipc, pid, net, uts, cgroup namespaces; fresh /proc; minimal /dev; no /sys;
              --die-with-parent; --new-session; --cap-drop ALL; --clearenv + allow-list;
              --disable-userns; tmpfs root (read-only view); tool bytes injected with --ro-bind-data
              from a sealed memfd that was hashed from the very same bytes (no path re-open);
              writable job dir at /job; classic-BPF seccomp filter (gen_seccomp.py) on --seccomp FD.
  launcher:   bwrap is started with an EMPTY environment (bwrap pid 1 keeps the original environ
              block readable in /proc/1/environ, so --clearenv alone is not enough);
              only the fds bwrap needs are passed (close_fds=True + explicit pass_fds);
              rlimits CPU/FSIZE/CORE/NOFILE/AS; cgroup v2 memory/pids via `systemd-run --user --scope`;
              wall-clock timeout; stdout/stderr size cap.
A required layer that is unavailable makes the launcher REFUSE (exit 3) and name the layer.
"""
import hashlib
import json
import os
import re
import resource
import selectors
import shutil
import signal
import subprocess
import sys
import time
import fcntl

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import gen_seccomp  # noqa: E402

EXIT_REFUSED = 3
MIN_BWRAP = (0, 8, 0)

BASE_ENV = {
    "PATH": "/tool",
    "HOME": "/job/home",
    "TMPDIR": "/job/tmp",
    "XDG_CONFIG_HOME": "/job/home/.config",
    "XDG_CACHE_HOME": "/job/home/.cache",
    "XDG_DATA_HOME": "/job/home/.local/share",
    "XDG_STATE_HOME": "/job/home/.local/state",
    "LANG": "C",
}
ENV_NAME_RE = re.compile(r"^[A-Z][A-Z0-9_]{0,63}$")
ENV_DENY_PREFIX = ("LD_", "BASH_", "PYTHON", "GCONV", "LOCPATH", "NLSPATH", "HOSTALIASES", "RES_OPTIONS")


class Refuse(Exception):
    def __init__(self, layer, detail):
        super().__init__(f"{layer}: {detail}")
        self.layer, self.detail = layer, detail


def find_bwrap(explicit=None):
    p = explicit or shutil.which("bwrap")
    if not p or not os.access(p, os.X_OK):
        raise Refuse("bubblewrap", "bwrap executable not found (searched PATH); install bubblewrap >= 0.8")
    p = os.path.realpath(p)
    out = subprocess.run([p, "--version"], capture_output=True, text=True, env={}, timeout=10).stdout
    m = re.search(r"(\d+)\.(\d+)\.(\d+)", out)
    if not m or tuple(map(int, m.groups())) < MIN_BWRAP:
        raise Refuse("bubblewrap", f"version {out.strip()!r} < required {'.'.join(map(str, MIN_BWRAP))} (--disable-userns)")
    return p


def classify_start_failure(stderr, rc):
    low = stderr.lower()
    if "bwrap:" in low:
        m = re.search(r"bwrap: (.*)", stderr)
        detail = m.group(1) if m else stderr.strip()
        if "namespace" in low or "uid map" in low or "unshare" in low or "operation not permitted" in low:
            return Refuse("user/ipc/pid/net/uts/cgroup namespaces", detail)
        return Refuse("bubblewrap", detail)
    if any(s in low for s in ("systemd", "bus", "scope", "failed to")):
        return Refuse("cgroup v2 limits (memory/pids via systemd user scope)", stderr.strip().splitlines()[-1] if stderr.strip() else f"rc={rc}")
    return Refuse("sandbox start", f"no start notification, rc={rc}, stderr={stderr.strip()[:200]!r}")


def prepare_job(job):
    if not os.path.isabs(job) or not os.path.isdir(job):
        raise Refuse("job directory", f"{job!r} must be an existing absolute directory")
    st = os.stat(job)
    if st.st_uid != os.getuid() or st.st_mode & 0o022:
        raise Refuse("job directory", "must be owned by the current user and not group/world writable")
    for sub in ("home", "tmp", "home/.config", "home/.cache", "home/.local/share", "home/.local/state"):
        os.makedirs(os.path.join(job, sub), mode=0o700, exist_ok=True)


def sealed_tool_memfd(tool, pinned_sha256):
    """Open the tool ONCE, hash the bytes that were read, copy exactly those bytes into a sealed memfd."""
    fd = os.open(tool, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW)
    try:
        st = os.fstat(fd)
        if not (st.st_mode & 0o170000) == 0o100000:
            raise Refuse("tool pin", f"{tool} is not a regular file")
        if st.st_size > 512 * 1024 * 1024:
            raise Refuse("tool pin", "tool binary larger than 512 MiB")
        data = b""
        while True:
            chunk = os.read(fd, 1 << 20)
            if not chunk:
                break
            data += chunk
    finally:
        os.close(fd)
    digest = hashlib.sha256(data).hexdigest()
    if digest != pinned_sha256.lower():
        raise Refuse("tool pin", f"sha256 mismatch: pinned {pinned_sha256.lower()} actual {digest}")
    mfd = os.memfd_create("ms-tool", os.MFD_CLOEXEC | os.MFD_ALLOW_SEALING)
    view = memoryview(data)
    while view:
        view = view[os.write(mfd, view):]
    F_ADD_SEALS, F_GET_SEALS = 1033, 1034          # not exported by Python's fcntl module (checked on 3.15.2)
    want = 1 | 2 | 4 | 8                          # F_SEAL_SEAL | F_SEAL_SHRINK | F_SEAL_GROW | F_SEAL_WRITE
    fcntl.fcntl(mfd, F_ADD_SEALS, want)
    if fcntl.fcntl(mfd, F_GET_SEALS) & want != want:
        raise Refuse("tool pin", "memfd sealing unavailable")
    if hashlib.sha256(os.pread(mfd, len(data), 0)).hexdigest() != digest:
        raise Refuse("tool pin", "memfd copy does not match hashed bytes")
    os.lseek(mfd, 0, os.SEEK_SET)
    return mfd, digest


def seccomp_memfd():
    prog = gen_seccomp.build("production")
    fd = os.memfd_create("ms-seccomp", os.MFD_CLOEXEC)
    os.write(fd, prog)
    os.lseek(fd, 0, os.SEEK_SET)
    return fd


def build_command(bwrap, job, tool_fd, name, seccomp_fd, info_fd, env, ro_binds, args, limits):
    cmd = [bwrap,
           "--unshare-user", "--unshare-ipc", "--unshare-pid", "--unshare-net", "--unshare-uts", "--unshare-cgroup",
           "--disable-userns", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv",
           "--proc", "/proc", "--dev", "/dev",
           "--perms", "0555", "--ro-bind-data", str(tool_fd), f"/tool/{name}"]
    for src, dst in ro_binds:
        cmd += ["--ro-bind", src, dst]
    cmd += ["--bind", job, "/job", "--remount-ro", "/dev", "--remount-ro", "/", "--chdir", "/job", "--seccomp", str(seccomp_fd), "--info-fd", str(info_fd)]
    for k, v in env.items():
        cmd += ["--setenv", k, v]
    cmd += ["--", f"/tool/{name}"] + list(args)
    sd = shutil.which("systemd-run")
    envbin = shutil.which("env")
    if not sd or not envbin:
        raise Refuse("cgroup v2 limits (memory/pids via systemd user scope)", "systemd-run or env not found")
    prefix = [sd, "--user", "--scope", "--quiet", "--collect",
              "-p", f"MemoryMax={limits['mem_mb']}M", "-p", "MemorySwapMax=0", "-p", f"TasksMax={limits['pids']}",
              "--", envbin, "-i"]
    return prefix + cmd


def run(job, tool, sha256, args=(), env_extra=None, ro_binds=(), bwrap=None,
        mem_mb=512, pids=64, cpu_sec=60, fsize_mb=64, as_mb=4096, timeout=120, max_output_mb=8):
    """Run `tool` in the sandbox. Returns dict. Raises Refuse when a required layer is unavailable."""
    env = dict(BASE_ENV)
    for k, v in (env_extra or {}).items():
        if not ENV_NAME_RE.match(k) or k.startswith(ENV_DENY_PREFIX) or k in BASE_ENV:
            raise Refuse("environment allow-list", f"variable name {k!r} not allowed")
        if "\n" in v or "\r" in v or "\0" in v:
            raise Refuse("environment allow-list", f"value of {k} contains control characters")
        env[k] = v
    bwrap = find_bwrap(bwrap)
    prepare_job(job)
    name = os.path.basename(tool)
    tool_fd, digest = sealed_tool_memfd(tool, sha256)
    sec_fd = seccomp_memfd()
    info_r, info_w = os.pipe()
    limits = dict(mem_mb=mem_mb, pids=pids)
    cmd = build_command(bwrap, job, tool_fd, name, sec_fd, info_w, env, ro_binds, args, limits)

    def preexec():
        resource.setrlimit(resource.RLIMIT_CPU, (cpu_sec, cpu_sec + 1))
        resource.setrlimit(resource.RLIMIT_FSIZE, (fsize_mb << 20, fsize_mb << 20))
        resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
        resource.setrlimit(resource.RLIMIT_NOFILE, (256, 256))
        resource.setrlimit(resource.RLIMIT_AS, (as_mb << 20, as_mb << 20))

    launch_env = {}
    if os.environ.get("XDG_RUNTIME_DIR"):
        launch_env["XDG_RUNTIME_DIR"] = os.environ["XDG_RUNTIME_DIR"]  # needed only by systemd-run; wiped by `env -i`
    t0 = time.monotonic()
    p = subprocess.Popen(cmd, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                         env=launch_env, close_fds=True, pass_fds=(tool_fd, sec_fd, info_w),
                         preexec_fn=preexec, start_new_session=True)
    for fd in (tool_fd, sec_fd, info_w):
        os.close(fd)
    sel = selectors.DefaultSelector()
    sel.register(p.stdout, selectors.EVENT_READ, "out")
    sel.register(p.stderr, selectors.EVENT_READ, "err")
    sel.register(info_r, selectors.EVENT_READ, "info")
    bufs = {"out": b"", "err": b"", "info": b""}
    cap = max_output_mb << 20
    total = 0
    truncated = timed_out = False
    killed = False

    def kill():
        nonlocal killed
        if not killed:
            killed = True
            try:
                os.killpg(p.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

    while sel.get_map():
        left = timeout - (time.monotonic() - t0)
        if left <= 0:
            timed_out = True
            kill()
            break
        for key, _ in sel.select(min(left, 0.5)):
            try:
                data = os.read(key.fileobj if isinstance(key.fileobj, int) else key.fileobj.fileno(), 65536)
            except OSError:
                data = b""
            if not data:
                sel.unregister(key.fileobj)
                continue
            if key.data == "info":
                bufs["info"] += data
                continue
            total += len(data)
            if total > cap:
                truncated = True
                kill()
            else:
                bufs[key.data] += data
        if truncated:
            break
    try:
        rc = p.wait(timeout=10)
    except subprocess.TimeoutExpired:
        kill()
        rc = p.wait()
    os.close(info_r)
    started = b"child-pid" in bufs["info"]
    stderr = bufs["err"].decode("utf-8", "replace")
    if not started and not timed_out:
        raise classify_start_failure(stderr, rc)
    return {"exit": rc, "stdout": bufs["out"].decode("utf-8", "replace"), "stderr": stderr,
            "wall_ms": int((time.monotonic() - t0) * 1000), "truncated": truncated, "timed_out": timed_out,
            "tool_sha256": digest, "started": started}


# ------------------------------------------------------------------ self-test
def _minimal_ns_check(bwrap, probe, kind):
    cmd = [bwrap, "--unshare-user", f"--unshare-{kind}", "--ro-bind", probe, "/p", "--", "/p", "true"] if kind != "user" \
        else [bwrap, "--unshare-user", "--ro-bind", probe, "/p", "--", "/p", "true"]
    r = subprocess.run(cmd, capture_output=True, text=True, env={}, timeout=20)
    if r.returncode != 0:
        raise Refuse(f"{kind} namespace" if kind != "user" else "unprivileged user namespaces",
                     r.stderr.strip() or f"rc={r.returncode}")


def self_test(probe, bwrap=None, verbose=True):
    log = (lambda m: print(m, file=sys.stderr)) if verbose else (lambda m: None)
    layers = []
    bwrap = find_bwrap(bwrap)
    layers.append("bubblewrap")
    if not os.path.isfile(probe):
        raise Refuse("self-test tool", f"{probe} missing")
    for kind in ("user", "ipc", "pid", "net", "uts", "cgroup"):
        _minimal_ns_check(bwrap, probe, kind)
    layers.append("namespaces(user,ipc,pid,net,uts,cgroup)")
    digest = hashlib.sha256(open(probe, "rb").read()).hexdigest()
    host_pidns = os.readlink("/proc/self/ns/pid")
    canary_name = "MS_SELFTEST_CANARY"
    os.environ[canary_name] = "canary-" + digest[:16]       # dummy marker in the launcher environment
    with_tmp = os.path.join(os.environ.get("TMPDIR", "/tmp"), f"ms-spike-sandbox-selftest-{os.getpid()}")
    os.makedirs(with_tmp, mode=0o700, exist_ok=True)
    try:
        r = run(with_tmp, probe, digest, ["selftest", canary_name, host_pidns], bwrap=bwrap, mem_mb=64, pids=32, cpu_sec=10, timeout=30)
        checks = {}
        for line in r["stdout"].splitlines():
            m = re.match(r"CHECK (\S+)\s+(PASS|FAIL) (.*)", line)
            if m:
                checks[m.group(1)] = (m.group(2), m.group(3))
                log(f"  {line}")
            m = re.match(r"ENVNAMES (.*)", line)
            if m:
                names = set(filter(None, m.group(1).split(",")))
                if names != set(BASE_ENV) | {"PWD"}:   # bwrap --chdir adds PWD itself
                    raise Refuse("environment", f"unexpected variable set {sorted(names)}")
        if r["exit"] != 0 or not checks:
            raise Refuse("sandbox start", f"selftest probe failed rc={r['exit']} stderr={r['stderr'][:200]!r}")
        for name, (st, detail) in checks.items():
            if st != "PASS":
                raise Refuse(name, detail)
        layers.append("production-argument run: " + ",".join(sorted(checks)))
        # resource limits: behavioural checks (each must really be enforced, otherwise refuse)
        def lim(label, args, **kw):
            kw.setdefault("mem_mb", 64); kw.setdefault("pids", 32); kw.setdefault("cpu_sec", 10); kw.setdefault("timeout", 30)
            return run(with_tmp, probe, digest, args, bwrap=bwrap, **kw)
        r = lim("memory", ["memhog", "256"])
        if "completed" in r["stdout"] or r["exit"] == 0:
            raise Refuse("limit:memory (cgroup memory.max)", "memory hog was not stopped")
        r = lim("pids", ["spawn", "100"])
        m = re.search(r"created=(\d+)", r["stdout"])
        if not m or int(m.group(1)) > 32:
            raise Refuse("limit:process count (cgroup pids.max)", f"spawned {m.group(1) if m else '?'} > 32")
        r = lim("cpu", ["cpuspin", "20"], cpu_sec=1, timeout=15)
        if "survived" in r["stdout"]:
            raise Refuse("limit:cpu time (RLIMIT_CPU)", "cpu spinner not stopped")
        r = lim("fsize", ["fsize", "8"], fsize_mb=1)
        if "NOT limited" in r["stdout"] or "FSIZE write failed" not in r["stdout"]:
            raise Refuse("limit:file size (RLIMIT_FSIZE)", r["stdout"].strip()[:100])
        r = lim("output", ["flood", "16"], max_output_mb=1)
        if not r["truncated"]:
            raise Refuse("limit:output size", "output cap not enforced")
        layers.append("limits: memory,pids,cpu,fsize,output")
    finally:
        shutil.rmtree(with_tmp, ignore_errors=True)
    return layers


def main(argv):
    if not argv or argv[0] not in ("run", "self-test"):
        print(__doc__, file=sys.stderr)
        return 2
    mode = argv[0]
    rest = argv[1:]
    opts = {"env": {}, "ro": []}
    tool_args = []
    if "--" in rest:
        i = rest.index("--")
        rest, tool_args = rest[:i], rest[i + 1:]
    i = 0
    while i < len(rest):
        a = rest[i]
        if a in ("--job", "--tool", "--sha256", "--probe", "--bwrap"):
            opts[a[2:]] = rest[i + 1]; i += 2
        elif a in ("--mem-mb", "--pids", "--cpu-sec", "--fsize-mb", "--as-mb", "--timeout", "--max-output-mb"):
            opts[a[2:].replace("-", "_")] = int(rest[i + 1]); i += 2
        elif a == "--env":
            k, _, v = rest[i + 1].partition("="); opts["env"][k] = v; i += 2
        elif a == "--ro-bind":
            opts["ro"].append((rest[i + 1], rest[i + 2])); i += 3
        else:
            print(f"unknown option {a}", file=sys.stderr); return 2
    try:
        if mode == "self-test":
            layers = self_test(opts["probe"], opts.get("bwrap"))
            print("SELFTEST PASS: " + "; ".join(layers))
            return 0
        kw = {k: opts[k] for k in ("mem_mb", "pids", "cpu_sec", "fsize_mb", "as_mb", "timeout", "max_output_mb") if k in opts}
        r = run(opts["job"], opts["tool"], opts["sha256"], tool_args, opts["env"], opts["ro"], opts.get("bwrap"), **kw)
        sys.stdout.write(r["stdout"]); sys.stderr.write(r["stderr"])
        print(json.dumps({k: r[k] for k in ("exit", "wall_ms", "truncated", "timed_out", "tool_sha256")}), file=sys.stderr)
        return r["exit"] if r["exit"] >= 0 else 128 - r["exit"]
    except Refuse as e:
        print(f"REFUSED: layer={e.layer}: {e.detail}", file=sys.stderr)
        return EXIT_REFUSED
    except KeyError as e:
        print(f"missing option {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
