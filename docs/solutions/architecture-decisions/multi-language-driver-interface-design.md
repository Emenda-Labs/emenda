---
title: "Multi-language driver interface design for dependency upgrade tool"
date: 2026-02-20
category: architecture-decisions
tags: [go, interfaces, multi-language, driver-pattern, type-separation]
module: core/driver, core/changespec, drivers/golang
symptoms:
  - Language-specific types leaking into shared interfaces
  - Interface methods too granular for cross-language abstraction
  - Go-specific concepts (receivers, import aliases) in core packages
---

# Multi-Language Driver Interface Design

## Problem

Emenda needs to support multiple programming languages (Go, Python, Rust, etc.) for
automated dependency upgrades. The initial interface design had 5 methods that exposed
Go-specific types (`Symbols`, `AliasMap`, `SymbolKind`) through the shared
`LanguageDriver` interface. This made it impossible for other languages to implement
the interface without importing Go-specific type definitions.

Specific issues:
- `ParseExports` returned `Symbols` which contained Go-specific `Receiver` field
- `ResolveImports` returned `AliasMap` which models Go's import aliasing
- `GenerateScript` and `ApplyScript` were tied to the rf execution model
- `SymbolKind` constants (`func`, `type`, `interface`) used Go terminology

## Root Cause

Designing the interface from a single language's perspective rather than identifying
the universal abstraction boundary. The pipeline was modeled as Go's internal processing
steps rather than as language-agnostic operations.

## Solution

### 1. Separate abstract types from language-specific types

**Universal types (core/changespec/):**
- `ChangeKind` -- renamed, removed, signature_changed, type_changed, package_moved
- `Change` -- single breaking API change
- `ChangeSpec` -- full set of changes between two versions
- `ApplyResult` -- which changes succeeded vs failed

**Go-specific types (drivers/golang/symbols/):**
- `Symbol`, `Symbols`, `SymbolKind`, `AliasMap`

### 2. Collapse interface to 3 universal methods

Before (5 methods, language-specific types exposed):
```go
type LanguageDriver interface {
    FetchSource(ctx, module, version) (path, cleanup, error)
    ParseExports(path, version) (Symbols, error)          // Go-specific return
    DiffExports(old, new Symbols) (ChangeSpec, error)      // Go-specific param
    ResolveImports(files) (AliasMap, error)                // Go-specific return
    ApplyChanges(ctx, spec, aliases, repoPath) (ApplyResult, error)  // Go-specific param
}
```

After (3 methods, only universal types):
```go
type LanguageDriver interface {
    FetchSource(ctx, module, version) (path, cleanup, error)
    ComputeChanges(ctx, oldPath, newPath, oldVersion, newVersion) (ChangeSpec, error)
    ApplyChanges(ctx, spec, files, repoPath) (ApplyResult, error)
}
```

`ComputeChanges` encapsulates ParseExports + DiffExports internally.
`ApplyChanges` encapsulates ResolveImports + script generation + application internally.

### 3. Use pkg/ for shared reusable code

Reusable packages that cross the core/driver boundary live in `pkg/`:
- `pkg/goproxy/` -- HTTP client for Go module proxy
- `pkg/gomod/` -- go.mod file parser
- `pkg/archive/` -- zip extraction with security protections

This avoids the architectural constraint that `core/` cannot import from `drivers/`.

## Key Design Principles

1. **The interface boundary should only expose universal concepts.** If a type name
   doesn't make sense for Python or Rust, it doesn't belong in the interface.

2. **Fewer, coarser methods beat many fine-grained ones.** The orchestrator doesn't
   need to control each step of the driver's internal pipeline.

3. **Language-specific types are the driver's private concern.** The Go driver uses
   `Symbols` and `AliasMap` internally but never exposes them upward.

4. **ApplyResult maps to PR sections.** `Applied` = auto-fixed, `Failed` = needs AI.
   This is universal across all languages.

## Prevention

When adding new interface methods or types to shared packages:
- Ask: "Would this make sense for a Python driver? A Rust driver?"
- If a type uses language-specific terminology, it belongs in `drivers/{lang}/`
- If an interface method requires language-specific params, consider collapsing it
  into a coarser method that hides the internals

## File References

- `core/driver/interfaces.go` -- 3-method LanguageDriver interface
- `core/changespec/types.go` -- universal types only
- `drivers/golang/symbols/types.go` -- Go-specific types
- `drivers/golang/driver.go` -- Go driver implementing the interface
