package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TD-M2-01: an actor of kind human is a proof that a person is present. Only the
// authentication use case builds one, from a live session. The rules below cover
// the three ways to get one: the constant KindHuman, a conversion or ParseKind
// that yields it from data, and a second implementation of the Principal that
// hands an actor to a use case.
const actorImport = "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"

var mayBuildHumanActors = []string{"internal/domain/actor", "internal/application/auth"}

func mayBuildHuman(rel string) bool {
	for _, p := range mayBuildHumanActors {
		if under(rel, p) {
			return true
		}
	}
	return false
}

func actorAlias(f *ast.File) string {
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != actorImport {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "actor"
	}
	return ""
}

func isSel(e ast.Expr, pkg, name string) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := s.X.(*ast.Ident)
	return ok && x.Name == pkg && s.Sel.Name == name
}

// humanActorViolations inspects one production source file. rel is the
// module-relative directory of its package.
func humanActorViolations(rel, name, src string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return []string{name + ": " + err.Error()}
	}
	if mayBuildHuman(rel) {
		return nil
	}
	var out []string
	report := func(n ast.Node, what string) {
		out = append(out, fset.Position(n.Pos()).String()+": "+what)
	}
	alias := actorAlias(f)
	if alias == "." {
		report(f, "dot-imports the actor package, which hides references to KindHuman")
	}
	called := map[ast.Expr]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if alias == "" {
				return true
			}
			if isSel(x, alias, "KindHuman") {
				report(x, "references actor.KindHuman")
			}
			if (isSel(x, alias, "New") || isSel(x, alias, "ParseKind")) && !called[x] {
				report(x, "uses actor.New or actor.ParseKind as a value, which hides the kind that is passed")
			}
		case *ast.CallExpr:
			called[x.Fun] = true
			if alias == "" {
				return true
			}
			switch {
			case isSel(x.Fun, alias, "ParseKind"):
				report(x, "calls actor.ParseKind, which can return KindHuman")
			case isSel(x.Fun, alias, "Kind"):
				report(x, "converts a value to actor.Kind, which can be KindHuman")
			case isSel(x.Fun, alias, "New"):
				if len(x.Args) == 0 || !(isSel(x.Args[0], alias, "KindSystem") || isSel(x.Args[0], alias, "KindAI")) {
					report(x, "calls actor.New with a kind that is not the constant KindSystem or KindAI")
				}
			}
		case *ast.FuncDecl:
			if x.Recv != nil && x.Name.Name == "Actor" && x.Type.Results != nil && len(x.Type.Results.List) == 1 &&
				alias != "" && isSel(x.Type.Results.List[0].Type, alias, "Actor") {
				report(x, "declares an Actor() actor.Actor method, which would be a second Principal")
			}
		}
		return true
	})
	return out
}

func TestHumanActorIsBuiltOnlyByTheAuthenticationUseCase(t *testing.T) {
	var problems []string
	root := ".." + string(filepath.Separator) + ".."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "tests" || rel == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		problems = append(problems, humanActorViolations(filepath.ToSlash(filepath.Dir(rel)), rel, string(src))...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

// The gate has its own regression tests: each of these sources must be refused,
// and the allowed shapes must pass.
func TestHumanActorGateDetectsViolations(t *testing.T) {
	const header = "package x\nimport \"" + actorImport + "\"\n"
	bad := map[string]string{
		"constant":          header + "var k = actor.KindHuman",
		"new human":         header + "func f() { _, _ = actor.New(actor.KindHuman, \"u\") }",
		"new from variable": header + "func f(k actor.Kind) { _, _ = actor.New(k, \"u\") }",
		"parse kind":        header + "func f() { _, _ = actor.ParseKind(\"human\") }",
		"conversion":        header + "func f() { _ = actor.Kind(1) }",
		"aliased import":    "package x\nimport a \"" + actorImport + "\"\nvar k = a.KindHuman",
		"second principal":  header + "type p struct{}\nfunc (p) Actor() actor.Actor { return actor.Actor{} }",
		"new without args":  header + "func f() { _, _ = actor.New() }",
		"dot import":        "package x\nimport . \"" + actorImport + "\"\nvar k = KindHuman",
		"new as value":      header + "func f() { n := actor.New; _, _ = n(actor.KindSystem-1, \"u\") }",
		"parse as value":    header + "func f() { p := actor.ParseKind; _, _ = p(\"human\") }",
	}
	for name, src := range bad {
		if v := humanActorViolations("internal/application/job", "x.go", src); len(v) == 0 {
			t.Errorf("%s was not detected", name)
		}
	}
	good := map[string]string{
		"system actor": header + "func f() { _, _ = actor.New(actor.KindSystem, \"w\") }",
		"ai actor":     header + "func f() { _, _ = actor.New(actor.KindAI, \"a\") }",
		"type use":     header + "func f(a actor.Actor) bool { return a.IsHuman() }",
	}
	for name, src := range good {
		if v := humanActorViolations("internal/application/job", "x.go", src); len(v) != 0 {
			t.Errorf("%s was refused: %v", name, v)
		}
	}
	for _, rel := range []string{"internal/application/auth", "internal/domain/actor"} {
		if v := humanActorViolations(rel, "x.go", header+"var k = actor.KindHuman"); len(v) != 0 {
			t.Errorf("%s may build a human actor but was refused: %v", rel, v)
		}
	}
	for _, rel := range []string{"internal/application/authx", "internal/adapters/inbound/http/handlers", "internal/application", "cmd/server"} {
		if v := humanActorViolations(rel, "x.go", header+"var k = actor.KindHuman"); len(v) == 0 {
			t.Errorf("%s must not build a human actor", rel)
		}
	}
}
