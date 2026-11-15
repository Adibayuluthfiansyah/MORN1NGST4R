#!/usr/bin/env python3
"""Host-side local fixtures for the sandbox spike (loopback and local IPC only).

Starts: a TCP listener on 127.0.0.1, a TCP listener on [::1] (if available), an abstract Unix
socket listener, and a POSIX shared-memory segment. Every accepted connection is counted, so the
caller can prove that a sandboxed process never reached the host side.

  host_fixtures.py STATE_JSON      writes {"port4":..,"port6":..,"abstract":..,"shm":..,"counts":FILE}
Stops on SIGTERM/SIGINT or after 300 s. Removes the shm segment on exit.
"""
import json
import os
import selectors
import signal
import socket
import sys
import time

state_path = sys.argv[1]
pid = os.getpid()
sel = selectors.DefaultSelector()
counts_path = state_path + ".counts"
open(counts_path, "w").close()
state = {"pid": pid, "counts": counts_path}


def listen(fam, addr, label):
    s = socket.socket(fam, socket.SOCK_STREAM)
    s.bind(addr)
    s.listen(8)
    s.setblocking(False)
    sel.register(s, selectors.EVENT_READ, label)
    return s


s4 = listen(socket.AF_INET, ("127.0.0.1", 0), "tcp4")
state["port4"] = s4.getsockname()[1]
try:
    s6 = listen(socket.AF_INET6, ("::1", 0), "tcp6")
    state["port6"] = s6.getsockname()[1]
except OSError as e:
    state["port6"] = None
    state["ipv6_error"] = str(e)
su = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
su.bind(("127.0.0.1", 0))
su.setblocking(False)
sel.register(su, selectors.EVENT_READ, "udp4")
state["udp4"] = su.getsockname()[1]
abs_name = f"ms-spike-abs-{pid}"
sa = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sa.bind("\0" + abs_name)
sa.listen(8)
sa.setblocking(False)
sel.register(sa, selectors.EVENT_READ, "abstract")
state["abstract"] = abs_name
shm_name = f"ms-spike-shm-{pid}"
shm_path = f"/dev/shm/{shm_name}"
fd = os.open(shm_path, os.O_CREAT | os.O_RDWR | os.O_EXCL, 0o600)
os.write(fd, b"MS_SPIKE_SHM_MARKER")
os.close(fd)
state["shm"] = shm_name
with open(state_path, "w") as f:
    json.dump(state, f)

stop = False


def on_term(*_):
    global stop
    stop = True


signal.signal(signal.SIGTERM, on_term)
signal.signal(signal.SIGINT, on_term)
deadline = time.time() + 300
try:
    while not stop and time.time() < deadline:
        for key, _ in sel.select(0.2):
            if key.data == "udp4":
                try:
                    key.fileobj.recvfrom(64)
                except OSError:
                    continue
                with open(counts_path, "a") as f:
                    f.write("udp4\n")
                continue
            try:
                c, _addr = key.fileobj.accept()
            except OSError:
                continue
            with open(counts_path, "a") as f:
                f.write(key.data + "\n")
            try:
                c.sendall(b"HTTP/1.0 200 OK\r\n\r\nms-spike-fixture\n")
            except OSError:
                pass
            c.close()
finally:
    try:
        os.unlink(shm_path)
    except FileNotFoundError:
        pass
