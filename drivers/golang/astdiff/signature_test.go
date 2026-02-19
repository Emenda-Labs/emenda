package astdiff

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestRenderTypeExpr(t *testing.T) {
	tests := []struct {
		name string
		src  string // type T = <expr>
		want string
	}{
		{"ident", "type T = int", "int"},
		{"selector", "type T = context.Context", "context.Context"},
		{"pointer", "type T = *int", "*int"},
		{"slice", "type T = []string", "[]string"},
		{"array", "type T = [3]byte", "[3]byte"},
		{"map", "type T = map[string]int", "map[string]int"},
		{"chan_bidir", "type T = chan int", "chan int"},
		{"chan_recv", "type T = <-chan int", "<-chan int"},
		{"chan_send", "type T = chan<- int", "chan<- int"},
		{"empty_interface", "type T = interface{}", "interface{}"},
		{"func_type", "type T = func(int) error", "func(int) error"},
		{"nested_pointer_slice", "type T = *[]int", "*[]int"},
		{"map_of_slices", "type T = map[string][]int", "map[string][]int"},
		{"paren", "type T = (int)", "(int)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			src := "package p\nimport \"context\"\nvar _ context.Context\n" + tt.src
			file, err := parser.ParseFile(fset, "", src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					if typeSpec.Name.Name != "T" {
						continue
					}
					got := renderTypeExpr(fset, typeSpec.Type)
					if got != tt.want {
						t.Errorf("renderTypeExpr = %q, want %q", got, tt.want)
					}
					return
				}
			}
			t.Error("type T not found in parsed source")
		})
	}
}

func TestRenderTypeExpr_Ellipsis(t *testing.T) {
	fset := token.NewFileSet()
	src := "package p\nfunc F(args ...string) {}"
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	funcDecl := file.Decls[0].(*ast.FuncDecl)
	lastParam := funcDecl.Type.Params.List[0]
	got := renderTypeExpr(fset, lastParam.Type)
	if got != "...string" {
		t.Errorf("ellipsis render = %q, want %q", got, "...string")
	}
}

func TestExtractFuncSignature(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantParams []string
		wantResult []string
	}{
		{
			name:       "simple",
			src:        "package p\nfunc F(a int, b string) error { return nil }",
			wantParams: []string{"int", "string"},
			wantResult: []string{"error"},
		},
		{
			name:       "no_params_no_results",
			src:        "package p\nfunc F() {}",
			wantParams: nil,
			wantResult: nil,
		},
		{
			name:       "variadic",
			src:        "package p\nfunc F(a int, rest ...string) {}",
			wantParams: []string{"int", "...string"},
			wantResult: nil,
		},
		{
			name:       "multi_return",
			src:        "package p\nfunc F() (int, error) { return 0, nil }",
			wantParams: nil,
			wantResult: []string{"int", "error"},
		},
		{
			name:       "shared_type_params",
			src:        "package p\nfunc F(a, b int) {}",
			wantParams: []string{"int", "int"},
			wantResult: nil,
		},
		{
			name:       "named_results",
			src:        "package p\nfunc F() (n int, err error) { return 0, nil }",
			wantParams: nil,
			wantResult: []string{"int", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			funcDecl := file.Decls[0].(*ast.FuncDecl)
			sig := extractFuncSignature(fset, funcDecl.Type)

			if !slicesEqual(sig.params, tt.wantParams) {
				t.Errorf("params = %v, want %v", sig.params, tt.wantParams)
			}
			if !slicesEqual(sig.results, tt.wantResult) {
				t.Errorf("results = %v, want %v", sig.results, tt.wantResult)
			}
		})
	}
}

func TestRenderFuncSignature(t *testing.T) {
	tests := []struct {
		name string
		sig  funcSignature
		want string
	}{
		{"no_results", funcSignature{params: []string{"int"}}, "(int)"},
		{"one_result", funcSignature{params: []string{"int"}, results: []string{"error"}}, "(int) error"},
		{"multi_results", funcSignature{params: []string{"int"}, results: []string{"string", "error"}}, "(int) (string, error)"},
		{"empty", funcSignature{}, "()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderFuncSignature(tt.sig)
			if got != tt.want {
				t.Errorf("renderFuncSignature = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractTypeSignature_Struct(t *testing.T) {
	src := `package p

type Config struct {
	Host    string
	Port    int
	secret  bool
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	genDecl := file.Decls[0].(*ast.GenDecl)
	typeSpec := genDecl.Specs[0].(*ast.TypeSpec)
	got := extractTypeSignature(fset, typeSpec)

	// Only exported fields should appear.
	want := "struct{Host string; Port int}"
	if got != want {
		t.Errorf("struct sig = %q, want %q", got, want)
	}
}

func TestExtractTypeSignature_Interface(t *testing.T) {
	src := `package p

import "context"

var _ context.Context

type Handler interface {
	Close() error
	Handle(ctx context.Context, req string) (string, error)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		typeSpec := genDecl.Specs[0].(*ast.TypeSpec)
		got := extractTypeSignature(fset, typeSpec)

		// Methods should be sorted alphabetically.
		want := "interface{Close() error; Handle(context.Context, string) (string, error)}"
		if got != want {
			t.Errorf("interface sig = %q, want %q", got, want)
		}
		return
	}
	t.Error("Handler type not found")
}

func TestExtractTypeSignature_Alias(t *testing.T) {
	src := `package p

type MyInt = int
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	genDecl := file.Decls[0].(*ast.GenDecl)
	typeSpec := genDecl.Specs[0].(*ast.TypeSpec)
	got := extractTypeSignature(fset, typeSpec)

	want := "= int"
	if got != want {
		t.Errorf("alias sig = %q, want %q", got, want)
	}
}

func TestExtractTypeSignature_Simple(t *testing.T) {
	src := `package p

type Token string
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	genDecl := file.Decls[0].(*ast.GenDecl)
	typeSpec := genDecl.Specs[0].(*ast.TypeSpec)
	got := extractTypeSignature(fset, typeSpec)

	if got != "string" {
		t.Errorf("sig = %q, want %q", got, "string")
	}
}

func TestExtractConstVarType(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "typed_const",
			src:  "package p\nconst MaxRetries int = 3",
			want: "int",
		},
		{
			name: "untyped_const",
			src:  `package p; const Name = "hello"`,
			want: "",
		},
		{
			name: "typed_var",
			src:  "package p\nvar ErrNotFound error",
			want: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.src, 0)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			genDecl := file.Decls[0].(*ast.GenDecl)
			valSpec := genDecl.Specs[0].(*ast.ValueSpec)
			got := extractConstVarType(fset, valSpec)
			if got != tt.want {
				t.Errorf("extractConstVarType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderStructSignature_EmptyStruct(t *testing.T) {
	src := "package p\ntype Empty struct{}"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	genDecl := file.Decls[0].(*ast.GenDecl)
	typeSpec := genDecl.Specs[0].(*ast.TypeSpec)
	got := extractTypeSignature(fset, typeSpec)
	if got != "struct{}" {
		t.Errorf("empty struct sig = %q, want %q", got, "struct{}")
	}
}

func TestRenderStructSignature_OnlyUnexported(t *testing.T) {
	src := "package p\ntype S struct{ x int; y string }"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	genDecl := file.Decls[0].(*ast.GenDecl)
	typeSpec := genDecl.Specs[0].(*ast.TypeSpec)
	got := extractTypeSignature(fset, typeSpec)
	if got != "struct{}" {
		t.Errorf("unexported-only struct sig = %q, want %q", got, "struct{}")
	}
}

// slicesEqual compares two string slices, treating nil and empty as equivalent.
func slicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
