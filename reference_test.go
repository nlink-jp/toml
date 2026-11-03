package toml

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The reference manual describes every exported name. A name added to the
// code without a section there is caught here, in both languages.
func TestTheReferenceNamesEveryExportedName(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := goparser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var want []string // as the reference writes them in its headings
	for _, f := range pkgs["toml"].Files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv == nil {
					want = append(want, "`"+d.Name.Name+"(")
					continue
				}
				recv, star := d.Recv.List[0].Type, ""
				if s, ok := recv.(*ast.StarExpr); ok {
					recv, star = s.X, "*"
				}
				name := recv.(*ast.Ident)
				if !name.IsExported() {
					continue // a method of an internal type is not part of the API
				}
				want = append(want, "`("+star+name.Name+") "+d.Name.Name+"(")
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
						want = append(want, "`type "+ts.Name.Name+"`")
					}
				}
			}
		}
	}
	if len(want) < 20 {
		t.Fatalf("found only %d exported names; the parse is wrong", len(want))
	}
	for _, doc := range []string{"docs/en/reference.md", "docs/ja/reference.ja.md"} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s has no section for %s", doc, w)
			}
		}
	}
}
