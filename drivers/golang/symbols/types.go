package symbols

// SymbolKind identifies what kind of exported Go symbol this is.
type SymbolKind string

const (
	SymbolFunc      SymbolKind = "func"
	SymbolType      SymbolKind = "type"
	SymbolMethod    SymbolKind = "method"
	SymbolField     SymbolKind = "field"
	SymbolConst     SymbolKind = "const"
	SymbolVar       SymbolKind = "var"
	SymbolInterface SymbolKind = "interface"
)

// Symbol represents a single exported Go symbol.
type Symbol struct {
	Name      string     `json:"name"`
	Kind      SymbolKind `json:"kind"`
	Package   string     `json:"package"`
	Receiver  string     `json:"receiver,omitempty"`
	Signature string     `json:"signature,omitempty"`
}

// Symbols is the full set of exports from a Go module version.
type Symbols struct {
	Module  string   `json:"module"`
	Version string   `json:"version"`
	Entries []Symbol `json:"entries"`
}

// AliasMap maps Go file paths to their import aliases.
// Key: file path, Value: map of alias to package import path.
// If a file uses the default import (no alias), the alias is the package name.
type AliasMap map[string]map[string]string
