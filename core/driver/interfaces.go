package driver

import (
	"context"

	"github.com/emenda-labs/emenda/core/changespec"
)

// LanguageDriver is the interface each language must implement to support
// automated dependency upgrades.
type LanguageDriver interface {
	// FetchSource downloads module source and unpacks it to a local directory.
	// Returns the path to the unpacked source and a cleanup function that
	// removes the temp directory.
	FetchSource(ctx context.Context, module, version string) (path string, cleanup func(), err error)

	// ComputeChanges diffs two unpacked module versions and returns the
	// breaking changes between them. Internally handles parsing exports
	// and computing the diff using language-specific logic.
	ComputeChanges(ctx context.Context, oldPath, newPath, oldVersion, newVersion string) (changespec.ChangeSpec, error)

	// ApplyChanges applies breaking change fixes to the affected files in the repository.
	// The driver decides the mechanism (rf scripts, codemods, AST rewriting, etc.).
	// Internally handles import alias resolution.
	// Returns which changes were applied and which failed.
	ApplyChanges(ctx context.Context, spec changespec.ChangeSpec, files []string, repoPath string) (changespec.ApplyResult, error)
}

// RepoResolver finds files affected by a module upgrade.
type RepoResolver interface {
	// FindAffectedFiles returns all files that import the given module.
	// Uses prefix matching: any import path starting with the module path is a match.
	FindAffectedFiles(module, repoPath string) ([]string, error)
}
