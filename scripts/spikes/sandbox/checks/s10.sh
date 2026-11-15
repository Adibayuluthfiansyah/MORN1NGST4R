#!/usr/bin/env bash
# S10: Landlock ABI, filesystem write restriction and scoping.
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
need_probes S10
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
echo "kernel: $(uname -r)"
echo "bwrap Landlock options: [$(bwrap --help 2>&1 | /usr/bin/grep -ci landlock)] (0 = bwrap 0.13.1 has none; Landlock must be applied by a helper inside the sandbox)"
echo; echo "== S10.1 highest supported ABI (landlock_create_ruleset(NULL,0,LANDLOCK_CREATE_RULESET_VERSION))"
abi=$("$PROBE" ll-abi); echo "$abi"
python3 - <<'EOF'
import ctypes, os
libc = ctypes.CDLL(None, use_errno=True)
v = libc.syscall(444, None, 0, 1)
print("python ctypes cross-check: landlock ABI =", v)
EOF
bg python3 "$FIX/host_fixtures.py" "$RUNDIR/fx10.json"
for _ in $(seq 1 50); do [ -s "$RUNDIR/fx10.json" ] && break; sleep 0.1; done
ABS=$(python3 -c "import json;print(json.load(open('$RUNDIR/fx10.json'))['abstract'])")
bg sleep 300; HP=$LAST_BG
mkdir -p "$RUNDIR/ll-allowed"
echo; echo "== S10.2 control: same writes WITHOUT a ruleset"
"$PROBE" write /tmp/ms-spike-sandbox-ll-outside.txt "$RUNDIR/ll-allowed/x"; rm -f /tmp/ms-spike-sandbox-ll-outside.txt
echo; echo "== S10.3 ruleset applied to a host process (writes only under $RUNDIR/ll-allowed; scoping of abstract unix sockets + signals if ABI>=6)"
ll=$("$PROBE" ll-write "$RUNDIR/ll-allowed" "$ABS" "$HP" 2>&1); echo "$ll"
rm -f /tmp/ms-spike-sandbox-ll-outside.txt
has 'LL abi=[0-9]+' "$ll" || fail "no abi"
has 'LL write inside .*: OK' "$ll" || fail "write inside denied"
has 'LL write outside .*: EACCES' "$ll" || fail "write outside not denied"
has 'LL abstract connect.*: EPERM' "$ll" || fail "abstract socket scoping not observed"
has 'LL kill\(.*\) to unconfined same-uid process: EPERM' "$ll" || fail "signal scoping not observed"
echo; echo "== S10.4 inside the production sandbox: seccomp allows the landlock syscalls; restrict /job/ll-ok, outside = /job/denied.txt"
SBX_OPTS=(--env LL_OUTSIDE=/job/denied.txt)
mkdir -p "$(newjob s10)/ll-ok"
in=$(sbx s10 ll-write /job/ll-ok "$ABS" 1 2>&1); echo "$in"
SBX_OPTS=()
has 'LL write inside .*: OK' "$in" || fail "inside sandbox: write inside denied"
has 'LL write outside /job/denied.txt: EACCES' "$in" || fail "inside sandbox: outside write not denied"
abinum=$(echo "$abi" | sed -n 's/.*ABI: \([0-9]*\).*/\1/p')
if [ $ok = 1 ]; then verdict S10 PASS "Landlock ABI $abinum (kernel $(uname -r)); fs write restriction works on host and inside the sandbox; abstract-unix-socket and signal scoping observed (both need ABI>=6). bwrap has no Landlock option, a helper must apply it"
else verdict S10 FAIL "see '!!' lines"; fi
