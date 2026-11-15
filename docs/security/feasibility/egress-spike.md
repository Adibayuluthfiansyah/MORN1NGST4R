# Egress enforcement feasibility spike

Status: spike result, not a product decision. Nothing here changes `security.md`, `architecture.md` or any ADR.
Scope: can MORN1NGST4R force outbound HTTP(S) to addresses that passed its scope/address policy, using (A) a first-party Go HTTP client and (B) an isolated policy proxy for external tools that run in an unprivileged network namespace?

Everything below was executed on this machine against local mock services. Where a result is not backed by a run, the verdict says NOT TESTED or BLOCKED.

## 0. Host, run and reproducibility

| Item | Observed |
| --- | --- |
| Run date/time | 2026-11-15, 22:41 to 22:43 (UTC+07:00, host clock) |
| Kernel / arch | Linux 7.2.11-zen1-1-zen x86_64 |
| Go | go1.27.3 linux/amd64, standard library only, `go.mod` has no `require` lines, `CGO_ENABLED=0`, `GOPROXY=off`, `GOTOOLCHAIN=local` |
| curl | 8.23.0 (OpenSSL 3.6.5, nghttp2 1.70.0) |
| socat | 1.8.1.3 |
| bubblewrap | 0.13.1 |
| unshare | util-linux 2.42.4 |
| User namespaces | `kernel.unprivileged_userns_clone=1`, `user.max_user_namespaces=30251`; run as uid 1000, no sudo |
| IPv6 loopback | available (`::1/128` on `lo`), so no IPv6 test is BLOCKED |
| Hardware | 8 CPUs, about 7.7 GB RAM |

Re-run (idempotent, no root, no installs, no network access):

```bash
bash scripts/spikes/egress/run-all.sh                    # about 40 s
KEEP_LOGS=/some/dir bash scripts/spikes/egress/run-all.sh   # also keeps the full logs
```

`run-all.sh` builds into `/tmp/ms-spike-egress-XXXXXX/`, runs E1 to E9, re-runs E1 to E6 inside a loopback-only namespace, prints a verdict table, kills every child process and deletes the scratch directory. The logs of the run documented here are in `scripts/spikes/egress/last-run/`. Ports differ on every run. Expected runtime 30 to 60 s. Required tools: go, curl, socat, unshare, bwrap, ip, timeout.

Code layout (`scripts/spikes/egress/`, module `spike/egress`):

| Path | Content |
| --- | --- |
| `policy.go`, `policy_test.go` | tiny evaluator (address classes, lab mode, ports, host canonicalization) and its unit tests |
| `dnsmock.go` | UDP mock DNS (A, AAAA, CNAME), per-name query counters, step-wise answers for rebinding |
| `client.go` | first-party client: canonical URL, resolve once, pinned dial, manual redirect loop |
| `proxy.go` | policy proxy (CONNECT and absolute-URI HTTP) |
| `fixtures.go`, `util.go` | local HTTP/HTTPS/HTTP2 fixtures, ephemeral in-process ECDSA CA, verdict reporter |
| `cmd/spike` | experiments `e1` to `e6` |
| `cmd/hostside`, `cmd/probe`, `cmd/relay`, `ns-checks.sh` | host-side fixtures + proxy on a Unix socket, in-namespace probes, Go relay, E7/E8 checks |

Policy under test (`policy.go`): an address is allowed only if it is global unicast and not loopback, private, link-local, unique-local, multicast, unspecified, reserved, a cloud metadata address (169.254.169.254, fd00:ec2::254) or assigned to a local interface. IPv4-mapped IPv6 is unmapped before evaluation. Lab mode allows 127.0.0.0/8 and ::1 on explicitly listed ports only. Metadata, unspecified and link-local are never allowed, not even in lab mode. If any resolved address is denied, the whole target is denied. A port outside 80/443 is denied unless it is a listed lab port on a lab prefix. Every scenario was also run with lab mode off.

Deviations to be aware of: the reserved-prefix list is hand-written, not generated from the IANA registries. Lab prefixes are configurable, and E1 narrows them to `127.0.0.1/32` so that `127.0.0.2` is a denied address that is still connectable.

## 1. Summary

| Question | Verdict | One-line result |
| --- | --- | --- |
| E1 DNS pinning / rebinding | PASS | One resolution per request (1 A + 1 AAAA). The connection goes to the validated IP. Letting net/http resolve again reached the rebound address. |
| E2 Mixed answers and CNAME | PASS | Any denied A, AAAA, CNAME-target or 4in6 address denies the whole name, with no dial. |
| E3 Happy Eyeballs / `Dialer.Control` | PASS | Control runs for every attempt, parallel or fallback, and a deny only fails that attempt. A single pinned IP is sufficient. |
| E4 Redirects | PASS | 50 checks over 23 Location shapes (each run through `http.Client` and `RoundTrip`) plus loop, port and control checks. Ambiguous Locations are refused. |
| E5 HTTP/2 coalescing | PASS (Go) | Go did not coalesce a.test and b.test, and the dial hook saw both origins. curl did not coalesce either. Other tools are NOT TESTED. |
| E6 Policy proxy | PASS | Denied address, denied port, mixed answer and the proxy's own port are refused. HTTPS redirects are only enforced per new CONNECT. |
| E7 Namespace and proxy reachability | PASS | Both `unshare -Urn` and `bwrap --unshare-net`: no direct path out, the Unix socket relay reaches only the proxy. |
| E8 Bypass attempts | PASS | Every attempt failed or was refused by the proxy. Section E8 states which control did the blocking. |
| E9 Limits | PASS (documented) | Blocked-only-because-no-route items observed. Routed-namespace cases are NOT TESTED. |
| Offline re-run | PASS | E1 to E6 give the same verdicts inside a namespace that has only `lo`. |

Findings that need a design decision are in section 3.

## E1. DNS pinning and rebinding

Commands:

```bash
cd scripts/spikes/egress && go build -o "$BIN" ./cmd/... && "$BIN/spike" e1
```

Setup: mock DNS on `127.0.0.1:<port>`. `rebind.test` answers `127.0.0.1` the first time and `127.0.0.2` from the second query on. Fixture A listens on `127.0.0.1:P`, fixture B on `127.0.0.2:P`. The resolver is `net.Resolver{PreferGo: true, Dial: <always the mock over UDP>}`.

Observed (`last-run/e1.log`):

```text
request 1 body: "fixture=fixture-A host=rebind.test:36645 path=/ok remote=127.0.0.1:42846 ..."
DNS log after request 1: [q rebind.test type=28 round=0 q rebind.test type=1 round=0]
ok  req1-one-lookup :: A queries=1 AAAA queries=1
ok  req2-rebound-answer-denied-no-connect :: err=denied: 127.0.0.2 denied: loopback address outside lab prefixes B.conns=0 (A queries 1 -> 2 ...)
ok  tls-sni-is-canonical-name   (server saw sni=rebind-tls.test, certificate verified against the name)
CONTROL step 1: validator lookup -> allowed=true addrs=[127.0.0.1] (A queries so far: 1)
CONTROL step 2: net/http did its own lookup -> A queries now 2; body="fixture=fixture-B host=rebind.test:36645 ..."
```

Reading:

- The first-party client resolves once, validates, then `Transport.DialContext` ignores the `name:port` it is handed and dials `127.0.0.1:P` explicitly. The server saw `Host: rebind.test:P` and, over TLS, `SNI=rebind-tls.test`, so name-based verification still works.
- The DNS server saw exactly one A and one AAAA query for the request. No query is made at dial time.
- A second request resolves again (A queries 1 to 2), gets the rebound answer, and is denied with zero connections to B. A new request is a new target, which is the intended behaviour.
- Control experiment: validate with one lookup, then give the URL to a `net/http` client whose dialer uses the same resolver. It does its own lookup and connects to fixture B (127.0.0.2), an address the policy had never evaluated. This is the difference the pinned dial removes.
- Lab off: the same URL is denied and fixture A's connection count does not change.

Verdict: PASS.
Limits: only Go's pure-Go resolver was exercised (the cgo/system resolver and `/etc/hosts` handling were not). Mock TTL is 0 and no resolver cache exists, so cache poisoning and TTL policy are NOT TESTED. A connection reused across requests is avoided by design (new transport per hop, keep-alives off), so the cost of that choice was not measured.

## E2. Mixed answers and CNAME

Command: `"$BIN/spike" e2`. Log: `last-run/e2.log`.

| Name | Answers | Result |
| --- | --- | --- |
| `ok-dual.test` | A 127.0.0.1, AAAA ::1 | allowed, connected |
| `ok-via-cname.test` | CNAME hop1, CNAME hop2, A 127.0.0.1 | allowed, connected |
| `mixed-a.test` | A 127.0.0.1 + A 10.0.0.5 | denied (10.0.0.5 private) |
| `mixed-a-meta.test` | A 127.0.0.1 + A 169.254.169.254 | denied (metadata) |
| `aaaa-bad.test` | A 127.0.0.1, AAAA fc00::5 (only AAAA denied) | denied |
| `aaaa-meta.test` | A 127.0.0.1, AAAA fd00:ec2::254 | denied (metadata) |
| `a-bad.test` | A 10.0.0.5, AAAA ::1 (only A denied) | denied |
| `mapped.test` | A 127.0.0.1, AAAA ::ffff:10.0.0.5 | denied (unmapped to 10.0.0.5) |
| `www.test` | CNAME cdn, CNAME edge, A 10.0.0.9 | denied |
| `www-meta.test` | CNAME m1, A 169.254.169.254 | denied |
| `www-mixed.test` | CNAME m2, A 127.0.0.1 + A 192.168.1.10 | denied |

Every denied name had `contacted=false` and `dial-events=0`. With lab mode off all 11 names were denied and the connection counters did not change. The dual-stack control connected to `[::1]` only (one validated address, dial events: `control ALLOW [::1]:P`, `connected`).

Verdict: PASS.
Limits: the evaluation is on final addresses. The CNAME names themselves are not checked against scope (the rules do not require it, but a scope rule on the CNAME target name is not implemented). The mock puts the whole chain in one answer, as a recursive resolver does. EDNS, DNSSEC and truncation/TCP fallback are NOT TESTED. Each resolution is 1 A + 1 AAAA query (counters show `2/2` for the evaluator call plus the fetch call, one pair each).

## E3. Happy Eyeballs and `net.Dialer.Control`

Command: `"$BIN/spike" e3`. Log: `last-run/e3.log`. Observed on go1.27.3 only.

`dual.test` resolves to `[::1 127.0.0.1]` (the order the resolver returned), so IPv6 is the "primary" family, because Go takes the family of the first address as primary. A broken primary is simulated three ways: nothing listening (refused), and a listener with `listen(fd, 0)` whose accept queue was filled (SYNs dropped, the connect hangs). `ControlContext` logs every call with a timestamp.

| Scenario | Control calls (observed) | Outcome |
| --- | --- | --- |
| both families listening, defaults | `tcp6 [::1]` @0ms | connected to `[::1]`, 1 call |
| primary refuses | `tcp6 [::1]` @0ms; `tcp4 127.0.0.1` @0ms | fallback starts immediately on refusal, 2 calls |
| primary hangs, default FallbackDelay | `tcp6` @0ms; `tcp4` @300ms | connected to 127.0.0.1 after 300 ms |
| primary hangs, FallbackDelay 50 ms / 1 s | `tcp4` @50ms / @1000ms | same, delay respected |
| primary hangs, FallbackDelay -1, Timeout 1 s | `tcp6` @0ms only | first address used the whole budget, second never attempted, `i/o timeout` after 1000 ms (no Control call for the second address) |
| primary hangs, FallbackDelay -1, Timeout 6 s | `tcp6` @0ms; `tcp4` @3002ms | serial dial, first address got half the remaining time |
| Control denies the primary family | `tcp6` DENY; `tcp4` ALLOW | dialer fell back, connected to 127.0.0.1, Control ran for the fallback too |
| Control denies everything | `tcp6` DENY; `tcp4` DENY | error `policy: [::1]:P denied`, listener accept count 0 |
| `Control` and `ControlContext` both set | only `ControlContext` | `Control` is ignored |
| dial the IP literal `[::1]:P` | 1 call | no Happy Eyeballs; a deny returns the error, nothing to fall back to |

Conclusions:

1. Control (and ControlContext) is invoked once per connection attempt, including the delayed parallel attempt and serial fallbacks. It receives the real remote `ip:port` and network `tcp4`/`tcp6`.
2. A deny from Control fails that attempt only. The dialer then tries the next address, and Control is called for it as well. So a Control-based check is a valid last line of defence, but it will not stop a lookup the dialer performed itself, and it cannot make a name "denied as a whole".
3. A short `Dialer.Timeout` with Happy Eyeballs disabled lets a hanging first address starve the others. This matches `partialDeadline` in `$GOROOT/src/net/dial.go` (`saneMinimum = 2s`).
4. Pre-resolving once and dialing only validated IPs (`PinnedDial`) is simpler and sufficient: one resolution, whole-target evaluation first, then one attempt at a time, each re-checked in Control. Observed cost: when the first validated address hangs, the serial dial waits for its per-address timeout (3 s in the spike, 3001 ms measured) before trying the next. Latency is the only cost.

Verdict: PASS.
Limits: behaviour is specific to go1.27.3. The hanging family was `::1` in all runs; the symmetric case (IPv4 primary) was not run. UDP and DNS dials are not covered by these Control hooks.

## E4. Redirects

Command: `"$BIN/spike" e4`. Log: `last-run/e4.log`.

The client uses `CheckRedirect = http.ErrUseLastResponse` and a manual loop. Each Location is resolved against the current URL, canonicalized (`CanonicalURL`), resolved once, and fully evaluated before it is followed. A new transport per hop prevents connection reuse across hops. Lab ports: P1 listed, P2 not listed (unless stated).

| Location from fixture | Code | Result |
| --- | --- | --- |
| `http://169.254.169.254:80/...` | 301 | hop 2 DENY (cloud metadata), not contacted |
| `http://10.0.0.5:80/` | 302 | DENY (private) |
| `//127.0.0.1:P1/ok` (scheme-relative) | 307 | followed, 200 |
| `//127.0.0.1:P2/ok` | 308 | DENY (lab port not listed), P2 connection count unchanged |
| `http:\\127.0.0.1:P1/ok` (backslash) | 302 | refused: backslash is ambiguous. Go parses it as opaque, a browser as `http://host/`. |
| `/ok` (relative) | 301, 302, 303, 307, 308 | followed, evaluated again as a new hop |
| `http://127.0.0.1:P2/ok` (other port, same host) | 302 | DENY. With P2 added to the lab list: followed (new target). |
| `http://127.0.0.1:P1@10.0.0.5/` (userinfo trick) | 302 | refused (userinfo) |
| `http://[::1/`, `http://%zz/`, `http://%31%32%37.0.0.1:P1/ok` | 302 | not followed |
| `http://127.1:P1/ok`, `http://0x7f.0.0.1:P1/ok` | 302 | refused (numeric shorthand) |
| empty Location, `ftp://`, `file:///etc/passwd`, `javascript:alert(1)`, space in Location | 302 | not followed |
| `http://nxdomain.test:P1/ok` | 302 | DENY (resolution failed, fail closed) |
| `/loop/a` to `/loop/b` to `/loop/a` | 302 | third hop refused as loop (3 hops) |

Findings:

- `http.Client` itself returns an error for an unparseable Location (`failed to parse Location header ...`) before `CheckRedirect` runs. That is safe (nothing is followed), but our parser never sees the value. The same 23-case table was therefore also run through `Transport.RoundTrip` (`Client.RawRoundTrip`), where the loop sees every Location and refuses it by its own rules. Both modes produced identical decisions (50 checks).
- Control: the default `http.Client` followed `//127.0.0.1:P2/ok` to the unlisted port (P2 connections 1 to 2), and rejected the backslash form with `http: no Host in request URL`.
- Lab off: the first hop is denied and no fixture connection is made.

Verdict: PASS.
Limits: GET only. 307/308 body replay, method handling, `Referer` and credential forwarding on cross-origin redirects were not tested. IDNA hosts are refused (not implemented). Redirect to a different scheme with the same host (http to https) was not run as a separate case.

## E5. HTTP/2 coalescing

Command: `"$BIN/spike" e5`. Log: `last-run/e5.log`.

Fixture: Go HTTP/2 TLS server (ALPN `h2`), one certificate with SAN `a.test` and `b.test`, both names resolving to `127.0.0.1` through the mock DNS. The Go client is one shared `http.Transport` (`ForceAttemptHTTP2`) whose `DialContext` evaluates `host:port`, logs, and dials the pinned address.

```text
GET a.test -> HTTP/2.0 ... remote=127.0.0.1:47318 sni=a.test      (4 requests, a,b,a,b)
GET b.test -> HTTP/2.0 ... remote=127.0.0.1:47330 sni=b.test
dial-hook origin=a.test:33133 allowed=true addrs=[127.0.0.1]
dial-hook origin=b.test:33133 allowed=true addrs=[127.0.0.1]
server accepted 2 TCP connection(s): [127.0.0.1:47318 127.0.0.1:47330]
concurrent (4 requests, 2 origins): dial-hook calls=4, new server connections=4
```

- Go (go1.27.3): no coalescing. Each origin (`host:port`) got its own HTTP/2 connection although the IP matched and the certificate covered both names. The dial hook ran once per connection and saw every origin. Server connections always equalled dial-hook calls (2 and 2, then 4 and 4).
- curl 8.23.0, one process, `curl --http2 --resolve a.test:P:127.0.0.1 --resolve b.test:P:127.0.0.1 --cacert ca.pem -w ... https://a.test:P/ https://b.test:P/ https://a.test:P/`:

```text
https://a.test:P/ http=2 new_connects=1 local_port=...518
https://b.test:P/ http=2 new_connects=1 local_port=...528
https://a.test:P/ http=2 new_connects=0 local_port=...518
server accepted 2 new connection(s) from curl
```

  curl reused the connection for the same name only. It did not coalesce a.test and b.test (two connections, `Established connection to b.test` in `-v`).
- Lab off: the hook denies the first dial, no connection is made.

Verdict: PASS for Go (the per-connection dial check sees every origin). curl: observed, no coalescing in this version.

What this does not show: RFC 9113 section 9.1.1 allows coalescing, and browsers and some HTTP/2 libraries do it. Only Go net/http (go1.27.3) and one curl build were tested. If a tool behind the proxy coalesces, the proxy would see one CONNECT and the second origin would ride on it. That is limited to the already validated IP and port, but the second name's own resolution would not have been evaluated. NOT TESTED in any other tool.

## E6. Policy proxy

Command: `"$BIN/spike" e6`. Log: `last-run/e6.log`.

`proxy.go` supports CONNECT and absolute-URI `http://` requests. It canonicalizes the host, resolves once, evaluates every address, pins the validated IP, enforces the port rule on CONNECT, never follows redirects and logs every decision. Lab ports: plain fixture and TLS fixture only. The forbidden-port fixture and the proxy's own TCP port are not listed. Driven by a Go client (`http.ProxyURL`) and by curl (`HTTPS_PROXY`, `http_proxy`).

| Case | Go client | curl |
| --- | --- | --- |
| allowed plain HTTP | 200, one A + one AAAA query | 200 |
| allowed HTTPS via CONNECT | 200, `sni=allowed.test` | 200 |
| name resolving to 10.0.0.5 | plain 403, CONNECT `Forbidden` | 403 / `CONNECT tunnel failed, response 403` |
| denied port (`127.0.0.1:<unlisted>`), plain and CONNECT | refused, fixture connection count 0 | refused |
| CONNECT to the proxy's own port | refused | not run |
| `mixed.test` (127.0.0.1 + 10.0.0.5), plain and CONNECT | refused | refused |
| lab off | all refused, no connection | not run |

What the proxy sees for HTTPS (from the proxy log):

```text
CONNECT allowed.test:PT ALLOW addrs=[127.0.0.1] remote=127.0.0.1:PT (request headers seen: [Proxy-Connection User-Agent])
TUNNEL allowed.test:PT closed c2s_bytes=1766 s2c_bytes=1209 first_client_byte=0x16 (TLS handshake record; contents not parsed, SNI not inspected)
```

- It sees: the CONNECT authority `host:port`, the CONNECT request headers, byte counts, and the first byte of the client stream.
- It cannot see: the inner `Host` header, the path, the status codes, redirects, or anything else inside TLS. SNI inspection is not performed. A client that does `CONNECT a.test:PT` and then sends SNI `b.test` was served (`sni=b.test host=a.test:PT` at the server) and the proxy log shows only `a.test`.
- Redirects over HTTPS are therefore enforced per new CONNECT. `curl -L https://allowed.test:PT/r?...loc=https://deny4.test:PT/` produced `CONNECT allowed.test ALLOW`, then `CONNECT deny4.test DENY 10.0.0.5`, and curl failed with `CONNECT tunnel failed, response 403`. A same-host redirect (`/r` to `/ok`) produced one CONNECT and two server requests, with no proxy-visible hop.
- Plain HTTP: every request carries an absolute URI, so every request, including each hop of a client-side redirect, is evaluated.
- Host header on plain HTTP: `net/http`'s server discards the client's `Host` header for absolute-form requests. A raw request `GET http://allowed.test:P/ok` with `Host: evil.test` reached the upstream as `Host: allowed.test:P` (curl `-H 'Host: evil.test'` gave the same). A Go client setting `req.Host=evil.test` rewrites its own request target, so the proxy evaluated `evil.test` and denied it. The override is neutralized, not refused and not audited. This proxy cannot detect the mismatch, because the header is gone before the handler runs.
- Non-HTTP bytes through CONNECT: after `CONNECT 127.0.0.1:P` (allowed) the proxy relayed `NOT-HTTP-AT-ALL` to the plain HTTP fixture, which answered `400 Bad Request`. The policy is `host:port`, the protocol is not enforced.

Verdict: PASS.
Limits: no HTTP/2 or WebSocket upgrade through the plain path, no chunked-upload or large-body test, no proxy authentication, no connection-count or duration limits beyond a 30 s tunnel deadline, no revocation of running tunnels (the product needs stop-within-30-s), no audit write before connect. A public address that the evaluator would allow could not be exercised end to end (no external targets), only through evaluator unit tests with no connection made.

## E7. Namespace isolation and proxy reachability

Commands (executed by `run-all.sh`, host-side fixtures, mock DNS and the proxy run in `cmd/hostside`, the proxy listens on `$SCR/proxy.sock` with mode 0600):

```bash
unshare -Urn bash scripts/spikes/egress/ns-checks.sh unshare
bwrap --unshare-net --ro-bind / / --dev /dev --proc /proc --die-with-parent --new-session \
      env DIR="$SCR" BIN="$BIN" bash scripts/spikes/egress/ns-checks.sh bwrap
# inside the namespace (ns-checks.sh):
ip link set lo up                                    # unshare mode only; bwrap brings lo up itself
socat TCP-LISTEN:3128,bind=127.0.0.1,fork,reuseaddr UNIX-CONNECT:"$SOCK" &
env -i PATH="$PATH" HTTPS_PROXY=http://127.0.0.1:3128 http_proxy=http://127.0.0.1:3128 curl -q --cacert "$CA" ...
```

Namespace state (identical in both modes):

```text
interfaces: lo (UNKNOWN)   IPv4: lo 127.0.0.1/8   IPv6: lo ::1/128
IPv4 routes: []  IPv6 routes: []   default route lines in /proc/net/route: 0
unshare mode, before 'ip link set lo up': lo DOWN
```

Host controls (same probes from the host namespace): TCP to the fixture on 127.0.0.1 and on `[::1]` connects, the abstract socket answers `ABSTRACT-SOCKET-OK`, the mock DNS answers. UDP to `127.0.0.1:443` is refused on the host too, so that probe alone proves nothing. The `192.0.2.1` UDP probe is the meaningful QUIC-case probe (below).

| Check (in the namespace) | Observed, both modes |
| --- | --- |
| TCP to the host fixtures (plain, TLS, admin, forbidden-port) on `127.0.0.1:<port>` | `connection refused` in 0 ms: the namespace's own loopback is a different, empty loopback |
| TCP to 192.0.2.1:80 | `connect: network is unreachable`, 2 to 3 ms wall time |
| UDP to 192.0.2.1:443 (QUIC case) | `dial udp ...: connect: network is unreachable` |
| UDP to 127.0.0.1:443 | `connection refused` (no listener in the namespace) |
| Host mock DNS `127.0.0.1:<dnsport>` | `connection refused`, no answer |
| `/etc/resolv.conf` (nameserver 127.0.0.53) and a Go lookup | `lookup allowed.test on 127.0.0.53:53: ... connection refused` |
| `[::1]:<fixture port>` | `connection refused`, and there is no IPv6 route |
| abstract Unix socket created by the host process | `dial unix @ms-spike-egress-abs-<pid>: connect: connection refused` |
| path-based Unix socket `$SCR/proxy.sock` | reachable: a `CONNECT` sent directly gets `HTTP/1.1 403 Forbidden` from the host proxy |

Proxy reachability through the relay (`socat` in the namespace to the host proxy socket):

| Request from a tool in the namespace | Result |
| --- | --- |
| `http://127.0.0.1:<fixture>/ok` and `http://allowed.test:<fixture>/ok` (name resolved by the proxy) | 200 |
| `https://127.0.0.1:<tls>/ok`, `https://allowed.test:<tls>/ok` | 200 via CONNECT |
| `http://[::1]:<fixture>/ok` (policy allows lab `::1`, the proxy dials from the host) | 200 |
| unlisted port plain and CONNECT, admin port, `deny.test` (10.0.0.5), `mixed.test` plain and CONNECT, `192.0.2.1`, `169.254.169.254` | 403 / `CONNECT tunnel failed, response 403` |
| `curl --noproxy '*' http://127.0.0.1:<fixture>/ok` after the proxy use | `curl: (7) Failed to connect ... Could not connect to server` |

Host side after both runs: `allowed=10 allowed_v6=3 tls=4 forbidden=0 abstract=1`. The forbidden-port fixture was never contacted, and the abstract socket was reached only by the host control.

Go relay instead of socat: `cmd/relay` (about 50 lines, loopback-only listen guard) started as `relay -listen 127.0.0.1:3129 -unix "$SOCK"` gave the same results (allowed 200, denied 403) in both modes.

Differences between the two tools:

| | `unshare -Urn` | `bwrap --unshare-net` |
| --- | --- | --- |
| process identity in the namespace | uid 0 (mapped), `CapEff 000001ffffffffff`, `NoNewPrivs 0` | uid 1000, `CapEff 0`, `NoNewPrivs 1` |
| `lo` | DOWN until `ip link set lo up` | already up |
| raw sockets | `AF_PACKET` socket created, ICMP raw send to 192.0.2.1 failed with `network is unreachable` | `socket(): operation not permitted` |
| `nsenter --net=/proc/<hostpid>/ns/net` | `Permission denied` | `Permission denied` |

Is `socat` acceptable? It works (`socat 1.8.1.3`) but it is an extra binary on the sandbox path, forks per connection, and printed `E read(6,...): Connection reset by peer` on every refused CONNECT. `security.md` requires pinned and digest-verified tool binaries, and a relay inside the sandbox boundary is such a binary. The Go relay passed the same checks, is built with the product and has no extra runtime dependency. Recommendation: use the Go relay. This is a recommendation from one run, not a measured comparison of overhead.

Verdict: PASS (62 checks GOOD, 0 BAD, both modes).
Limits: the namespace blocks everything because it has no route and no non-loopback interface. These tests do not show how a namespace with a route (veth, slirp4netns, pasta) would behave. The harness binds `/` read-only into bwrap, so a sandboxed tool can see the host filesystem, including the proxy socket path. Filesystem isolation, seccomp, PID/IPC namespaces and resource limits from `security.md` section 10 were not tested.

## E8. Bypass attempts from inside the namespace

Each attempt was run in both modes. "Blocked by" says which control stopped it.

| # | Attempt | Result | Blocked by |
| --- | --- | --- | --- |
| 1 | direct TCP to a host fixture (`127.0.0.1:<port>`) | `connection refused` | namespace |
| 2a | raw bytes to the relay port: `GET /ok HTTP/1.1` (origin-form) | `HTTP/1.1 400 Bad Request` | proxy |
| 2b | raw `CONNECT 127.0.0.1:<forbidden port>` | `HTTP/1.1 403 Forbidden` | proxy |
| 2c | raw `CONNECT 169.254.169.254:80` | `HTTP/1.1 403 Forbidden` | proxy |
| 2d | garbage bytes (`hello\r\n\r\n`) | `HTTP/1.1 400 Bad Request` | proxy |
| 2e | raw `CONNECT 127.0.0.1:<allowed port>` | `200 Connection Established`, a tunnel opens (policy permits it) | not blocked, by design |
| 3 | tool that ignores proxy env vars: direct TCP, and `curl --noproxy '*'` | refused (curl exit 7) | namespace |
| 4a | raw socket / `AF_PACKET`, ICMP send to 192.0.2.1 | unshare: sockets created, send fails with `network is unreachable`. bwrap: `operation not permitted`. | namespace (no route); bwrap also by dropped capabilities |
| 4b | UDP to 192.0.2.1:443 and to 127.0.0.1:443 | `network is unreachable` / `connection refused` | namespace |
| 5 | IPv6: `[::1]:<fixture port>` direct | `connection refused`, no IPv6 route | namespace |
| 5b | IPv6 literal through the proxy | 200 for the lab-listed `::1` fixture | proxy policy (allowed on purpose) |
| 6 | abstract Unix socket created on the host | `connection refused` | namespace (abstract sockets are per network namespace) |
| 7 | `/etc/resolv.conf` shows `nameserver 127.0.0.53`. Go lookup of `allowed.test` and a direct UDP DNS query to the host mock resolver | `connection refused` for both | namespace |
| 8 | `nsenter` into the host's network namespace via `/proc/<pid>/ns/net` | `Permission denied` | kernel permission check |
| 9 | path-based proxy socket connected directly (no relay) | reachable, proxy still answered 403 for the forbidden port | not blocked, proxy policy still applies |

Summary: attempts 1, 3, 4, 5, 6, 7 and 8 are blocked by the namespace itself (no interface other than `lo`, no route, separate loopback and abstract-socket namespace), independent of the proxy. Attempts 2a to 2d are stopped by the proxy. 2e and 9 show that anything that can reach the proxy can use it for any policy-allowed `host:port`, with any protocol. A per-job socket is needed so that one job cannot use another job's proxy.

Verdict: PASS (30 checks GOOD, 0 BAD, both modes).
Limits: attempts that need a route (a veth, a gateway, a DNS server on the path) cannot be tested here and are NOT TESTED.

## E9. Limits and what is not covered

Observed blocked only because the namespace has no route or no non-loopback interface (all of these would depend on the route's own filtering if a route existed): TCP to 192.0.2.1 (`network is unreachable`), UDP including port 443 (`network is unreachable`), direct DNS and the resolv.conf resolver (`connection refused` on the namespace's own loopback), IPv6 (no route; only `::1` was probed), raw-socket sends (`network is unreachable`).

| Case | Covered by this design? | Evidence |
| --- | --- | --- |
| Non-HTTP protocols over an allowed `host:port` | No. CONNECT relays arbitrary bytes. | E6, E8 2e: `NOT-HTTP-AT-ALL` relayed, `400 Bad Request` from the fixture |
| QUIC / HTTP/3 (UDP) | Only because there is no route. The proxy speaks TCP only. | E7: UDP 192.0.2.1:443 `network is unreachable`. With a route: NOT TESTED. |
| Tool-initiated DNS in a namespace that has a route | Not covered. The proxy resolves names for proxied requests only. | E7/E8 show DNS fails only because no resolver is reachable. With a route: NOT TESTED. |
| Certificate status fetches (OCSP, CRL, AIA) | Not covered. A tool that fetches them would need a route or the proxy and a declared destination. | NOT TESTED |
| TLS name mismatch inside a CONNECT tunnel (SNI or Host differs from CONNECT host) | Not detected by the proxy. Limited to the validated IP and port. | E6: `sni=b.test` through `CONNECT a.test` |
| Redirects inside HTTPS | Enforced only per new CONNECT, no per-hop visibility | E6 |
| HTTP/2 coalescing in other tools | Unknown | E5 (Go and curl only) |
| `Host` header override on plain HTTP | Neutralized, not refused or audited | E6 |
| Same IP and port hosting other virtual hosts | A name is not the target. The target is `IP:port`. | design observation, not run |
| Proxy sees nothing after CONNECT (path, headers, response) | Evidence and audit of request detail need another mechanism | E6 |
| Stopping running tunnels when scope or approval changes | Not implemented (30 s tunnel deadline only) | NOT TESTED |
| Resolver variants (system resolver, DoH, `/etc/hosts` aliasing) | Only the pure-Go resolver against the mock | NOT TESTED |
| Public addresses end to end | Evaluator unit tests only, no connection to any public host | NOT TESTED |

Verdict: PASS for the documented limits whose evidence came from E7/E8 (12 no-route, UDP, DNS and IPv6 checks GOOD). All routed-namespace cases are NOT TESTED.

## 2. Offline re-run

`run-all.sh` re-ran E1 to E6 as `unshare -Urn bash -c 'ip link set lo up && exec .../spike eN'`. All six printed `VERDICT ... PASS` inside a namespace that has only `lo`, which shows the Go experiments need no external network and no real DNS resolver.

## 3. Proven on this host, failed, not tested, blocked

Proven on this host (go1.27.3, Linux 7.2.11, unprivileged):

- Resolve-once, validate-all, dial-the-validated-IP prevents the rebinding that a second lookup allows (E1).
- Mixed A/AAAA, CNAME chains and 4in6 addresses deny the whole target (E2).
- `Dialer.Control`/`ControlContext` runs for every parallel and fallback attempt and a deny does not abort the dialer, so it is defence in depth and not the primary control (E3).
- A manual redirect loop (also over `RoundTrip`) evaluates every hop and refuses ambiguous Locations (E4).
- Go does not coalesce HTTP/2 across names, and its dial hook saw every origin. curl 8.23.0 does not either (E5).
- A Go policy proxy enforces address, port and mixed-answer rules per CONNECT and per absolute-URI request (E6).
- In both `unshare -Urn` and `bwrap --unshare-net`, a tool has no direct path out and reaches only the policy proxy through a path-based Unix socket relayed to a loopback TCP port (E7, E8).

Failed (a requirement from `security.md` that this spike could not meet as built):

- Refusing and auditing a `Host` header override at the proxy. It is neutralized, not detected. Meeting the rule needs raw request parsing before `net/http` drops the header.
- Per-hop redirect enforcement for HTTPS through the proxy. It is not possible by design (only per new CONNECT).
- Stopping running tunnels within 30 s of a revocation. Not implemented. There is only a 30 s tunnel deadline.

Not tested: routed namespaces, QUIC with a route, tool-side DNS with a route, OCSP/CRL/AIA fetches, other HTTP/2 clients (browsers), system and cgo resolver, DNS cache/TTL behaviour, IDNA, non-GET redirects, public addresses end to end, per-job proxy sockets, WebSocket/Upgrade, large bodies, proxy resource limits, seccomp/PID/IPC/filesystem isolation and resource limits, the startup isolation self-test, audit-before-connect.

Blocked: nothing. Unprivileged user namespaces and bwrap worked on this host. On a host that restricts them (for example AppArmor `apparmor_restrict_unprivileged_userns`), E7 and E8 would be BLOCKED. That configuration was not available to test.

Needed before production:

1. Generate the denied address list from the IANA special-purpose registries (the spike list is hand-written).
2. Make the proxy a real component: raw request parsing for Host handling, per-job Unix sockets, job identity, connection and duration limits, tunnel teardown on scope change or approval revocation, audit written before connect.
3. Harden the sandbox: drop capabilities, set `no_new_privs`, add seccomp and filesystem isolation (`bwrap` gave zero capabilities and `NoNewPrivs 1` here, `unshare -Urn` gave full capabilities inside the user namespace), and run the startup self-test with production arguments.
4. Decide the resolver: pure-Go resolver pointed at a configured upstream, caching policy, and where passive-recon DNS goes.
5. Decide how tools that do their own DNS or QUIC are kept off the network if a namespace ever gets a route.
6. Implement scope grammar, public-suffix wildcard refusal and lab registrations from the product rules. The spike only has the address and port part.
7. Test other HTTP/2 clients and non-GET redirects if tools other than the first-party client are allowed to follow redirects themselves.
8. Replace `socat` by the Go relay, or pin and verify it as a tool binary.

## 4. Side effects of the spike

- Files created: `scripts/spikes/egress/` (code, scripts, `last-run/` logs) and this document. Scratch: `/tmp/ms-spike-egress-*/`, removed by `run-all.sh` on exit.
- Early development `go` invocations (before `run-all.sh` redirected `XDG_CONFIG_HOME` into the scratch directory) updated Go's own local telemetry counters in `~/.config/go/telemetry/local/`. `run-all.sh` now keeps those and the build cache under `/tmp/ms-spike-egress-*/`.
- No external host was contacted, no DNS query left the machine (all resolvers are the loopback mock, and the offline re-run passed in a loopback-only namespace). The only non-loopback address used is the RFC 5737 address 192.0.2.1, in namespaces with no route.
