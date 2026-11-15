#!/usr/bin/env bash
# S2: isolation proofs from inside the production-like sandbox.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S2
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }

# Dummy marker "secret" in THIS (parent/worker) environment. It is not a real secret.
export MS_SPIKE_MARKER_ENV="MSMARK-dummy-$$"

# Host-side fixtures + stand-in worker process + evidence and other-job stand-in directories.
bg python3 "$FIX/host_fixtures.py" "$RUNDIR/fx.json"
for _ in $(seq 1 50); do [ -s "$RUNDIR/fx.json" ] && break; sleep 0.1; done
P4=$(python3 -c "import json;print(json.load(open('$RUNDIR/fx.json'))['port4'])")
P6=$(python3 -c "import json;print(json.load(open('$RUNDIR/fx.json'))['port6'])")
bg sleep 300; WORKER_PID=$LAST_BG
EVID="$MS_SCRATCH/evidence"; OTHER="$JOBS/other-job"
mkdir -p "$EVID" "$OTHER"; echo MS_EVIDENCE_DUMMY > "$EVID/secret.txt"; echo MS_OTHER_JOB_DUMMY > "$OTHER/file.txt"
echo "parent pid=$$ stand-in worker pid=$WORKER_PID  fixtures: tcp4=$P4 tcp6=$P6 evidence=$EVID other_job=$OTHER"

echo; echo "== S2.1 environment: baseline with only --clearenv (parent env contains the marker)"
base=$(raw_bwrap --new-session --clearenv -- /tool/probe env MSMARK 2>&1); echo "$base" | /usr/bin/grep -E "ENV_COUNT|marker_found=1|MARKER_HITS"
echo "(baseline marker hits: $(echo "$base" | /usr/bin/grep MARKER_HITS))"
echo "== S2.1 same bwrap args but launched with an EMPTY environment (env -i)"
fixed=$(env -i /usr/bin/bwrap --unshare-all --die-with-parent --new-session --clearenv --ro-bind "$PROBE" /tool/probe --proc /proc --dev /dev --tmpfs /job -- /tool/probe env MSMARK 2>&1); echo "$fixed" | /usr/bin/grep -E "ENV_COUNT|MARKER_HITS"
echo "== S2.1 production path (sandbox.py)"
out=$(sbx s2a env MSMARK 2>&1); echo "$out" | /usr/bin/grep -E "^ENV |ENV_COUNT|PID |MARKER_HITS"
has '^MARKER_HITS 0$' "$out" || fail "marker visible in sandbox"
has '^ENV_COUNT 9$' "$out" || fail "env var count != 9 (8 allow-list + PWD)"

echo; echo "== S2.2 worker pid invisible; pid namespace content"
out=$(sbx s2b pids "$$" "$WORKER_PID" 2>&1); echo "$out"
has "HOSTPID $WORKER_PID kill\(0\): ESRCH" "$out" || fail "worker pid visible"
has "HOSTPID $WORKER_PID open\(environ\): ENOENT" "$out" || fail "worker environ openable"
has "HOSTPID $WORKER_PID open\(mem\): ENOENT" "$out" || fail "worker mem openable"
has "HOSTPID $$ open\(environ\): ENOENT" "$out" || fail "parent environ openable"
has '^VISIBLE_PIDS( [0-9]+){1,3}$' "$out" || fail "more than 3 pids visible"
echo "-- in-sandbox pid 1 (bwrap) and pid 2 (the tool itself): can they be peeked?"
out=$(sbx s2c peek 1 2 2>&1); echo "$out"

echo; echo "== S2.3 /proc fresh, /sys absent"
out=$(sbx s2d proc 2>&1); echo "$out"
has '^NS pid pid:\[' "$out" || fail "no pid ns"
has '^STAT /sys: ENOENT' "$out" || fail "/sys exists"
has 'MNT .* /proc rw,nosuid,nodev,noexec.* - proc proc ' "$out" || fail "no fresh proc mount"
echo "host pid ns: $(readlink /proc/self/ns/pid)"

echo; echo "== S2.4 run dir, dbus, X11/Wayland, evidence, other job dir, credential stores not visible"
paths=(/run /run/user /run/user/$HOST_UID /run/user/$HOST_UID/bus /run/user/$HOST_UID/wayland-1 /run/user/$HOST_UID/keyring /tmp/.X11-unix
       /home "$HOME" "$HOME/.ssh" "$HOME/.gnupg" "$HOME/.config" /etc/passwd /usr /bin /var /root /sys
       "$EVID" "$EVID/secret.txt" "$OTHER" "$OTHER/file.txt" /dev/dri /dev/nvidia0 /dev/kvm /dev/snd /dev/input /dev/fuse)
# NOTE: only stat() is performed on these paths. Nothing is read from them.
out=$(sbx s2e paths "${paths[@]}" 2>&1); echo "$out"
if has 'VISIBLE' "$out"; then fail "a host path is visible"; fi
echo "-- what /dev contains:"; sbx s2e2 paths /dev/null /dev/zero /dev/full /dev/random /dev/urandom /dev/tty /dev/shm /dev/pts /dev/ptmx 2>&1

echo; echo "== S2.5 writes outside the job dir"
out=$(sbx s2f write /rootwrite /tool/x /tmp/x /etc/x "$EVID/x" "$OTHER/x" /proc/x /dev/x /job/ok.txt 2>&1); echo "$out"
has '^WRITE /job/ok.txt: OK' "$out" || fail "job dir not writable"
n_ok=$(echo "$out" | /usr/bin/grep -c '^WRITE .*: OK')
[ "$n_ok" = 1 ] || fail "a write outside the job dir succeeded"
ls "$EVID" "$OTHER" | tr '\n' ' '; echo "<- host listing of evidence + other-job dirs (must hold only the original files)"
[ "$(ls "$EVID" | wc -l)" = 1 ] && [ "$(ls "$OTHER" | wc -l)" = 1 ] || fail "host dirs modified"
ls "$JOBS/s2f" | tr '\n' ' '; echo "<- job dir contents"

echo; echo "== S2.6 network"
: > "$RUNDIR/fx.json.counts"
before=$(wc -l < "$RUNDIR/fx.json.counts")
P_UDP=$(python3 -c "import json;print(json.load(open('$RUNDIR/fx.json'))['udp4'])")
out=$(sbx s2g net "$P4" "$P6" "$P_UDP" 2>&1); echo "$out"
after=$(wc -l < "$RUNDIR/fx.json.counts")
echo "host fixture connections caused by sandboxed process: $((after-before))"
[ "$((after-before))" = 0 ] || fail "sandboxed process reached the host fixture"
has 'NET tcp 127.0.0.1 \(host fixture\): (ECONNREFUSED|ENETUNREACH|EHOSTUNREACH)' "$out" || fail "tcp4 loopback not refused"
has 'NET tcp \[::1\] \(host fixture\): (ECONNREFUSED|ENETUNREACH|EADDRNOTAVAIL|EAFNOSUPPORT)' "$out" || fail "tcp6 loopback not refused"
has 'NET tcp 192.0.2.1:80 \(RFC5737\): ENETUNREACH after [0-9]ms' "$out" || fail "192.0.2.1 not immediately unreachable"
has 'NET udp 127.0.0.1: sendto OK\(into the sandbox-private' "$out" || fail "udp loopback line missing"
has 'NET udp 192.0.2.1: sendto ENETUNREACH' "$out" || fail "udp 192.0.2.1 send did not fail"
has '^NETDEV: lo$' "$out" || fail "more than lo present"
has '^ROUTE_LINES .*: 1$' "$out" || fail "routes exist"
echo "-- control: the fixtures ARE reachable from the host (loopback only)"
python3 - "$P4" "$P6" <<'EOF'
import socket, sys
for fam, addr, port in ((socket.AF_INET, "127.0.0.1", int(sys.argv[1])), (socket.AF_INET6, "::1", int(sys.argv[2]))):
    s = socket.socket(fam); s.settimeout(2); s.connect((addr, port)); print("host control connect", addr, "OK", s.recv(64).split(b"\n")[-2:-1]); s.close()
EOF
sleep 0.3; echo "host fixture connection log now: $(tr '\n' ' ' < "$RUNDIR/fx.json.counts")"

echo; echo "== S2.7 --die-with-parent: SIGKILL the launcher, the sandboxed tool must die"
cp "$PROBE" "$BIN/probe-diewp"; DIEHASH=$(sha256sum "$BIN/probe-diewp" | cut -d' ' -f1)
dj=$(newjob s2h)
python3 "$SB_PY" run --job "$dj" --tool "$BIN/probe-diewp" --sha256 "$DIEHASH" -- sleep 120 >/dev/null 2>&1 &
LPID=$!
for _ in $(seq 1 40); do pgrep -x probe-diewp >/dev/null && break; sleep 0.1; done
echo "launcher pid=$LPID; sandboxed tool processes before kill: $(pgrep -x probe-diewp | wc -l)"
kill -9 "$LPID"; wait "$LPID" 2>/dev/null
gone=0; for _ in $(seq 1 30); do pgrep -x probe-diewp >/dev/null || { gone=1; break; }; sleep 0.1; done
echo "sandboxed tool processes after kill -9 of launcher: $(pgrep -x probe-diewp | wc -l)"
[ $gone = 1 ] || { fail "sandboxed process survived the death of its parent"; pkill -9 -x probe-diewp; }

if [ $ok = 1 ]; then verdict S2 PASS "env, worker pid, /proc, /sys, run dir, dbus, evidence, other job dir, writes, network (v4, v6, UDP, 192.0.2.1) and die-with-parent all behaved as required"
else verdict S2 FAIL "at least one expectation failed, see '!!' lines"; fi
