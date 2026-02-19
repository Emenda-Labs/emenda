package astdiff

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

// funcSignature holds structured function parameter and result types.
// Used for param overlap comparison in DiffExports Pass 5.
type funcSignature struct {
	params  []string // parameter type strings only
	results []string // result type strings only
}

// renderTypeExpr converts any ast.Expr to its canonical string representation.
// This is the single source of truth for type rendering across the package.
func renderTypeExpr(fset *token.FileSet, expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name

	case *ast.SelectorExpr:
		return renderTypeExpr(fset, e.X) + "." + e.Sel.Name

	case *ast.StarExpr:
		return "*" + renderTypeExpr(fset, e.X)

	case *ast.ArrayType:
		if e.Len != nil {
			return fmt.Sprintf("[%s]%s", renderTypeExpr(fset, e.Len), renderTypeExpr(fset, e.Elt))
		}
		return "[]" + renderTypeExpr(fset, e.Elt)

	case *ast.MapType:
		return "map[" + renderTypeExpr(fset, e.Key) + "]" + renderTypeExpr(fset, e.Value)

	case *ast.InterfaceType:
		if e.Methods == nil || len(e.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"

	case *ast.FuncType:
		sig := extractFuncSignature(fset, e)
		return "func" + renderFuncSignature(sig)

	case *ast.Ellipsis:
		return "..." + renderTypeExpr(fset, e.Elt)

	case *ast.ChanType:
		switch e.Dir {
		case ast.RECV:
			return "<-chan " + renderTypeExpr(fset, e.Value)
		case ast.SEND:
			return "chan<- " + renderTypeExpr(fset, e.Value)
		default:
			return "chan " + renderTypeExpr(fset, e.Value)
		}

	case *ast.StructType:
		return "struct{...}"

	case *ast.IndexExpr:
		return renderTypeExpr(fset, e.X) + "[" + renderTypeExpr(fset, e.Index) + "]"

	case *ast.IndexListExpr:
		indices := make([]string, len(e.Indices))
		for i, idx := range e.Indices {
			indices[i] = renderTypeExpr(fset, idx)
		}
		return renderTypeExpr(fset, e.X) + "[" + strings.Join(indices, ", ") + "]"

	case *ast.ParenExpr:
		return "(" + renderTypeExpr(fset, e.X) + ")"

	case *ast.BasicLit:
		return e.Value

	default:
		return "unknown"
	}
}

// extractFuncSignature extracts structured parameter and result types from a function type.
// Handles multiple names per field (e.g. a, b int) and variadic parameters.
func extractFuncSignature(fset *token.FileSet, funcType *ast.FuncType) funcSignature {
	if funcType == nil {
		return funcSignature{}
	}

	var params []string
	if funcType.Params != nil {
		for i, field := range funcType.Params.List {
			typeStr := renderTypeExpr(fset, field.Type)

			// Handle variadic: last param field may have *ast.Ellipsis type.
			isLastField := i == len(funcType.Params.List)-1
			if isLastField {
				if _, ok := field.Type.(*ast.Ellipsis); ok {
					typeStr = renderTypeExpr(fset, field.Type)
				}
			}

			if len(field.Names) == 0 {
				// Unnamed parameter (common in interface method signatures).
				params = append(params, typeStr)
			} else {
				// Expand one entry per name sharing the same type.
				for range field.Names {
					params = append(params, typeStr)
				}
			}
		}
	}

	var results []string
	if funcType.Results != nil {
		for _, field := range funcType.Results.List {
			typeStr := renderTypeExpr(fset, field.Type)
			if len(field.Names) == 0 {
				results = append(results, typeStr)
			} else {
				for range field.Names {
					results = append(results, typeStr)
				}
			}
		}
	}

	return funcSignature{params: params, results: results}
}

// renderFuncSignature renders a funcSignature to its canonical string form.
// Format: "(Type1, Type2) RetType" or "(Type1) (RetType1, RetType2)".
func renderFuncSignature(sig funcSignature) string {
	paramStr := "(" + strings.Join(sig.params, ", ") + ")"

	switch len(sig.results) {
	case 0:
		return paramStr
	case 1:
		return paramStr + " " + sig.results[0]
	default:
		return paramStr + " (" + strings.Join(sig.results, ", ") + ")"
	}
}

// extractTypeSignature produces a canonical signature string for a type spec.
// Struct types list exported fields; interface types list methods sorted alphabetically.
func extractTypeSignature(fset *token.FileSet, typeSpec *ast.TypeSpec) string {
	// Alias types: type Foo = Bar
	if typeSpec.Assign.IsValid() {
		return "= " + renderTypeExpr(fset, typeSpec.Type)
	}

	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		return renderStructSignature(fset, t)
	case *ast.InterfaceType:
		return renderInterfaceSignature(fset, t)
	default:
		return renderTypeExpr(fset, typeSpec.Type)
	}
}

// renderStructSignature produces "struct{Field1 Type1; Field2 Type2}" with exported fields only.
func renderStructSignature(fset *token.FileSet, structType *ast.StructType) string {
	if structType.Fields == nil || len(structType.Fields.List) == 0 {
		return "struct{}"
	}

	var fields []string
	for _, field := range structType.Fields.List {
		typeStr := renderTypeExpr(fset, field.Type)

		if len(field.Names) == 0 {
			// Embedded field: include as just the type string.
			fields = append(fields, typeStr)
			continue
		}

		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			fields = append(fields, name.Name+" "+typeStr)
		}
	}

	if len(fields) == 0 {
		return "struct{}"
	}
	return "struct{" + strings.Join(fields, "; ") + "}"
}

// renderInterfaceSignature produces "interface{Method1(sig); Method2(sig)}" sorted alphabetically.
func renderInterfaceSignature(fset *token.FileSet, interfaceType *ast.InterfaceType) string {
	if interfaceType.Methods == nil || len(interfaceType.Methods.List) == 0 {
		return "interface{}"
	}

	var entries []string
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) > 0 {
			// Named method.
			name := method.Names[0].Name
			if funcType, ok := method.Type.(*ast.FuncType); ok {
				sig := extractFuncSignature(fset, funcType)
				entries = append(entries, name+renderFuncSignature(sig))
			}
		} else {
			// Embedded interface.
			entries = append(entries, renderTypeExpr(fset, method.Type))
		}
	}

	sort.Strings(entries)

	if len(entries) == 0 {
		return "interface{}"
	}
	return "interface{" + strings.Join(entries, "; ") + "}"
}

// extractFieldType renders a struct field's type expression to a string.
func extractFieldType(fset *token.FileSet, expr ast.Expr) string {
	return renderTypeExpr(fset, expr)
}

// extractConstVarType returns the explicit type of a const or var spec.
// Returns an empty string for untyped constants/variables.
func extractConstVarType(fset *token.FileSet, spec *ast.ValueSpec) string {
	if spec == nil || spec.Type == nil {
		return ""
	}
	return renderTypeExpr(fset, spec.Type)
}
