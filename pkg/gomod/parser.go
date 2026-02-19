package gomod

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// FindModuleVersion reads the go.mod at the given repo path and returns
// the version of the specified module. Returns an error if the module
// is not found in the require directives.
// If the module has a replace directive, a warning is printed to stderr
// but the version from the require line is still returned.
func FindModuleVersion(repoPath, module string) (string, error) {
	gomodPath := filepath.Join(repoPath, "go.mod")

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no go.mod found at %s", gomodPath)
		}
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}

	f, err := modfile.Parse(gomodPath, data, nil)
	if err != nil {
		return "", fmt.Errorf("failed to parse go.mod: %w", err)
	}

	// Check for replace directives targeting this module
	for _, rep := range f.Replace {
		if rep.Old.Path == module {
			fmt.Fprintf(os.Stderr, "warning: module %s has a replace directive (%s -> %s), proxy version may differ from local source\n",
				module, rep.Old.Path, rep.New.Path)
			break
		}
	}

	// Search require directives for the module (exact match)
	for _, req := range f.Require {
		if req.Mod.Path == module {
			return req.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("module %s not found in go.mod at %s", module, gomodPath)
}
