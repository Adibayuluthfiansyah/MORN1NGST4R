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
// Not covered yet: the domain dependency order table of structure.md 5.6,
// because no domain package exists. Add it with the first domain package.
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
}

var rules = []rule{
	{prefix: "internal/domain", forbidStd: std("net/http", "database/sql")},
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
	Main     bool // belongs to the main module
}

// classifier maps an import path to go list's classification of it.
type classifier map[string]pkgInfo

// violations returns every rule that the package (module-relative path and
// import path) breaks by importing imports.
func (c classifier) violations(rel, importPath string, imports []string) []string {
	if under(rel, "tests") {
		return nil // tests may reach any package
	}
	r := ruleFor(rel)
	if r == nil {
		return []string{fmt.Sprintf("%s matches no architecture rule. Add one to deps_test.go", rel)}
	}
	var out []string
	for _, imp := range imports {
		if imp == importPath {
			continue // an external test package imports the package under test
		}
		info, known := c[imp]
		switch {
		case !known:
			out = append(out, fmt.Sprintf("%s imports %s, which go list does not report (cgo or an unresolved import)", rel, imp))
		case info.Standard:
			for _, f := range r.forbidStd {
				if under(imp, f) {
					out = append(out, fmt.Sprintf("%s must not import %s (forbidden family %s)", rel, imp, f))
				}
			}
		case info.Main:
			target := strings.TrimPrefix(imp, module+"/")
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
		"net/httpx", // not a real package: proves a family match needs a path boundary
		"database/sql", "database/sql/driver", "os", "os/exec", "os/signal", "syscall", "plugin", "unsafe", "time",
	} {
		c[p] = pkgInfo{Standard: true}
	}
	for _, p := range []string{
		"internal/domain/scope", "internal/application/health", "internal/application/other", "internal/ports/inbound",
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
		{"domain imports a nested net/http package", "internal/domain/scope", []string{"net/http/httputil"}, "family net/http"},
		{"domain imports net/http/cgi", "internal/domain/scope", []string{"net/http/cgi"}, "family net/http"},
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
		{"a package whose name only looks like a forbidden family", "internal/domain/scope", []string{"net/httpx"}},
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
		info := pkgInfo{Standard: p.Standard}
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
				imports := append(append(append([]string{}, p.Imports...), p.TestImports...), p.XTestImports...)
				all = append(all, c.violations(rel, p.ImportPath, imports)...)
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
