#!/usr/bin/env bash
# Runs INSIDE a fresh network namespace (E7/E8). Invoked by run-all.sh as
#   unshare -Urn  bash ns-checks.sh unshare
#   bwrap --unshare-net ... bash ns-checks.sh bwrap
# Required env: DIR (scratch dir with hostside.env), BIN (built binaries).
# Prints "CHECK <mode> <id> GOOD|BAD want=<x> :: <observed>" lines. GOOD means
# the observation matched the expectation of the design under test.
set -u
MODE=${1:?mode}
# shellcheck disable=SC1091
. "$DIR/hostside.env"
P="$BIN/probe"
PROXY=http://127.0.0.1:3128
BG=()
cleanup() { for p in "${BG[@]:-}"; do [ -n "$p" ] && kill "$p" 2>/dev/null; done; wait 2>/dev/null; }
trap cleanup EXIT

say() { echo "INFO  $MODE $*"; }
verdict() { # id want observed-text good(0/1)
  local tag=BAD; [ "$4" = 1 ] && tag=GOOD
  echo "CHECK $MODE $1 $tag want=$2 :: $(echo "$3" | tr '\n' ' ' | cut -c1-260)"
}
pexpect() { # id want(OK|FAIL) probe-args...
  local id=$1 want=$2; shift 2
  local line; line=$(timeout 10 "$P" "$@" 2>&1 | head -1)
  local got; got=$(echo "$line" | awk '{print $2}')
  local good=0; [ "$got" = "$want" ] && good=1
  verdict "$id" "probe:$want" "$line" "$good"
}
pcontains() { # id substring probe-args...   (probe output must contain substring)
  local id=$1 sub=$2; shift 2
  local line; line=$(timeout 10 "$P" "$@" 2>&1 | head -1)
  local good=0; [[ "$line" == *"$sub"* ]] && good=1
  verdict "$id" "contains:$sub" "$line" "$good"
}
cexpect() { # id want(200|deny|fail) url [curl args]  (through the proxy unless --noproxy is given)
  local id=$1 want=$2 url=$3; shift 3
  local out rc
  out=$(env -i PATH="$PATH" HTTPS_PROXY="$PROXY" http_proxy="$PROXY" curl -q -sS --max-time 8 --connect-timeout 4 --cacert "$CA" \
    -w '\n[http_code=%{http_code} proxy_connect=%{http_connect} exit=%{exitcode}]' "$@" "$url" 2>&1); rc=$?
  local good=0
  case $want in
    200)  [ $rc = 0 ] && [[ "$out" == *"http_code=200"* ]] && good=1 ;;
    deny) { [ $rc = 0 ] && [[ "$out" == *"http_code=403"* ]]; } || { [ $rc != 0 ] && [[ "$out" == *"proxy_connect=403"* ]]; } && good=1 ;;
    fail) [ $rc != 0 ] && good=1 ;;
  esac
  verdict "$id" "curl:$want" "rc=$rc $out" "$good"
}

echo "=== namespace checks: mode=$MODE pid=$$ ns=$(readlink /proc/self/ns/net) (host ns recorded by runner) ==="
if [ "$MODE" = unshare ]; then
  say "before 'ip link set lo up': $(ip -o link show lo 2>&1 | awk '{print $2, $9, $10}')"
  ip link set lo up 2>&1 | sed 's/^/INFO  lo-up: /'
fi
say "interfaces: $(ip -o link show 2>&1 | awk '{print $2, $9, $10}' | tr '\n' ';')"
say "IPv4 addresses: $(ip -o -4 addr show 2>&1 | awk '{print $2,$4}' | tr '\n' ';')"
say "IPv6 addresses: $(ip -o -6 addr show 2>&1 | awk '{print $2,$4}' | tr '\n' ';')"
say "IPv4 routes: [$(ip -4 route show 2>&1 | tr '\n' ';')]  IPv6 routes: [$(ip -6 route show 2>&1 | tr '\n' ';')]"
say "default route lines in /proc/net/route: $(awk 'NR>1 && $2=="00000000"' /proc/net/route | wc -l)"
"$P" caps x | sed "s/^/INFO  $MODE /"
say "/etc/resolv.conf nameservers: $(grep -h '^nameserver' /etc/resolv.conf | tr '\n' ' ')"
nl=$(ip -o link show | grep -vc ': lo')
verdict E7.only-loopback-interface "no interface except lo" "non-lo interfaces=$nl" "$([ "$nl" = 0 ] && echo 1 || echo 0)"
dr=$(awk 'NR>1 && $2=="00000000"' /proc/net/route | wc -l)
verdict E7.no-default-route "no default route" "default routes=$dr" "$([ "$dr" = 0 ] && echo 1 || echo 0)"

# ---------------------------------------------------------------- E7 / E8 direct attempts (no proxy)
pexpect E7.E8.direct-tcp-host-fixture FAIL tcp "127.0.0.1:$PA"
pexpect E7.direct-tcp-host-tls-fixture FAIL tcp "127.0.0.1:$PT"
pexpect E7.direct-tcp-host-admin FAIL tcp "127.0.0.1:$PADM"
pexpect E7.direct-tcp-forbidden-port-fixture FAIL tcp "127.0.0.1:$PF"
t0=$(date +%s%N); line=$(timeout 10 "$P" tcp 192.0.2.1:80 | head -1); ms=$(( ($(date +%s%N) - t0) / 1000000 ))
g=0; [[ "$line" == PROBE\ FAIL* && "$line" == *"unreachable"* && $ms -lt 1500 ]] && g=1
verdict E7.no-route-192.0.2.1-tcp "FAIL, 'network is unreachable', fast" "$line (wall ${ms}ms)" $g
pexpect E7.udp-noroute-192.0.2.1:443-quic-case FAIL udp 192.0.2.1:443
pcontains E7.udp-noroute-error-is-unreachable "unreachable" udp 192.0.2.1:443
pexpect E7.udp-local-127.0.0.1:443 FAIL udp 127.0.0.1:443
pexpect E7.E8.no-dns-direct-to-host-mock-resolver FAIL dnsudp "127.0.0.1:$DNSPORT" allowed.test
pexpect E7.E8.no-dns-via-resolv.conf FAIL dns allowed.test
pexpect E7.E8.ipv6-loopback-host-fixture FAIL tcp "[::1]:$PA"
pexpect E7.E8.abstract-socket-not-reachable FAIL abstract "$ABSNAME"
pexpect E8.tool-ignoring-proxy-env-direct-tcp FAIL tcp "127.0.0.1:$PA"
line=$(timeout 10 nsenter --net="/proc/$HOSTSIDE_PID/ns/net" -- "$P" tcp "127.0.0.1:$PA" 2>&1 | head -1)
g=0; [[ "$line" != PROBE\ OK* ]] && g=1
verdict E8.setns-into-host-netns-denied "nsenter into the host network namespace must fail" "${line:-<empty>}" $g
cexpect E8.curl-noproxy-direct fail "http://127.0.0.1:$PA/ok" --noproxy '*'
echo "--- raw socket / packet socket attempts (E8)"
"$P" rawsock x | sed "s/^/INFO  $MODE /"
rs=$("$P" rawsock x | grep -c '^PROBE OK rawsock sendto')
verdict E8.raw-socket-cannot-send-off-host "no successful raw send to 192.0.2.1" "successful raw sends=$rs (raw socket creation itself may or may not be permitted: see INFO above)" "$([ "$rs" = 0 ] && echo 1 || echo 0)"

# ---------------------------------------------------------------- proxy reachability via socat
socat TCP-LISTEN:3128,bind=127.0.0.1,fork,reuseaddr UNIX-CONNECT:"$SOCK" &
BG+=($!)
for _ in $(seq 1 30); do "$P" tcp 127.0.0.1:3128 | grep -q '^PROBE OK' && break; sleep 0.1; done
pexpect E7.socat-relay-listening OK tcp 127.0.0.1:3128
cexpect E7.proxy-allowed-plain-literal-ip 200 "http://127.0.0.1:$PA/ok"
cexpect E7.proxy-allowed-plain-name-resolved-by-proxy 200 "http://allowed.test:$PA/ok"
cexpect E7.proxy-allowed-tls-literal-ip 200 "https://127.0.0.1:$PT/ok"
cexpect E7.proxy-allowed-tls-name 200 "https://allowed.test:$PT/ok"
cexpect E8.ipv6-literal-allowed-by-policy-via-proxy 200 "http://[::1]:$PA/ok" -g
cexpect E7.proxy-denied-port-plain deny "http://127.0.0.1:$PF/ok"
cexpect E7.proxy-denied-port-connect deny "https://127.0.0.1:$PF/ok"
cexpect E7.proxy-denied-admin-port deny "http://127.0.0.1:$PADM/hits"
cexpect E7.proxy-denied-private-name deny "http://deny.test:$PA/ok"
cexpect E7.proxy-denied-mixed-name-plain deny "http://mixed.test:$PA/ok"
cexpect E7.proxy-denied-mixed-name-connect deny "https://mixed.test:$PT/ok"
cexpect E7.proxy-denied-documentation-addr deny "http://192.0.2.1/"
cexpect E7.proxy-denied-metadata-addr deny "http://169.254.169.254/latest/meta-data/"
cexpect E7.direct-still-fails-after-proxy-use fail "http://127.0.0.1:$PA/ok" --noproxy '*'

# ---------------------------------------------------------------- E8: raw bytes to the relay port, bypassing the proxy protocol
pcontains E8.raw-origin-form-request-refused "400" rawsend 127.0.0.1:3128 "GET /ok HTTP/1.1\r\nHost: 127.0.0.1:$PA\r\nConnection: close\r\n\r\n"
pcontains E8.raw-CONNECT-forbidden-port-refused "403" rawsend 127.0.0.1:3128 "CONNECT 127.0.0.1:$PF HTTP/1.1\r\nHost: 127.0.0.1:$PF\r\n\r\n"
pcontains E8.raw-garbage-refused "400" rawsend 127.0.0.1:3128 "hello\r\n\r\n"
pcontains E8.raw-CONNECT-metadata-refused "403" rawsend 127.0.0.1:3128 "CONNECT 169.254.169.254:80 HTTP/1.1\r\nHost: 169.254.169.254:80\r\n\r\n"
pcontains E8.raw-CONNECT-allowed-opens-tunnel "200" rawsend 127.0.0.1:3128 "CONNECT 127.0.0.1:$PA HTTP/1.1\r\nHost: 127.0.0.1:$PA\r\n\r\n"
# path-based Unix socket on the shared filesystem is reachable across the namespace boundary
pcontains E7.path-unix-socket-reachable-from-namespace "403" unix "$SOCK" "CONNECT 127.0.0.1:$PF HTTP/1.1\r\nHost: 127.0.0.1:$PF\r\n\r\n"

# ---------------------------------------------------------------- Go relay instead of socat
"$BIN/relay" -listen 127.0.0.1:3129 -unix "$SOCK" &
BG+=($!)
for _ in $(seq 1 30); do "$P" tcp 127.0.0.1:3129 | grep -q '^PROBE OK' && break; sleep 0.1; done
PROXY=http://127.0.0.1:3129
cexpect E7.go-relay-allowed 200 "http://allowed.test:$PA/ok"
cexpect E7.go-relay-denied deny "http://127.0.0.1:$PF/ok"
PROXY=http://127.0.0.1:3128
echo "=== done mode=$MODE ==="
