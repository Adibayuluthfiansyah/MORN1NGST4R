# Verification record: security libraries and data sources

## 1. Header

- Fetch date for every source below: **2026-11-15** (`date -u +%F` on the research host). Every URL in this document was fetched on that date unless a row says otherwise.
- Method: read-only `web_fetch` against primary sources (IANA, rfc-editor.org, unicode.org, WHATWG, publicsuffix.org, vendor documentation, OWASP cheat sheets, proxy.golang.org, pkg.go.dev, raw.githubusercontent.com for LICENSE, go.mod and source files, api.github.com for repository metadata). Local read-only commands: `date`, `go version` (result: `go version go1.27.3 linux/amd64`). Nothing was downloaded, installed, compiled or executed. No library under test was run. No address was probed.
- Version and date facts come from `proxy.golang.org/<module>/@latest` (the `Time` field is the proxy publication time of the tag).
- Secondary sources are labelled "SECONDARY" where used (Wikipedia, GitHub search API for candidate discovery, GitHub license detection).
- Everything marked "PROPOSED, not decided" is a recommendation for the owner. Nothing here is a decision.

### What was NOT verified (summary; full list in section 12)

- Behaviour of any Go library at run time (x/net/idna, x/net/publicsuffix, x/crypto/argon2, go-keyring). Only documentation and source text were read.
- A primary source for "Host header validation mitigates DNS rebinding against localhost services" (Q9). Only a secondary source states it.
- Hetzner metadata address (Hetzner docs are a single-page app; the fetch returned no text).
- Oracle Cloud IPv6 metadata address, AWS container-credential endpoints from AWS's own documentation (only OWASP lists them).
- The WHATWG URL spec revision date, the exact URL of IdnaTestV2.txt, the web-platform-tests URL test data.
- Licence of `github.com/disciplinedware/go-confusables`.
- IANA terms of use for registry data; legal effect of MPL-2.0 on a generated Go table (legal question, not a fact).
- SHA-256 of any data file (no file was downloaded, so no hash was computed).

---

## 2. Q1 (O-32): Public suffix data

### Verified facts

| Fact | Source URL | Quote or evidence |
| --- | --- | --- |
| The PSL list file is licensed MPL-2.0. | https://publicsuffix.org/list/public_suffix_list.dat (first lines); https://raw.githubusercontent.com/publicsuffix/list/main/LICENSE | File header: "This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0." LICENSE file starts "Mozilla Public License Version 2.0". |
| Canonical download location. | https://publicsuffix.org/list/ and the header of the file | "Please pull this list from, and only from https://publicsuffix.org/list/public_suffix_list.dat, rather than any other VCS sites. Pulling from any other URL is not guaranteed to be supported." |
| Update cadence and download etiquette. | https://publicsuffix.org/list/ | "The copy on publicsuffix.org ... is updated daily from GitHub." "download an updated list ... no more than once per day." "The list usually changes a few times per week." |
| The list is under source control on GitHub. | https://publicsuffix.org/list/ | "The list is kept in source code control on GitHub." |
| Current file version at fetch time. | https://publicsuffix.org/list/public_suffix_list.dat | `// VERSION: 2026-11-14_18-45-38_UTC` and `// COMMIT: dfe55f3548d0fe51c04b8d2903507a01fdec57ad`. Matches the newest commit from https://api.github.com/repos/publicsuffix/list/commits?per_page=1 (committed 2026-11-14T18:45:27Z). |
| Rule syntax and algorithm. | https://github.com/publicsuffix/list/wiki/Format | Wildcard `*` only in the leftmost position and must wildcard an entire label. `!` marks an exception that "takes priority over any other matching rule". "If no rules match, the prevailing rule is `*`." The list is UTF-8 Unicode, "not Punycode forms". Domain and rules "must be canonicalized ... lower-case, Punycode (RFC 3492) - prior to being compared." |
| Wildcard rule does not imply its base is itself a suffix. | same wiki page | "WARNING: Many libraries interpret `*.foo.com` as implying `foo.com`. So this would behave differently. See: https://github.com/publicsuffix/list/issues/694" |
| ICANN vs PRIVATE sections. | same wiki page | "ICANN domains ... are those delegated by ICANN or part of the IANA root zone database." "PRIVATE domains are amendments submitted by the domain holder, as an expression of how they operate their domain security policy." "some applications, such as browsers when considering cookie-setting, treat all entries the same, other applications may wish to treat ICANN domains and PRIVATE domains differently." Example given: `*.appspot.com` is PRIVATE. Section markers in the file: `// ===BEGIN ICANN DOMAINS===` (seen in the file header region). |
| "Public suffix list will diverge from client to client". | https://url.spec.whatwg.org/ section 3.2 | "Specifications should prefer the origin concept for security decisions. The notion of 'public suffix' and 'registrable domain' cannot be relied-upon to provide a hard security boundary, as the public suffix list will diverge from client to client." |

### Go package golang.org/x/net/publicsuffix

| Item | Value | Source |
| --- | --- | --- |
| Latest x/net release | v0.62.0, proxy time 2026-10-27T19:00:16Z | https://proxy.golang.org/golang.org/x/net/@latest |
| Licence | BSD-3-Clause text ("Redistribution and use in source and binary forms ... Neither the name of Google LLC ...") | https://raw.githubusercontent.com/golang/net/v0.62.0/LICENSE |
| go.mod `go` directive | `go 1.26.0` | https://raw.githubusercontent.com/golang/net/v0.62.0/go.mod |
| Embedded snapshot in v0.62.0 | `publicsuffix.org's public_suffix_list.dat, git revision d6c92f1bbb7433e5db7b8405c25d4035fb8ff376 (2026-02-06T07:36:33Z)`; `numTLD = 1450`; data in `//go:embed data/text`, `data/nodes`, `data/children` | https://raw.githubusercontent.com/golang/net/v0.62.0/publicsuffix/table.go |
| Snapshot age relative to current PSL | The snapshot is from 2026-02-06. The current PSL commit is from 2026-11-14. The snapshot is about 9 months older than the live list on the fetch date (computed from the two dates above). | table.go; PSL API |
| How it is regenerated | `go run gen.go`; default URL `https://publicsuffix.org/list/effective_tld_names.dat`; the version string is fetched from `https://api.github.com/repos/publicsuffix/list/commits?path=public_suffix_list.dat` unless `-version` and `-url` are given. Generation is a manual maintainer step. | https://raw.githubusercontent.com/golang/net/v0.62.0/publicsuffix/gen.go |
| Package doc | "This package provides a pre-compiled snapshot of Mozilla's PSL (Public Suffix List) data". `PublicSuffix(domain)` returns `(publicSuffix string, icann bool)`. "icann is whether the public suffix is managed by ICANN". Unlisted TLDs are returned as unmanaged (example output: `there.is.no.such-tld` returns `such-tld`, `icann=false`). | https://pkg.go.dev/golang.org/x/net/publicsuffix |
| Snapshot version readable at run time? | The constant is unexported (`const version = ...`). No public API exposes it (pkg.go.dev lists only `EffectiveTLDPlusOne`, `PublicSuffix`, `List`). | table.go; pkg.go.dev |
| Updatable without a dependency bump? | No. The table is part of module `golang.org/x/net`. A newer table needs a newer x/net release, or our own run of `gen.go` (a fork or vendored copy). | table.go header "generated by go run gen.go; DO NOT EDIT" |

Not verified: whether x/net ships the PSL's MPL-2.0 notice alongside its BSD-3-Clause LICENSE.

### Trade-offs

| Option | Deterministic tests | Freshness | Effort | Risk |
| --- | --- | --- | --- | --- |
| A. `golang.org/x/net/publicsuffix` | Deterministic per x/net version. The version is only visible by reading the module source, so a test cannot assert it. | Lags the live list (about 9 months at fetch time). Refresh only through an x/net bump. | Lowest. x/net is needed anyway for idna. | A wildcard base that became a public suffix after the snapshot is accepted. The project rule says "cannot be confidently distinguished" must be refused, so staleness weakens the guarantee. |
| B. Vendored `public_suffix_list.dat` with pinned SHA-256 and our own parser | Fully deterministic. The test asserts the pinned hash and the recorded `VERSION`/`COMMIT` lines. | Updated by a reviewed pull request, on our schedule. | Moderate. The format is small (wiki Format page) but the parser is ours to test. The algorithm text is explicit. | Parser bugs. Must implement exception rules, wildcard, ICANN/PRIVATE markers and the canonicalization rule. |
| C. Download at run time | Not deterministic. | Freshest. | Adds a network dependency and an egress destination. | Conflicts with `security.md` section 10 (undeclared destinations disabled) and the "fail closed" rule when the download fails. Not recommended. |

Legal note (question, not a verified fact): `gen.go` produces Go files derived from MPL-2.0 data. MPL-2.0 section 1.10 defines "Modifications" as "any new file in Source Code Form that contains any Covered Software". A generated table in this repository may need to keep MPL-2.0 file-level terms while the project is Apache-2.0. Needs owner or counsel review.

### PROPOSED, not decided

Option B. Commit the unmodified `public_suffix_list.dat` as a data file (not a generated Go table), record its `VERSION`, `COMMIT` and SHA-256 in a manifest, parse it with a small in-repo parser, and refresh by a reviewed PR with a script. Treat PRIVATE-section entries as public suffixes for the "refuse wildcard under a public suffix" rule (a wildcard under `github.io` or `appspot.com` would cover other tenants). Treat a base whose TLD is not in the list as refused (the implicit `*` rule means an unknown TLD is itself a suffix). Keep `x/net/publicsuffix` out of security decisions. Per `security.md`, every wildcard rule stays refused until this source is reviewed and in place.

### Validation work still needed

- Parser test against the wiki's worked examples: `*.jp`, `*.hokkaido.jp`, `!pref.hokkaido.jp`, `!metro.tokyo.jp`.
- Test that a wildcard whose base is `com`, `co.uk`, `github.io`, `appspot.com`, an unknown TLD, and an IDN suffix (U-label and A-label forms) is refused.
- Test that the base itself is not matched by the wildcard (project rule) while PSL semantics treat `*.foo.com` differently (issue 694).
- Test that the pinned hash check fails closed on a modified file.
- Decide the refresh process and who approves it.

---

## 3. Q2 (O-33): IDNA, numeric hosts, UTS #39

### x/net/idna facts

| Item | Value | Source |
| --- | --- | --- |
| Latest module version | golang.org/x/net v0.62.0 (2026-10-27) | proxy.golang.org (see Q1) |
| Licence | BSD-3-Clause | https://raw.githubusercontent.com/golang/net/v0.62.0/LICENSE |
| Package purpose | "Package idna implements IDNA2008 using the compatibility processing defined by UTS (Unicode Technical Standard) #46" | https://pkg.go.dev/golang.org/x/net/idna |
| Unicode tables | `tables17.0.0.go` with `const UnicodeVersion = "17.0.0"` (build tag `go1.27`); `tables15.0.0.go` also present for older toolchains. UTS #46 itself is at version 18.0.0 (2026-08-31), so the library tables lag the standard by one Unicode version. | https://raw.githubusercontent.com/golang/net/v0.62.0/idna/tables17.0.0.go; https://api.github.com/repos/golang/net/contents/idna?ref=v0.62.0; https://www.unicode.org/reports/tr46/ |
| Is it the module the Go standard library vendors? | Yes, as an internal vendored copy. `src/vendor/modules.txt` on Go master lists `# golang.org/x/net v0.61.1-0.20261022225259-05c76ee9adc5` with `golang.org/x/net/idna`. The vendored copy is not importable by user code. Release tags of Go were not checked. | https://raw.githubusercontent.com/golang/go/master/src/vendor/modules.txt |
| Package-level `ToASCII` | "ToASCII is a wrapper for Punycode.ToASCII." So `idna.ToASCII` does raw Punycode with no mapping and minimal validation. Doc example: `idna.ToASCII("*.GÖPHER.com")` gives `*.xn--GPHER-1oa.com` (case preserved, no error). | pkg.go.dev idna |
| Profiles | `Punycode` ("raw punycode processing with a minimum of validation"), `Lookup` ("the recommended profile for looking up domain names, according to Section 5 of RFC 5891. The exact configuration of this profile may change over time."), `Display` ("The configuration of this profile may change over time."), `Registration` ("whether a given IDN is valid for registration, according to Section 4 of RFC 5891"). | https://raw.githubusercontent.com/golang/net/v0.62.0/idna/idna.go |
| Exact profile definitions at v0.62.0 | `lookup`: transitional=false, useSTD3Rules=true, checkHyphens=true, checkJoiners=true, mapping=validateAndMap, bidirule=ValidString, verifyDNSLength not set. `display`: the same without the transitional field. `registration`: useSTD3Rules, verifyDNSLength, checkHyphens, checkJoiners true, mapping=validateRegistration, bidirule. `punycode = &Profile{}`. Comment: "Transitional processing is disabled by default as of Go 1.18." | idna.go |
| Options | `MapForLookup` (sets mapping=validateAndMap, `StrictDomainName(true)`, `ValidateLabels(true)`; "It does not add the Bidi Rule"), `BidiRule` ("corresponds to the CheckBidi flag in UTS #46"), `CheckHyphens` (corresponds to CheckHyphens), `CheckJoiners` (corresponds to CheckJoiners), `StrictDomainName` (corresponds to UseSTD3ASCIIRules; "set by default for MapForLookup and ValidateForRegistration"), `Transitional` ("only meaningful if combined with MapForLookup"), `VerifyDNSLength` (corresponds to VerifyDnsLength), `ValidateLabels` (sets CheckHyphens and CheckJoiners), `RemoveLeadingDots`, `ValidateForRegistration`. "With no options, the returned Profile is the most permissive and equals the Punycode Profile." "later options override earlier options." | pkg.go.dev idna; idna.go (`MapForLookup`, `ValidateForRegistration` bodies) |
| No `IgnoreInvalidPunycode` option | A selective search of `idna.go` for `IgnoreInvalidPunycode` found no match. Other files were not searched. | idna.go |
| Error behaviour | "If an error is encountered it will return an error and a (partially) processed result." The caller must discard the result on error. | pkg.go.dev idna |

### WHATWG URL spec (https://url.spec.whatwg.org/, fetched 2026-11-15; the spec revision date was not captured)

The current text no longer uses the phrase "domain to ASCII" for the algorithm. The algorithm is in **section 3.3 "IDNA"** and is called the **domain parser** (with a helper "domain parser ToASCII"). "domain-to-ASCII" survives only as a validation error name. Section 3.5 "Host parsing" holds the host parser, the ends in a number checker, the IPv4 parser and the IPv4 number parser.

- Domain parser, quoted: "Let strictResult be the result of running domain parser ToASCII with domain and true. ... If beStrict is true: If strictResult is a failure value, then return failure. Return strictResult." For beStrict false: "If domain is an ASCII string, then set result to domain, lowercased." and "When beStrict is false and domain is an ASCII string, the algorithm returns domain lowercased regardless of Unicode ToASCII's outcome, due to web compatibility." Otherwise: ToASCII with false; "If result is a failure value, then return failure. If result is the empty string, then return failure. If result contains a forbidden domain code point, then return failure."
- Domain parser ToASCII, quoted: "Unicode ToASCII with domain_name set to domain, CheckHyphens set to beStrict, CheckBidi set to true, CheckJoiners set to true, UseSTD3ASCIIRules set to beStrict, Transitional_Processing set to false, VerifyDnsLength set to beStrict, and IgnoreInvalidPunycode set to false."
- "This document and the web platform at large use Unicode IDNA Compatibility Processing and not IDNA2008. For instance, ☕.example becomes xn--53h.example and not failure."
- Forbidden domain code point (section 3.2): a forbidden host code point (U+0000, U+0009, U+000A, U+000D, U+0020, `#`, `/`, `:`, `<`, `>`, `?`, `@`, `[`, `\`, `]`, `^`, `|`), a C0 control, U+0025 (%), or U+007F.
- Host parser: percent-decodes the host, runs the domain parser with beStrict=false, "If asciiDomain ends in a number: If domain is not an ASCII string, IPv4-non-ASCII-input validation error. Return the result of IPv4 parsing asciiDomain."
- Ends in a number checker: split on `.`; drop one trailing empty label; "If last is not the empty string and contains only ASCII digits, then return true." Also true if the last label parses as an IPv4 number (equivalent to `0x`/`0X` followed by zero or more hex digits). "The erroneous input '09' will be caught by the IPv4 parser at a later stage."
- IPv4 parser: more than 4 parts is failure; fewer than 4 parts is a validation error but accepted; a non-final part above 255 is failure; the last part must be below 256^(5 − number of parts). IPv4 number parser accepts a `0x`/`0X` hex prefix and a leading `0` as octal. So `127.1`, `0x7f.1`, `2130706433`, `0177.0.0.1` all denote loopback.
- Valid domain (section 3.4): the domain parser with beStrict=true must not fail and the result must not end in a number.

Mapping to the Go library (derived by reading the option definitions; **not run**):

| WHATWG | UTS #46 flag | x/net/idna |
| --- | --- | --- |
| beStrict=true | CheckHyphens, UseSTD3ASCIIRules, VerifyDnsLength true; CheckBidi, CheckJoiners true; Transitional false | `idna.New(idna.MapForLookup(), idna.BidiRule(), idna.VerifyDNSLength(true))` (MapForLookup gives STD3 + CheckHyphens + CheckJoiners; Transitional defaults false). The package `Lookup` profile is the same minus `VerifyDNSLength`. |
| beStrict=false | CheckHyphens false, UseSTD3ASCIIRules false, VerifyDnsLength false; CheckBidi, CheckJoiners true | `idna.New(idna.MapForLookup(), idna.BidiRule(), idna.CheckHyphens(false), idna.StrictDomainName(false), idna.VerifyDNSLength(false), idna.CheckJoiners(true), idna.Transitional(false))`. Options must be listed after `MapForLookup` so they override it. |
| WHATWG extras not in the library | ASCII fast path with lowercase, forbidden-domain-code-point check, empty-result failure, IgnoreInvalidPunycode=false | Must be hand-written. |

UTS #46 (https://www.unicode.org/reports/tr46/, version 18.0.0, revision 36, dated 2026-08-31): section 4 "Processing" lists the inputs "UseSTD3ASCIIRules, CheckHyphens, CheckBidi, CheckJoiners, Transitional_Processing (deprecated), IgnoreInvalidPunycode" and the steps Map, Normalize, Break, Convert/Validate. Section 4.2 is ToASCII. Section 8 says "A conformance testing file (IdnaTestV2.txt) is provided for each version of Unicode starting with Unicode 6.0 ... It only provides test cases for UseSTD3ASCIIRules=true." Transitional processing is marked deprecated.

### UTS #39 (https://www.unicode.org/reports/tr39/, version 18.0.0, revision 34, dated 2026-08-27)

- Scope, quoted from the Introduction and Summary: "This document specifies mechanisms that can be used to detect possible security problems."
- Restriction levels (section 5.2) apply to identifiers generally: "Restriction Levels 1-5 are defined here for use in implementations. These place restrictions on the use of identifiers according to the appropriate identifier profile". Levels in order: ASCII-Only, Single Script, Highly Restrictive, Moderately Restrictive, Minimally Restrictive, Unrestricted. The detection procedure uses the Identifier Profile, a "set of script sets" (SOSS, section 5.1), and the Recommended Scripts list from UAX #31.
- Mixed-script detection (section 5.1): a set of scripts "covers" a string if it intersects the augmented script sets of every character; "Typically an API that returns the scripts in a string will return one of the minimal cover sets."
- Domain names: section 3.2 "IDN Security Profiles for Identifiers": "implementations can claim conformance to other features of this document as applied to domain names, such as Restriction Levels." UTS #46 section 3 lists, among conditions an implementation may flag, "labels that do not satisfy Restriction Level 4, Moderately Restrictive from Unicode Technical Standard #39". (The introductory sentence of that UTS #46 list was not captured.)
- Answer to "DNS labels or identifiers generally?": UTS #39 is written for identifiers in general. It applies to domain labels through section 3.2 and by reference from UTS #46. It does not define a DNS-specific algorithm.
- Go libraries: none verified that implements restriction levels or mixed-script detection. Candidates found:
  - `github.com/mtibben/confusables`: GitHub description "Go implementation of Unicode TR39 skeleton algorithm" (skeleton only). Proxy `@latest`: `v0.0.0-20210201002637-9d1b0723b659` (2021-02-01). Licence BSD-3-Clause per GitHub API (SECONDARY: https://api.github.com/search/repositories?q=confusables+language:go). Not maintained since 2021.
  - `github.com/disciplinedware/go-confusables`: proxy `@latest` v0.1.1 (2026-02-18). Description from pkg.go.dev search: "utilities to detect and normalize Unicode confusable characters". Licence NOT VERIFIED.
  - Conclusion: restriction-level and mixed-script detection must be implemented in-repo, using data files from unicode.org (Scripts, ScriptExtensions, IdentifierStatus). Whether the Go `unicode` package exposes Script_Extensions was not verified.

### Numeric-host ambiguity (security consequence)

The WHATWG host parser accepts `127.1`, `0x7f.1`, `2130706433` and similar as IPv4 addresses, and fails on host names whose last label is numeric but not a valid IPv4 form (for example `1.2.3.4.5`, `test.42`). Other parsers (resolver libraries, tool binaries) may read these differently. OWASP SSRF cheat sheet: "Treat parser disagreement as a rejection ... Reject a URL whose host is not read identically by every parser in play, rather than reconciling the readings." (https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)

### PROPOSED, not decided

- Canonicalize scope rules and targets with the strict profile (`MapForLookup` + `BidiRule` + `VerifyDNSLength(true)`) and compare only A-label ASCII. Discard any returned value when an error is returned.
- Also run the relaxed (beStrict=false-like) path in tests only. In production, if strict fails, deny (do not fall back to the relaxed result).
- Treat any host whose last label (after stripping one trailing dot) is all ASCII digits, or `0x`+hex, as numeric. Accept it only when it is exactly four canonical decimal parts (no leading zeros, each 0 to 255). Everything else numeric is ambiguous and denied.
- Do not use UTS #39 in the allow/deny decision. Canonical A-label comparison already removes homograph ambiguity from the comparison. Use restriction levels, at most, to warn in the UI.

### Validation work still needed (only a test can show)

Cannot be known without running the library: the exact accept/reject result and output of each option combination above; whether `fromPuny` validation is active when building a profile with `New(MapForLookup(), ...)` versus using `Lookup`; how the library treats `_`, `*`, trailing dot, empty labels, and labels over 63 octets under each profile; behaviour differences between the Unicode 15 and 17 tables; whether results differ from Go's own `net/http` host handling.

Test inputs the suite must contain (expected outputs must come from an independent oracle, not from the library under test: UTS #46 IdnaTestV2.txt and the WHATWG URL test data; their exact URLs were not verified):

- IPv4 numeric forms: `127.0.0.1`, `127.1`, `127.0.1`, `0x7f.1`, `0X7F.0.0.1`, `0x7f000001`, `2130706433`, `017700000001`, `0177.0.0.1`, `127.0.0.1.` (trailing dot), `1.2.3`, `1.2.3.4.5`, `256.1.1.1`, `1.2.3.256`, `09`, `0x`, `1.2.3.0x`, `test.42`, `example.123`, `example.0x7f`, `1.2.3.4e` (last label not numeric), `4294967296`.
- Obfuscated digits and dots: fullwidth `１２７.０.０.１`, ideographic full stop `127。0。0。1`, `127%2e0.0.1`, `%31%32%37.0.0.1`.
- IPv6 and embedded IPv4: `[::1]`, `[::ffff:127.0.0.1]`, `[0:0:0:0:0:ffff:7f00:1]`, `[64:ff9b::127.0.0.1]`, `[2002:7f00:1::]`, `[fe80::1%25eth0]` (zone id: the spec omits support), `[::1` (unclosed), `::1` without brackets.
- IDNA: `bücher.example.com` (doc says `xn--bcher-kva.example.com`), `BÜCHER.example.com`, the same in NFD, `xn--bcher-kva.example.com`, `faß.com` and `ß` (non-transitional keeps ß; the doc example shows `*.faß.com` with `Transitional(true)` giving `*.fass.com`), `ΟΣ`-style final sigma, `☕.example` (WHATWG says `xn--53h.example`), `xn--8i7caa` (WHATWG: decodes to `ｗｗｗ`, status "mapped"), `xn--`, `xn--a`, labels with U+200C and U+200D, a right-to-left label mixed with an LTR label (bidi rule), `-a.example`, `a-.example`, `ab--cd.example`, `a_b.example`, `*.example`, `exa mple.com`, `exa%23mple.org` (WHATWG error example), NUL and control characters, soft hyphen U+00AD, fullwidth `ｅｘａｍｐｌｅ．ｃｏｍ`, `example。com`, uppercase `EXAMPLE.COM`, `example.com.`, `.example.com`, `a..b`, empty string, a 64-octet label, a 254-octet name, a Cyrillic `а` homograph of `apple.com`.
- Doc-verified oracle values usable as seed cases: `idna.Lookup.ToASCII("www.GÖPHER.com")` gives `www.xn--gpher-jua.com`; `idna.Registration.ToASCII("www.GÖPHER.com")` gives an error `idna: disallowed rune U+0047`; `idna.New().ToASCII("*.faß.com")` gives `*.xn--fa-hia.com`; `New(MapForLookup(), Transitional(true)).ToASCII("*.faß.com")` gives error `idna: disallowed rune U+002A` (from the pkg.go.dev examples).

---

## 4. Q3 (O-34): Special-purpose address data

### Registries (fetched 2026-11-15)

| Registry | HTML URL | "Last Updated" shown | Machine-readable |
| --- | --- | --- | --- |
| IPv4 Special-Purpose Address Space | https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml | 2025-10-09 (created 2009-08-19) | CSV: https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry-1.csv ; XML: https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xml (references `iana-ipv4-special-registry.rng`) |
| IPv6 Special-Purpose Address Space | https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry.xhtml | 2025-10-09 (created 2006-01-10) | CSV: https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry-1.csv |

The XML for IPv6 was not fetched. The IPv4 XML starts with `<registry id="iana-ipv4-special-registry">`, a nested `<registry id="iana-ipv4-special-registry-1">`, and `<record>` elements with `<address>`, `<name>`, `<spec>`, `<allocation>`, `<source>`, `<destination>`, `<forwardable>`, `<global>`, `<reserved>` (the `<global>` element carries the "Globally Reachable" value).

Registry title on the page: "Formerly known as IANA IPv4 Special-Purpose Address Registry (pursuant to IESG decision)". References: RFC 5736, RFC 6890, RFC 8190 (IPv4); RFC 4773, RFC 6890, RFC 8190 (IPv6). Registration procedure: IETF Review.

### Columns and "Globally Reachable" semantics

CSV header: `Address Block,Name,RFC,Allocation Date,Termination Date,Source,Destination,Forwardable,Globally Reachable,Reserved-by-Protocol`.

- Page text: "Globally Reachable - A boolean value indicating whether an IP datagram whose destination address is drawn from the allocated special-purpose address block is forwardable beyond a specified administrative domain." "If the value of 'Destination' is FALSE, the values of 'Forwardable' and 'Globally Reachable' must also be false."
- RFC 8190 section 2.1 (https://www.rfc-editor.org/rfc/rfc8190.txt) replaces the RFC 6890 term "global" with "globally reachable" and gives the same definition.
- Page note: "Address prefixes listed in the Special-Purpose Address Registry are not guaranteed routability in any particular local or global context."

### Parsing hazards seen in the CSV (verified in the fetched files)

- A cell can hold two prefixes: `"192.0.0.170/32, 192.0.0.171/32"`.
- Address cells can carry a footnote suffix: `192.0.0.0/24 [2]`, `2002::/16 [3]`.
- Boolean cells can carry a footnote: `False [1]`, `N/A [2]`, `False [4]`.
- Deprecated rows have empty flag cells and a Termination Date: `192.88.99.0/24` (2015-03) and `2001:10::/28` (2014-03).
- Name cells can contain doubled quotes: `"""This network"""`.
- RFC cells use brackets: `[RFC 1122, Section 3.2.1.3]`.
- Nested allocations: `2001::/23` ("False [1]", "Unless allowed by a more specific allocation") contains `2001:1::1/128` (True) and others; `192.0.0.0/24` contains `192.0.0.9/32` and `192.0.0.10/32` (True). A lookup must use longest-prefix match, not "any match".

### Rows where a naive "Globally Reachable is False means deny" rule fails or is incomplete

| Prefix | Registry values | Consequence |
| --- | --- | --- |
| `64:ff9b::/96` NAT64 (RFC 6052) | Globally Reachable True | A "False means deny" rule would allow `64:ff9b::7f00:1`, which embeds 127.0.0.1. The embedded IPv4 must be extracted and evaluated. |
| `64:ff9b:1::/48` (RFC 8215) | False | Denied by the rule, but local-use translation space also embeds IPv4. |
| `2002::/16` 6to4 (RFC 3056) | Globally Reachable `N/A [3]` | Not a boolean. Treat N/A as deny. Embedded IPv4 sits in bits 16 to 47. |
| `2001::/32` Teredo (RFC 4380, RFC 8190) | Globally Reachable `N/A [2]` ("See Section 5 of RFC 4380") | Treat as deny. |
| `::ffff:0:0/96` IPv4-mapped (RFC 4291) | False | Denied. Better: unmap and evaluate the embedded IPv4 so that public mapped addresses get a clear reason. |
| `192.88.99.0/24`, `2001:10::/28` | blank (deprecated) | Blank is not False. Treat as deny. |
| `fc00::/7` Unique-Local (RFC 4193, RFC 8190) | `False [4]` ("drawn from the IPv6 Global Unicast Address range, but is specified as not globally routed") | Denied. |
| `192.0.0.9/32`, `192.0.0.10/32`, `192.31.196.0/24`, `192.52.193.0/24`, `192.175.48.0/24`, `2001:1::1/128`, `2001:1::2/128`, `2001:1::3/128`, `2001:3::/32`, `2001:4:112::/48`, `2001:20::/28`, `2001:30::/28`, `2620:4f:8000::/48` | True | These are globally reachable service anycast or protocol prefixes. A "False means deny" rule allows them. Project rule: "Addresses that are not globally routable unicast addresses must be denied." They are not general unicast hosts; an allow-list design should still deny them (PROPOSED). |

IPv4 rows with Globally Reachable False (denied): 0.0.0.0/8, 0.0.0.0/32, 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8 (`False [1]`), 169.254.0.0/16, 172.16.0.0/12, 192.0.0.0/24, 192.0.0.0/29, 192.0.0.8/32, 192.0.0.170/32 and 192.0.0.171/32, 192.0.2.0/24, 192.88.99.2/32, 192.168.0.0/16, 198.18.0.0/15, 198.51.100.0/24, 203.0.113.0/24, 240.0.0.0/4, 255.255.255.255/32.
IPv6 rows with False: ::1/128, ::/128, ::ffff:0:0/96, 64:ff9b:1::/48, 100::/64, 100:0:0:1::/64, 2001::/23 (`False [1]`), 2001:2::/48, 2001:db8::/32, 3fff::/20, 5f00::/16, fc00::/7, fe80::/10.

### Multicast

The special-purpose registries do **not** contain multicast. The IPv4 registry has 240.0.0.0/4 ("Reserved") but no 224.0.0.0/4 row (checked in the CSV above). The IPv6 registry has no ff00::/8 row. Multicast is in the address-space registries:

- IPv4: https://www.iana.org/assignments/ipv4-address-space/ipv4-address-space.xhtml lists 224/8 through 239/8 as "Multicast", status RESERVED, footnote [14] (the page was searched for "Multicast"; the registry's own "Last Updated" date was not captured).
- IPv6: https://www.iana.org/assignments/ipv6-address-space/ipv6-address-space.xhtml lists `ff00::/8` "Multicast", RFC 3513; RFC 4291, "See IPv6 Multicast Address Space for details." The same page lists `c000::/3`, `e000::/4`, `f000::/5`, `f800::/6`, `fe00::/9` as "Reserved by IETF" and `fec0::/10` as deprecated (RFC 3879).
- The IANA multicast-address assignment registries themselves (per-address assignments) were not fetched and are not needed for a prefix-level deny.

### Prefixes with RFC citations (from the registry rows above)

IPv4-mapped `::ffff:0:0/96` RFC 4291. NAT64 `64:ff9b::/96` RFC 6052; `64:ff9b:1::/48` RFC 8215. 6to4 `2002::/16` RFC 3056 (relay anycast `192.88.99.0/24` deprecated by RFC 7526). Teredo `2001::/32` RFC 4380, RFC 8190. Loopback `::1/128`, unspecified `::/128`, link-local `fe80::/10` RFC 4291. The RFC bodies for 6052, 8215, 3056 and 4380 were not read; only the registry citation was verified.

### PROPOSED generation procedure (not executed, not decided)

1. A maintainer downloads the two CSV files (and the IPv4/IPv6 multicast and address-space data needed for multicast and reserved space) from the URLs above, records the fetch date and the "Last Updated" value.
2. Compute SHA-256 of each downloaded file and store the hashes and URLs in a manifest committed beside the files.
3. A generator (a `go run` tool in `scripts/`, build-ignored) reads the committed files, verifies the hashes, parses with the hazards above handled, and emits a Go table (sorted prefixes, flags, RFC text) into the platform-independent address-policy package. The generator fails on any unparsed row or unknown flag value.
4. The table carries a header with source file hashes, registry "Last Updated", and generator version. A test compares the table to a fresh parse of the committed CSV.
5. Add by hand, with a comment and RFC reference each: 224.0.0.0/4, ff00::/8, the NAT64/6to4/Teredo embedded-IPv4 evaluation, and the cloud metadata list from section 5.
6. Policy form: allow only addresses that are inside a permitted unicast range (IPv4 outside the listed blocks; IPv6 inside 2000::/3) AND not in any deny entry. This is an allow-with-denials design, so reserved space not listed in the registries is denied by default.
7. Refresh is a reviewed pull request; no run-time download.

Open items: IANA's terms of use for registry data were not verified.

### Validation work still needed

Property tests that every row of the CSV is denied (except the intended `True` anycast rows, which must also be denied by the proposed policy); embedded-IPv4 tests for `64:ff9b::7f00:1`, `2002:7f00:1::`, `::ffff:127.0.0.1`; nested-prefix longest-match tests (`2001:1::1`, `192.0.0.9`); a test that fails if the manifest hash or the table differs from the committed CSV.

---

## 5. Q4 (O-34): Cloud metadata endpoints

| Provider | Address(es) per vendor doc | URL (fetched 2026-11-15) | Quote | Inside a registry-covered range? |
| --- | --- | --- | --- | --- |
| AWS EC2 | IPv4 `169.254.169.254`; IPv6 `fd00:ec2::254` | https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html | "These IP addresses are link-local addresses and are valid only from the instance." "enable and use the IPv6 address of the IMDS `[fd00:ec2::254]` instead of the IPv4 address `169.254.169.254`." | `169.254.169.254` in 169.254.0.0/16 (Link Local, False). `fd00:ec2::254` in fc00::/7 (Unique-Local, `False [4]`). Both covered. |
| AWS container credentials | `169.254.170.2` (ECS tasks), `169.254.170.23` (EKS Pod Identity) | https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html (OWASP; AWS's own page NOT fetched) | "169.254.0.0/16 (RFC 3927), which also covers the AWS credential endpoints for ECS tasks (169.254.170.2) and EKS Pod Identity (169.254.170.23)" | In 169.254.0.0/16, covered. |
| Google Cloud | DNS `metadata.google.internal`; IPv4 `169.254.169.254`; IPv6 `fd20:ce::254` (IPv6-only VMs) | https://docs.cloud.google.com/compute/docs/metadata/overview (the old `cloud.google.com` URL returns a 301 to this host) | "DNS Name (Recommended): `http://metadata.google.internal/computeMetadata/v1`", "IPv4 address: `http://169.254.169.254/computeMetadata/v1`", "IPv6 address (only for IPv6-only VMs): `http://[fd20:ce::254]/computeMetadata/v1`". Also: "any process that can query the metadata URL, has access to all values in the metadata server." | 169.254.169.254 covered. `fd20:ce::254` in fc00::/7, covered. The name resolves only inside GCP; resolved-address evaluation covers it. |
| Azure IMDS | `169.254.169.254` | https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service (page `updated_at` 2026-05-04) | "IMDS is a REST API that's available at a well-known, non-routable IP address (`169.254.169.254`). You can only access it from within the VM." Requests need `Metadata: true` and no `X-Forwarded-For`. Purpose: VM metadata (SKU, storage, network, maintenance events, managed identity tokens at `/metadata/identity`). | Covered (link-local). |
| Azure WireServer | `168.63.129.16` | https://learn.microsoft.com/en-us/azure/virtual-network/what-is-ip-address-168-63-129-16 (page `updated_at` 2026-03-12) | "Azure IP address 168.63.129.16 is a virtual public IP address that facilitates communication channels to Azure platform resources." Used for: VM Agent "Ready" signal, filtered DNS, Load Balancer health probes, DHCP, Guest Agent heartbeat. "The virtual machine Agent requires outbound communication over ports 80/tcp and 32526/tcp with WireServer (168.63.129.16)." "Microsoft owns this special public IP address and it doesn't change." | **Not covered.** 168.63.129.16 is not in the IPv4 special-purpose registry (checked against the full CSV). A registry-derived deny list would allow it. |
| Alibaba Cloud ECS | `100.100.100.200` | https://www.alibabacloud.com/help/en/ecs/user-guide/view-instance-metadata | `curl -X PUT "http://100.100.100.200/latest/api/token"`; "100.100.100.200 is a link-local address reachable only from within an ECS instance over its virtual network interface." Default mode is token-free ("normal mode ... poses a high risk of SSRF attacks"). DNS servers appear as `100.100.XX.XX`. | In 100.64.0.0/10 (Shared Address Space, RFC 6598, Globally Reachable False). Covered by the registry, but that is a larger block than a link-local range, so a lab registration for "shared address space" would include it. The vendor calls it link-local; by the registry it is shared address space. |
| Oracle Cloud | `169.254.169.254` (`/opc/v2/...`) | https://docs.oracle.com/en-us/iaas/Content/Compute/Tasks/gettingmetadata.htm | "The paths are relative to http://169.254.169.254/opc/v2/instance/." Requests carry `Authorization: Bearer Oracle`. Volume-attachment example uses `169.254.2.2` for iSCSI. IPv6 metadata address: NOT VERIFIED. | Covered (link-local). |
| DigitalOcean | `169.254.169.254` | https://docs.digitalocean.com/products/droplets/how-to/retrieve-droplet-metadata/ (`last_updated: 2026-07-13`) | "Droplets can access the metadata service using the special, static, link-local IP address `169.254.169.254`." | Covered. |
| Hetzner | NOT VERIFIED | Fetches of https://docs.hetzner.cloud/#server-metadata, https://docs.hetzner.cloud/reference/cloud and https://docs.hetzner.com/cloud/servers/overview/ returned no matching text. | none | none |
| Kubernetes node-local | No fixed address documented | https://kubernetes.io/docs/tasks/administer-cluster/nodelocaldns/ | "The local listen IP address for NodeLocal DNSCache can be any address that can be guaranteed to not collide with any existing IP in your cluster. It's recommended to use an address with a local scope, for example, from the 'link-local' range '169.254.0.0/16' for IPv4 or from the 'Unique Local Address' range in IPv6 'fd00::/8'." | Operator-chosen; the recommended ranges are registry-covered. No fixed entry to list. |

OWASP SSRF cheat sheet deny-list (last resort) corroborates AWS (`169.254.169.254`, `fd00:ec2::254`), GCP (`metadata.google.internal`, `169.254.169.254`), Azure IMDS (`169.254.169.254`). It does not list 168.63.129.16 or the Alibaba address.

### PROPOSED additional-address list (not decided)

Hard-coded "never authorize, even for lab registration" entries. The project's own rule says link-local and cloud-metadata addresses can never be authorized, but a lab registration can authorize shared-address, private and unique-local classes. Without explicit entries, a lab registration of 100.64.0.0/10 or fc00::/7 would cover the Alibaba, AWS-IPv6 and GCP-IPv6 metadata addresses.

| Entry | Evidence | Covered by registry? |
| --- | --- | --- |
| `169.254.169.254/32` | AWS, GCP, Azure, Oracle, DigitalOcean docs | yes (kept explicit) |
| `fd00:ec2::254/128` | AWS doc | yes (kept explicit; unique-local is lab-authorizable) |
| `fd20:ce::254/128` | GCP doc (IPv6-only VMs) | yes (kept explicit; same reason) |
| `100.100.100.200/32` | Alibaba doc | yes (kept explicit; shared space is lab-authorizable) |
| `168.63.129.16/32` | Azure doc: "virtual public IP address" | **no. Add by hand.** |
| `169.254.170.2/32`, `169.254.170.23/32` | OWASP only | yes (inside link-local; optional) |

Names (optional, defense in depth; resolved-address checks already cover them): `metadata.google.internal`.

Validation work still needed: a test that each entry is denied with and without any lab registration; a test that `168.63.129.16` is denied although it is "public"; Hetzner and Oracle IPv6 evidence from the vendors' own documentation.

---

## 6. Q5: Password hashing for a single local account

| Item | Value | Source |
| --- | --- | --- |
| Module | golang.org/x/crypto, latest v0.59.0, proxy time 2026-11-11T07:23:42Z | https://proxy.golang.org/golang.org/x/crypto/@latest |
| Licence | BSD-3-Clause | https://raw.githubusercontent.com/golang/crypto/v0.59.0/LICENSE |
| Minimum Go | `go 1.26.0` in go.mod (applies to v0.59.0; older x/crypto tags may allow older Go) | https://raw.githubusercontent.com/golang/crypto/v0.59.0/go.mod |
| API | `IDKey(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte`. "memory parameter specifies the size of the memory in KiB." "If you aren't sure which function you need, use Argon2id (IDKey)". No encode/decode helper is documented. | https://pkg.go.dev/golang.org/x/crypto/argon2 |
| RFC 9106 options quoted by the package doc | First: time=1, memory=2*1024*1024 KiB (2 GiB), threads=4. Second ("If much less memory is available"): time=3, memory=64*1024 KiB (64 MiB), threads=4. | same |

### OWASP Password Storage Cheat Sheet (https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html, fetched 2026-11-15)

Argon2id minimum configurations, quoted:

- m=47104 (46 MiB), t=1, p=1 (Do not use with Argon2i)
- m=19456 (19 MiB), t=2, p=1 (Do not use with Argon2i)
- m=12288 (12 MiB), t=3, p=1
- m=9216 (9 MiB), t=4, p=1
- m=7168 (7 MiB), t=5, p=1

"These configuration settings provide an equal level of defense, and the only difference is a trade off between CPU and RAM usage." Summary line: "Use Argon2id with a minimum configuration of 19 MiB of memory, an iteration count of 2, and 1 degree of parallelism." Also: "increasing parallelism is not a guarantee of slower password guessing"; "As a general rule, calculating a hash should take less than one second."; "Benchmark the chosen parameters on the target system".

Alternatives (OWASP page, same fetch):

| Algorithm | OWASP parameters | One-line trade-off |
| --- | --- | --- |
| scrypt | N=2^17 (128 MiB), r=8, p=1 (or N=2^16/p=2, 2^15/p=3, 2^14/p=5, 2^13/p=10) | Memory-hard and in x/crypto, but "should be used when [Argon2id] is not available" and costs more memory for equal defense. |
| bcrypt | work factor 10 or more; "maximum length input length of 72 bytes" | "only ... for password storage in legacy systems"; 72-byte truncation. |
| PBKDF2 (FIPS) | PBKDF2-HMAC-SHA256 600,000 iterations; SHA512 220,000; SHA1 1,400,000 (legacy only) | The choice when FIPS-140 validated implementations are required; not memory-hard, so weaker against GPUs. |

### Encoded-hash format

PHC string format, now maintained as C2SP "phc-strings" (https://raw.githubusercontent.com/C2SP/C2SP/main/phc-strings.md; the old P-H-C repo file redirects there). Grammar: `$<id>[$v=<version>][$<param>=<value>(,<param>=<value>)*][$<salt>[$<hash>]]`. Argon2 example from the spec: `$argon2id$v=19$m=65536,t=2,p=1$gZiV/M1gPc22ElAH/Jh1Hw$CWOrkoo7oJBQ/iyh7uJ0LO2aLEfrHwTWllSAxT0zRno` (salt and hash are base64 without padding in the example; the spec's exact base64 rule was beyond the truncated fetch). Identifier chars `[a-z0-9-]`, name at most 32 characters. OWASP also refers to "the modular PHC string format" for storing algorithm and work factor with the hash. Not verified: whether `v=19` is the value of the package's `argon2.Version` constant (the doc page does not show the value).

### Memory implication on an 8 GB laptop

One hash uses about `m` KiB. 19,456 KiB is 19 MiB, about 0.23 percent of 8 GiB. 46 MiB is about 0.56 percent. 64 MiB (RFC 9106 second option) is about 0.78 percent. Concurrent logins multiply it: 10 concurrent hashes at 64 MiB is 640 MiB. For a single local account the realistic concurrency is 1 to 2, but an attacker on loopback can send many requests, so concurrency should be bounded (a semaphore around hashing). Arithmetic is ours, not a source claim.

### PROPOSED, not decided (tunable policy)

Argon2id via `golang.org/x/crypto/argon2.IDKey`, parameters m=65536 KiB (64 MiB), t=3, p=4, salt 16 random bytes, key length 32 bytes, stored as a PHC string with `v=19`, verified with constant-time compare. These meet or exceed every OWASP minimum set above and are the RFC 9106 second recommendation quoted by the x/crypto doc. Keep the parameters in a policy setting, store them in each hash, rehash on login when the policy changes, bound concurrent hashing, and benchmark on the target machine (target under one second). The 16-byte salt and 32-byte key sizes are the author's choice and were not checked against RFC 9106 in this session. Parser and encoder for the PHC string are hand-written (about 60 lines) because x/crypto provides none.

Validation work still needed: PHC encode/decode round trip against the spec example; known-answer test vectors from RFC 9106 (not fetched); timing benchmark on a real 8 GB laptop; rejection of oversized or malformed parameter strings in a stored hash (denial of service through attacker-chosen `m`).

---

## 7. Q6: Linux credential storage

### Specification

Freedesktop Secret Service API: https://specifications.freedesktop.org/secret-service/latest/ (the `secret-service-spec` URL redirects to plain HTTP, which the fetch tool refuses). Status shown: "Secret Service API Draft ... Publication Date: 2026-10-19, Version: Secret Service 0.2 DRAFT", copyright 2008-2011, authors Stef Walter (GNOME Keyring) and Michael Leupold (KWallet). Chapter 1 (https://specifications.freedesktop.org/secret-service/latest/ch01.html): "The Secret Service API allows client applications to store secrets securely in a service running in the user's login session." "The service may need to be unlocked by the user before the secrets become available for retrieval by client applications." The spec text on the bus (session vs system) beyond chapter 1 was not read.

### Go clients (all fetched 2026-11-15)

| Library | Latest version and date | Licence | Min Go (go.mod) | Transport | Notes |
| --- | --- | --- | --- | --- | --- |
| github.com/zalando/go-keyring | v0.2.8, 2026-03-23 (about 8 months before fetch) | MIT (https://raw.githubusercontent.com/zalando/go-keyring/master/LICENSE) | `go 1.18` (master go.mod; requires godbus/dbus v5.2.2) | Secret Service over D-Bus: `secret_service.NewSecretService()` calls `dbus.SessionBus()`. Opens a session with algorithm `"plain"`. Uses the default/`login` collection and unlocks it. | README: depends on "the Secret Service dbus interface, which is provided by GNOME Keyring"; "It's expected that the default collection `login` exists". Has `ErrNotFound`. `MockInit()` for tests. |
| github.com/godbus/dbus/v5 | v5.2.4, 2026-10-13 | BSD-2-Clause (two numbered clauses in LICENSE) | `go 1.20` | D-Bus client library | See session bus behaviour below. |
| github.com/99designs/keyring | v1.2.2, 2022-12-19 (about 3 years before fetch) | MIT | `go 1.19`; depends on godbus/dbus `v0.0.0-20190726142602` and `gsterjov/go-libsecret v0.0.0-20161001094733` | Secret Service, KWallet, Pass, KeyCtl, encrypted file, Keychain, WinCred | `Open()` with `AllowedBackends == nil` uses all available backends in order `SecretService, KWallet, KeyCtl, Pass, File` and moves to the next one when a backend fails to open. That is a silent fallback to the file backend, which breaks the "never fall back" rule unless `AllowedBackends` is set to the Secret Service backend only. |

No other maintained alternative was verified. `gsterjov/go-libsecret` (used by 99designs) has a 2016 pseudo-version date.

### Behaviour without a session bus or keyring daemon (from source, not run)

- `godbus/dbus` `SessionBus()` uses `DBUS_SESSION_BUS_ADDRESS` when set (and not `autolaunch:`). Otherwise it checks `/run/user/<uid>/bus` and `/run/user/<uid>/dbus-session`. If nothing is found it **runs `dbus-launch`** via `os/exec` and sets the environment variable from its output (https://raw.githubusercontent.com/godbus/dbus/v5.2.4/conn.go and conn_other.go). `SessionBusPrivateNoAutoStartup` exists and never launches a bus. go-keyring calls `dbus.SessionBus()`, so it can trigger autolaunch. This is a process-execution side effect in a security tool and needs an owner decision.
- If a bus is reachable but no Secret Service owner exists, or the collection is locked or missing, the D-Bus calls return errors. go-keyring returns those errors from `Set`, `Get` and `Delete` unchanged (`keyring_unix.go`). It returns `ErrNotFound` only when a search finds nothing. The exact error values for "no service" and "locked" were not observed. D-Bus service activation of a keyring daemon on first use was not verified.
- Can a service started outside a desktop session (a worker) use the Secret Service? The spec describes a service "running in the user's login session" and one that "may need to be unlocked by the user". A worker run by a system-level service manager has no user session bus. A worker run as a `systemd --user` unit may find `/run/user/<uid>/bus`, but the keyring may still be locked until an interactive login unlocks it. Whether this works on a given distribution was not verified. It must be tested.

### PROPOSED, not decided

- Use `github.com/zalando/go-keyring` v0.2.8 behind an outbound port `CredentialStore` (adapter under `adapters/outbound/credentials`, Linux-only code under `platform/linux`). Do not use 99designs/keyring (stale, silent file/pass fallback, 2019-era D-Bus dependency).
- Only the process that needs a credential (the server, not the tool sandbox) may touch the keyring; tool processes must not see the session bus (isolation rule in `security.md` section 10).
- Preflight before any keyring call: verify `DBUS_SESSION_BUS_ADDRESS` or `/run/user/<uid>/bus` exists. If not, fail without calling `SessionBus()` so that `dbus-launch` is never executed (go-keyring offers no switch for this; the preflight avoids the call).
- Fail closed: any error other than a clean `ErrNotFound` returns a typed "credential store unavailable" error with the list of alternatives (start a desktop session, `systemd --user` with an unlocked keyring, systemd credentials, a restricted secret file, an external secret manager). No plaintext, file, or environment fallback may be added silently. Audit the failure (never the value).
- `ErrNotFound` and "unavailable" must map to different error categories.

### Validation work still needed

Run on the target Linux desktop and on a headless host: no session bus, bus present but keyring locked, no `login` collection, daemon not running. Record the exact errors. Confirm that no `dbus-launch` process starts. Test the worker started from a `systemd --user` unit and from a system unit. Check licences and versions of transitive dependencies after the module graph is resolved.

---

## 8. Q7: Web security libraries for Gin

| Library | Latest version, proxy time | Licence | go.mod `go` | Maintenance read | Source |
| --- | --- | --- | --- | --- | --- |
| github.com/gin-gonic/gin | v1.12.0, 2026-02-28 | MIT | 1.25.0 | Active at fetch time (about 8.5 months since this tag; commit activity not checked) | https://proxy.golang.org/github.com/gin-gonic/gin/@latest ; https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/go.mod ; .../LICENSE |
| net/http `CrossOriginProtection` (stdlib) | added in Go 1.25 | Go BSD-3-Clause | n/a | Standard library | https://go.dev/doc/go1.25 ("The new CrossOriginProtection implements protections against Cross-Site Request Forgery (CSRF) by rejecting non-safe cross-origin browser requests. It uses modern browser Fetch metadata, doesn't require tokens or cookies"); https://pkg.go.dev/net/http |
| github.com/gin-contrib/csrf | proxy returns HTTP 404: no such module found | n/a | n/a | Not verified to exist | https://proxy.golang.org/github.com/gin-contrib/csrf/@latest |
| github.com/gorilla/csrf | v1.7.3, 2025-01-23 | BSD-3-Clause ("Copyright (c) 2023 The Gorilla Authors") | not read | Last tag about 22 months before fetch; archive status not checked | https://proxy.golang.org/github.com/gorilla/csrf/@latest ; .../v1.7.3/LICENSE |
| github.com/gin-contrib/cors | v1.7.11, 2026-11-08 | MIT | 1.26.10 | Tagged one week before fetch | https://proxy.golang.org/github.com/gin-contrib/cors/@latest ; go.mod ; LICENSE |
| golang.org/x/time (rate) | v0.16.0, 2026-08-19 | BSD-3-Clause | 1.26.0 | Go project | https://proxy.golang.org/golang.org/x/time/@latest ; https://pkg.go.dev/golang.org/x/time/rate |
| github.com/ulule/limiter/v3 | v3.11.2, 2023-05-24 | MIT | not read | Last tag about 3.5 years before fetch | https://proxy.golang.org/github.com/ulule/limiter/v3/@latest ; .../v3.11.2/LICENSE |
| github.com/gin-contrib/limiter | proxy returns HTTP 404: no such module found | n/a | n/a | Not verified to exist | https://proxy.golang.org/github.com/gin-contrib/limiter/@latest |
| github.com/unrolled/secure | v1.18.0, 2026-11-10 | MIT | 1.13 | Tagged five days before fetch; no third-party dependencies in go.mod | https://proxy.golang.org/github.com/unrolled/secure/@latest ; go.mod ; LICENSE |
| github.com/gin-contrib/secure | v1.1.7, 2026-11-08 | MIT | 1.26.10 | Tagged one week before fetch | https://proxy.golang.org/github.com/gin-contrib/secure/@latest ; go.mod ; LICENSE |

Facts from the docs:

- `CrossOriginProtection`: "Cross-origin requests are currently detected with the Sec-Fetch-Site header ... or by comparing the hostname of the Origin header with the Host header." "The GET, HEAD, and OPTIONS methods are safe methods and are always allowed." "Requests without Sec-Fetch-Site or Origin headers are currently assumed to be either same-origin or non-browser requests, and are allowed." API: `NewCrossOriginProtection`, `AddTrustedOrigin`, `AddInsecureBypassPattern`, `Check`, `Handler`, `SetDenyHandler`.
- `x/time/rate`: "implements a 'token bucket' of size b, initially full and refilled at rate r tokens per second." "Limiter is safe for simultaneous use by multiple goroutines." It is one limiter; per-key limiting (per client or per endpoint) needs a hand-written map with eviction.
- `unrolled/secure` options include `AllowedHosts` ("Default is empty list, which allows any and all host names"), `ContentSecurityPolicy`, `ContentTypeNosniff`, `ReferrerPolicy`, `CrossOriginOpenerPolicy`, `CrossOriginResourcePolicy`, `PermissionsPolicy`, `FrameDeny`, `STS*`, `SSLRedirect`. It has `IsDevelopment`, which turns off `AllowedHosts`, `SSLRedirect` and STS. The default bad-host response is `500 Bad Host`. A `$NONCE` template in the CSP produces a per-request nonce. Source: https://raw.githubusercontent.com/unrolled/secure/v1.18.0/README.md.
- Not read: gin-contrib/cors and gin-contrib/secure README text; gorilla/csrf API; ulule/limiter API.

### Gaps against the project rules (`security.md` sections 8, 9, 11)

- Required: "Reject state-changing requests whose Origin is missing or not in the configured allow-list." `CrossOriginProtection` allows requests with no Origin and no Sec-Fetch-Site, so it does not meet this rule alone.
- Required: Host allow-list and CSRF protection that does not rely on SameSite alone. Neither is provided by `x/time/rate` or by `CrossOriginProtection`.
- `unrolled/secure.AllowedHosts` can implement a Host allow-list, but it matches the Host header (and optionally `X-Forwarded-Host` through `HostsProxyHeaders`; do not enable that on loopback) and `IsDevelopment` disables it, which conflicts with "no development mode may bypass" style rules if used for security controls.

### PROPOSED, not decided

Minimum-dependency default: Gin only, plus `golang.org/x/time/rate` (already a Go-project module). Write by hand in the inbound HTTP adapter, each with tests:

1. Host allow-list middleware (exact match on normalized host:port; reject otherwise).
2. Origin check for state-changing methods (reject missing or unlisted Origin). `http.CrossOriginProtection` may be used as an additional layer with `AddTrustedOrigin`.
3. CSRF token (synchronizer token bound to the server-side session, sent in a custom header) in addition to the Origin check.
4. Security headers: CSP without inline scripts, `X-Content-Type-Options: nosniff`, `Content-Disposition: attachment` on evidence downloads (about 20 lines).
5. Per-client and per-endpoint rate limiting using `rate.Limiter` in a keyed map with eviction; stricter limiter for the authentication endpoint.
6. CORS: use `gin-contrib/cors` v1.7.11 with an explicit origin list (the only library here that saves meaningful code), or hand-write because the requirement is "explicitly configured origins only". Owner choice.

Not recommended: `gorilla/csrf` (generic net/http, cookie-based double submit), `ulule/limiter` (older, adds storage abstractions not needed for one local account), `gin-contrib/secure` and `unrolled/secure` (the header set is small to write; `IsDevelopment` conflicts with the no-bypass stance).

Validation work still needed: confirm Gin v1.12.0 works with the toolchain required by each `go` directive (gin-contrib modules require `go 1.26.10`); read the READMEs of the libraries before adopting them; license review of the transitive modules in `go.sum` after the graph is resolved.

---

## 9. Q8: Cookies, SameSite and localhost

| Fact | Source | Quote |
| --- | --- | --- |
| Cookies are not isolated by port. | RFC 6265 section 8.5 "Weak Confidentiality", https://www.rfc-editor.org/rfc/rfc6265.txt | "Cookies do not provide isolation by port. If a cookie is readable by a service running on one port, the cookie is also readable by a service running on another port of the same server. If a cookie is writable by a service on one port, the cookie is also writable by a service running on another port of the same server. For this reason, servers SHOULD NOT both run mutually distrusting services on different ports of the same host and use cookies to store security-sensitive information." Also no isolation by scheme or (always) by path. |
| Current draft of RFC 6265bis | IETF datatracker API https://datatracker.ietf.org/api/v1/doc/document/draft-ietf-httpbis-rfc6265bis/ | `name draft-ietf-httpbis-rfc6265bis`, `rev 22`, title "Cookies: HTTP State Management Mechanism", `rfc` is null (not yet an RFC), abstract "This document obsoletes RFC 6265". The `expires` field shows 2026-06-04 and the document `time` shows 2026-10-19, so the draft is past its nominal expiry date at fetch time. |
| Text of revision 22 | https://www.ietf.org/archive/id/draft-ietf-httpbis-rfc6265bis-22.txt | Page header: "Expires 4 June 2026 ... December 2025". |
| Same-site definition is external | same file, section 5.2 | "Two origins are same-site if they satisfy the 'same site' criteria defined in [SAMESITE]." The reference target of `[SAMESITE]` was not captured. The request's "site for cookies" comes from the top-level origin (section 5.2.1). |
| SameSite attribute at cookie retrieval | same file, cookie retrieval algorithm | "If the cookie's same-site-flag is not 'None' and the retrieval's same-site status is 'cross-site', then exclude the cookie unless all of the following conditions are met: The retrieval's type is 'HTTP'. The same-site-flag is 'Lax' or 'Default'. The HTTP request ... uses a 'safe' method. The target browsing context ... is the active browsing context or a top-level traversable." |
| Host-only cookies | same file | "The cookie's host-only-flag is true and retrieval-host-canonical is identical to the cookie's domain." So a cookie set by `localhost` without a Domain attribute is not sent to `127.0.0.1`. |
| localhost treated as trusted | same file | "most user agents consider 'https' to be a scheme that denotes a secure protocol and 'localhost' to be trusted host." |
| HTML Standard "same site" | https://html.spec.whatwg.org/multipage/browsers.html, section 7.1.1.1 "Sites" | "To obtain a site ... If origin's host's registrable domain is null, then return (origin's scheme, origin's host). Return (origin's scheme, origin's host's registrable domain)." Same site requires equal scheme and equal site host. "for schemelessly same site and same site, the port and domain components are ignored." "the same site and schemelessly same site concepts should be avoided when possible, in favor of same origin checks." |
| Registrable domain | https://url.spec.whatwg.org/ section 3.2 | "If host's public suffix is null or host's public suffix equals host, then return null." Applies to domains; an IP address is not a domain, so its public suffix is null. |

Derived conclusions (read from the texts above; not tested in a browser):

- `http://localhost:3000` and `http://localhost:8080` are **same-site** (same scheme, same host, port ignored). SameSite cookies give no separation between two localhost ports. This matches RFC 6265 section 8.5. Any local web page on another port is same-site with the application, so SameSite alone cannot stop a hostile local page. This agrees with the project rule "Do not rely on SameSite alone".
- `http://localhost:<p>` and `http://127.0.0.1:<p>` are **not** same-site: both have a null registrable domain (`localhost` is its own public suffix under the implicit `*` rule of the PSL algorithm; an IP is not a domain), so the hosts are compared and differ. A host-only cookie set on one is not sent to the other.
- Not verified: how each browser treats `__Host-` and `Secure` cookies on `http://localhost`; whether `*.localhost` subdomains share cookies with `localhost`; the default SameSite behaviour of each browser (the draft mentions a "Default" flag value without the details captured here).

PROPOSED, not decided: pick one canonical host (`127.0.0.1` or `localhost`), reject every other Host (the Host allow-list from Q7), use host-only cookies with `HttpOnly`, `SameSite=Strict` or `Lax`, and `Secure` only if TLS is used, and add the CSRF token and Origin check. Validation work still needed: browser tests for the cookie behaviours listed as not verified.

---

## 10. Q9: DNS rebinding and Host header validation

Primary source for the exact claim "validating the Host header mitigates DNS rebinding against localhost services": **NOT VERIFIED.** The OWASP community page for DNS rebinding, an OWASP WSTG host-header page, and the Stanford/Barth paper could not be fetched (404, redirect to an unapproved origin, or PDF not supported).

What was verified:

| Source | URL | Statement | Class |
| --- | --- | --- | --- |
| OWASP SSRF Prevention Cheat Sheet | https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html | "Domain allowlisting alone does not prevent DNS rebinding. Validate the resolved destination IP addresses against the application's permitted networks, then ensure the HTTP client connects only to validated addresses. A second, unchecked DNS lookup between validation and connection can bypass these checks." This concerns outbound requests (our tool egress), not inbound Host validation. | primary (OWASP) |
| WICG Private Network Access, section 5.3 | https://wicg.github.io/private-network-access/ | "The mitigation described here operates upon the IP address which the user agent actually connects to ... This check MUST be performed for each new connection made, as DNS rebinding attacks may otherwise trick the user agent into revealing information it shouldn't." Browser-side mitigation; it does not replace server checks (section 5.4: "a router's web-based administration interface must be designed and implemented to defend against CSRF on its own"). | primary (draft community spec) |
| MCP specification, Streamable HTTP transport | https://modelcontextprotocol.io/specification/2025-06-18/basic/transports | "Servers MUST validate the Origin header on all incoming connections to prevent DNS rebinding attacks. When running locally, servers SHOULD bind only to localhost (127.0.0.1) rather than all network interfaces (0.0.0.0)." Origin validation, not Host validation, and not a standards body. | protocol spec, indirect |
| Wikipedia "DNS rebinding" | https://en.wikipedia.org/wiki/DNS_rebinding | "Web servers can reject HTTP requests with an unrecognized Host header." | **SECONDARY** |

Mechanism (secondary but consistent with the sources above): under rebinding the browser sends the attacker's host name in `Host` and `Origin`, so a server that allows only its own configured host names rejects the request even though the connection reaches loopback. The project rules already require both a Host allow-list and an Origin allow-list (`security.md` section 8).

Validation work still needed: find a primary source (the Stanford/Barth paper "Protecting Browsers from DNS Rebinding Attacks" is cited by Wikipedia and was not readable; RFC 9110 section 7.4 on misdirected requests was not read), and test rebinding with a test resolver in a lab.

---

## 11. Consolidated table of recommended pins

All are PROPOSED. Versions are the `@latest` results on 2026-11-15; none was resolved into a `go.mod` or `go.sum` here.

| Name | Version | Licence | Role | Source URL |
| --- | --- | --- | --- | --- |
| golang.org/x/net (idna) | v0.62.0 (2026-10-27) | BSD-3-Clause | IDNA / UTS #46 canonicalization | https://proxy.golang.org/golang.org/x/net/@latest |
| golang.org/x/net (publicsuffix) | v0.62.0, snapshot PSL rev d6c92f1b... (2026-02-06) | BSD-3-Clause (data MPL-2.0) | Not recommended for security decisions (stale snapshot) | https://raw.githubusercontent.com/golang/net/v0.62.0/publicsuffix/table.go |
| public_suffix_list.dat (vendored data) | VERSION 2026-11-14_18-45-38_UTC, COMMIT dfe55f3548d0fe51c04b8d2903507a01fdec57ad (re-pin at adoption; SHA-256 not computed) | MPL-2.0 | Public suffix source for wildcard refusal | https://publicsuffix.org/list/public_suffix_list.dat |
| IANA IPv4 special-purpose CSV | registry Last Updated 2025-10-09 (SHA-256 not computed) | IANA terms NOT VERIFIED | Generated deny table | https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry-1.csv |
| IANA IPv6 special-purpose CSV | registry Last Updated 2025-10-09 (SHA-256 not computed) | IANA terms NOT VERIFIED | Generated deny table | https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry-1.csv |
| golang.org/x/crypto (argon2) | v0.59.0 (2026-11-11), `go 1.26.0` | BSD-3-Clause | Argon2id password hashing | https://proxy.golang.org/golang.org/x/crypto/@latest |
| github.com/zalando/go-keyring | v0.2.8 (2026-03-23) | MIT | Secret Service client behind `CredentialStore` port | https://proxy.golang.org/github.com/zalando/go-keyring/@latest |
| github.com/godbus/dbus/v5 | v5.2.4 (2026-10-13) | BSD-2-Clause | D-Bus client (transitive of go-keyring) | https://proxy.golang.org/github.com/godbus/dbus/v5/@latest |
| github.com/gin-gonic/gin | v1.12.0 (2026-02-28) | MIT | HTTP framework (already selected) | https://proxy.golang.org/github.com/gin-gonic/gin/@latest |
| golang.org/x/time | v0.16.0 (2026-08-19), `go 1.26.0` | BSD-3-Clause | Token-bucket rate limiting | https://proxy.golang.org/golang.org/x/time/@latest |
| github.com/gin-contrib/cors | v1.7.11 (2026-11-08), `go 1.26.10` | MIT | CORS with explicit origins (optional) | https://proxy.golang.org/github.com/gin-contrib/cors/@latest |
| net/http CrossOriginProtection | Go 1.25 or later (local `go1.27.3`) | BSD-3-Clause (Go) | Extra CSRF layer; does not meet the missing-Origin rule alone | https://go.dev/doc/go1.25 |

Not recommended: github.com/99designs/keyring v1.2.2 (2022-12-19, silent fallback), github.com/gorilla/csrf v1.7.3, github.com/ulule/limiter/v3 v3.11.2, github.com/unrolled/secure v1.18.0, github.com/gin-contrib/secure v1.1.7, github.com/mtibben/confusables (2021).

---

## 12. Claims attempted but not verified

1. A primary source stating Host header validation mitigates DNS rebinding against localhost services (Q9). Only Wikipedia (secondary), an Origin-validation requirement in the MCP spec, and browser-side and outbound mitigations were found.
2. Hetzner metadata endpoint address (Q4): docs fetches returned no text.
3. Oracle Cloud IPv6 metadata address; AWS container-credential endpoints (169.254.170.2, 169.254.170.23) from AWS's own documentation (only OWASP states them).
4. The "Last Updated" dates of the IANA IPv4 and IPv6 address-space registries (used only for multicast); the IANA multicast-address assignment registries; the IPv6 special-purpose XML file.
5. RFC bodies for 6052, 8215, 3056, 4380, 4291 (prefix and RFC citations were verified only through the IANA registry rows). RFC 6890's own text (a selective search for "Globally Reachable" found nothing, as expected for the older term "global").
6. IANA terms of use for registry data.
7. Whether x/net ships the MPL-2.0 notice for PSL-derived data; whether MPL-2.0 applies to a generated Go table (legal question).
8. WHATWG URL spec revision date; WHATWG web-platform-tests data location; the exact URL of UTS #46 `IdnaTestV2.txt`.
9. The introductory sentence of the UTS #46 section 3 list that mentions UTS #39 Restriction Level 4.
10. Whether Go's standard `unicode` package exposes Script_Extensions (needed for an in-repo UTS #39 implementation).
11. Licence of `github.com/disciplinedware/go-confusables`; licence of `mtibben/confusables` was taken from GitHub's detection (secondary).
12. Any run-time behaviour of x/net/idna option combinations, x/net/publicsuffix lookups, argon2, go-keyring error values, `dbus-launch` autolaunch in practice, D-Bus activation of a keyring daemon.
13. Whether a worker outside a desktop session can use the Secret Service on any specific distribution (only the spec's "user's login session" wording and the library source were read).
14. The Secret Service specification's text on session bus versus system bus beyond chapter 1.
15. PHC string base64 padding rule (the fetched text was truncated); value of `argon2.Version`; RFC 9106 recommended salt and tag lengths.
16. Argon2id and OWASP parameters were verified only as text; no timing was measured.
17. gorilla/csrf archive status and API; ulule/limiter, gin-contrib/cors and gin-contrib/secure READMEs; maintenance activity beyond tag dates.
18. Browser behaviour for `__Host-` and `Secure` cookies on `http://localhost`, `*.localhost` cookie sharing, and each browser's default SameSite handling.
19. The `[SAMESITE]` reference target in RFC 6265bis draft revision 22.
20. gin-contrib/csrf and gin-contrib/limiter: proxy returned 404 for both module paths. That shows the paths are not resolvable on the proxy, not that no similar package exists elsewhere.
