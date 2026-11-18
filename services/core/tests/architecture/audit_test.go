package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
)

// M5 and TD-M4-03: one path writes audit events. The statement that inserts into
// audit_events or system_alerts lives in persistence/audit.go, in the sqlc query
// file of the audit queries, and nowhere else. The port of M4 (AuditLog and
// AuditEvent) does not exist any more.
const auditAdapter = "internal/adapters/outbound/persistence/audit.go"

// auditInsert finds a statement that writes the two tables, however the name is
// spelled: quoted, schema qualified, behind a comment, or after COPY or MERGE. It
// is a guard against accidents. The real boundary is the grants of the roles
// (M3), and a statement that is assembled at run time is not found.
var auditInsert = regexp.MustCompile(`(?is)\b(insert|copy|merge)\b[^;]{0,80}?\b(audit_events|system_alerts)\b`)

// sinkOnly are the generated query names that only the audit adapter may call.
var sinkOnly = map[string]bool{"InsertAuditEvent": true, "RaiseAuditRetryAlert": true}

// auditPathViolations inspects one production Go file. rel is the path relative
// to services/core.
func auditPathViolations(rel, src string) []string {
	var out []string
	if rel != auditAdapter && !strings.HasPrefix(rel, "internal/adapters/outbound/persistence/sqlcgen/") && auditInsert.MatchString(src) {
		out = append(out, rel+": writes audit_events or system_alerts outside the audit adapter")
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, 0)
	if err != nil {
		return append(out, rel+": "+err.Error())
	}
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == "AuditLog" || id.Name == "AuditEvent" {
			out = append(out, fset.Position(id.Pos()).String()+": uses the removed audit port "+id.Name)
		}
		if sinkOnly[id.Name] && rel != auditAdapter && !strings.HasPrefix(rel, "internal/adapters/outbound/persistence/sqlcgen/") {
			out = append(out, fset.Position(id.Pos()).String()+": calls "+id.Name+" outside the audit adapter")
		}
		return true
	})
	return out
}

func TestEveryAuditWriteGoesThroughTheSinkAdapter(t *testing.T) {
	var problems []string
	root := ".." + string(filepath.Separator) + ".."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "tests" || rel == "bin" || rel == "migrations" {
				return filepath.SkipDir
			}
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch {
		case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
			problems = append(problems, auditPathViolations(rel, string(src))...)
		case strings.HasPrefix(rel, "queries/") && strings.HasSuffix(path, ".sql") && rel != "queries/audit.sql":
			if auditInsert.Match(src) {
				problems = append(problems, rel+": writes audit_events or system_alerts outside queries/audit.sql")
			}
		}
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

func TestTheAuditPathGateDetectsViolations(t *testing.T) {
	bad := map[string]struct{ rel, src string }{
		"raw insert in the recovery repository":  {"internal/adapters/outbound/persistence/recovery.go", "package persistence\nconst q = `INSERT INTO morningstar.audit_events (id) VALUES ($1)`"},
		"raw alert insert in an application":     {"internal/application/job/x.go", "package job\nconst q = \"insert into system_alerts (id) values ($1)\""},
		"multi line insert":                      {"internal/adapters/outbound/persistence/auth.go", "package persistence\nconst q = `INSERT INTO\n\tmorningstar.audit_events (id)`"},
		"the removed port":                       {"internal/ports/outbound/x.go", "package outbound\ntype AuditLog interface{}"},
		"the removed event":                      {"internal/application/auth/x.go", "package auth\nvar e outbound.AuditEvent"},
		"a quoted name":                          {"internal/adapters/outbound/persistence/x.go", "package persistence\nconst q = `INSERT INTO \"morningstar\".\"audit_events\" (id) VALUES ($1)`"},
		"a name behind a comment":                {"internal/adapters/outbound/persistence/x.go", "package persistence\nconst q = `INSERT /* x */ INTO morningstar.audit_events (id) VALUES ($1)`"},
		"spaces around the dot":                  {"internal/adapters/outbound/persistence/x.go", "package persistence\nconst q = `INSERT INTO morningstar . audit_events (id) VALUES ($1)`"},
		"copy":                                   {"internal/adapters/outbound/persistence/x.go", "package persistence\nconst q = `COPY morningstar.audit_events FROM STDIN`"},
		"merge":                                  {"internal/adapters/outbound/persistence/x.go", "package persistence\nconst q = `MERGE INTO audit_events USING t ON true WHEN NOT MATCHED THEN INSERT VALUES (1)`"},
		"the generated insert from another file": {"internal/adapters/outbound/persistence/auth.go", "package persistence\nfunc f(q *sqlcgen.Queries) { _ = q.InsertAuditEvent }"},
		"the generated alert from another file":  {"internal/application/auth/x.go", "package auth\nfunc f(q *sqlcgen.Queries) { _, _ = q.RaiseAuditRetryAlert }"},
	}
	for name, c := range bad {
		if v := auditPathViolations(c.rel, c.src); len(v) == 0 {
			t.Errorf("%s was not detected", name)
		}
	}
	good := map[string]struct{ rel, src string }{
		"the adapter":    {auditAdapter, "package persistence\nconst q = `INSERT INTO morningstar.system_alerts (id) VALUES ($1)`"},
		"generated sqlc": {"internal/adapters/outbound/persistence/sqlcgen/audit.sql.go", "package sqlcgen\nconst q = `INSERT INTO morningstar.audit_events (id) VALUES ($1)`"},
		"a read":         {"internal/adapters/outbound/persistence/recovery.go", "package persistence\nconst q = `SELECT max(seq) FROM morningstar.audit_events`"},
		"the sink port":  {"internal/ports/outbound/audit.go", "package outbound\ntype AuditSink interface{}"},
	}
	for name, c := range good {
		if v := auditPathViolations(c.rel, c.src); len(v) != 0 {
			t.Errorf("%s was refused: %v", name, v)
		}
	}
}

// The catalog in code and the catalog of the design document are one list. A
// new action is added to both or the test fails.
func TestAuditCatalogMatchesTheDesignDocument(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "design", "database", "domain-model.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	a := strings.Index(doc, "## 5. Audit event catalog")
	b := strings.Index(doc, "## 6. Requirement to data mapping")
	if a < 0 || b < a {
		t.Fatal("the catalog section of domain-model.md was not found")
	}
	inDoc := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z_]+\\.[a-z_]+)` \\|").FindAllStringSubmatch(doc[a:b], -1) {
		inDoc[m[1]] = true
	}
	inCode := map[string]bool{}
	for _, x := range audit.Actions() {
		inCode[string(x)] = true
	}
	for x := range inDoc {
		if !inCode[x] {
			t.Errorf("%s is in the document and not in the code", x)
		}
	}
	for x := range inCode {
		if !inDoc[x] {
			t.Errorf("%s is in the code and not in the document", x)
		}
	}
}
