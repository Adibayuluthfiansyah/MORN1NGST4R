#!/usr/bin/env bash
# S12: start-up self-test, PASS on this host and fail-closed under simulated missing layers.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S12
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
PY=$(command -v python3); BW=$(command -v bwrap)

echo "== S12.1 self-test on this host (production arguments, then limit enforcement)"
out=$(timeout 120 "$PY" "$SB_PY" self-test --probe "$PROBE" 2>&1); rc=$?; echo "$out"; echo "[exit $rc]"
[ $rc = 0 ] && has '^SELFTEST PASS' "$out" || fail "self-test did not pass on this host"

refuses() { # label expected-layer-regex  command...
  local label="$1" re="$2"; shift 2
  echo; echo "== $label"; echo "\$ $*"
  local o r; o=$("$@" 2>&1); r=$?
  echo "$o" | /usr/bin/grep -E "REFUSED|SELFTEST" | head -3 | cut -c1-230; echo "[exit $r]"
  [ "$r" = 3 ] || fail "$label: exit code $r, expected 3"
  has "REFUSED: layer=.*($re)" "$o" || fail "$label: message does not name layer /$re/"
  has 'SELFTEST PASS' "$o" && fail "$label: self-test claimed PASS"
}

refuses "S12.2 SIMULATION 1: bubblewrap missing (PATH points to an empty dir; python run by absolute path)" 'bubblewrap' \
  env PATH=/nonexistent "$PY" "$SB_PY" self-test --probe "$PROBE"
refuses "S12.3 SIMULATION 2: user namespaces denied (self-test started under a seccomp filter that returns EPERM for clone/unshare(CLONE_NEWUSER))" 'unprivileged user namespaces' \
  "$PY" "$GEN_PY" --profile deny-userns --exec -- "$PY" "$SB_PY" self-test --probe "$PROBE"
refuses "S12.4 SIMULATION 3: network namespace creation denied (filter denies CLONE_NEWNET)" 'net namespace' \
  "$PY" "$GEN_PY" --profile deny-newnet --exec -- "$PY" "$SB_PY" self-test --probe "$PROBE"
refuses "S12.5 SIMULATION 4: cgroup limits unavailable (user service manager unreachable: XDG_RUNTIME_DIR points to an empty dir)" 'cgroup v2 limits' \
  env XDG_RUNTIME_DIR="$MS_SCRATCH/no-user-manager" "$PY" "$SB_PY" self-test --probe "$PROBE"
echo "(simulation 4 note: sandbox.py hands systemd-run ONLY XDG_RUNTIME_DIR, never DBUS_SESSION_BUS_ADDRESS, so there is no fallback to the session bus)"
refuses "S12.6 SIMULATION 5: systemd-run missing (bwrap given by absolute path, PATH empty)" 'cgroup v2 limits|bubblewrap' \
  env PATH=/nonexistent "$PY" "$SB_PY" self-test --probe "$PROBE" --bwrap "$BW"

echo; echo "== S12.7 the same refusals apply to real runs, not only the self-test: tool must NOT start"
job=$(newjob s12run)
r=$(env XDG_RUNTIME_DIR="$MS_SCRATCH/no-user-manager" "$PY" "$SB_PY" run --job "$job" --tool "$PROBE" --sha256 "$PROBE_SHA" -- write /job/ran.txt 2>&1); rc=$?
echo "$r" | cut -c1-200; echo "[exit $rc]"; [ $rc = 3 ] || fail "run did not refuse"
[ -e "$job/ran.txt" ] && fail "tool ran although a limit layer was unavailable"
echo "tool marker file present: $([ -e "$job/ran.txt" ] && echo YES || echo no)"
echo "== S12.8 wrong pin (tool bytes differ from the pinned digest)"
r=$("$PY" "$SB_PY" run --job "$job" --tool "$BIN/probeB" --sha256 "$PROBE_SHA" -- true 2>&1); rc=$?; echo "$r" | cut -c1-200; echo "[exit $rc]"
[ $rc = 3 ] && has 'layer=tool pin' "$r" || fail "pin mismatch not refused"

if [ $ok = 1 ]; then verdict S12 PASS "self-test passes here (all checks + limit enforcement, ~1.3 s); refuses with exit 3 naming the layer for: bwrap missing, userns denied, netns denied, user service manager unreachable (cgroup limits), systemd-run missing, pin mismatch; run path refuses too and the tool does not start"
else verdict S12 FAIL "see '!!' lines"; fi
