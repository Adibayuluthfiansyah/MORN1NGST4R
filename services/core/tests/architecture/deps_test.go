// Package architecture_test enforces the dependency directions of
// .kiro/steering/structure.md section 5.6 and the process-creation policy of
// structure.md section 12 and security.md section 10 over `go list -json`
// (gate G-arch).
//
// The rules are fail-closed:
//   - An internal package that matches no rule fails.
//   - An import that `go list` does not report fails (for example cgo "C").
//   - A third-party import that is not listed in `external` fails.
//   - Source that the checked platform excludes (build constraints) fails,
//     because it would escape the check.
//
// Standard library versus third-party is decided from go list's Standard field
// and Module data, not from the shape of the import path.
//
// Platform scope: the MVP platform is Linux x86_64 only (D-16, D-17, D-19).
// structure.md section 3 reserves platform/windows, platform/darwin and
// cmd/morningstar and says they are not created without approval. The check
// therefore runs for linux/amd64 and refuses code it cannot see. When another
// platform is approved, add it to `platforms` and the check runs per platform.
//
// Domain packages follow the dependency order table of structure.md 5.6
// (domainOrder below). A domain package that is not in the table fails.
package architecture_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core"

// platforms are the GOOS/GOARCH pairs the architecture defines. Do not add a
// pair without an approved platform decision.
var platforms = []struct{ goos, goarch string }{
	{"linux", "amd64"},
}

// std builds a forbidden standard-library list. Every group forbids process
// creation (os/exec, syscall) and plugin loading unless it says otherwise,
// because only a tool adapter may start processes (structure.md section 12)
// and none exists yet.
func std(extra ...string) []string {
	return append([]string{"os/exec", "syscall", "plugin"}, extra...)
}

type rule struct {
	// prefix selects the packages (relative to the module root). The longest
	// matching prefix wins.
	prefix string
	// internal lists the module-relative path prefixes the package may import.
	// A package may always import itself (an external test package does).
	internal []string
	// forbidStd lists standard library package families the group must not
	// import. "net/http" forbids net/http and everything below it.
	forbidStd []string
	// exceptStd lists standard library packages that stay allowed although a
	// forbidden family contains them. "net/netip" is the one member of the
	// "net" family that the domain may use: pure value types, no resolver.
	exceptStd []string
	// forbidStdProd lists families that are forbidden in the production files
	// of the group and allowed in its test files. math/rand is out of the
	// scope engine because it must be deterministic, while its seeded property
	// tests use math/rand/v2.
	forbidStdProd []string
}

// domainOrder is the table of structure.md 5.6: the domain packages that a
// domain package may import. Only ID types and value objects may cross.
var domainOrder = map[string][]string{
	"operation":  {},
	"actor":      {},
	"program":    {},
	"identity":   {"actor"},
	"audit":      {"actor"},
	"target":     {"program"},
	"approval":   {"operation", "actor"},
	"scope":      {"target", "operation", "approval"},
	"assessment": {"program", "target"},
	"asset":      {"assessment", "target"},
	"job":        {"assessment", "operation", "approval", "actor"},
	"evidence":   {"assessment", "job", "operation"},
	"finding":    {"assessment", "asset", "evidence"},
	"validation": {"finding", "evidence", "job"},
}

// strictDomainStd is the forbidden standard library for the domain packages of
// the policy core. It has no "time": see the comment on the rules below.
var strictDomainStd = std("net", "database/sql", "os", "io/fs", "path/filepath", "log", "crypto/tls", "crypto/rand", "context")

var rules = []rule{
	// The domain never resolves names or touches the network, so "net" is out.
	// Domain-to-domain imports are checked against domainOrder.
	{prefix: "internal/domain", internal: []string{"internal/domain"}, forbidStd: std("net", "database/sql"), exceptStd: []string{"net/netip"}},
	// The scope engine is deterministic: no clock, no file system, no environment.
	// Only net/netip is allowed from the net family. No TLS, no randomness.
	// The same group for every domain package that M1 and M2 built: scope,
	// actor, operation and approval. The time family is allowed (time.Time and
	// time.Duration are value types the domain needs). Reading the clock is
	// refused by a source scan instead (domain_rules_test.go), so time always
	// comes in as an argument.
	{prefix: "internal/domain/scope", internal: []string{"internal/domain"}, forbidStd: strictDomainStd, exceptStd: []string{"net/netip"}, forbidStdProd: []string{"math/rand"}},
	{prefix: "internal/domain/actor", internal: []string{"internal/domain"}, forbidStd: strictDomainStd, exceptStd: []string{"net/netip"}, forbidStdProd: []string{"math/rand"}},
	{prefix: "internal/domain/operation", internal: []string{"internal/domain"}, forbidStd: strictDomainStd, exceptStd: []string{"net/netip"}, forbidStdProd: []string{"math/rand"}},
	{prefix: "internal/domain/approval", internal: []string{"internal/domain"}, forbidStd: strictDomainStd, exceptStd: []string{"net/netip"}, forbidStdProd: []string{"math/rand"}},
	{prefix: "internal/application", internal: []string{"internal/domain", "internal/ports"}, forbidStd: std("net/http", "database/sql")},
	{prefix: "internal/ports", internal: []string{"internal/domain", "internal/ports"}, forbidStd: std("net/http", "database/sql")},
	{prefix: "internal/adapters/inbound", internal: []string{"internal/ports/inbound", "internal/domain"}, forbidStd: std("database/sql")},
	{prefix: "internal/adapters/inbound/http", internal: []string{"internal/ports/inbound", "internal/domain", "internal/adapters/inbound/http"}, forbidStd: std("database/sql")},
	{prefix: "internal/adapters/outbound", internal: []string{"internal/ports/outbound", "internal/domain", "internal/platform", "internal/infrastructure"}, forbidStd: std("net/http")},
	{prefix: "internal/infrastructure", internal: []string{"internal/infrastructure"}, forbidStd: std()},
	{prefix: "internal/platform", internal: []string{"internal/platform"}, forbidStd: []string{"plugin"}},
	{prefix: "internal/config", forbidStd: std("net/http")},
	{prefix: "internal/bootstrap", internal: []string{"internal"}, forbidStd: std()},
	// cmd needs syscall for signal numbers (syscall.SIGTERM).
	{prefix: "cmd", internal: []string{"internal/bootstrap", "internal/config"}, forbidStd: []string{"os/exec", "plugin"}},
}

// external lists every permitted third-party module path and the packages
// that may import it. Anything else is refused. Each entry must be in tech.md.
var external = []struct {
	path      string
	allowedIn []string
}{
	// Gin is isolated in the inbound HTTP adapter (structure.md section 7).
	{path: "github.com/gin-gonic/gin", allowedIn: []string{"internal/adapters/inbound/http"}},
}

// processCreation lists the packages that may call os.StartProcess or
// syscall.Exec, ForkExec and StartProcess. It is empty: tool execution
// belongs to an outbound tools adapter that does not exist yet (M10). Add that
// package here, with its own rule above, when it is built.
var processCreation = []string{}

func under(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func ruleFor(rel string) *rule {
	var best *rule
	for i := range rules {
		r := &rules[i]
		if under(rel, r.prefix) && (best == nil || len(r.prefix) > len(best.prefix)) {
			best = r
		}
	}
	return best
}

// pkgInfo is what go list says about one package that is imported somewhere.
type pkgInfo struct {
	Standard bool
	Main     bool     // belongs to the main module
	Imports  []string // direct production imports, as go list reports them
}

// classifier maps an import path to go list's classification of it.
type classifier map[string]pkgInfo

// violations returns every rule that the package (module-relative path and
// import path) breaks by importing imports.
func (c classifier) violations(rel, importPath string, imports []string) []string {
	return c.check(rel, importPath, imports, false)
}

// testViolations is violations for the imports of test files. Families in
// forbidStdProd are allowed there.
func (c classifier) testViolations(rel, importPath string, imports []string) []string {
	return c.check(rel, importPath, imports, true)
}

func (c classifier) check(rel, importPath string, imports []string, isTest bool) []string {
	if under(rel, "tests") {
		return nil // tests may reach any package
	}
	r := ruleFor(rel)
	if r == nil {
		return []string{fmt.Sprintf("%s matches no architecture rule. Add one to deps_test.go", rel)}
	}
	var out []string
	if !isTest && under(rel, "internal/domain") && rel != "internal/domain" {
		if _, ok := domainOrder[domainPkg(rel)]; !ok {
			out = append(out, fmt.Sprintf("%s has no entry in the domain dependency order table (structure.md 5.6). Add it to domainOrder", rel))
		}
	}
	for _, imp := range imports {
		if imp == importPath {
			continue // an external test package imports the package under test
		}
		info, known := c[imp]
		switch {
		case !known:
			out = append(out, fmt.Sprintf("%s imports %s, which go list does not report (cgo or an unresolved import)", rel, imp))
		case info.Standard:
			exempt := false
			for _, e := range r.exceptStd {
				if under(imp, e) {
					exempt = true
				}
			}
			if exempt {
				break
			}
			for _, f := range r.forbidStd {
				if under(imp, f) {
					out = append(out, fmt.Sprintf("%s must not import %s (forbidden family %s)", rel, imp, f))
				}
			}
			if !isTest {
				for _, f := range r.forbidStdProd {
					if under(imp, f) {
						out = append(out, fmt.Sprintf("%s must not import %s in production code (forbidden family %s)", rel, imp, f))
					}
				}
			}
		case info.Main:
			target := strings.TrimPrefix(imp, module+"/")
			if under(rel, "internal/domain") && under(target, "internal/domain") {
				out = append(out, domainOrderViolations(rel, target)...)
				continue
			}
			ok := false
			for _, allowed := range r.internal {
				if under(target, allowed) {
					ok = true
				}
			}
			if !ok {
				out = append(out, fmt.Sprintf("%s must not import %s", rel, target))
			}
		default:
			ok := false
			for _, e := range external {
				if under(imp, e.path) {
					for _, in := range e.allowedIn {
						if under(rel, in) {
							ok = true
						}
					}
				}
			}
			if !ok {
				out = append(out, fmt.Sprintf("%s must not import third-party package %s", rel, imp))
			}
		}
	}
	return out
}

// domainTransitiveBan lists the standard library families that no domain
// package may reach, directly or through any chain of imports: the network
// (except net/netip), TLS, randomness, process execution and plugins. A direct
// import is already refused by the rules above. This closes the indirect
// route, for example a domain package that imports another package that
// imports net/smtp. os, time and syscall are not listed because fmt and os
// import them for everyone. The scope package refuses them by direct import.
var domainTransitiveBan = []string{"net", "crypto/tls", "crypto/rand", "math/rand", "os/exec", "plugin"}

// domainTransitiveExcept are exceptions to domainTransitiveBan.
var domainTransitiveExcept = []string{"net/netip"}

func transitivelyBanned(path string) bool {
	for _, e := range domainTransitiveExcept {
		if under(path, e) {
			return false
		}
	}
	for _, f := range domainTransitiveBan {
		if under(path, f) {
			return true
		}
	}
	return false
}

// transitiveViolations walks the import graph from start and reports every
// reachable package that is banned, with the chain that reaches it.
func (c classifier) transitiveViolations(rel string, start []string) []string {
	parent := map[string]string{}
	queue := append([]string{}, start...)
	for _, s := range start {
		parent[s] = ""
	}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if transitivelyBanned(cur) {
			chain := []string{cur}
			for p := parent[cur]; p != ""; p = parent[p] {
				chain = append([]string{p}, chain...)
			}
			out = append(out, fmt.Sprintf("%s reaches %s through %s", rel, cur, strings.Join(chain, " -> ")))
			continue
		}
		for _, next := range c[cur].Imports {
			if _, seen := parent[next]; !seen {
				parent[next] = cur
				queue = append(queue, next)
			}
		}
	}
	return out
}

// domainPkg returns the first path element below internal/domain.
func domainPkg(rel string) string {
	parts := strings.Split(strings.TrimPrefix(rel, "internal/domain/"), "/")
	return parts[0]
}

// domainOrderViolations checks one domain-to-domain import against domainOrder.
func domainOrderViolations(rel, target string) []string {
	src, dst := domainPkg(rel), domainPkg(target)
	if src == dst {
		return nil
	}
	for _, allowed := range domainOrder[src] {
		if dst == allowed {
			return nil
		}
	}
	return []string{fmt.Sprintf("domain package %s must not import domain package %s (structure.md 5.6 order table)", src, dst)}
}

// processCreationUses reports calls to os.StartProcess and to the syscall
// functions that start or replace a process. Importing os/exec is handled by
// the import rules. This catches the calls that need no extra import. A dot
// import of os or syscall is refused because it hides the package name.
func processCreationUses(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	local := map[string]string{} // local name -> import path, for os and syscall only
	var out []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || (path != "os" && path != "syscall") {
			continue
		}
		switch {
		case spec.Name == nil:
			local[path] = path
		case spec.Name.Name == ".":
			out = append(out, fmt.Sprintf("%s: dot import of %s hides process-creation calls", filename, path))
		case spec.Name.Name != "_":
			local[spec.Name.Name] = path
		}
	}
	banned := map[string][]string{
		"os":      {"StartProcess"},
		"syscall": {"Exec", "ForkExec", "StartProcess"},
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, ok := local[id.Name]
		if !ok {
			return true
		}
		for _, name := range banned[path] {
			if sel.Sel.Name == name {
				out = append(out, fmt.Sprintf("%s: %s.%s starts a process", filename, path, name))
			}
		}
		return true
	})
	return out, nil
}

// ---- unit tests of the rule engine (regression tests for the gate itself) ----

// fakeList builds the classification go list would give for the paths used in
// the tests below.
func fakeList() classifier {
	c := classifier{}
	for _, p := range []string{
		"context", "errors", "log/slog", "net/http", "net/http/httputil", "net/http/httptest", "net/http/cgi",
		"netx", // not a real package: proves a family match needs a path boundary
		"database/sql", "database/sql/driver", "os", "os/exec", "os/signal", "syscall", "plugin", "unsafe", "time",
		"net", "net/netip", "io/fs", "path/filepath", "log", "strings", "strconv", "sort", "fmt", "unicode/utf8", "slices",
		"net/smtp", "net/rpc", "net/url", "net/textproto", "net/mail", "net/rpc/jsonrpc", "crypto/tls", "crypto/rand", "crypto/sha256",
		"encoding/binary",
		"math/rand", "math/rand/v2", "math", "reflect", "runtime", "internal/netip",
	} {
		c[p] = pkgInfo{Standard: true}
	}
	for _, p := range []string{
		"internal/domain/scope", "internal/domain/target", "internal/domain/operation", "internal/domain/approval", "internal/domain/assessment",
		"internal/domain/job", "internal/domain/actor", "internal/domain/unlisted", "internal/application/health", "internal/application/other", "internal/ports/inbound",
		"internal/ports/outbound", "internal/adapters/inbound/http/router", "internal/adapters/inbound/http/handlers",
		"internal/adapters/outbound/persistence", "internal/infrastructure/logging", "internal/platform/linux",
		"internal/bootstrap", "internal/config",
	} {
		c[module+"/"+p] = pkgInfo{Main: true}
	}
	for _, p := range []string{
		"github.com/gin-gonic/gin", "github.com/gin-gonic/gin/binding", "github.com/gin-gonic/ginx", "github.com/some/library",
		"golang.org/x/sys/unix",
		"mylib/dotless", // a third-party module whose path has no dot
	} {
		c[p] = pkgInfo{}
	}
	return c
}

func TestRuleEngineDetectsViolations(t *testing.T) {
	const m = module + "/"
	tests := []struct {
		name       string
		rel        string
		imports    []string
		wantSubstr string
	}{
		{"domain imports gin", "internal/domain/scope", []string{"github.com/gin-gonic/gin"}, "third-party"},
		{"domain imports net/http", "internal/domain/scope", []string{"net/http"}, "net/http"},
		{"domain imports a nested net/http package", "internal/domain/scope", []string{"net/http/httputil"}, "family net"},
		{"domain imports net/http/cgi", "internal/domain/scope", []string{"net/http/cgi"}, "family net"},
		{"domain imports database/sql/driver", "internal/domain/scope", []string{"database/sql/driver"}, "family database/sql"},
		{"domain imports application", "internal/domain/scope", []string{m + "internal/application/health"}, "internal/application/health"},
		{"domain imports ports", "internal/domain/scope", []string{m + "internal/ports/inbound"}, "internal/ports/inbound"},
		{"application imports gin", "internal/application/health", []string{"github.com/gin-gonic/gin"}, "third-party"},
		{"application imports net/http", "internal/application/health", []string{"net/http"}, "net/http"},
		{"application imports net/http/httptest", "internal/application/health", []string{"net/http/httptest"}, "family net/http"},
		{"application imports an adapter", "internal/application/health", []string{m + "internal/adapters/inbound/http/router"}, "adapters"},
		{"application imports another application package", "internal/application/health", []string{m + "internal/application/other"}, "internal/application/other"},
		{"application runs commands", "internal/application/health", []string{"os/exec"}, "os/exec"},
		{"application imports syscall", "internal/application/health", []string{"syscall"}, "syscall"},
		{"application loads plugins", "internal/application/health", []string{"plugin"}, "plugin"},
		{"application imports x/sys", "internal/application/health", []string{"golang.org/x/sys/unix"}, "third-party"},
		{"ports import application", "internal/ports/inbound", []string{m + "internal/application/health"}, "application"},
		{"ports import syscall", "internal/ports/inbound", []string{"syscall"}, "syscall"},
		{"inbound adapter imports application", "internal/adapters/inbound/http/handlers", []string{m + "internal/application/health"}, "internal/application/health"},
		{"inbound adapter imports outbound port", "internal/adapters/inbound/http/handlers", []string{m + "internal/ports/outbound"}, "internal/ports/outbound"},
		{"inbound adapter imports an outbound adapter", "internal/adapters/inbound/http/handlers", []string{m + "internal/adapters/outbound/persistence"}, "persistence"},
		{"inbound adapter runs commands", "internal/adapters/inbound/http/handlers", []string{"os/exec"}, "os/exec"},
		{"inbound adapter imports syscall", "internal/adapters/inbound/http/handlers", []string{"syscall"}, "syscall"},
		{"inbound adapter imports database/sql", "internal/adapters/inbound/http/handlers", []string{"database/sql"}, "database/sql"},
		{"outbound adapter imports application", "internal/adapters/outbound/persistence", []string{m + "internal/application/health"}, "application"},
		{"outbound adapter imports inbound adapter", "internal/adapters/outbound/persistence", []string{m + "internal/adapters/inbound/http/router"}, "adapters/inbound"},
		{"outbound adapter runs commands", "internal/adapters/outbound/persistence", []string{"os/exec"}, "os/exec"},
		{"infrastructure imports domain", "internal/infrastructure/logging", []string{m + "internal/domain/scope"}, "domain"},
		{"infrastructure runs commands", "internal/infrastructure/logging", []string{"os/exec"}, "os/exec"},
		{"gin outside the http adapter", "internal/bootstrap", []string{"github.com/gin-gonic/gin"}, "third-party"},
		{"gin in an inbound worker adapter", "internal/adapters/inbound/worker", []string{"github.com/gin-gonic/gin"}, "third-party"},
		{"gin sub-package outside the http adapter", "internal/bootstrap", []string{"github.com/gin-gonic/gin/binding"}, "third-party"},
		{"a module whose name only starts like gin", "internal/adapters/inbound/http/router", []string{"github.com/gin-gonic/ginx"}, "third-party"},
		{"unlisted third-party library", "internal/adapters/inbound/http/router", []string{"github.com/some/library"}, "third-party"},
		{"third-party module with a dotless path is not mistaken for the standard library", "internal/domain/scope", []string{"mylib/dotless"}, "third-party"},
		{"cgo import is not reported by go list", "internal/domain/scope", []string{"C"}, "does not report"},
		{"unresolved import", "internal/domain/scope", []string{"example.invalid/missing"}, "does not report"},
		{"config imports another package", "internal/config", []string{m + "internal/bootstrap"}, "bootstrap"},
		{"config runs commands", "internal/config", []string{"os/exec"}, "os/exec"},
		{"bootstrap runs commands", "internal/bootstrap", []string{"os/exec"}, "os/exec"},
		{"cmd imports an adapter", "cmd/server", []string{m + "internal/adapters/inbound/http/router"}, "adapters"},
		{"cmd imports the domain", "cmd/server", []string{m + "internal/domain/scope"}, "domain"},
		{"cmd runs commands", "cmd/server", []string{"os/exec"}, "os/exec"},
		{"platform loads plugins", "internal/platform/linux", []string{"plugin"}, "plugin"},
		{"unknown internal package", "internal/utils", nil, "no architecture rule"},
		{"unknown top-level package", "pkg/helpers", nil, "no architecture rule"},
	}
	c := fakeList()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(c.violations(tc.rel, module+"/"+tc.rel, tc.imports), "; ")
			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("violations = %q, want one containing %q", got, tc.wantSubstr)
			}
		})
	}
}

func TestRuleEngineAllowsTheDocumentedDirections(t *testing.T) {
	const m = module + "/"
	tests := []struct {
		name    string
		rel     string
		imports []string
	}{
		{"application uses ports and domain", "internal/application/health", []string{m + "internal/ports/outbound", m + "internal/domain/scope", "log/slog", "context"}},
		{"inbound http adapter uses gin, inbound port and its own packages", "internal/adapters/inbound/http/router", []string{"github.com/gin-gonic/gin", m + "internal/ports/inbound", m + "internal/adapters/inbound/http/handlers", "net/http", "net/http/httptest"}},
		{"gin sub-package inside the http adapter", "internal/adapters/inbound/http/handlers", []string{"github.com/gin-gonic/gin/binding"}},
		{"outbound adapter uses outbound port and infrastructure", "internal/adapters/outbound/persistence", []string{m + "internal/ports/outbound", m + "internal/infrastructure/logging", m + "internal/platform/linux"}},
		{"bootstrap composes everything", "internal/bootstrap", []string{m + "internal/application/health", m + "internal/adapters/inbound/http/router", m + "internal/config", m + "internal/infrastructure/logging", "net/http"}},
		{"cmd uses bootstrap, config and syscall for signal numbers", "cmd/server", []string{m + "internal/bootstrap", m + "internal/config", "os/signal", "syscall"}},
		{"an external test package imports its own package", "internal/application/health", []string{m + "internal/application/health"}},
		{"a package whose name only looks like a forbidden family", "internal/domain/scope", []string{"netx"}},
		{"unsafe is standard library", "internal/domain/scope", []string{"unsafe"}},
		{"architecture tests reach everything", "tests/architecture", []string{"os/exec", m + "internal/domain/scope", "github.com/some/library"}},
	}
	c := fakeList()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.violations(tc.rel, module+"/"+tc.rel, tc.imports); len(got) != 0 {
				t.Errorf("unexpected violations: %v", got)
			}
		})
	}
}

func TestProcessCreationScanner(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"os.StartProcess", `package p; import "os"; func f() { os.StartProcess("x", nil, nil) }`, 1},
		{"aliased os", `package p; import o "os"; func f() { o.StartProcess("x", nil, nil) }`, 1},
		{"syscall.Exec", `package p; import "syscall"; func f() { syscall.Exec("x", nil, nil) }`, 1},
		{"syscall.ForkExec", `package p; import "syscall"; func f() { syscall.ForkExec("x", nil, nil) }`, 1},
		{"syscall.StartProcess", `package p; import sc "syscall"; func f() { sc.StartProcess("x", nil, nil) }`, 1},
		{"function value, not a call", `package p; import "os"; var f = os.StartProcess`, 1},
		{"dot import of os", `package p; import . "os"; func f() { StartProcess("x", nil, nil) }`, 1},
		{"dot import of syscall", `package p; import . "syscall"; func f() { Exec("x", nil, nil) }`, 1},
		{"two uses", `package p; import "os"; func f() { os.StartProcess("x", nil, nil); os.StartProcess("y", nil, nil) }`, 2},
		{"harmless os use", `package p; import "os"; func f() string { return os.Getenv("X") }`, 0},
		{"harmless syscall use", `package p; import "syscall"; var s = syscall.SIGTERM`, 0},
		{"a local value named StartProcess", `package p; type t struct{}; func (t) StartProcess() {}; func f(x t) { x.StartProcess() }`, 0},
		{"blank import", `package p; import _ "os"`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := processCreationUses("p.go", tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(got) != tc.want {
				t.Errorf("found %d uses %v, want %d", len(got), got, tc.want)
			}
		})
	}
	if _, err := processCreationUses("p.go", "package p; func ("); err == nil {
		t.Error("a file that does not parse was accepted, want an error (fail closed)")
	}
}

// ---- the real module ----

type listedPackage struct {
	ImportPath string
	Dir        string
	Standard   bool
	DepOnly    bool
	Module     *struct {
		Path string
		Main bool
	}
	GoFiles        []string
	CgoFiles       []string
	IgnoredGoFiles []string
	Imports        []string
	TestImports    []string
	XTestImports   []string
}

// listModule runs `go list -deps -test -json ./...` for one platform and
// returns the module's own packages and the classification of every package
// that any of them imports, test imports included.
func listModule(t *testing.T, goos, goarch string) ([]listedPackage, classifier) {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-test", "-json", "./...")
	cmd.Dir = "../.."
	cmd.Env = append(cmd.Environ(), "GOTOOLCHAIN=local", "GOOS="+goos, "GOARCH="+goarch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list for %s/%s failed: %v\n%s", goos, goarch, err, stderr.String())
	}

	c := classifier{}
	var own []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		// Test variants are listed as "pkg [pkg.test]" and "pkg.test". They add
		// nothing the plain entry does not carry.
		path := p.ImportPath
		if strings.Contains(path, " [") || strings.HasSuffix(path, ".test") {
			continue
		}
		info := pkgInfo{Standard: p.Standard, Imports: p.Imports}
		if p.Module != nil {
			info.Main = p.Module.Main
		}
		c[path] = info
		if info.Main && !p.DepOnly && (path == module || strings.HasPrefix(path, module+"/")) {
			own = append(own, p)
		}
	}
	if len(own) == 0 {
		t.Fatal("go list returned no package of the module")
	}
	return own, c
}

// TestModuleHonoursTheDependencyDirections applies the rules to the real
// packages of the module, test files included, on every defined platform.
func TestModuleHonoursTheDependencyDirections(t *testing.T) {
	for _, pf := range platforms {
		t.Run(pf.goos+"_"+pf.goarch, func(t *testing.T) {
			own, c := listModule(t, pf.goos, pf.goarch)
			var all []string
			for _, p := range own {
				rel := strings.TrimPrefix(p.ImportPath, module+"/")
				testImports := append(append([]string{}, p.TestImports...), p.XTestImports...)
				all = append(all, c.violations(rel, p.ImportPath, p.Imports)...)
				all = append(all, c.testViolations(rel, p.ImportPath, testImports)...)
			}
			sort.Strings(all)
			for _, v := range all {
				t.Error(v)
			}
		})
	}
}

// TestModuleStartsNoProcessOutsideThePolicy scans the non-test source of every
// package for calls that start a process without importing os/exec.
func TestModuleStartsNoProcessOutsideThePolicy(t *testing.T) {
	for _, pf := range platforms {
		t.Run(pf.goos+"_"+pf.goarch, func(t *testing.T) {
			own, _ := listModule(t, pf.goos, pf.goarch)
			scanned := 0
			for _, p := range own {
				rel := strings.TrimPrefix(p.ImportPath, module+"/")
				if under(rel, "tests") {
					continue
				}
				allowed := false
				for _, a := range processCreation {
					allowed = allowed || under(rel, a)
				}
				if allowed {
					continue
				}
				for _, f := range p.GoFiles {
					scanned++
					uses, err := processCreationUses(filepath.Join(p.Dir, f), nil)
					if err != nil {
						t.Errorf("%s: %v", filepath.Join(rel, f), err)
					}
					for _, u := range uses {
						t.Error(strings.Replace(u, p.Dir, rel, 1))
					}
				}
			}
			if scanned == 0 {
				t.Error("no source file was scanned")
			}
		})
	}
}

// TestNoSourceEscapesTheCheckedPlatforms fails when a file of the module is
// excluded by build constraints on every checked platform. Such a file (a
// `_windows.go` file, a `//go:build ignore` file) would never be inspected.
func TestNoSourceEscapesTheCheckedPlatforms(t *testing.T) {
	excluded := map[string]int{} // file -> number of platforms that exclude it
	for _, pf := range platforms {
		own, _ := listModule(t, pf.goos, pf.goarch)
		for _, p := range own {
			for _, f := range p.IgnoredGoFiles {
				excluded[filepath.Join(strings.TrimPrefix(p.ImportPath, module+"/"), f)]++
			}
		}
	}
	var files []string
	for f, n := range excluded {
		if n == len(platforms) {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	for _, f := range files {
		t.Errorf("%s is excluded on every checked platform and is not inspected. The architecture defines only %v (structure.md section 3). Remove the file or add its platform to `platforms` after an approved decision", f, platforms)
	}
}

func TestDomainRules(t *testing.T) {
	const m = module + "/"
	c := fakeList()
	detect := []struct {
		name, rel string
		imports   []string
		want      string
	}{
		{"scope resolves names", "internal/domain/scope", []string{"net"}, "forbidden family net"},
		{"another domain package resolves names", "internal/domain/job", []string{"net"}, "forbidden family net"},
		{"actor reads files", "internal/domain/actor", []string{"os"}, "family os"},
		{"operation resolves names", "internal/domain/operation", []string{"net"}, "forbidden family net"},
		{"approval logs", "internal/domain/approval", []string{"log"}, "family log"},
		{"approval uses a random source", "internal/domain/approval", []string{"crypto/rand"}, "family crypto/rand"},
		{"approval reads the clock through context", "internal/domain/approval", []string{"context"}, "family context"},
		{"scope reads the clock through context", "internal/domain/scope", []string{"context"}, "family context"},
		{"approval imports scope (reverse direction)", "internal/domain/approval", []string{m + "internal/domain/scope"}, "must not import domain package scope"},
		{"actor imports operation", "internal/domain/actor", []string{m + "internal/domain/operation"}, "must not import domain package operation"},
		{"approval imports job (reverse direction)", "internal/domain/approval", []string{m + "internal/domain/job"}, "must not import domain package job"},
		{"scope imports actor (not in the order table)", "internal/domain/scope", []string{m + "internal/domain/actor"}, "must not import domain package actor"},
		{"scope reads files", "internal/domain/scope", []string{"os"}, "family os"},
		{"scope uses os/signal", "internal/domain/scope", []string{"os/signal"}, "family os"},
		{"scope uses io/fs", "internal/domain/scope", []string{"io/fs"}, "family io/fs"},
		{"scope uses path/filepath", "internal/domain/scope", []string{"path/filepath"}, "family path/filepath"},
		{"scope logs", "internal/domain/scope", []string{"log"}, "family log"},
		{"scope imports an unrelated domain package", "internal/domain/scope", []string{m + "internal/domain/assessment"}, "must not import domain package assessment"},
		{"scope imports the job package", "internal/domain/scope", []string{m + "internal/domain/job"}, "must not import domain package job"},
		{"target imports scope (reverse direction)", "internal/domain/target", []string{m + "internal/domain/scope"}, "must not import domain package scope"},
		{"operation imports anything", "internal/domain/operation", []string{m + "internal/domain/actor"}, "must not import domain package actor"},
		{"domain package missing from the table", "internal/domain/unlisted", nil, "no entry in the domain dependency order table"},
		{"domain package missing from the table, with imports", "internal/domain/unlisted", []string{"errors"}, "no entry in the domain dependency order table"},
		{"domain imports a port", "internal/domain/scope", []string{m + "internal/ports/inbound"}, "internal/ports/inbound"},
	}
	for _, tc := range detect {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(c.violations(tc.rel, module+"/"+tc.rel, tc.imports), "; ")
			if !strings.Contains(got, tc.want) {
				t.Errorf("violations = %q, want one containing %q", got, tc.want)
			}
		})
	}

	allow := []struct {
		name, rel string
		imports   []string
	}{
		{"scope uses pure value types and the standard library", "internal/domain/scope", []string{"net/netip", "errors", "strings", "strconv", "sort", "fmt", "unicode/utf8", "slices"}},
		{"scope imports the domain packages of the order table", "internal/domain/scope", []string{m + "internal/domain/target", m + "internal/domain/operation", m + "internal/domain/approval"}},
		{"scope may hold time values", "internal/domain/scope", []string{"time", "net/netip", "errors"}},
		{"approval imports its table entries and holds time values", "internal/domain/approval", []string{m + "internal/domain/operation", m + "internal/domain/actor", "time", "errors", "fmt"}},
		{"operation hashes with the standard library", "internal/domain/operation", []string{"crypto/sha256", "encoding/binary", "sort", "time", "unicode/utf8", "errors"}},
		{"actor needs only the standard library", "internal/domain/actor", []string{"errors", "fmt"}},
		{"job imports its table entries and may use the clock", "internal/domain/job", []string{m + "internal/domain/assessment", m + "internal/domain/operation", m + "internal/domain/approval", m + "internal/domain/actor", "time"}},
		{"a package imports itself (external test package)", "internal/domain/scope", []string{m + "internal/domain/scope"}},
		{"a sub-package imports its own domain package", "internal/domain/scope/sub", []string{m + "internal/domain/scope"}},
	}
	for _, tc := range allow {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.violations(tc.rel, module+"/"+tc.rel, tc.imports); len(got) != 0 {
				t.Errorf("unexpected violations: %v", got)
			}
		})
	}
}

// TestDomainPackagesExist is Task 1.1 of the plan: the scope domain package is
// part of the module and is being checked.
func TestDomainPackagesExist(t *testing.T) {
	own, _ := listModule(t, platforms[0].goos, platforms[0].goarch)
	for _, want := range []string{"internal/domain/scope", "internal/domain/actor", "internal/domain/operation", "internal/domain/approval"} {
		found := false
		for _, p := range own {
			// A directory with only test files is not a package of the product.
			found = found || (strings.TrimPrefix(p.ImportPath, module+"/") == want && len(p.GoFiles) > 0)
		}
		if !found {
			t.Errorf("package %s has no non-test Go file", want)
		}
	}
}

// F-7 of the M1 review and the owner's decision: the scope engine may use
// net/netip and nothing else from the net family, no TLS, no randomness.
// Milestone M2 applies the same group to actor, operation and approval, and
// allows the time family (value types only, see domain_rules_test.go).
func TestScopePackageUsesOnlyNetNetip(t *testing.T) {
	const m = module + "/"
	c := fakeList()
	forbidden := []string{
		"net", "net/http", "net/http/httputil", "net/smtp", "net/rpc", "net/rpc/jsonrpc", "net/url", "net/textproto", "net/mail",
		"crypto/tls", "crypto/rand", "math/rand", "math/rand/v2", "os", "os/exec", "os/signal", "io/fs", "path/filepath", "log", "syscall", "plugin",
	}
	for _, rel := range []string{"internal/domain/scope", "internal/domain/scope/sub", "internal/domain/scope/deep/er", "internal/domain/actor", "internal/domain/operation", "internal/domain/approval"} {
		for _, imp := range forbidden {
			if got := c.violations(rel, m+rel, []string{imp}); len(got) == 0 {
				t.Errorf("%s importing %s was not refused", rel, imp)
			}
		}
		for _, imp := range []string{"net/netip", "errors", "fmt", "sort", "strconv", "strings", "unicode/utf8", "slices", "math", "crypto/sha256", "time", "encoding/binary"} {
			if got := c.violations(rel, m+rel, []string{imp}); len(got) != 0 {
				t.Errorf("%s importing %s was refused: %v", rel, imp, got)
			}
		}
	}
	// math/rand is out of the production code only. Seeded tests may use math/rand/v2.
	for _, imp := range []string{"math/rand", "math/rand/v2"} {
		if got := c.violations("internal/domain/scope", m+"internal/domain/scope", []string{imp}); len(got) == 0 {
			t.Errorf("production import of %s was not refused", imp)
		}
		if got := c.testViolations("internal/domain/scope", m+"internal/domain/scope", []string{imp}); len(got) != 0 {
			t.Errorf("test import of %s was refused: %v", imp, got)
		}
	}
	// Everything else stays refused in tests.
	for _, imp := range []string{"net", "net/smtp", "crypto/rand", "crypto/tls", "os"} {
		if got := c.testViolations("internal/domain/scope", m+"internal/domain/scope", []string{imp}); len(got) == 0 {
			t.Errorf("test import of %s was not refused", imp)
		}
	}
	// The same family rule applies to every domain package, with the same exception.
	if got := c.violations("internal/domain/job", m+"internal/domain/job", []string{"net/smtp"}); len(got) == 0 {
		t.Error("job importing net/smtp was not refused")
	}
	if got := c.violations("internal/domain/job", m+"internal/domain/job", []string{"net/netip"}); len(got) != 0 {
		t.Errorf("job importing net/netip was refused: %v", got)
	}
}

// The indirect route: scope imports an allowed domain package that imports the
// network. The direct rules cannot see it, so the transitive check must.
func TestTransitiveCheckFindsIndirectNetworkAndRandomness(t *testing.T) {
	const m = module + "/"
	graph := func(edges map[string][]string) classifier {
		c := fakeList()
		for path, imports := range edges {
			info := c[path]
			info.Imports = imports
			c[path] = info
		}
		return c
	}
	target, approval, operation := m+"internal/domain/target", m+"internal/domain/approval", m+"internal/domain/operation"
	cases := []struct {
		name  string
		edges map[string][]string
		start []string
		want  string // substring of the violation, empty for none
	}{
		{"direct network import of an allowed package", map[string][]string{target: {"net/smtp"}}, []string{target}, "net/smtp"},
		{"two steps down", map[string][]string{target: {approval}, approval: {"crypto/tls"}}, []string{target}, "target -> " + approval + " -> crypto/tls"},
		{"three steps down", map[string][]string{target: {approval}, approval: {operation}, operation: {"math/rand"}}, []string{target}, "math/rand"},
		{"randomness", map[string][]string{target: {"crypto/rand"}}, []string{target}, "crypto/rand"},
		{"plugin", map[string][]string{target: {"plugin"}}, []string{target}, "plugin"},
		{"process execution", map[string][]string{target: {"os/exec"}}, []string{target}, "os/exec"},
		{"net/http is part of the net family", map[string][]string{target: {"net/http"}}, []string{target}, "net/http"},
		{"net/netip is the exception", map[string][]string{target: {"net/netip"}}, []string{target}, ""},
		{"a cycle terminates", map[string][]string{target: {approval}, approval: {target, "strings"}}, []string{target}, ""},
		{"nothing banned", map[string][]string{target: {"strings", "fmt"}}, []string{target}, ""},
		{"a start that is itself banned", map[string][]string{}, []string{"net/smtp"}, "net/smtp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(graph(tc.edges).transitiveViolations("internal/domain/scope", tc.start), "; ")
			if tc.want == "" && got != "" {
				t.Errorf("unexpected violations: %s", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("violations = %q, want one containing %q", got, tc.want)
			}
		})
	}
}

// The real import graph of every domain package, through the whole standard
// library, reaches none of the banned packages.
func TestDomainPackagesReachNoNetworkOrRandomness(t *testing.T) {
	for _, pf := range platforms {
		t.Run(pf.goos+"_"+pf.goarch, func(t *testing.T) {
			own, c := listModule(t, pf.goos, pf.goarch)
			checked := 0
			for _, p := range own {
				rel := strings.TrimPrefix(p.ImportPath, module+"/")
				if !under(rel, "internal/domain") {
					continue
				}
				checked++
				for _, v := range c.transitiveViolations(rel, p.Imports) {
					t.Error(v)
				}
			}
			if checked == 0 {
				t.Error("no domain package was checked")
			}
		})
	}
}
