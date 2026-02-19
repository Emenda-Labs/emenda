# Emenda

Automated dependency upgrade tool that fixes breaking API changes across codebases.
Currently supports Go. Designed for multi-language expansion.

## Architecture

emenda/
├── cmd/emenda/ # CLI entrypoint + wiring
├── core/
│ ├── cli/ # cobra command definitions
│ ├── changespec/ # shared types (ChangeSpec, Symbols, AliasMap)
│ ├── driver/ # LanguageDriver + RepoResolver interfaces
│ ├── intersect/ # impact intersection: files × change spec
│ ├── ai/ # AI agent orchestration
│ └── pr/ # git branch + PR creation
├── drivers/
│ └── golang/ # Go language driver
│ ├── astdiff/ # AST diff between two module versions
│ ├── resolver/ # import alias resolver
│ └── scriptgen/ # change spec → rf script generator
├── pkg/
│ ├── goproxy/ # Go module proxy HTTP client (GOPROXY-aware)
│ ├── gomod/ # go.mod parser (version detection)
│ └── archive/ # zip extraction with zip-slip protection
├── resolvers/
│ ├── gomodules/ # recursive .go file scanner
│ └── bazel/ # bazel query + BUILD parser (future)

## Key Concepts

- **Language Driver**: interface each language implements (FetchSource, ParseExports, DiffExports, ResolveImports, GenerateScript, ApplyScript)
- **Repo Resolver**: interface for finding affected files (GoModules or Bazel)
- **pkg/**: reusable packages shared across drivers and core (goproxy, gomod, archive)
- **rf**: forked rsc/rf — execution engine that applies rf scripts and writes files to disk (will not be in this project repo) (git@github.com:Emenda-Labs/emenda-rf.git)
- **Path 1**: known patterns → Script Generator → rf → writes files
- **Path 2**: unknown patterns → AI Agent → generates rf script → rf → writes files (last step, will do in future)

## Hard Rules

- **No emojis** in code, logs, or status messages
- **Go 1.26.0** exists
- **No magic numbers** - use YAML config files or named constants
- **No step numbers in comments** - describe what the code does, not sequence
- **Comments in English** only
- **Always use Context7 MCP** when I need library/API documentation, code generation, setup or configuration steps without me having to explicitly ask.
- **Always use const or yaml** for values that are not self-explanatory
- **snake_case JSON** - all API fields use `json:"snake_case"` tags, never camelCase
- **Business errors** return HTTP status **467** (custom), server errors return 500
- **No duplicate functionality** - search codebase first, reuse or extend existing code
- **NEVER run `git push` or `git commit`** - user handles git themselves
- **Always update Documentation if we added new feature** - for All new features

## Documentation Requirements

- Update `BACKLOG.md` when closing issues listed there

### Issue Tracking

- Use `BACKLOG.md` for all issues (never create `todos/` directory)
- Use existing table format with P1/P2/P3 priority sections

## Module versions fetched via Go module proxy

Respects GOPROXY environment variable. Falls back to https://proxy.golang.org,direct if unset.

No git cloning. No GitHub API. Two zips downloaded, unpacked to temp dir, AST diffed.

## Build & Run

go build ./cmd/emenda
go test ./...

## CLI Usage

emenda upgrade go \
 --module github.com/acme/foo \
 --to v2.3.0 \
 --repo . \
 --dry-run

## Key Interfaces

Language Driver (core/driver/interfaces.go):
FetchSource(ctx, module, version) (path, cleanup, error)
ComputeChanges(ctx, oldPath, newPath, oldVersion, newVersion) (ChangeSpec, error)
ApplyChanges(ctx, spec, files, repoPath) (ApplyResult, error)

Go-specific types (drivers/golang/symbols/):
Symbol, Symbols, SymbolKind, AliasMap — internal to Go driver

Repo Resolver (core/driver/interfaces.go):
FindAffectedFiles(module, repoPath string) ([]string, error)

## Conventions

- All language-specific code lives inside drivers/{language}/
- core/ and resolvers/ must not import from drivers/
- rf/ is treated as external dependency
- Change Spec types defined in core/changespec/ package, shared across drivers
- Reusable packages live in pkg/ (goproxy, gomod, archive)
- AI agents receive: single file + broken symbols + change spec only — no extra context (future)
- Commits are separated: mechanical (path 1) and AI-generated (path 2)
- PR description always has three sections: auto-fixed, AI-fixed, TODO

## What rf does and does not do

rf applies a script of refactoring commands and writes changes directly to disk.
rf does NOT return suggestions — it writes files.
AI agents in path 2 generate rf scripts (not raw code) which rf then applies.

## License

Core: open source (Apache 2.0)
rf fork: BSD 3-Clause (must retain Russ Cox copyright notice)
Commercial features: proprietary
