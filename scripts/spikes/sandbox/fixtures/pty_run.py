#!/usr/bin/env python3
"""Run a command with a PTY that THIS script creates as its controlling terminal (never the user's
terminal). Prints everything the command wrote to the PTY, then the exit status, then whether the
injection marker byte appeared on the PTY master (= something was pushed into the input queue and echoed).

  pty_run.py MARKER -- CMD...
"""
import os
import pty
import select
import sys
import time

marker = sys.argv[1].encode()
cmd = sys.argv[sys.argv.index("--") + 1:]
pid, master = pty.fork()          # child: new session, PTY slave is stdin/stdout/stderr + controlling tty
if pid == 0:
    os.execvp(cmd[0], cmd)
buf = b""
deadline = time.time() + 15
while time.time() < deadline:
    r, _, _ = select.select([master], [], [], 0.2)
    if r:
        try:
            d = os.read(master, 4096)
        except OSError:
            break
        if not d:
            break
        buf += d
    else:
        done, _ = os.waitpid(pid, os.WNOHANG)
        if done:
            try:
                while True:
                    r2, _, _ = select.select([master], [], [], 0.2)
                    if not r2:
                        break
                    d = os.read(master, 4096)
                    if not d:
                        break
                    buf += d
            except OSError:
                pass
            pid = 0
            break
st = 0
if pid:
    try:
        _, st = os.waitpid(pid, 0)
    except ChildProcessError:
        pass
text = buf.decode("utf-8", "replace").replace("\r\n", "\n")
print(text, end="")
print(f"[pty child finished] injected_marker_seen_on_pty={'YES' if marker in buf.replace(b'inject', b'') else 'no'}")
