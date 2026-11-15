#!/usr/bin/env bash
# MORN1NGST4R egress feasibility spike: build, run every scenario, print a
# verdict table, clean up. Local loopback and own network namespaces only.
#
#   bash scripts/spikes/egress/run-all.sh            # run everything (~1.5 min)
#   KEEP_LOGS=/some/dir bash .../run-all.sh          # also copy the full logs there
#
# Requires: go (stdlib only, no downloads), curl, socat, unshare, bwrap, ip, timeout.
# Writes only below /tmp/ms-spike-egress-*/ (removed on exit) and, if KEEP_LOGS is
# set, to that directory. Needs no root, sudo, package installs or sysctl changes.
set -u
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
SCR=$(mktemp -d /tmp/ms-spike-egress-XXXXXX) || exit 2
chmod 700 "$SCR"
OUT=$SCR/out; BIN=$SCR/bin
mkdir -p "$OUT" "$BIN" "$SCR/xdg"

export MS_SPIKE_TMP=$SCR DIR=$SCR BIN
# Keep every Go side effect (build cache, telemetry counters) inside the scratch dir.
export GOCACHE=$SCR/gocache GOPATH=$SCR/gopath XDG_CONFIG_HOME=$SCR/xdg GOPROXY=off GOFLAGS=-mod=readonly GOTOOLCHAIN=local CGO_ENABLED=0 GOTMPDIR=$SCR
mkdir -p "$GOCACHE" "$GOPATH"

HOSTSIDE_PID=""
cleanup() {
  [ -n "$HOSTSIDE_PID" ] && kill -TERM "$HOSTSIDE_PID" 2>/dev/null
  sleep 0.3
  pkill -TERM -f "$SCR/" 2>/dev/null
  sleep 0.3
  pkill -KILL -f "$SCR/" 2>/dev/null
  if [ -n "${KEEP_LOGS:-}" ]; then mkdir -p "$KEEP_LOGS" && cp -f "$OUT"/*.log "$OUT"/*.txt "$KEEP_LOGS"/ 2>/dev/null; fi
  rm -rf "$SCR"
}
trap cleanup EXIT INT TERM HUP

declare -a ROWS=()
add() { ROWS+=("$1|$2|$3"); }   # question | verdict | note

for t in go curl socat unshare bwrap ip timeout; do
  command -v "$t" >/dev/null || { echo "missing tool: $t"; add "prereq" "BLOCKED" "missing $t"; }
done
echo "# host: $(uname -srmo)  go: $(go version 2>/dev/null)  curl: $(curl --version | head -1 | cut -d' ' -f1-2)  socat: $(socat -V | sed -n 2p | cut -d' ' -f1-3)  bwrap: $(bwrap --version)  date: $(date -Is)"

echo "== build + unit tests"
cd "$HERE" || exit 2
if ! gofmt -l . | grep -q .; then echo "gofmt clean"; else echo "gofmt differences:"; gofmt -l .; fi
go vet ./... >"$OUT/vet.log" 2>&1 && echo "go vet: ok" || { echo "go vet failed"; cat "$OUT/vet.log"; }
go test ./... >"$OUT/unit.log" 2>&1; rc=$?
if [ $rc = 0 ]; then add "unit" "PASS" "evaluator/canonicalization unit tests"; else add "unit" "FAIL" "go test rc=$rc"; cat "$OUT/unit.log"; fi
if ! go build -o "$BIN" ./cmd/... >"$OUT/build.log" 2>&1; then cat "$OUT/build.log"; echo "BUILD FAILED"; add build FAIL "see log"; fi
grep -c 'require\|replace' go.mod | sed 's/^/go.mod dependency lines: /'

echo "== E1..E6 (Go experiments)"
for q in e1 e2 e3 e4 e5 e6; do
  Q=$(echo "$q" | tr a-z A-Z)
  start=$(date +%s)
  timeout 150 "$BIN/spike" "$q" >"$OUT/$q.log" 2>&1; rc=$?
  line=$(grep '^VERDICT' "$OUT/$q.log" | tail -1)
  if [ -n "$line" ]; then
    v=$(echo "$line" | sed -E 's/^VERDICT [A-Z0-9]+ (PASS|FAIL|NOT TESTED|BLOCKED) \|.*/\1/')
    n=$(echo "$line" | sed -E 's/^[^|]*\| //')
    add "$Q" "$v" "$n"
  else
    add "$Q" "FAIL" "no verdict line (rc=$rc, timeout or crash); see $q.log"
  fi
  echo "$Q: $(grep -c '^\[.*\] ok ' "$OUT/$q.log") ok, $(grep -c '^\[.*\] FAIL' "$OUT/$q.log") FAIL, $(( $(date +%s) - start ))s"
done

echo "== E7/E8 (network namespaces)"
"$BIN/hostside" >"$OUT/hostside.log" 2>&1 &
HOSTSIDE_PID=$!
for _ in $(seq 1 50); do [ -e "$SCR/hostside.ready" ] && break; sleep 0.1; done
if [ ! -e "$SCR/hostside.ready" ]; then
  add E7 BLOCKED "host-side fixtures did not start"; add E8 BLOCKED "host-side fixtures did not start"
else
  # shellcheck disable=SC1091
  . "$SCR/hostside.env"
  {
    echo "--- controls on the HOST (same targets must be reachable from the host namespace, otherwise a namespace 'FAIL' proves nothing)"
    echo "host netns: $(readlink /proc/self/ns/net)"
    "$BIN/probe" tcp "127.0.0.1:$PA"
    "$BIN/probe" tcp "[::1]:$PA"
    "$BIN/probe" abstract "$ABSNAME"
    "$BIN/probe" dnsudp "127.0.0.1:$DNSPORT" allowed.test
    "$BIN/probe" udp 127.0.0.1:443
    curl -q -sS --max-time 5 --noproxy '*' "http://127.0.0.1:$PA/ok"
    echo "--- end controls"
  } >"$OUT/ns-controls.log" 2>&1
  cat "$OUT/ns-controls.log"
  ctl_ok=$(grep -c '^PROBE OK' "$OUT/ns-controls.log")
  [ "$ctl_ok" -ge 3 ] || echo "WARNING: host controls did not behave as expected ($ctl_ok OK probes)"

  declare -A MODE_RAN=()
  if timeout 10 unshare -Urn true 2>"$OUT/unshare-probe.txt"; then
    timeout 150 unshare -Urn bash "$HERE/ns-checks.sh" unshare >"$OUT/ns-unshare.log" 2>&1; MODE_RAN[unshare]=1
  else
    add "E7/E8 unshare" BLOCKED "unshare -Urn failed: $(head -c 150 "$OUT/unshare-probe.txt")"
  fi
  if timeout 10 bwrap --unshare-net --ro-bind / / true 2>"$OUT/bwrap-probe.txt"; then
    timeout 150 bwrap --unshare-net --ro-bind / / --dev /dev --proc /proc --die-with-parent --new-session \
      env DIR="$SCR" BIN="$BIN" bash "$HERE/ns-checks.sh" bwrap >"$OUT/ns-bwrap.log" 2>&1; MODE_RAN[bwrap]=1
  else
    add "E7/E8 bwrap" BLOCKED "bwrap --unshare-net failed: $(head -c 150 "$OUT/bwrap-probe.txt")"
  fi
  curl -q -sS --max-time 5 --noproxy '*' "http://127.0.0.1:$PADM/hits" >"$OUT/hostside-hits.txt" 2>&1
  curl -q -sS --max-time 5 --noproxy '*' "http://127.0.0.1:$PADM/proxylog" >"$OUT/proxylog.txt" 2>&1
  echo "host-side counters after namespace runs: $(cat "$OUT/hostside-hits.txt")"
  forb=$(sed -E 's/.*forbidden=([0-9]+).*/\1/' "$OUT/hostside-hits.txt")
  absn=$(sed -E 's/.*abstract=([0-9]+).*/\1/' "$OUT/hostside-hits.txt")

  evalq() { # E7|E8
    local q=$1 bad=0 good=0 ran=0 m
    for m in unshare bwrap; do
      [ -n "${MODE_RAN[$m]:-}" ] || continue
      ran=$((ran+1))
      local b g
      b=$(grep -E "^CHECK $m [^ ]*$q[^ ]* BAD" "$OUT/ns-$m.log" | wc -l)
      g=$(grep -E "^CHECK $m [^ ]*$q[^ ]* GOOD" "$OUT/ns-$m.log" | wc -l)
      bad=$((bad+b)); good=$((good+g))
      grep -E "^CHECK $m [^ ]*$q[^ ]* BAD" "$OUT/ns-$m.log" | cut -c1-200
    done
    if [ $ran = 0 ]; then add "$q" BLOCKED "no namespace tool worked"
    elif [ $bad -gt 0 ]; then add "$q" FAIL "$bad BAD checks ($good GOOD) in $ran mode(s)"
    elif [ $ran -lt 2 ]; then add "$q" BLOCKED "only $ran of 2 namespace modes could run ($good GOOD checks)"
    else add "$q" PASS "$good GOOD checks, 0 BAD, unshare -Urn and bwrap --unshare-net"; fi
  }
  evalq E7; evalq E8
  if [ "${forb:-x}" = 0 ]; then echo "host-side: forbidden-port fixture was never contacted (GOOD)"; else add "E7/E8 forbidden-fixture" FAIL "forbidden fixture saw connections: $forb"; fi
  if [ "${absn:-x}" = 1 ]; then echo "host-side: abstract socket served only the host control (1 hit, GOOD)"; else add "E8 abstract" FAIL "abstract socket hits=$absn (expected exactly the 1 host control)"; fi

  # E9 is a limits analysis. Only the evidence-backed part is graded here.
  nr=0
  for m in unshare bwrap; do
    [ -n "${MODE_RAN[$m]:-}" ] && nr=$((nr + $(grep -cE "^CHECK $m (E7.no-route|E7.udp-noroute|E7.E8.no-dns|E7.E8.ipv6).* GOOD" "$OUT/ns-$m.log")))
  done
  if [ $nr -gt 0 ]; then add E9 PASS "limits documented; $nr no-route/UDP/DNS/IPv6 checks GOOD (blocked only by missing route). Routed-namespace cases NOT TESTED"
  else add E9 NOT\ TESTED "no namespace evidence available"; fi
fi

echo
echo "== offline re-run: E1..E6 again inside a namespace that has only 'lo' (proves the Go experiments need no real network or DNS)"
if timeout 10 unshare -Urn true 2>/dev/null; then
  offbad=0
  for q in e1 e2 e3 e4 e5 e6; do
    timeout 150 unshare -Urn bash -c 'ip link set lo up && exec "$@"' _ "$BIN/spike" "$q" >"$OUT/offline-$q.log" 2>&1
    v=$(grep '^VERDICT' "$OUT/offline-$q.log" | tail -1 | sed -E 's/^VERDICT [A-Z0-9]+ (PASS|FAIL|NOT TESTED|BLOCKED) \|.*/\1/')
    echo "offline $q: ${v:-no verdict}"
    [ "$v" = PASS ] || offbad=$((offbad+1))
  done
  if [ $offbad = 0 ]; then add "offline-rerun" PASS "E1-E6 give PASS inside a lo-only netns (no external network or DNS used)"
  else add "offline-rerun" FAIL "$offbad of 6 experiments did not PASS in the lo-only netns"; fi
else
  add "offline-rerun" BLOCKED "unshare -Urn unavailable"
fi

echo
echo "== VERDICT TABLE"
printf '%-24s %-11s %s\n' QUESTION VERDICT NOTE
for r in "${ROWS[@]}"; do IFS='|' read -r a b c <<<"$r"; printf '%-24s %-11s %s\n' "$a" "$b" "$c"; done
rm -f "$SCR/hostside.ready"
fails=$(printf '%s\n' "${ROWS[@]}" | grep -c '|FAIL|')
echo
echo "cleanup follows (processes and $SCR)"
exit $(( fails > 0 ? 1 : 0 ))
