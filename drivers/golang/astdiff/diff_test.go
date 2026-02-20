package astdiff

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/emenda-labs/emenda/core/changespec"
	"github.com/emenda-labs/emenda/drivers/golang/symbols"
)

// buildSymbols is a helper to construct a Symbols set from a slice of Symbol.
func buildSymbols(module string, entries []symbols.Symbol) symbols.Symbols {
	return symbols.Symbols{Module: module, Entries: entries}
}

func TestDiffExports_IdenticalModules(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "old")
	syms, sigs, err := ParseExports(context.Background(), dir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports: %v", err)
	}

	changes := DiffExports(syms, syms, sigs, sigs)
	if len(changes) != 0 {
		t.Errorf("identical modules should produce 0 changes, got %d", len(changes))
		for _, c := range changes {
			t.Logf("  %s %s %s", c.Kind, c.Package, c.Symbol)
		}
	}
}

func TestDiffExports_FullFixtures(t *testing.T) {
	oldDir := filepath.Join(testdataDir(t), "old")
	newDir := filepath.Join(testdataDir(t), "new")

	oldSyms, oldSigs, err := ParseExports(context.Background(), oldDir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports old: %v", err)
	}

	newSyms, newSigs, err := ParseExports(context.Background(), newDir, "github.com/acme/testmod")
	if err != nil {
		t.Fatalf("ParseExports new: %v", err)
	}

	changes := DiffExports(oldSyms, newSyms, oldSigs, newSigs)

	bySymbol := make(map[string]changespec.Change)
	for _, c := range changes {
		bySymbol[c.Symbol] = c
	}

	// DoWork: signature changed (added opts param).
	if c, ok := bySymbol["DoWork"]; ok {
		if c.Kind != changespec.ChangeKindSignatureChanged {
			t.Errorf("DoWork kind = %q, want signature_changed", c.Kind)
		}
		if c.Confidence != changespec.ConfidenceHigh {
			t.Errorf("DoWork confidence = %q, want high", c.Confidence)
		}
	} else {
		t.Error("missing change for DoWork")
	}

	// HelperFunc: renamed to HelperFunction (same signature).
	if c, ok := bySymbol["HelperFunc"]; ok {
		if c.Kind != changespec.ChangeKindRenamed {
			t.Errorf("HelperFunc kind = %q, want renamed", c.Kind)
		}
		if c.NewName != "HelperFunction" {
			t.Errorf("HelperFunc new_name = %q, want HelperFunction", c.NewName)
		}
		if c.Confidence != changespec.ConfidenceHigh {
			t.Errorf("HelperFunc confidence = %q, want high", c.Confidence)
		}
	} else {
		t.Error("missing change for HelperFunc")
	}

	// Token: type_changed (string -> int).
	if c, ok := bySymbol["Token"]; ok {
		if c.Kind != changespec.ChangeKindTypeChanged {
			t.Errorf("Token kind = %q, want type_changed", c.Kind)
		}
		if c.Confidence != changespec.ConfidenceHigh {
			t.Errorf("Token confidence = %q, want high", c.Confidence)
		}
	} else {
		t.Error("missing change for Token")
	}

	// ComputeHash: cross-kind change (var -> func).
	if c, ok := bySymbol["ComputeHash"]; ok {
		if c.Kind != changespec.ChangeKindTypeChanged {
			t.Errorf("ComputeHash kind = %q, want type_changed", c.Kind)
		}
		if c.Confidence != changespec.ConfidenceHigh {
			t.Errorf("ComputeHash confidence = %q, want high", c.Confidence)
		}
	} else {
		t.Error("missing change for ComputeHash")
	}

	// Config: renamed to Settings (same struct signature).
	if c, ok := bySymbol["Config"]; ok {
		if c.Kind != changespec.ChangeKindRenamed {
			t.Errorf("Config kind = %q, want renamed", c.Kind)
		}
		if c.NewName != "Settings" {
			t.Errorf("Config new_name = %q, want Settings", c.NewName)
		}
	} else {
		t.Error("missing change for Config")
	}

	// Handler: interface signature changed.
	if c, ok := bySymbol["Handler"]; ok {
		if c.Kind != changespec.ChangeKindTypeChanged {
			t.Errorf("Handler kind = %q, want type_changed", c.Kind)
		}
	} else {
		t.Error("missing change for Handler")
	}

	// OldOnly: removed.
	if c, ok := bySymbol["OldOnly"]; ok {
		if c.Kind != changespec.ChangeKindRemoved {
			t.Errorf("OldOnly kind = %q, want removed", c.Kind)
		}
	} else {
		t.Error("missing change for OldOnly")
	}

	// DefaultConfig: var type changed (Config -> Settings).
	if c, ok := bySymbol["DefaultConfig"]; ok {
		if c.Kind != changespec.ChangeKindSignatureChanged && c.Kind != changespec.ChangeKindTypeChanged {
			t.Errorf("DefaultConfig kind = %q, want signature_changed or type_changed", c.Kind)
		}
	} else {
		t.Error("missing change for DefaultConfig")
	}

	// Verify all changes have confidence set.
	for _, c := range changes {
		if c.Confidence == "" {
			t.Errorf("change %s %s has empty confidence", c.Kind, c.Symbol)
		}
	}

	// Verify no change for unchanged symbols.
	unchangedSymbols := []string{"SimpleFunc", "Variadic", "MaxRetries", "UntypedConst", "ErrNotFound"}
	for _, name := range unchangedSymbols {
		if _, ok := bySymbol[name]; ok {
			t.Errorf("unchanged symbol %s should not appear in changes", name)
		}
	}

	// Verify NewFeature (added in new) is NOT in changes.
	if _, ok := bySymbol["NewFeature"]; ok {
		t.Error("added symbol NewFeature should not appear in changes")
	}
}

func TestDiffExports_Pass1_ExactMatch(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Foo", Package: "mod", Signature: "(int) string"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Foo", Package: "mod", Signature: "(int) string"},
	})
	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 0 {
		t.Errorf("exact match should produce 0 changes, got %d", len(changes))
	}
}

func TestDiffExports_Pass2_SignatureChanged(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Foo", Package: "mod", Signature: "(int) string"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Foo", Package: "mod", Signature: "(int, bool) string"},
	})
	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindSignatureChanged {
		t.Errorf("kind = %q, want signature_changed", changes[0].Kind)
	}
	if changes[0].Confidence != changespec.ConfidenceHigh {
		t.Errorf("confidence = %q, want high", changes[0].Confidence)
	}
}

func TestDiffExports_Pass2_TypeChanged(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolType, Name: "Token", Package: "mod", Signature: "string"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolType, Name: "Token", Package: "mod", Signature: "int"},
	})
	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindTypeChanged {
		t.Errorf("kind = %q, want type_changed", changes[0].Kind)
	}
}

func TestDiffExports_Pass2_CrossKind(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolVar, Name: "Compute", Package: "mod", Signature: "string"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Compute", Package: "mod", Signature: "(string) string"},
	})
	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindTypeChanged {
		t.Errorf("kind = %q, want type_changed", changes[0].Kind)
	}
	if changes[0].Confidence != changespec.ConfidenceHigh {
		t.Errorf("confidence = %q, want high", changes[0].Confidence)
	}
}

func TestDiffExports_Pass3_Renamed(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "OldName", Package: "mod", Signature: "(int, string) error"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "NewName", Package: "mod", Signature: "(int, string) error"},
	})
	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindRenamed {
		t.Errorf("kind = %q, want renamed", changes[0].Kind)
	}
	if changes[0].NewName != "NewName" {
		t.Errorf("new_name = %q, want NewName", changes[0].NewName)
	}
	if changes[0].Confidence != changespec.ConfidenceHigh {
		t.Errorf("confidence = %q, want high", changes[0].Confidence)
	}
}

func TestDiffExports_Pass3_TrivialSigGuard(t *testing.T) {
	// No-arg functions with trivial signatures should NOT match in Pass 3.
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "OldInit", Package: "mod", Signature: "()"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "NewSetup", Package: "mod", Signature: "()"},
	})
	changes := DiffExports(old, new, nil, nil)
	// Should NOT be renamed at HIGH confidence — should fall to Pass 5 or 6.
	for _, c := range changes {
		if c.Kind == changespec.ChangeKindRenamed && c.Confidence == changespec.ConfidenceHigh {
			t.Errorf("trivial sig rename should not be HIGH: %+v", c)
		}
	}
}

func TestDiffExports_Pass3_EmptySigGuard(t *testing.T) {
	// Symbols with empty signatures should NOT match in Pass 3.
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolConst, Name: "OldConst", Package: "mod", Signature: ""},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolConst, Name: "NewConst", Package: "mod", Signature: ""},
	})
	changes := DiffExports(old, new, nil, nil)
	for _, c := range changes {
		if c.Kind == changespec.ChangeKindRenamed && c.Confidence == changespec.ConfidenceHigh {
			t.Errorf("empty sig rename should not be HIGH: %+v", c)
		}
	}
}

func TestDiffExports_Pass3_Collision(t *testing.T) {
	// Two old symbols with same sig: should NOT match in Pass 3 (collision).
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "FuncA", Package: "mod", Signature: "(int) error"},
		{Kind: symbols.SymbolFunc, Name: "FuncB", Package: "mod", Signature: "(int) error"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "FuncC", Package: "mod", Signature: "(int) error"},
	})
	changes := DiffExports(old, new, nil, nil)
	// Neither should be renamed at HIGH from Pass 3.
	for _, c := range changes {
		if c.Kind == changespec.ChangeKindRenamed && c.Confidence == changespec.ConfidenceHigh {
			t.Errorf("collision rename should not be HIGH: %+v", c)
		}
	}
}

func TestDiffExports_Pass4_TypeRenameCorrelation(t *testing.T) {
	// When a type is renamed, its methods should be correlated.
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolType, Name: "Client", Package: "mod", Signature: "struct{Host string}"},
		{Kind: symbols.SymbolMethod, Name: "Client.Do", Package: "mod", Signature: "(string) error"},
		{Kind: symbols.SymbolField, Name: "Client.Host", Package: "mod", Signature: "string"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolType, Name: "HTTPClient", Package: "mod", Signature: "struct{Host string}"},
		{Kind: symbols.SymbolMethod, Name: "HTTPClient.Do", Package: "mod", Signature: "(string) error"},
		{Kind: symbols.SymbolField, Name: "HTTPClient.Host", Package: "mod", Signature: "string"},
	})
	changes := DiffExports(old, new, nil, nil)

	bySymbol := make(map[string]changespec.Change)
	for _, c := range changes {
		bySymbol[c.Symbol] = c
	}

	// Type rename.
	if c, ok := bySymbol["Client"]; ok {
		if c.Kind != changespec.ChangeKindRenamed || c.NewName != "HTTPClient" {
			t.Errorf("Client: kind=%q new_name=%q, want renamed HTTPClient", c.Kind, c.NewName)
		}
		if c.Confidence != changespec.ConfidenceHigh {
			t.Errorf("Client confidence = %q, want high", c.Confidence)
		}
	} else {
		t.Error("missing change for Client")
	}

	// Method should be correlated (renamed via Pass 4).
	if c, ok := bySymbol["Client.Do"]; ok {
		if c.Kind != changespec.ChangeKindRenamed || c.NewName != "HTTPClient.Do" {
			t.Errorf("Client.Do: kind=%q new_name=%q, want renamed HTTPClient.Do", c.Kind, c.NewName)
		}
		if c.Confidence != changespec.ConfidenceHigh {
			t.Errorf("Client.Do confidence = %q, want high", c.Confidence)
		}
	} else {
		t.Error("missing change for Client.Do")
	}

	// Field should be correlated.
	if c, ok := bySymbol["Client.Host"]; ok {
		if c.Kind != changespec.ChangeKindRenamed || c.NewName != "HTTPClient.Host" {
			t.Errorf("Client.Host: kind=%q new_name=%q, want renamed HTTPClient.Host", c.Kind, c.NewName)
		}
	} else {
		t.Error("missing change for Client.Host")
	}

	// No symbols should be marked as removed.
	for _, c := range changes {
		if c.Kind == changespec.ChangeKindRemoved {
			t.Errorf("unexpected removed: %s", c.Symbol)
		}
	}
}

func TestDiffExports_Pass5_FuzzyMatch(t *testing.T) {
	// ProcessRequest(ctx, name, opts) vs ProcessReq(ctx, name, opts, extra)
	// nameSimilarity ~0.71 >= 0.7, paramOverlap: 4 overlap out of 5 union = 0.8 >= 0.8
	oldSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "ProcessRequest"}: {
			params: []string{"context.Context", "string", "int"}, results: []string{"error"},
		},
	}
	newSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "ProcessReq"}: {
			params: []string{"context.Context", "string", "int", "bool"}, results: []string{"error"},
		},
	}

	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "ProcessRequest", Package: "mod", Signature: "(context.Context, string, int) error"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "ProcessReq", Package: "mod", Signature: "(context.Context, string, int, bool) error"},
	})

	changes := DiffExports(old, new, oldSigs, newSigs)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindRenamed {
		t.Errorf("kind = %q, want renamed", changes[0].Kind)
	}
	if changes[0].Confidence != changespec.ConfidenceMedium {
		t.Errorf("confidence = %q, want medium", changes[0].Confidence)
	}
	if changes[0].NewName != "ProcessReq" {
		t.Errorf("new_name = %q, want ProcessReq", changes[0].NewName)
	}
}

func TestDiffExports_Pass5_ShortNameRejection(t *testing.T) {
	// "Get" vs "Set": similarity = 0.67, below 0.7 threshold (and below 0.85 short-name guard).
	oldSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "Get"}: {
			params: []string{"string"}, results: []string{"int"},
		},
	}
	newSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "Set"}: {
			params: []string{"string"}, results: []string{"int"},
		},
	}

	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Get", Package: "mod", Signature: "(string) int"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Set", Package: "mod", Signature: "(string) int"},
	})

	// Use different sigs to avoid Pass 3.
	old.Entries[0].Signature = "(string) int"
	new.Entries[0].Signature = "(string, bool) int"
	newSigs[symbolKey{pkg: "mod", kind: symbols.SymbolFunc, name: "Set"}] = funcSignature{
		params: []string{"string", "bool"}, results: []string{"int"},
	}

	changes := DiffExports(old, new, oldSigs, newSigs)
	// Should NOT be matched as MEDIUM rename.
	for _, c := range changes {
		if c.Kind == changespec.ChangeKindRenamed {
			t.Errorf("Get/Set should not match as rename: %+v", c)
		}
	}
}

func TestDiffExports_Pass5_ZeroParamOverlap(t *testing.T) {
	// Both functions have no params and no results — paramOverlap should be 1.0.
	oldSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "Initialize"}: {
			params: nil, results: nil,
		},
	}
	newSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "Initialise"}: {
			params: nil, results: nil,
		},
	}

	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Initialize", Package: "mod", Signature: "special1"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Initialise", Package: "mod", Signature: "special2"},
	})

	changes := DiffExports(old, new, oldSigs, newSigs)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindRenamed {
		t.Errorf("kind = %q, want renamed", changes[0].Kind)
	}
	if changes[0].Confidence != changespec.ConfidenceMedium {
		t.Errorf("confidence = %q, want medium", changes[0].Confidence)
	}
}

func TestDiffExports_Pass6_Removed(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Deprecated", Package: "mod", Signature: "(int) error"},
	})
	new := buildSymbols("mod", nil)

	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != changespec.ChangeKindRemoved {
		t.Errorf("kind = %q, want removed", changes[0].Kind)
	}
	if changes[0].Confidence != changespec.ConfidenceLow {
		t.Errorf("confidence = %q, want low", changes[0].Confidence)
	}
}

func TestDiffExports_EmptyOld(t *testing.T) {
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "NewFunc", Package: "mod", Signature: "() error"},
	})
	changes := DiffExports(buildSymbols("mod", nil), new, nil, nil)
	if len(changes) != 0 {
		t.Errorf("empty old module should produce 0 changes, got %d", len(changes))
	}
}

func TestDiffExports_EmptyNew(t *testing.T) {
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "Foo", Package: "mod", Signature: "(int) error"},
		{Kind: symbols.SymbolType, Name: "Bar", Package: "mod", Signature: "struct{}"},
	})
	changes := DiffExports(old, buildSymbols("mod", nil), nil, nil)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	for _, c := range changes {
		if c.Kind != changespec.ChangeKindRemoved {
			t.Errorf("kind = %q, want removed for %s", c.Kind, c.Symbol)
		}
	}
}

func TestDiffExports_Pass5_NonFuncFallsToRemoved(t *testing.T) {
	// Non-function symbols cannot match in Pass 5 (no param overlap).
	// Use collision (2 old consts with same sig) to prevent Pass 3 matching.
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolConst, Name: "OldConstA", Package: "mod", Signature: "int"},
		{Kind: symbols.SymbolConst, Name: "OldConstB", Package: "mod", Signature: "int"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolConst, Name: "NewConst", Package: "mod", Signature: "int"},
	})
	changes := DiffExports(old, new, nil, nil)
	// Both old consts should be removed (can't fuzzy match consts).
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	for _, c := range changes {
		if c.Kind != changespec.ChangeKindRemoved {
			t.Errorf("kind = %q, want removed for %s", c.Kind, c.Symbol)
		}
	}
}

func TestDiffExports_Pass5_TieBreaking(t *testing.T) {
	// Two pairs with similar scores: lexicographic tie-break by old name.
	// Use high param overlap (5/6 = 0.833 >= 0.8) and high name similarity (~0.91).
	sharedOldParams := []string{"context.Context", "string", "int", "bool"}
	sharedNewParams := []string{"context.Context", "string", "int", "bool", "float64"}
	oldSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "CreateUser"}: {
			params: sharedOldParams, results: []string{"error"},
		},
		{pkg: "mod", kind: symbols.SymbolFunc, name: "DeleteUser"}: {
			params: sharedOldParams, results: []string{"error"},
		},
	}
	newSigs := FuncSigMap{
		{pkg: "mod", kind: symbols.SymbolFunc, name: "CreateUsers"}: {
			params: sharedNewParams, results: []string{"error"},
		},
		{pkg: "mod", kind: symbols.SymbolFunc, name: "DeleteUsers"}: {
			params: sharedNewParams, results: []string{"error"},
		},
	}

	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "CreateUser", Package: "mod", Signature: "sig-a"},
		{Kind: symbols.SymbolFunc, Name: "DeleteUser", Package: "mod", Signature: "sig-b"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "CreateUsers", Package: "mod", Signature: "sig-a2"},
		{Kind: symbols.SymbolFunc, Name: "DeleteUsers", Package: "mod", Signature: "sig-b2"},
	})

	changes := DiffExports(old, new, oldSigs, newSigs)

	// Sort changes by symbol for deterministic comparison.
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Symbol < changes[j].Symbol
	})

	// Verify results are deterministic.
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].Symbol != "CreateUser" || changes[0].NewName != "CreateUsers" {
		t.Errorf("first match: %s -> %s, want CreateUser -> CreateUsers", changes[0].Symbol, changes[0].NewName)
	}
	if changes[1].Symbol != "DeleteUser" || changes[1].NewName != "DeleteUsers" {
		t.Errorf("second match: %s -> %s, want DeleteUser -> DeleteUsers", changes[1].Symbol, changes[1].NewName)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"Get", "Set", 1},
		{"ProcessRequest", "ProcessReq", 4},
	}
	for _, tt := range tests {
		got := levenshteinDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNameSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"abc", "abc", 1.0, 1.0},
		{"", "", 1.0, 1.0},
		{"Get", "Set", 0.60, 0.70},
		{"ProcessRequest", "ProcessReq", 0.70, 0.75},
		{"Initialize", "Initialise", 0.85, 1.0},
	}
	for _, tt := range tests {
		got := nameSimilarity(tt.a, tt.b)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("nameSimilarity(%q, %q) = %.3f, want [%.2f, %.2f]",
				tt.a, tt.b, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestParamOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b funcSignature
		want float64
	}{
		{
			name: "identical",
			a:    funcSignature{params: []string{"int", "string"}, results: []string{"error"}},
			b:    funcSignature{params: []string{"int", "string"}, results: []string{"error"}},
			want: 1.0,
		},
		{
			name: "both_empty",
			a:    funcSignature{},
			b:    funcSignature{},
			want: 1.0,
		},
		{
			name: "one_empty",
			a:    funcSignature{params: []string{"int"}},
			b:    funcSignature{},
			want: 0.0,
		},
		{
			name: "partial_overlap",
			a:    funcSignature{params: []string{"int", "string"}, results: []string{"error"}},
			b:    funcSignature{params: []string{"int", "bool"}, results: []string{"error"}},
			want: 0.5, // intersection=2 (int,error), union=4 (int,string,bool,error)
		},
		{
			name: "no_overlap",
			a:    funcSignature{params: []string{"int"}},
			b:    funcSignature{params: []string{"string"}},
			want: 0.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paramOverlap(tt.a, tt.b)
			diff := got - tt.want
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("paramOverlap = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestDiffExports_ChangeFieldSemantics(t *testing.T) {
	// Verify Symbol/Package are always from old side, NewName is new side.
	old := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "OldFunc", Package: "github.com/acme/old", Signature: "(int) error"},
	})
	new := buildSymbols("mod", []symbols.Symbol{
		{Kind: symbols.SymbolFunc, Name: "NewFunc", Package: "github.com/acme/old", Signature: "(int) error"},
	})
	changes := DiffExports(old, new, nil, nil)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.Symbol != "OldFunc" {
		t.Errorf("Symbol = %q, want OldFunc (old side)", c.Symbol)
	}
	if c.Package != "github.com/acme/old" {
		t.Errorf("Package = %q, want github.com/acme/old", c.Package)
	}
	if c.NewName != "NewFunc" {
		t.Errorf("NewName = %q, want NewFunc (new side)", c.NewName)
	}
}
