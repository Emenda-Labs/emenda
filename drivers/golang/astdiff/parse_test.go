package astdiff

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/emenda-labs/emenda/drivers/golang/symbols"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	return filepath.Join(filepath.Dir(file), "testdata")
}

func TestParseExports_OldFixture(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "old")
	syms, sigMap, err := ParseExports(dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	if syms.Module != "github.com/acme/testmod" {
		t.Errorf("module = %q, want %q", syms.Module, "github.com/acme/testmod")
	}

	byName := make(map[string]symbols.Symbol)
	for _, s := range syms.Entries {
		byName[s.Package+"."+s.Name] = s
	}

	// Verify expected symbols exist.
	expectedSymbols := []struct {
		key  string
		kind symbols.SymbolKind
	}{
		{"github.com/acme/testmod.DoWork", symbols.SymbolFunc},
		{"github.com/acme/testmod.SimpleFunc", symbols.SymbolFunc},
		{"github.com/acme/testmod.HelperFunc", symbols.SymbolFunc},
		{"github.com/acme/testmod.OldOnly", symbols.SymbolFunc},
		{"github.com/acme/testmod.Variadic", symbols.SymbolFunc},
		{"github.com/acme/testmod.Config", symbols.SymbolType},
		{"github.com/acme/testmod.Handler", symbols.SymbolInterface},
		{"github.com/acme/testmod.Token", symbols.SymbolType},
		{"github.com/acme/testmod.Config.Validate", symbols.SymbolMethod},
		{"github.com/acme/testmod.Config.Apply", symbols.SymbolMethod},
		{"github.com/acme/testmod.Config.Host", symbols.SymbolField},
		{"github.com/acme/testmod.Config.Port", symbols.SymbolField},
		{"github.com/acme/testmod.Config.Timeout", symbols.SymbolField},
		{"github.com/acme/testmod.MaxRetries", symbols.SymbolConst},
		{"github.com/acme/testmod.UntypedConst", symbols.SymbolConst},
		{"github.com/acme/testmod.ErrNotFound", symbols.SymbolVar},
		{"github.com/acme/testmod.DefaultConfig", symbols.SymbolVar},
		{"github.com/acme/testmod.ComputeHash", symbols.SymbolVar},
		{"github.com/acme/testmod/sub.SubFunc", symbols.SymbolFunc},
		{"github.com/acme/testmod/sub.SubType", symbols.SymbolType},
		{"github.com/acme/testmod/sub.SubType.Value", symbols.SymbolField},
	}

	for _, exp := range expectedSymbols {
		sym, ok := byName[exp.key]
		if !ok {
			t.Errorf("missing symbol %s", exp.key)
			continue
		}
		if sym.Kind != exp.kind {
			t.Errorf("symbol %s kind = %q, want %q", exp.key, sym.Kind, exp.kind)
		}
	}

	// Verify filtered-out symbols are absent.
	unwanted := []string{
		"github.com/acme/testmod.unexportedType",
		"github.com/acme/testmod.unexportedType.Hidden",
		"github.com/acme/testmod.Config.secret",
		"github.com/acme/testmod.BrokenFunc",
		"github.com/acme/testmod/internal.InternalFunc",
		"github.com/acme/testmod/_examples.ExampleFunc",
	}

	for _, key := range unwanted {
		if _, ok := byName[key]; ok {
			t.Errorf("unwanted symbol present: %s", key)
		}
	}

	// Verify FuncSigMap has entries for functions/methods.
	funcKeys := []symbolKey{
		{pkg: "github.com/acme/testmod", kind: symbols.SymbolFunc, name: "DoWork"},
		{pkg: "github.com/acme/testmod", kind: symbols.SymbolMethod, name: "Config.Validate"},
	}
	for _, key := range funcKeys {
		if _, ok := sigMap[key]; !ok {
			t.Errorf("FuncSigMap missing key %+v", key)
		}
	}
}

func TestParseExports_NewFixture(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "new")
	syms, _, err := ParseExports(dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	byName := make(map[string]symbols.Symbol)
	for _, s := range syms.Entries {
		byName[s.Package+"."+s.Name] = s
	}

	// Package main should be skipped.
	if _, ok := byName["main.MainFunc"]; ok {
		t.Error("package main symbol should be skipped")
	}

	// ComputeHash should be a func in new (was var in old).
	sym, ok := byName["github.com/acme/testmod.ComputeHash"]
	if !ok {
		t.Fatal("missing ComputeHash in new")
	}
	if sym.Kind != symbols.SymbolFunc {
		t.Errorf("ComputeHash kind = %q, want %q", sym.Kind, symbols.SymbolFunc)
	}

	// Config renamed to Settings.
	if _, ok := byName["github.com/acme/testmod.Config"]; ok {
		t.Error("Config should not exist in new (renamed to Settings)")
	}
	if _, ok := byName["github.com/acme/testmod.Settings"]; !ok {
		t.Error("Settings should exist in new")
	}
}

func TestParseExports_VariadicSignature(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "old")
	syms, _, err := ParseExports(dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	for _, s := range syms.Entries {
		if s.Name == "Variadic" {
			if s.Signature != "(...string) int" {
				t.Errorf("Variadic signature = %q, want %q", s.Signature, "(...string) int")
			}
			return
		}
	}
	t.Error("Variadic symbol not found")
}

func TestParseExports_MethodReceiver(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "old")
	syms, _, err := ParseExports(dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	for _, s := range syms.Entries {
		if s.Name == "Config.Validate" {
			if s.Receiver != "Config" {
				t.Errorf("receiver = %q, want %q", s.Receiver, "Config")
			}
			if s.Kind != symbols.SymbolMethod {
				t.Errorf("kind = %q, want %q", s.Kind, symbols.SymbolMethod)
			}
			return
		}
	}
	t.Error("Config.Validate method not found")
}

func TestParseExports_SubPackagePath(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "old")
	syms, _, err := ParseExports(dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	for _, s := range syms.Entries {
		if s.Name == "SubFunc" {
			if s.Package != "github.com/acme/testmod/sub" {
				t.Errorf("SubFunc package = %q, want %q", s.Package, "github.com/acme/testmod/sub")
			}
			return
		}
	}
	t.Error("SubFunc not found")
}

func TestParseExports_EmptyModule(t *testing.T) {
	dir := t.TempDir()
	// Write a minimal go.mod with no Go files.
	gomod := filepath.Join(dir, "go.mod")
	if err := writeFile(gomod, "module github.com/empty/mod\n\ngo 1.21\n"); err != nil {
		t.Fatal(err)
	}

	syms, sigMap, err := ParseExports(dir, "github.com/empty/mod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}
	if len(syms.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(syms.Entries))
	}
	if len(sigMap) != 0 {
		t.Errorf("expected empty sigMap, got %d entries", len(sigMap))
	}
}

func TestFindSourceRoot_DirectGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "go.mod"), "module test\n"); err != nil {
		t.Fatal(err)
	}
	root, err := FindSourceRoot(dir)
	if err != nil {
		t.Fatalf("FindSourceRoot: %v", err)
	}
	if root != dir {
		t.Errorf("root = %q, want %q", root, dir)
	}
}

func TestFindSourceRoot_NestedGoMod(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "module@v1.0.0")
	if err := mkdirAll(nested); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(nested, "go.mod"), "module test\n"); err != nil {
		t.Fatal(err)
	}
	root, err := FindSourceRoot(dir)
	if err != nil {
		t.Fatalf("FindSourceRoot: %v", err)
	}
	if root != nested {
		t.Errorf("root = %q, want %q", root, nested)
	}
}

func TestFindSourceRoot_NoGoMod(t *testing.T) {
	dir := t.TempDir()
	_, err := FindSourceRoot(dir)
	if err == nil {
		t.Error("expected error for missing go.mod")
	}
}

func TestComputePackagePath(t *testing.T) {
	tests := []struct {
		sourceRoot string
		filePath   string
		module     string
		want       string
	}{
		{"/src", "/src/foo.go", "github.com/acme/mod", "github.com/acme/mod"},
		{"/src", "/src/sub/bar.go", "github.com/acme/mod", "github.com/acme/mod/sub"},
		{"/src", "/src/a/b/c.go", "github.com/acme/mod", "github.com/acme/mod/a/b"},
	}
	for _, tt := range tests {
		got := computePackagePath(tt.sourceRoot, tt.filePath, tt.module)
		if got != tt.want {
			t.Errorf("computePackagePath(%q, %q, %q) = %q, want %q",
				tt.sourceRoot, tt.filePath, tt.module, got, tt.want)
		}
	}
}

func TestReceiverTypeName(t *testing.T) {
	// Tested implicitly through ParseExports, but verify the direct function.
	// Since receiverTypeName takes *ast.FieldList, we test through ParseExports.
	dir := filepath.Join(testdataDir(t), "old")
	syms, _, err := ParseExports(dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	methods := make(map[string]string) // name -> receiver
	for _, s := range syms.Entries {
		if s.Kind == symbols.SymbolMethod {
			methods[s.Name] = s.Receiver
		}
	}

	wantMethods := map[string]string{
		"Config.Validate": "Config",
		"Config.Apply":    "Config",
	}
	for name, wantRecv := range wantMethods {
		gotRecv, ok := methods[name]
		if !ok {
			t.Errorf("method %s not found", name)
			continue
		}
		if gotRecv != wantRecv {
			t.Errorf("method %s receiver = %q, want %q", name, gotRecv, wantRecv)
		}
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}
