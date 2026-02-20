package astdiff

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/emenda-labs/emenda/drivers/golang/symbols"
)

// symbolKey uniquely identifies a symbol for index lookups in DiffExports.
type symbolKey struct {
	pkg  string
	kind symbols.SymbolKind
	name string // for methods: "Receiver.Method"
}

// FuncSigMap caches structured function signatures keyed by symbolKey.
// Built during ParseExports, consumed by DiffExports Pass 5 for param overlap.
type FuncSigMap map[symbolKey]funcSignature

// ParseExports walks the Go module source at rootDir and collects all exported symbols.
// The module parameter is the Go module import path (e.g. "github.com/acme/foo").
// Returns the symbol set and a cached map of structured function signatures for
// use in DiffExports fuzzy matching.
func ParseExports(ctx context.Context, rootDir, module string) (symbols.Symbols, FuncSigMap, error) {
	sourceRoot, err := FindSourceRoot(rootDir)
	if err != nil {
		return symbols.Symbols{}, nil, fmt.Errorf("finding source root in %s: %w", rootDir, err)
	}

	fset := token.NewFileSet()
	var entries []symbols.Symbol
	sigMap := make(FuncSigMap)

	walkErr := filepath.WalkDir(sourceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip symlinks to prevent symlink-based path escapes.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			base := d.Name()
			if base == "internal" || base == "testdata" || base == "vendor" || strings.HasPrefix(base, "_") {
				return fs.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, parseErr)
			return nil
		}

		if file.Name.Name == "main" {
			return nil
		}

		pkgPath := computePackagePath(sourceRoot, path, module)

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				collectFunc(fset, d, pkgPath, &entries, sigMap)
			case *ast.GenDecl:
				switch d.Tok {
				case token.TYPE:
					collectTypes(fset, d, pkgPath, &entries)
				case token.CONST:
					collectValues(fset, d, pkgPath, symbols.SymbolConst, &entries)
				case token.VAR:
					collectValues(fset, d, pkgPath, symbols.SymbolVar, &entries)
				}
			}
		}

		return nil
	})
	if walkErr != nil {
		return symbols.Symbols{}, nil, fmt.Errorf("walking source at %s: %w", sourceRoot, walkErr)
	}

	return symbols.Symbols{Module: module, Entries: entries}, sigMap, nil
}

// collectFunc processes a single function or method declaration and appends
// the resulting symbol to entries. Methods on unexported receivers are skipped.
func collectFunc(fset *token.FileSet, funcDecl *ast.FuncDecl, pkgPath string, entries *[]symbols.Symbol, sigMap FuncSigMap) {
	if funcDecl.Name == nil || !funcDecl.Name.IsExported() {
		return
	}

	var sym symbols.Symbol
	var key symbolKey

	if funcDecl.Recv != nil {
		recvName := receiverTypeName(funcDecl.Recv)
		if recvName == "" || !ast.IsExported(recvName) {
			return
		}
		sym = symbols.Symbol{
			Kind:     symbols.SymbolMethod,
			Name:     recvName + "." + funcDecl.Name.Name,
			Package:  pkgPath,
			Receiver: recvName,
		}
		key = symbolKey{pkg: pkgPath, kind: symbols.SymbolMethod, name: sym.Name}
	} else {
		sym = symbols.Symbol{
			Kind:    symbols.SymbolFunc,
			Name:    funcDecl.Name.Name,
			Package: pkgPath,
		}
		key = symbolKey{pkg: pkgPath, kind: symbols.SymbolFunc, name: sym.Name}
	}

	sig := extractFuncSignature(fset, funcDecl.Type)
	sym.Signature = renderFuncSignature(sig)
	sigMap[key] = sig

	*entries = append(*entries, sym)
}

// collectTypes processes a GenDecl with token.TYPE, extracting exported types,
// their struct fields, and interface declarations.
func collectTypes(fset *token.FileSet, genDecl *ast.GenDecl, pkgPath string, entries *[]symbols.Symbol) {
	for _, spec := range genDecl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || !typeSpec.Name.IsExported() {
			continue
		}

		typeName := typeSpec.Name.Name

		var kind symbols.SymbolKind
		if _, isIface := typeSpec.Type.(*ast.InterfaceType); isIface {
			kind = symbols.SymbolInterface
		} else {
			kind = symbols.SymbolType
		}

		*entries = append(*entries, symbols.Symbol{
			Kind:      kind,
			Name:      typeName,
			Package:   pkgPath,
			Signature: extractTypeSignature(fset, typeSpec),
		})

		// Extract exported fields from struct types.
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			continue
		}

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				// Embedded field: emit if the type name is exported.
				embName := baseTypeName(field.Type)
				if embName == "" || !ast.IsExported(embName) {
					continue
				}
				*entries = append(*entries, symbols.Symbol{
					Kind:      symbols.SymbolField,
					Name:      typeName + "." + embName,
					Package:   pkgPath,
					Signature: renderTypeExpr(fset, field.Type),
				})
				continue
			}

			for _, name := range field.Names {
				if !name.IsExported() {
					continue
				}
				*entries = append(*entries, symbols.Symbol{
					Kind:      symbols.SymbolField,
					Name:      typeName + "." + name.Name,
					Package:   pkgPath,
					Signature: renderTypeExpr(fset, field.Type),
				})
			}
		}
	}
}

// collectValues processes a GenDecl with token.CONST or token.VAR.
func collectValues(fset *token.FileSet, genDecl *ast.GenDecl, pkgPath string, kind symbols.SymbolKind, entries *[]symbols.Symbol) {
	for _, spec := range genDecl.Specs {
		valSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range valSpec.Names {
			if !name.IsExported() {
				continue
			}
			*entries = append(*entries, symbols.Symbol{
				Kind:      kind,
				Name:      name.Name,
				Package:   pkgPath,
				Signature: extractConstVarType(fset, valSpec),
			})
		}
	}
}

// computePackagePath derives the full Go import path for the package
// containing the file at filePath, relative to the module source root.
func computePackagePath(sourceRoot, filePath, module string) string {
	dir := filepath.Dir(filePath)
	relDir, err := filepath.Rel(sourceRoot, dir)
	if err != nil || relDir == "." || relDir == "" {
		return module
	}
	return module + "/" + filepath.ToSlash(relDir)
}

// baseTypeName extracts the base type name from an AST expression,
// stripping pointers, type parameters (generics), and package selectors.
// Examples: *Client -> "Client", Foo[T] -> "Foo", *Bar[T, U] -> "Bar"
func baseTypeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	// Strip pointer.
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	// Strip type parameters (generics).
	if idx, ok := expr.(*ast.IndexExpr); ok {
		expr = idx.X
	}
	if idx, ok := expr.(*ast.IndexListExpr); ok {
		expr = idx.X
	}

	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}

	return ""
}

// receiverTypeName extracts the base type name from a method receiver.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return baseTypeName(recv.List[0].Type)
}

// FindSourceRoot walks from dir looking for go.mod to find the module source root.
// The Go proxy zip extracts to tmpDir/module@version/, so go.mod may be nested.
func FindSourceRoot(dir string) (string, error) {
	// Check dir itself first.
	if hasGoMod(dir) {
		return dir, nil
	}

	// Walk at most 2 levels deep looking for go.mod.
	var found string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		// Limit depth to 2 levels below the starting directory.
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/")
		if depth > 2 {
			return fs.SkipDir
		}

		if hasGoMod(path) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("searching for go.mod: %w", err)
	}

	if found == "" {
		return "", fmt.Errorf("no go.mod found under %s", dir)
	}
	return found, nil
}

// hasGoMod reports whether the directory contains a go.mod file.
func hasGoMod(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil && !info.IsDir()
}
