# Foundations Brainstorm

**Date:** 2026-02-20
**Status:** Decided
**Scope:** Bottom-up foundations for emenda v0.1

## What We're Building

The foundational layer of emenda: shared types, interfaces, a real Go proxy client,
go.mod version detection, and a CLI skeleton. No tests in this phase -- focus on
getting the structure right and wiring the pieces together.

**End state:** `emenda upgrade go --module X --to Y --repo . --dry-run` parses flags,
detects the current version from the target repo's go.mod, downloads both module
versions from proxy.golang.org, and prints what would change.

## Key Decisions

### 1. Build order: Types-up

Build in dependency order -- each layer is usable before the next one starts:

1. `go.mod` + project directory structure
2. `changespec/` package (ChangeSpec, Symbols, AliasMap types)
3. Language Driver + Repo Resolver interfaces
4. Go proxy client (real HTTP download + zip unpack)
5. go.mod version parser (auto-detect current version)
6. CLI skeleton with cobra
7. Wire CLI -> proxy client -> types

### 2. ChangeSpec: full breaking changes from day one

ChangeSpec covers the complete set of breaking API changes:
- Function/type/method renames
- Signature changes (params added/removed/reordered)
- Removed exports
- Type changes
- Moved packages

No incremental expansion needed -- design the types to handle all cases upfront.

### 3. CLI structure: action first, language as sub-subcommand

```
emenda upgrade go --module github.com/acme/foo --to v2.3.0 --repo . [--dry-run]
```

- `upgrade` is the top-level subcommand (primary action)
- `go` is a sub-subcommand under upgrade (language qualifier)
- Future languages register as new sub-subcommands: `upgrade python`, `upgrade rust`
- Each language sub-subcommand has its own flag set (Go uses --module, Python might use --package)
- Uses cobra framework

### 4. No --from flag

Current version is always auto-detected from the target repo's go.mod.
No override capability. Keeps the CLI simple and avoids user error.

### 5. rf as Go module dependency

emenda-rf (git@github.com:Emenda-Labs/emenda-rf.git) is added as a Go module
dependency from the start. ApplyScript calls rf directly, no shell exec wrapper.

### 6. Go proxy client: fully implemented

Real HTTP client that downloads module zips from `https://proxy.golang.org/{module}/@v/{version}.zip`,
unpacks to a temp directory. No stubs -- the real implementation from the start.

### 7. No tests in this phase

Focus on getting the architecture and types right. Tests come in the next phase
when we build the AST diff engine and have real data to validate against.

## Build Phases (Ordered)

| Phase | Package | Deliverable |
|-------|---------|-------------|
| 1 | root | go.mod, directory scaffold |
| 2 | changespec/ | ChangeSpec, Symbols, AliasMap, ChangeKind types |
| 3 | core/ | LanguageDriver interface, RepoResolver interface |
| 4 | drivers/golang/proxy/ | Go module proxy HTTP client, zip download + unpack |
| 5 | core/gomod/ | go.mod parser for current version detection |
| 6 | cmd/emenda/, core/cli/ | cobra CLI: upgrade go --module --to --repo --dry-run |
| 7 | cmd/emenda/ | Wire CLI -> proxy client -> types, dry-run output |

## Open Questions

- What rf commands/syntax does scriptgen need to produce? (Deferred to scriptgen phase)
- What does the dry-run output look like exactly? (Decide during CLI implementation)
- How to handle modules with no proxy.golang.org availability? (Edge case, handle later)

## What This Does NOT Include

- AST diff engine (next phase)
- Import alias resolution (next phase)
- Script generation (next phase)
- rf script execution (next phase)
- AI agent path (future)
- PR creation (future)
- Bazel resolver (future)
- Tests (next phase)
