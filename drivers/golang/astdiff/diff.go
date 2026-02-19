package astdiff

import (
	"sort"
	"strings"

	"github.com/emenda-labs/emenda/core/changespec"
	"github.com/emenda-labs/emenda/drivers/golang/symbols"
)

const (
	// MinNameSimilarity is the minimum normalized Levenshtein similarity for fuzzy rename matching.
	MinNameSimilarity = 0.7

	// MinParamOverlap is the minimum Jaccard overlap on parameter types for fuzzy rename matching.
	MinParamOverlap = 0.8

	// ShortNameLength is the threshold below which stricter name similarity is required.
	ShortNameLength = 4

	// ShortNameMinSimilarity is the stricter threshold for names shorter than ShortNameLength.
	ShortNameMinSimilarity = 0.85
)

// diffState holds the working state across all diff passes.
type diffState struct {
	oldByKey   map[symbolKey]*symbols.Symbol
	newByKey   map[symbolKey]*symbols.Symbol
	matchedOld map[symbolKey]bool
	matchedNew map[symbolKey]bool
	oldSigs    FuncSigMap
	newSigs    FuncSigMap
	changes    []changespec.Change
}

// nameKey is a secondary index for cross-kind detection (same name, different kind).
type nameKey struct {
	pkg  string
	name string
}

// scoredPair holds a candidate fuzzy match with its composite score.
type scoredPair struct {
	oldKey  symbolKey
	newKey  symbolKey
	score   float64
	oldName string // for tie-breaking
}

// newDiffState builds indexed lookup maps from old and new symbol sets.
func newDiffState(old, new symbols.Symbols, oldSigs, newSigs FuncSigMap) *diffState {
	s := &diffState{
		oldByKey:   make(map[symbolKey]*symbols.Symbol, len(old.Entries)),
		newByKey:   make(map[symbolKey]*symbols.Symbol, len(new.Entries)),
		matchedOld: make(map[symbolKey]bool),
		matchedNew: make(map[symbolKey]bool),
		oldSigs:    oldSigs,
		newSigs:    newSigs,
	}
	for i := range old.Entries {
		sym := &old.Entries[i]
		key := symbolKey{pkg: sym.Package, kind: sym.Kind, name: sym.Name}
		s.oldByKey[key] = sym
	}
	for i := range new.Entries {
		sym := &new.Entries[i]
		key := symbolKey{pkg: sym.Package, kind: sym.Kind, name: sym.Name}
		s.newByKey[key] = sym
	}
	return s
}

// DiffExports compares two symbol sets and classifies all breaking changes with confidence levels.
// Runs six passes: exact match, changed, renamed, correlate methods, fuzzy match, leftovers.
func DiffExports(old, new symbols.Symbols, oldSigs, newSigs FuncSigMap) []changespec.Change {
	s := newDiffState(old, new, oldSigs, newSigs)
	s.exactMatch()
	s.changed()
	s.renamed()
	s.correlateMethods()
	s.fuzzyMatch()
	s.leftovers()
	return s.changes
}

func (s *diffState) markMatched(oldKey, newKey symbolKey) {
	s.matchedOld[oldKey] = true
	s.matchedNew[newKey] = true
}

func (s *diffState) emit(c changespec.Change) {
	s.changes = append(s.changes, c)
}

func (s *diffState) unmatchedOld() []symbolKey {
	var keys []symbolKey
	for k := range s.oldByKey {
		if !s.matchedOld[k] {
			keys = append(keys, k)
		}
	}
	return keys
}

func (s *diffState) unmatchedNew() []symbolKey {
	var keys []symbolKey
	for k := range s.newByKey {
		if !s.matchedNew[k] {
			keys = append(keys, k)
		}
	}
	return keys
}

// Pass 1: exact matches (same key, same signature) are silently consumed.
func (s *diffState) exactMatch() {
	for key, oldSym := range s.oldByKey {
		newSym, ok := s.newByKey[key]
		if !ok {
			continue
		}
		if oldSym.Signature == newSym.Signature {
			s.markMatched(key, key)
		}
	}
}

// Pass 2: same-key signature changes and cross-kind type changes.
func (s *diffState) changed() {
	// Part A: same key, different signature.
	for _, key := range s.unmatchedOld() {
		newSym, ok := s.newByKey[key]
		if !ok {
			continue
		}
		oldSym := s.oldByKey[key]
		if oldSym.Signature == newSym.Signature {
			continue
		}

		kind := changespec.ChangeKindSignatureChanged
		if oldSym.Kind == symbols.SymbolType || oldSym.Kind == symbols.SymbolInterface {
			kind = changespec.ChangeKindTypeChanged
		}

		s.emit(changespec.Change{
			Kind:         kind,
			Symbol:       oldSym.Name,
			Package:      oldSym.Package,
			OldSignature: oldSym.Signature,
			NewSignature: newSym.Signature,
			Confidence:   changespec.ConfidenceHigh,
		})
		s.markMatched(key, key)
	}

	// Part B: cross-kind changes (same name+pkg, different kind).
	oldByName := make(map[nameKey]symbolKey)
	for _, key := range s.unmatchedOld() {
		nk := nameKey{pkg: key.pkg, name: key.name}
		oldByName[nk] = key
	}

	newByName := make(map[nameKey]symbolKey)
	for _, key := range s.unmatchedNew() {
		nk := nameKey{pkg: key.pkg, name: key.name}
		newByName[nk] = key
	}

	for nk, oldKey := range oldByName {
		newKey, ok := newByName[nk]
		if !ok {
			continue
		}
		if oldKey.kind == newKey.kind {
			continue
		}

		oldSym := s.oldByKey[oldKey]
		newSym := s.newByKey[newKey]

		s.emit(changespec.Change{
			Kind:         changespec.ChangeKindTypeChanged,
			Symbol:       oldSym.Name,
			Package:      oldSym.Package,
			OldSignature: oldSym.Signature,
			NewSignature: newSym.Signature,
			Confidence:   changespec.ConfidenceHigh,
		})
		s.markMatched(oldKey, newKey)
	}
}

// Pass 3: exact-signature renames (unique 1:1 mapping by signature+kind+package).
// Also builds typeRenames for Pass 4.
func (s *diffState) renamed() {
	removedBySig := make(map[string][]symbolKey)
	for _, key := range s.unmatchedOld() {
		oldSym := s.oldByKey[key]
		sigKey := oldSym.Signature + "|" + string(oldSym.Kind) + "|" + oldSym.Package
		removedBySig[sigKey] = append(removedBySig[sigKey], key)
	}

	addedBySig := make(map[string][]symbolKey)
	for _, key := range s.unmatchedNew() {
		newSym := s.newByKey[key]
		sigKey := newSym.Signature + "|" + string(newSym.Kind) + "|" + newSym.Package
		addedBySig[sigKey] = append(addedBySig[sigKey], key)
	}

	typeRenames := make(map[string]string)

	for sig, oldKeys := range removedBySig {
		newKeys, ok := addedBySig[sig]
		if !ok {
			continue
		}

		// Collision: more than one on either side, defer to Pass 5.
		if len(oldKeys) > 1 || len(newKeys) > 1 {
			continue
		}

		oldKey := oldKeys[0]
		newKey := newKeys[0]
		oldSym := s.oldByKey[oldKey]
		newSym := s.newByKey[newKey]

		// Trivial signature guard: skip empty or "()" signatures.
		if oldSym.Signature == "" || oldSym.Signature == "()" {
			continue
		}

		s.emit(changespec.Change{
			Kind:         changespec.ChangeKindRenamed,
			Symbol:       oldSym.Name,
			Package:      oldSym.Package,
			NewName:      newSym.Name,
			OldSignature: oldSym.Signature,
			NewSignature: newSym.Signature,
			Confidence:   changespec.ConfidenceHigh,
		})
		s.markMatched(oldKey, newKey)

		// Track type/interface renames for Pass 4.
		if oldSym.Kind == symbols.SymbolType || oldSym.Kind == symbols.SymbolInterface {
			typeRenames[oldSym.Name] = newSym.Name
		}

		_ = sig // consumed
	}

	// Store type renames for correlateMethods.
	s.correlateWithTypeRenames(typeRenames)
}

// Pass 4: correlate methods and fields whose receiver/parent type was renamed.
func (s *diffState) correlateWithTypeRenames(typeRenames map[string]string) {
	if len(typeRenames) == 0 {
		return
	}

	for _, oldKey := range s.unmatchedOld() {
		oldSym := s.oldByKey[oldKey]

		if oldSym.Kind != symbols.SymbolMethod && oldSym.Kind != symbols.SymbolField {
			continue
		}

		// Extract receiver/type name (part before the dot).
		dotIdx := strings.Index(oldSym.Name, ".")
		if dotIdx < 0 {
			continue
		}
		receiver := oldSym.Name[:dotIdx]
		member := oldSym.Name[dotIdx+1:]

		newReceiver, ok := typeRenames[receiver]
		if !ok {
			continue
		}

		expectedNewName := newReceiver + "." + member
		expectedNewKey := symbolKey{pkg: oldSym.Package, kind: oldSym.Kind, name: expectedNewName}

		newSym, found := s.newByKey[expectedNewKey]
		if !found || s.matchedNew[expectedNewKey] {
			continue
		}

		if oldSym.Signature == newSym.Signature {
			s.emit(changespec.Change{
				Kind:         changespec.ChangeKindRenamed,
				Symbol:       oldSym.Name,
				Package:      oldSym.Package,
				NewName:      newSym.Name,
				OldSignature: oldSym.Signature,
				NewSignature: newSym.Signature,
				Confidence:   changespec.ConfidenceHigh,
			})
		} else {
			s.emit(changespec.Change{
				Kind:         changespec.ChangeKindSignatureChanged,
				Symbol:       oldSym.Name,
				Package:      oldSym.Package,
				OldSignature: oldSym.Signature,
				NewSignature: newSym.Signature,
				Confidence:   changespec.ConfidenceHigh,
			})
		}
		s.markMatched(oldKey, expectedNewKey)
	}
}

// correlateMethods is the public Pass 4 entry point; actual work is done
// inside renamed() which calls correlateWithTypeRenames.
func (s *diffState) correlateMethods() {
	// Type renames are collected and correlated inside renamed().
	// This method exists to maintain the pass structure in DiffExports.
}

// Pass 5: fuzzy matching for functions/methods using name similarity and param overlap.
func (s *diffState) fuzzyMatch() {
	unmatchedOldKeys := s.unmatchedOld()
	unmatchedNewKeys := s.unmatchedNew()

	// Filter to only symbols with funcSignature entries.
	var oldFuncKeys []symbolKey
	for _, key := range unmatchedOldKeys {
		if _, ok := s.oldSigs[key]; ok {
			oldFuncKeys = append(oldFuncKeys, key)
		}
	}

	var newFuncKeys []symbolKey
	for _, key := range unmatchedNewKeys {
		if _, ok := s.newSigs[key]; ok {
			newFuncKeys = append(newFuncKeys, key)
		}
	}

	if len(oldFuncKeys) == 0 || len(newFuncKeys) == 0 {
		return
	}

	// Build candidate pairs.
	var candidates []scoredPair
	for _, oldKey := range oldFuncKeys {
		oldSym := s.oldByKey[oldKey]
		oldSig := s.oldSigs[oldKey]

		for _, newKey := range newFuncKeys {
			newSym := s.newByKey[newKey]
			newSig := s.newSigs[newKey]

			nameSim := nameSimilarity(oldSym.Name, newSym.Name)
			overlap := paramOverlap(oldSig, newSig)

			// Apply short name guard.
			nameThreshold := MinNameSimilarity
			maxLen := max(len(oldSym.Name), len(newSym.Name))
			if maxLen < ShortNameLength {
				nameThreshold = ShortNameMinSimilarity
			}

			if nameSim >= nameThreshold && overlap >= MinParamOverlap {
				candidates = append(candidates, scoredPair{
					oldKey:  oldKey,
					newKey:  newKey,
					score:   nameSim * overlap,
					oldName: oldSym.Name,
				})
			}
		}
	}

	// Sort by descending score, tie-break by old symbol name.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].oldName < candidates[j].oldName
	})

	// Greedy matching.
	for _, pair := range candidates {
		if s.matchedOld[pair.oldKey] || s.matchedNew[pair.newKey] {
			continue
		}

		oldSym := s.oldByKey[pair.oldKey]
		newSym := s.newByKey[pair.newKey]

		s.emit(changespec.Change{
			Kind:         changespec.ChangeKindRenamed,
			Symbol:       oldSym.Name,
			Package:      oldSym.Package,
			NewName:      newSym.Name,
			OldSignature: oldSym.Signature,
			NewSignature: newSym.Signature,
			Confidence:   changespec.ConfidenceMedium,
		})
		s.markMatched(pair.oldKey, pair.newKey)
	}
}

// Pass 6: all remaining unmatched old symbols are classified as removed.
func (s *diffState) leftovers() {
	for _, key := range s.unmatchedOld() {
		oldSym := s.oldByKey[key]
		s.emit(changespec.Change{
			Kind:         changespec.ChangeKindRemoved,
			Symbol:       oldSym.Name,
			Package:      oldSym.Package,
			OldSignature: oldSym.Signature,
			Confidence:   changespec.ConfidenceLow,
		})
	}
}

// levenshteinDistance computes the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two rows instead of full matrix.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost

			curr[j] = min(del, ins, sub)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

// nameSimilarity returns the normalized Levenshtein similarity between two strings.
// Returns a value in [0.0, 1.0] where 1.0 means identical.
func nameSimilarity(a, b string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	maxLen := max(len(a), len(b))
	return 1.0 - float64(levenshteinDistance(a, b))/float64(maxLen)
}

// paramOverlap computes the Jaccard similarity of parameter type multisets.
// Special case: if both params AND results are empty, returns 1.0 (vacuously true).
func paramOverlap(a, b funcSignature) float64 {
	aAll := append(append([]string{}, a.params...), a.results...)
	bAll := append(append([]string{}, b.params...), b.results...)

	if len(aAll) == 0 && len(bAll) == 0 {
		return 1.0
	}

	aSet := make(map[string]int)
	for _, t := range aAll {
		aSet[t]++
	}

	bSet := make(map[string]int)
	for _, t := range bAll {
		bSet[t]++
	}

	// Jaccard on multisets: intersection = sum of min counts, union = sum of max counts.
	var intersection, union int
	allKeys := make(map[string]bool)
	for k := range aSet {
		allKeys[k] = true
	}
	for k := range bSet {
		allKeys[k] = true
	}

	for k := range allKeys {
		ac := aSet[k]
		bc := bSet[k]
		intersection += min(ac, bc)
		union += max(ac, bc)
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}
