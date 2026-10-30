package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The bindings generator types every bound method that returns a pointer as
// nullable; scripts/bindings/types.mjs narrows all but the methods that can
// answer a nil pointer with no error (a No, nothing to report), which keep
// the nullable type so their callers handle it. This keeps that list the
// methods whose body says so.
func TestNullableBindingsAreListed(t *testing.T) {
	want := nilWithoutError(t)
	src, err := os.ReadFile(filepath.Join("..", "..", "scripts", "bindings", "types.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)NULLABLE = new Set\(\[(.*?)\]\)`).FindSubmatch(src)
	if block == nil {
		t.Fatal("scripts/bindings/types.mjs has no NULLABLE set")
	}
	var got []string
	for _, m := range regexp.MustCompile(`"(\w+\.\w+)"`).FindAllSubmatch(block[1], -1) {
		got = append(got, string(m[1]))
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("NULLABLE in scripts/bindings/types.mjs is\n  %v\nbut the bound methods that return a nil pointer with no error are\n  %v", got, want)
	}
}

// nilWithoutError lists "<type>.<Method>" (the type lower-cased, as the
// binding file is named) for each exported method of a bound service whose
// first result is a pointer and which has a return that gives nil for it and
// nil for the error.
func nilWithoutError(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || !fd.Name.IsExported() || fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
				continue
			}
			recv, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			name, ok := recv.X.(*ast.Ident)
			if !ok || !strings.HasSuffix(name.Name, "API") {
				continue
			}
			if _, ptr := fd.Type.Results.List[0].Type.(*ast.StarExpr); !ptr {
				continue
			}
			single := len(fd.Type.Results.List) == 1 && len(fd.Type.Results.List[0].Names) <= 1
			found := false
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if _, lit := n.(*ast.FuncLit); lit {
					return false
				}
				if rs, ok := n.(*ast.ReturnStmt); ok && len(rs.Results) > 0 && isNilIdent(rs.Results[0]) &&
					(single || (len(rs.Results) == 2 && isNilIdent(rs.Results[1]))) {
					found = true
				}
				return true
			})
			if found {
				out = append(out, strings.ToLower(name.Name)+"."+fd.Name.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}
