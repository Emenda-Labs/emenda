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
	oldByKey        map[symbolKey]*symbols.Symbol
	newByKey        map[symbolKey]*symbols.Symbol
	unmatchedOldSet map[symbolKey]struct{}
	unmatchedNewSet map[symbolKey]struct{}
	oldSigs         FuncSigMap
	newSigs         FuncSigMap
	typeRenames     map[string]string
	changes         []changespec.Change
}

// nameKey is a secondary index for cross-kind detection (same name, different kind).
type nameKey struct {
	pkg  string
	name string
}

// sigGroupKey is the composite key for grouping symbols by signature in Pass 3.
// Using a struct avoids ambiguity from string concatenation with delimiters.
type sigGroupKey struct {
	sig  string
	kind symbols.SymbolKind
	pkg  string
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
		oldByKey:        make(map[symbolKey]*symbols.Symbol, len(old.Entries)),
		newByKey:        make(map[symbolKey]*symbols.Symbol, len(new.Entries)),
		unmatchedOldSet: make(map[symbolKey]struct{}, len(old.Entries)),
		unmatchedNewSet: make(map[symbolKey]struct{}, len(new.Entries)),
		oldSigs:         oldSigs,
		newSigs:         newSigs,
		typeRenames:     make(map[string]string),
	}
	for i := range old.Entries {
		sym := &old.Entries[i]
		key := symbolKey{pkg: sym.Package, kind: sym.Kind, name: sym.Name}
		s.oldByKey[key] = sym
		s.unmatchedOldSet[key] = struct{}{}
	}
	for i := range new.Entries {
		sym := &new.Entries[i]
		key := symbolKey{pkg: sym.Package, kind: sym.Kind, name: sym.Name}
		s.newByKey[key] = sym
		s.unmatchedNewSet[key] = struct{}{}
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
	delete(s.unmatchedOldSet, oldKey)
	delete(s.unmatchedNewSet, newKey)
}

func (s *diffState) emit(c changespec.Change) {
	s.changes = append(s.changes, c)
}

func (s *diffState) unmatchedOld() []symbolKey {
	keys := make([]symbolKey, 0, len(s.unmatchedOldSet))
	for k := range s.unmatchedOldSet {
		keys = append(keys, k)
	}
	return keys
}

func (s *diffState) unmatchedNew() []symbolKey {
	keys := make([]symbolKey, 0, len(s.unmatchedNewSet))
	for k := range s.unmatchedNewSet {
		keys = append(keys, k)
	}
	return keys
}

// Pass 1: exact matches (same key, same signature) are silently consumed.
func (s *diffState) exactMatch() {
	for key := range s.unmatchedOldSet {
		newSym, ok := s.newByKey[key]
		if !ok {
			continue
		}
		oldSym := s.oldByKey[key]
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
	removedBySig := make(map[sigGroupKey][]symbolKey)
	for _, key := range s.unmatchedOld() {
		oldSym := s.oldByKey[key]
		gk := sigGroupKey{sig: oldSym.Signature, kind: oldSym.Kind, pkg: oldSym.Package}
		removedBySig[gk] = append(removedBySig[gk], key)
	}

	addedBySig := make(map[sigGroupKey][]symbolKey)
	for _, key := range s.unmatchedNew() {
		newSym := s.newByKey[key]
		gk := sigGroupKey{sig: newSym.Signature, kind: newSym.Kind, pkg: newSym.Package}
		addedBySig[gk] = append(addedBySig[gk], key)
	}

	for gk, oldKeys := range removedBySig {
		newKeys, ok := addedBySig[gk]
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
			s.typeRenames[oldSym.Name] = newSym.Name
		}
	}
}

// Pass 4: correlate methods and fields whose receiver/parent type was renamed.
func (s *diffState) correlateMethods() {
	if len(s.typeRenames) == 0 {
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

		newReceiver, ok := s.typeRenames[receiver]
		if !ok {
			continue
		}

		expectedNewName := newReceiver + "." + member
		expectedNewKey := symbolKey{pkg: oldSym.Package, kind: oldSym.Kind, name: expectedNewName}

		if _, unmatched := s.unmatchedNewSet[expectedNewKey]; !unmatched {
			continue
		}

		newSym := s.newByKey[expectedNewKey]

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
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].oldName < candidates[j].oldName
	})

	// Greedy matching.
	for _, pair := range candidates {
		if _, ok := s.unmatchedOldSet[pair.oldKey]; !ok {
			continue
		}
		if _, ok := s.unmatchedNewSet[pair.newKey]; !ok {
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
	for key := range s.unmatchedOldSet {
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
	totalA := len(a.params) + len(a.results)
	totalB := len(b.params) + len(b.results)

	if totalA == 0 && totalB == 0 {
		return 1.0
	}

	aSet := make(map[string]int, totalA)
	for _, t := range a.params {
		aSet[t]++
	}
	for _, t := range a.results {
		aSet[t]++
	}

	bSet := make(map[string]int, totalB)
	for _, t := range b.params {
		bSet[t]++
	}
	for _, t := range b.results {
		bSet[t]++
	}

	// Jaccard on multisets: intersection = sum of min counts, union = sum of max counts.
	var intersection, union int
	for k, ac := range aSet {
		bc := bSet[k]
		intersection += min(ac, bc)
		union += max(ac, bc)
	}
	for k, bc := range bSet {
		if _, ok := aSet[k]; !ok {
			union += bc
		}
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}
