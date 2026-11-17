package architecture_test

// Source scans for the domain packages (milestone M2, Task 2.1).
//
// The import rules in deps_test.go cannot see two things that the domain needs
// to stay pure and that P-04 requires:
//
//   - the clock. Domain code may use the types time.Time and time.Duration, so
//     importing "time" is allowed, but it must never read the clock. Time comes
//     in as an argument, which keeps every decision reproducible.
//   - the caller of the decision function. scope.Decide is the only producer of
//     an AuthorizedOperation. It may be called only from internal/application/scope
//     (architecture.md section 8, P-04).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// clockFunctions are the names in package time that read the clock, wait on it,
// or read the machine's time zone from the environment or the file system.
// time.UTC is a constant zone and stays allowed. Package context is closed by an
// import rule, because context.WithTimeout and WithDeadline read the clock too.
var clockFunctions = []string{
	"Now", "Since", "Until", "After", "AfterFunc", "Tick", "NewTimer", "NewTicker", "Sleep",
	"Local", "LoadLocation", "LoadLocationFromTZData",
}

// importLocalNames returns the local names under which a file imports path. A
// dot import is reported as ".", because it hides the package name.
func importLocalNames(file *ast.File, path string) (names []string, dot bool) {
	for _, spec := range file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil || p != path {
			continue
		}
		switch {
		case spec.Name == nil:
			names = append(names, path[strings.LastIndex(path, "/")+1:])
		case spec.Name.Name == ".":
			dot = true
		case spec.Name.Name != "_":
			names = append(names, spec.Name.Name)
		}
	}
	return names, dot
}

// selectorUses reports every use of pkgName.fn for one of the functions.
func selectorUses(file *ast.File, localNames []string, fns []string) []string {
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		for _, ln := range localNames {
			if id.Name != ln {
				continue
			}
			for _, fn := range fns {
				if sel.Sel.Name == fn {
					out = append(out, sel.Sel.Name)
				}
			}
		}
		return true
	})
	return out
}

// clockUses reports reads of the clock in one Go file. A dot import of time is
// refused because it would hide the calls.
func clockUses(filename string, src any) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return nil, err
	}
	names, dot := importLocalNames(file, "time")
	var out []string
	if dot {
		out = append(out, fmt.Sprintf("%s: dot import of time hides clock reads", filename))
	}
	for _, fn := range selectorUses(file, names, clockFunctions) {
		out = append(out, fmt.Sprintf("%s: time.%s reads the clock or the machine's time zone. Domain code takes time as an argument", filename, fn))
	}
	return out, nil
}

// decideUses reports references to scope.Decide in one Go file.
func decideUses(filename string, src any) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return nil, err
	}
	names, dot := importLocalNames(file, module+"/internal/domain/scope")
	var out []string
	if dot {
		out = append(out, fmt.Sprintf("%s: dot import of the scope package hides calls to Decide", filename))
	}
	for range selectorUses(file, names, []string{"Decide"}) {
		out = append(out, fmt.Sprintf("%s: references scope.Decide", filename))
	}
	return out, nil
}

// decideAllowedIn lists the packages that may reference scope.Decide in
// production code (P-04).
var decideAllowedIn = []string{"internal/application/scope"}

func TestClockScanner(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"time.Now", `package p; import "time"; func f() time.Time { return time.Now() }`, 1},
		{"time.Since", `package p; import "time"; func f(t time.Time) time.Duration { return time.Since(t) }`, 1},
		{"time.Until", `package p; import "time"; func f(t time.Time) time.Duration { return time.Until(t) }`, 1},
		{"time.Sleep", `package p; import "time"; func f() { time.Sleep(1) }`, 1},
		{"time.After", `package p; import "time"; func f() { <-time.After(1) }`, 1},
		{"time.AfterFunc", `package p; import "time"; func f() { time.AfterFunc(1, func() {}) }`, 1},
		{"time.Tick", `package p; import "time"; func f() { _ = time.Tick(1) }`, 1},
		{"time.NewTimer", `package p; import "time"; func f() { _ = time.NewTimer(1) }`, 1},
		{"time.NewTicker", `package p; import "time"; func f() { _ = time.NewTicker(1) }`, 1},
		{"time.Local", `package p; import "time"; func f(t time.Time) time.Time { return t.In(time.Local) }`, 1},
		{"time.LoadLocation", `package p; import "time"; func f() { _, _ = time.LoadLocation("Europe/Berlin") }`, 1},
		{"time.LoadLocationFromTZData", `package p; import "time"; func f() { _, _ = time.LoadLocationFromTZData("x", nil) }`, 1},
		{"time.UTC is a constant zone", `package p; import "time"; func f(t time.Time) time.Time { return t.In(time.UTC) }`, 0},
		{"a function value", `package p; import "time"; var now = time.Now`, 1},
		{"a method value of time.Time", `package p; import "time"; func f(t time.Time) bool { g := t.Before; return g(t) }`, 0},
		{"aliased import", `package p; import tm "time"; func f() { _ = tm.Now() }`, 1},
		{"dot import", `package p; import . "time"; func f() { _ = Now() }`, 1},
		{"two reads", `package p; import "time"; func f() { _ = time.Now(); _ = time.Now() }`, 2},
		{"value types only", `package p; import "time"; type T struct{ At time.Time; D time.Duration }; func f(t T) bool { return t.At.Before(t.At.Add(t.D)) }`, 0},
		{"constants and parsing", `package p; import "time"; const D = 30 * time.Second; func f() (time.Time, error) { return time.Parse(time.RFC3339, "x") }`, 0},
		{"a local value named Now", `package p; type c struct{}; func (c) Now() int { return 0 }; func f(x c) int { return x.Now() }`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := clockUses("p.go", tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("clockUses = %v, want %d finding(s)", got, tc.want)
			}
		})
	}
	if _, err := clockUses("p.go", "package p; func ("); err == nil {
		t.Error("a file that does not parse must be an error, not a pass")
	}
}

func TestDecideScanner(t *testing.T) {
	const imp = `import "` + module + `/internal/domain/scope"`
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"a call", `package p; ` + imp + `; func f() { _ = scope.Decide(scope.DecisionInput{}) }`, 1},
		{"a function value", `package p; ` + imp + `; var f = scope.Decide`, 1},
		{"aliased import", `package p; import s "` + module + `/internal/domain/scope"; func f() { _ = s.Decide(s.DecisionInput{}) }`, 1},
		{"dot import", `package p; import . "` + module + `/internal/domain/scope"; func f() { _ = Decide(DecisionInput{}) }`, 1},
		{"other scope names", `package p; ` + imp + `; func f(rs scope.RuleSet) bool { return rs.Evaluate(scope.Target{}).Allowed }`, 0},
		{"a local value named Decide", `package p; type d struct{}; func (d) Decide() {}; func f(x d) { x.Decide() }`, 0},
		{"another package with a Decide", `package p; import "example.com/other"; func f() { other.Decide() }`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decideUses("p.go", tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("decideUses = %v, want %d finding(s)", got, tc.want)
			}
		})
	}
	if _, err := decideUses("p.go", "package p; func ("); err == nil {
		t.Error("a file that does not parse must be an error, not a pass")
	}
}

// TestDomainReadsNoClock applies the clock scan to the production files of
// every domain package.
func TestDomainReadsNoClock(t *testing.T) {
	own, _ := listModule(t, platforms[0].goos, platforms[0].goarch)
	scanned := 0
	for _, p := range own {
		rel := strings.TrimPrefix(p.ImportPath, module+"/")
		if !under(rel, "internal/domain") {
			continue
		}
		for _, f := range p.GoFiles {
			scanned++
			uses, err := clockUses(filepath.Join(p.Dir, f), nil)
			if err != nil {
				t.Errorf("%s: %v", filepath.Join(rel, f), err)
			}
			for _, u := range uses {
				t.Error(strings.Replace(u, p.Dir, rel, 1))
			}
		}
	}
	if scanned == 0 {
		t.Error("no domain source file was scanned")
	}
}

// TestDecideIsCalledOnlyFromApplicationScope is the check that P-04 promises:
// the decision function that returns an AuthorizedOperation is referenced from
// no production package except internal/application/scope. Go cannot restrict
// an exported function to one caller, so this test is where the rule is kept.
func TestDecideIsCalledOnlyFromApplicationScope(t *testing.T) {
	own, _ := listModule(t, platforms[0].goos, platforms[0].goarch)
	scanned := 0
	for _, p := range own {
		rel := strings.TrimPrefix(p.ImportPath, module+"/")
		if under(rel, "tests") || rel == "internal/domain/scope" {
			continue
		}
		allowed := false
		for _, a := range decideAllowedIn {
			allowed = allowed || under(rel, a)
		}
		for _, f := range p.GoFiles {
			scanned++
			uses, err := decideUses(filepath.Join(p.Dir, f), nil)
			if err != nil {
				t.Errorf("%s: %v", filepath.Join(rel, f), err)
			}
			if allowed {
				continue
			}
			for _, u := range uses {
				t.Error(strings.Replace(u, p.Dir, rel, 1) + " (P-04: only internal/application/scope may call it)")
			}
		}
	}
	if scanned == 0 {
		t.Error("no source file was scanned")
	}
}
