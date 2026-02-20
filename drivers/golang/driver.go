package golang

import (
	"context"
	"fmt"

	"github.com/emenda-labs/emenda/core/changespec"
	"github.com/emenda-labs/emenda/core/driver"
	"github.com/emenda-labs/emenda/drivers/golang/astdiff"
	"github.com/emenda-labs/emenda/pkg/archive"
	"github.com/emenda-labs/emenda/pkg/gomod"
	"github.com/emenda-labs/emenda/pkg/goproxy"
)

var _ driver.LanguageDriver = (*Driver)(nil)

// Driver implements driver.LanguageDriver for Go modules.
type Driver struct {
	proxyClient *goproxy.Client
}

// NewDriver creates a Driver with a default goproxy.Client.
func NewDriver() *Driver {
	return &Driver{
		proxyClient: goproxy.NewClient(),
	}
}

// FetchSource downloads the module zip from the proxy and extracts it to a temp directory.
func (d *Driver) FetchSource(ctx context.Context, module, version string) (string, func(), error) {
	data, err := d.proxyClient.DownloadZip(ctx, module, version)
	if err != nil {
		return "", nil, fmt.Errorf("downloading zip for %s@%s: %w", module, version, err)
	}

	dir, cleanup, err := archive.ExtractZip(data, version)
	if err != nil {
		return "", nil, fmt.Errorf("extracting zip for %s@%s: %w", module, version, err)
	}

	return dir, cleanup, nil
}

// ComputeChanges diffs two unpacked Go module versions.
// Internally parses exports from both versions and computes the diff.
func (d *Driver) ComputeChanges(ctx context.Context, oldPath, newPath, oldVersion, newVersion string) (changespec.ChangeSpec, error) {
	oldRoot, err := astdiff.FindSourceRoot(oldPath)
	if err != nil {
		return changespec.ChangeSpec{}, fmt.Errorf("finding module root in %s: %w", oldVersion, err)
	}

	module, err := gomod.FindModulePath(oldRoot)
	if err != nil {
		return changespec.ChangeSpec{}, fmt.Errorf("reading module path from %s: %w", oldVersion, err)
	}

	newRoot, err := astdiff.FindSourceRoot(newPath)
	if err != nil {
		return changespec.ChangeSpec{}, fmt.Errorf("finding module root in %s: %w", newVersion, err)
	}

	// Validate both zips contain the same module.
	newModule, err := gomod.FindModulePath(newRoot)
	if err != nil {
		return changespec.ChangeSpec{}, fmt.Errorf("reading module path from %s: %w", newVersion, err)
	}
	if module != newModule {
		return changespec.ChangeSpec{}, fmt.Errorf("module mismatch: old=%s new=%s", module, newModule)
	}

	old, oldSigs, err := astdiff.ParseExports(ctx, oldRoot, module)
	if err != nil {
		return changespec.ChangeSpec{}, fmt.Errorf("parsing exports from %s: %w", oldVersion, err)
	}

	new, newSigs, err := astdiff.ParseExports(ctx, newRoot, module)
	if err != nil {
		return changespec.ChangeSpec{}, fmt.Errorf("parsing exports from %s: %w", newVersion, err)
	}

	changes := astdiff.DiffExports(old, new, oldSigs, newSigs)

	return changespec.ChangeSpec{
		Module:     module,
		OldVersion: oldVersion,
		NewVersion: newVersion,
		Changes:    changes,
	}, nil
}

// ApplyChanges applies breaking change fixes to Go source files.
// Internally resolves import aliases and uses rf to apply changes.
func (d *Driver) ApplyChanges(ctx context.Context, spec changespec.ChangeSpec, files []string, repoPath string) (changespec.ApplyResult, error) {
	return changespec.ApplyResult{}, fmt.Errorf("not implemented")
}
