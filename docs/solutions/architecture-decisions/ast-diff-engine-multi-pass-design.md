---
title: AST Diff Engine — Multi-Pass Design
category: architecture-decisions
tags: [go, ast, diff, symbols, confidence, breaking-changes]
module: drivers/golang/astdiff
date: 2026-02-20
---

# AST Diff Engine — Multi-Pass Design

## Problem

Given two versions of a Go module (unpacked from proxy zips), detect all breaking API changes and classify each with a confidence level that drives downstream routing:
- **HIGH** → mechanical fix via rf script (Path 1)
- **MEDIUM** → AI agent generates rf script (Path 2)
- **LOW** → listed as TODO in PR description

The core tension: a wrong rename at HIGH confidence silently generates wrong code. An honest LOW is always safer than a wrong HIGH.

## Solution

### Architecture: 3 files, single package

```
drivers/golang/astdiff/
    parse.go      — ParseExports: AST walk + symbol collection
    signature.go  — renderTypeExpr + signature helpers
    diff.go       — diffState + 6 passes + similarity helpers
```

### Key Pattern: diffState with Per-Pass Methods

Instead of a single large diff function, use a shared state struct with methods per pass:

```go
type diffState struct {
    oldByKey   map[symbolKey]*symbols.Symbol
    newByKey   map[symbolKey]*symbols.Symbol
    matchedOld map[symbolKey]bool
    matchedNew map[symbolKey]bool
    oldSigs    FuncSigMap
    newSigs    FuncSigMap
    changes    []changespec.Change
}

func DiffExports(...) []changespec.Change {
    s := newDiffState(...)
    s.exactMatch()          // Pass 1
    s.changed()             // Pass 2
    s.renamed()             // Pass 3
    s.correlateMethods()    // Pass 4
    s.fuzzyMatch()          // Pass 5
    s.leftovers()           // Pass 6
    return s.changes
}
```

Benefits: each pass is independently testable, state flows naturally, easy to insert new passes.

### Key Pattern: FuncSigMap Caching

Structured function signatures (`funcSignature{params, results []string}`) are built during `ParseExports` and passed to `DiffExports`. This avoids re-parsing rendered signature strings back into structured form for Pass 5 fuzzy matching.

```go
type FuncSigMap map[symbolKey]funcSignature

func ParseExports(rootDir, module string) (symbols.Symbols, FuncSigMap, error)
```

### Key Pattern: Trivial Signature Guard

Pass 3 (exact-signature rename) requires non-trivial signatures. Without this guard, `OldInit()` and `NewSetup()` would match as HIGH confidence renames since both have `"()"` signature — clearly wrong.

```go
if oldSym.Signature == "" || oldSym.Signature == "()" {
    continue // fall to Pass 5 fuzzy or Pass 6 removed
}
```

### Key Pattern: Type Rename Correlation (Pass 4)

When a type is renamed (e.g., `Client` → `HTTPClient`), all its methods and fields would otherwise appear as N separate removals at LOW confidence. Pass 4 detects this pattern:

1. Pass 3 discovers type rename `Client → HTTPClient`
2. Pass 4 re-keys unmatched methods: `Client.Do` → look for `HTTPClient.Do`
3. If found with same signature → `renamed`, HIGH
4. If found with different signature → `signature_changed`, HIGH

### Key Pattern: Cross-Kind Detection

A symbol changing kind (e.g., `var ComputeHash string` → `func ComputeHash(string) string`) is detected via a secondary `nameKey{pkg, name}` index in Pass 2, emitted as `type_changed`, HIGH.

### Key Pattern: Short Name Guard

Names shorter than 4 characters require higher similarity (0.85 vs 0.7) in Pass 5 fuzzy matching. This prevents `Get` → `Set` (similarity 0.67) from matching as a rename.

## Gotchas

1. **Collision handling in Pass 3**: When multiple old symbols share the same signature, do NOT pick one arbitrarily at HIGH confidence. Fall through to Pass 5 (MEDIUM) where name similarity can disambiguate.

2. **Zero-param overlap**: When both functions have no params and no results, `paramOverlap` must return 1.0 (vacuously true), not 0/0. Otherwise no-arg renames always fall to LOW.

3. **Zip path prefix**: Go module proxy zips extract to `tmpDir/module@version/`. `FindSourceRoot` must walk for `go.mod` rather than assuming the root is the extraction directory.

4. **Module validation**: Always compare module paths from old and new `go.mod` before diffing. A mismatch means the wrong zips were fetched.

5. **Non-function symbols cannot fuzzy match**: Types, consts, vars have no structured signature for param overlap. They correctly fall to Pass 6 (removed, LOW) if not matched earlier.

## Test Strategy

- **Table-driven tests** with stdlib `testing` only
- **Per-pass unit tests** via `diffState` methods with crafted symbol sets
- **Integration tests** through `DiffExports` with fixture Go modules in `testdata/`
- **Fixture modules** include intentional edge cases: syntax errors, `package main`, `internal/`, unexported receivers, cross-kind changes

## References

- Brainstorm: `docs/brainstorms/2026-02-20-ast-diff-engine-brainstorm.md`
- Plan: `docs/plans/2026-02-20-feat-ast-diff-engine-plan.md`
- Implementation: `drivers/golang/astdiff/`
- Driver wiring: `drivers/golang/driver.go`
