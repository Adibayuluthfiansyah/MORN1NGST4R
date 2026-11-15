#!/usr/bin/env bash
# S3: --new-session and TIOCSTI, using a PTY created by this script (never the user's terminal).
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S3
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
PTY="$FIX/pty_run.py"
echo "dev.tty.legacy_tiocsti = $(cat /proc/sys/dev/tty/legacy_tiocsti)  (sysctl -n dev.tty.legacy_tiocsti)"

RAW=(bwrap --unshare-all --die-with-parent --ro-bind "$PROBE" /tool/probe --proc /proc --dev /dev --tmpfs /job --chdir /job)
echo; echo "== A. no sandbox at all, PTY is our controlling terminal"
a=$(python3 "$PTY" Z -- "$PROBE" tiocsti 2>&1); echo "$a"
echo; echo "== B. bwrap WITHOUT --new-session"
b=$(python3 "$PTY" Z -- "${RAW[@]}" -- /tool/probe tiocsti 2>&1); echo "$b"
echo; echo "== C. bwrap WITH --new-session"
c=$(python3 "$PTY" Z -- "${RAW[@]}" --new-session -- /tool/probe tiocsti 2>&1); echo "$c"
echo; echo "== D. bwrap WITHOUT --new-session but WITH the production seccomp filter (filter denies ioctl TIOCSTI/TIOCLINUX)"
python3 "$GEN_PY" --profile production --out "$RUNDIR/prod.bpf"
d=$(python3 "$PTY" Z -- "${RAW[@]}" --seccomp 9 -- /tool/probe tiocsti 9<"$RUNDIR/prod.bpf" 2>&1); echo "$d"
echo; echo "== E. production launcher (sandbox.py), stdin is /dev/null"
e=$(sbx s3e tiocsti 2>&1); echo "$e"

# A..E: kernel blocks TIOCSTI globally (legacy_tiocsti=0): injection must never be observed
for t in "$a" "$b" "$c" "$d"; do has 'injected_marker_seen_on_pty=no' "$t" || fail "marker injected"; done
has 'TIOCSTI ioctl\(0,TIOCSTI,inject\): ret=-1 errno=EIO' "$a" || fail "A: kernel did not block with EIO"
has 'TIOCSTI ioctl\(0,TIOCSTI,inject\): ret=-1 errno=EIO' "$b" || fail "B: expected EIO"
has 'OPEN /dev/tty: OK' "$b" || fail "B: /dev/tty should be reachable without --new-session"
has 'WRITE /dev/tty: [0-9]+' "$b" || fail "B: could not write to controlling tty"
has 'OPEN /dev/tty: ENXIO' "$c" || fail "C: /dev/tty should be unreachable with --new-session"
has 'ret=-1 errno=EPERM' "$d" || fail "D: filter did not deny TIOCSTI"
has 'OPEN /dev/tty: ENXIO' "$e" || fail "E: production launcher still has a controlling tty"
legacy=$(cat /proc/sys/dev/tty/legacy_tiocsti)
if [ $ok = 1 ]; then
  verdict S3 PASS "legacy_tiocsti=$legacy: kernel returns EIO for TIOCSTI everywhere, so injection could NOT be demonstrated without --new-session on this host (mitigation is belt-and-braces here). Verified: without --new-session the sandbox reaches the PTY via /dev/tty and writes to it; with --new-session /dev/tty is ENXIO; seccomp filter also denies TIOCSTI (EPERM)"
else verdict S3 FAIL "see '!!' lines"; fi
