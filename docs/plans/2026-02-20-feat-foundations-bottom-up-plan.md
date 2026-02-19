---
title: "feat: Bottom-up foundations for emenda"
type: feat
date: 2026-02-20
brainstorm: docs/brainstorms/2026-02-20-foundations-brainstorm.md
---

# Bottom-Up Foundations for Emenda

## Overview

Build the foundational layer of emenda from the ground up: shared types, interfaces,
a GOPROXY-aware module proxy client, go.mod version detection, and a cobra CLI skeleton.
No tests in this phase. Focus on getting the structure, types, and wiring right.

**End state:** `emenda upgrade go --module github.com/acme/foo --to v2.3.0 --repo . --dry-run`
parses flags, detects the current version from the target repo's go.mod, downloads both
module versions from the Go module proxy, unpacks them, and prints a summary of what was
detected.

## Problem Statement / Motivation

Emenda has a complete architecture spec and zero code. The project needs a solid
foundation that the AST diff engine, script generator, and rf integration will build on.
Getting the types, interfaces, and package boundaries right now prevents costly rework later.

## Directory Structure

```
emenda/
├── cmd/emenda/              # CLI entrypoint + wiring
│   └── main.go
├── core/
│   ├── cli/                 # cobra command definitions
│   │   ├── root.go
│   │   ├── upgrade.go
│   │   └── upgrade_go.go
│   ├── changespec/          # shared types across all drivers
│   │   └── types.go
│   └── driver/              # LanguageDriver + RepoResolver interfaces
│       └── interfaces.go
├── drivers/
│   └── golang/              # Go language driver (skeleton)
│       └── driver.go
├── pkg/
│   ├── goproxy/             # Go module proxy HTTP client
│   │   └── client.go
│   ├── gomod/               # go.mod parser
│   │   └── parser.go
│   └── archive/             # zip download + extraction
│       └── zip.go
├── go.mod
├── go.sum
├── CLAUDE.md
└── BACKLOG.md
```

## Technical Approach

### Phase 1: Project Scaffold + go.mod

Create the Go module and directory structure.

**Deliverables:**
- `go.mod` targeting Go 1.26.0, module path `github.com/emenda-labs/emenda`
- Add `github.com/spf13/cobra` dependency
- Add `github.com/emenda-labs/emenda-rf` dependency (from git@github.com:Emenda-Labs/emenda-rf.git)
- Add `golang.org/x/mod` dependency (for go.mod parsing)
- All directories created with placeholder or real files
- `.gitignore` already exists

**Files:**
- `go.mod`

---

### Phase 2: Shared Types (core/changespec/)

Define the ChangeSpec, Symbols, and AliasMap types that every driver and core package depends on.

**Deliverables in `core/changespec/types.go`:**

```go
// ChangeKind represents the type of breaking API change.
type ChangeKind string

const (
    ChangeKindRenamed          ChangeKind = "renamed"
    ChangeKindSignatureChanged ChangeKind = "signature_changed"
    ChangeKindRemoved          ChangeKind = "removed"
    ChangeKindTypeChanged      ChangeKind = "type_changed"
    ChangeKindPackageMoved     ChangeKind = "package_moved"
)
```

**Symbol types:**

```go
// SymbolKind identifies what kind of exported symbol this is.
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

// Symbol represents a single exported symbol from a module.
type Symbol struct {
    Name      string     `json:"name"`
    Kind      SymbolKind `json:"kind"`
    Package   string     `json:"package"`
    Receiver  string     `json:"receiver,omitempty"`  // for methods
    Signature string     `json:"signature,omitempty"` // full signature string
}

// Symbols is the full set of exports from a module version.
type Symbols struct {
    Module  string   `json:"module"`
    Version string   `json:"version"`
    Entries []Symbol `json:"entries"`
}
```

**ChangeSpec types:**

```go
// Change represents a single breaking API change between two versions.
type Change struct {
    Kind         ChangeKind `json:"kind"`
    Symbol       string     `json:"symbol"`
    Package      string     `json:"package"`
    OldSignature string     `json:"old_signature,omitempty"`
    NewSignature string     `json:"new_signature,omitempty"`
    NewName      string     `json:"new_name,omitempty"`
    NewPackage   string     `json:"new_package,omitempty"`
}

// ChangeSpec is the full set of breaking changes between two module versions.
type ChangeSpec struct {
    Module     string   `json:"module"`
    OldVersion string   `json:"old_version"`
    NewVersion string   `json:"new_version"`
    Changes    []Change `json:"changes"`
}
```

**AliasMap type (per-file import aliases):**

```go
// AliasMap maps file paths to their import aliases.
// Key: file path, Value: map of alias -> package import path.
// If a file uses the default import (no alias), the alias is the package name.
type AliasMap map[string]map[string]string
```

**Files:**
- `core/changespec/types.go`

---

### Phase 3: Interfaces (core/driver/)

Define the LanguageDriver and RepoResolver interfaces.

**Deliverables in `core/driver/interfaces.go`:**

```go
// LanguageDriver is the interface each language must implement.
type LanguageDriver interface {
    // FetchSource downloads module source and unpacks it to a local directory.
    // Returns the path to the unpacked source and a cleanup function.
    FetchSource(ctx context.Context, module, version string) (path string, cleanup func(), err error)

    // ParseExports extracts all exported symbols from the unpacked module source.
    ParseExports(path, version string) (changespec.Symbols, error)

    // DiffExports computes the breaking changes between two sets of symbols.
    DiffExports(old, new changespec.Symbols) (changespec.ChangeSpec, error)

    // ResolveImports finds import aliases for the given files.
    ResolveImports(files []string) (changespec.AliasMap, error)

    // GenerateScript produces an rf script from a change spec and alias map.
    GenerateScript(spec changespec.ChangeSpec, aliases changespec.AliasMap) (string, error)

    // ApplyScript runs the rf script against the repository.
    ApplyScript(script, repoPath string) error
}

// RepoResolver finds files affected by a module upgrade.
type RepoResolver interface {
    // FindAffectedFiles returns all files that import the given module.
    // Matches any import path that starts with the module path (prefix match).
    FindAffectedFiles(module, repoPath string) ([]string, error)
}
```

**Files:**
- `core/driver/interfaces.go`

---

### Phase 4: Go Module Proxy Client (pkg/goproxy/)

HTTP client that downloads module zips from the Go module proxy.

**Key decisions:**
- Respects `GOPROXY` environment variable (falls through proxy chain like `go` tooling)
- Falls back to `https://proxy.golang.org,direct` if `GOPROXY` is not set
- 30-second timeout per request
- User-Agent: `emenda/<version>`
- Accepts `context.Context` for cancellation

**Deliverables in `pkg/goproxy/client.go`:**

```go
// Client downloads module source from the Go module proxy.
type Client struct {
    httpClient *http.Client
    userAgent  string
    proxies    []string // parsed from GOPROXY
}

// NewClient creates a proxy client that respects GOPROXY env var.
func NewClient() *Client

// DownloadZip downloads the module zip for the given module and version.
// Returns the raw zip bytes.
func (c *Client) DownloadZip(ctx context.Context, module, version string) ([]byte, error)
```

**Proxy chain logic:**
- Parse `GOPROXY` env var (comma or pipe separated)
- Try each proxy in order
- `direct` means skip proxy (not supported in phase 1, log warning)
- `off` means fail
- `noproxy` entries from `GONOPROXY` are respected
- Default: `https://proxy.golang.org,direct`

**Error handling:**
- 404: "version %s of %s not found on %s"
- 410 (Gone): same as 404
- Network errors: include proxy URL in error message
- Non-2xx: "unexpected status %d from %s"

**Files:**
- `pkg/goproxy/client.go`

---

### Phase 5: Zip Extraction (pkg/archive/)

Unpack module zip files to temp directories with zip-slip protection.

**Deliverables in `pkg/archive/zip.go`:**

```go
// ExtractZip unpacks a module zip to a temp directory.
// Returns the path to the extracted directory and a cleanup function.
// Validates all paths against zip-slip (path traversal) attacks.
func ExtractZip(data []byte, prefix string) (dir string, cleanup func(), err error)
```

**Zip-slip protection:**
- Resolve each file path after joining with the target directory
- Verify the resolved path starts with the target directory
- Reject any entry with `..` components or absolute paths

**Temp directory:**
- Use `os.MkdirTemp("", "emenda-"+prefix+"-*")`
- Cleanup function calls `os.RemoveAll` on the temp dir

**Files:**
- `pkg/archive/zip.go`

---

### Phase 6: go.mod Parser (pkg/gomod/)

Parse a go.mod file to extract the current version of a given module.

**Deliverables in `pkg/gomod/parser.go`:**

```go
// FindModuleVersion reads the go.mod at the given repo path and returns
// the version of the specified module. Returns an error if the module
// is not found in the require directives.
func FindModuleVersion(repoPath, module string) (version string, err error)
```

**Implementation:**
- Use `golang.org/x/mod/modfile` to parse go.mod
- Search both `Require` entries (handles both single-line and block syntax)
- Exact match on module path (user must provide full path including /v2 suffix)
- Check for `Replace` directives on the module -- if found, print a warning to stderr
  but return the version from the `Require` line
- Handle indirect deps the same as direct deps

**Error cases:**
- No go.mod at path: "no go.mod found at %s"
- Malformed go.mod: "failed to parse go.mod: %s"
- Module not found: "module %s not found in go.mod at %s"

**Files:**
- `pkg/gomod/parser.go`

---

### Phase 7: Go Driver Skeleton (drivers/golang/)

Minimal Go driver that implements FetchSource using the proxy client and archive packages.
Other methods return "not implemented" errors.

**Deliverables in `drivers/golang/driver.go`:**

```go
// Driver implements core/driver.LanguageDriver for Go.
type Driver struct {
    proxyClient *goproxy.Client
}

func NewDriver() *Driver

// FetchSource downloads and unpacks a Go module from the proxy.
func (d *Driver) FetchSource(ctx context.Context, module, version string) (string, func(), error)

// ParseExports - not implemented in this phase.
func (d *Driver) ParseExports(path, version string) (changespec.Symbols, error)

// DiffExports - not implemented in this phase.
func (d *Driver) DiffExports(old, new changespec.Symbols) (changespec.ChangeSpec, error)

// ResolveImports - not implemented in this phase.
func (d *Driver) ResolveImports(files []string) (changespec.AliasMap, error)

// GenerateScript - not implemented in this phase.
func (d *Driver) GenerateScript(spec changespec.ChangeSpec, aliases changespec.AliasMap) (string, error)

// ApplyScript - not implemented in this phase.
func (d *Driver) ApplyScript(script, repoPath string) error
```

**Files:**
- `drivers/golang/driver.go`

---

### Phase 8: CLI Skeleton (core/cli/)

Cobra-based CLI with `upgrade go` subcommand.

**Command structure:**
```
emenda
├── upgrade
│   └── go  --module  --to  --repo  [--dry-run]
└── --version
```

**Deliverables:**

`core/cli/root.go`:
```go
// NewRootCmd creates the top-level emenda command.
func NewRootCmd(version string) *cobra.Command
```

`core/cli/upgrade.go`:
```go
// NewUpgradeCmd creates the "upgrade" parent command.
func NewUpgradeCmd() *cobra.Command
```

`core/cli/upgrade_go.go`:
```go
// UpgradeGoOptions holds the parsed flags for "upgrade go".
type UpgradeGoOptions struct {
    Module string
    To     string
    Repo   string
    DryRun bool
}

// NewUpgradeGoCmd creates the "upgrade go" subcommand.
// RunFunc is injected by the wiring layer (cmd/emenda/main.go).
func NewUpgradeGoCmd(runFunc func(ctx context.Context, opts UpgradeGoOptions) error) *cobra.Command
```

**Flag validation (in PreRunE):**
- `--module` required, non-empty
- `--to` required, non-empty, must start with `v`
- `--repo` required, must be an existing directory

**Version comparison (in RunE, before download):**
- Same version: error "module %s is already at %s"
- Downgrade (target < current): warning to stderr, proceed

**Files:**
- `core/cli/root.go`
- `core/cli/upgrade.go`
- `core/cli/upgrade_go.go`

---

### Phase 9: Wiring + Dry-Run Output (cmd/emenda/)

Connect all pieces in main.go. Implement the upgrade-go run function.

**Deliverables in `cmd/emenda/main.go`:**

```go
func main() {
    // Create dependencies
    goDriver := golang.NewDriver()

    // Create run function that wires everything together
    runUpgradeGo := func(ctx context.Context, opts cli.UpgradeGoOptions) error {
        // 1. Parse go.mod for current version
        // 2. Version comparison (same = error, downgrade = warning)
        // 3. FetchSource for old version via driver
        // 4. FetchSource for new version via driver
        // 5. Print dry-run summary (or same output for non-dry-run in this phase)
        // 6. Cleanup temp dirs
    }

    // Build command tree
    root := cli.NewRootCmd(version)
    upgradeCmd := cli.NewUpgradeCmd()
    upgradeCmd.AddCommand(cli.NewUpgradeGoCmd(runUpgradeGo))
    root.AddCommand(upgradeCmd)

    // Execute
    root.ExecuteContext(context.Background())
}
```

**Dry-run output format (human-readable):**
```
Module:          github.com/acme/foo
Current version: v2.1.0
Target version:  v2.3.0
Old source:      /tmp/emenda-old-xxxxx/github.com/acme/foo@v2.1.0
New source:      /tmp/emenda-new-xxxxx/github.com/acme/foo@v2.3.0

[dry-run] No changes applied.
```

Without `--dry-run`, same output but last line reads:
```
[no-op] Change application not yet implemented.
```

**Signal handling:**
- Use `signal.NotifyContext` for SIGINT/SIGTERM
- Pass context to all proxy client calls
- Defer cleanup functions from FetchSource

**Files:**
- `cmd/emenda/main.go`

---

## Acceptance Criteria

- [ ] `go build ./cmd/emenda` compiles without errors
- [ ] `emenda --version` prints the version
- [ ] `emenda upgrade go --module X --to Y --repo .` parses all flags correctly
- [ ] Missing required flags produce clear error messages with usage help
- [ ] Current version is auto-detected from go.mod at the repo path
- [ ] Module not found in go.mod produces a clear error
- [ ] Replace directives on the target module trigger a warning to stderr
- [ ] Both module versions are downloaded from the Go module proxy
- [ ] GOPROXY env var is respected when set
- [ ] Zip extraction includes zip-slip protection
- [ ] `--dry-run` prints the detected versions and downloaded paths
- [ ] Same version as target produces an error
- [ ] Downgrade (target < current) prints a warning but proceeds
- [ ] Ctrl+C during download cleans up temp directories
- [ ] All JSON struct tags use snake_case
- [ ] No emojis in code, logs, or output

## Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `golang.org/x/mod/modfile` | go.mod parsing |
| `golang.org/x/mod/semver` | Version comparison |
| `github.com/emenda-labs/emenda-rf` | rf execution engine (wired but not called in this phase) |

## What This Does NOT Include

- AST diff engine (next phase -- `drivers/golang/astdiff/`)
- Import alias resolution (next phase -- `drivers/golang/resolver/`)
- Script generation (next phase -- `drivers/golang/scriptgen/`)
- rf script execution (next phase)
- RepoResolver / GoModules resolver implementation (next phase)
- AI agent path (future)
- PR creation (future)
- Bazel resolver (future)
- Tests (next phase)
- `--verbose` / `--json` output flags (future)
- GOPROXY `direct` mode (log warning, skip)

## References

- Brainstorm: `docs/brainstorms/2026-02-20-foundations-brainstorm.md`
- Architecture: `CLAUDE.md`
- Go module proxy spec: https://go.dev/ref/mod#module-proxy
- GOPROXY docs: https://go.dev/ref/mod#environment-variables
