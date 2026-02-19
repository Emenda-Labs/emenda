package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// UpgradeGoOptions holds the parsed flags for "upgrade go".
type UpgradeGoOptions struct {
	Module string
	To     string
	Repo   string
	DryRun bool
}

// UpgradeGoRunFunc is the function signature for the upgrade go command handler.
// It is injected by the wiring layer (cmd/emenda/main.go).
type UpgradeGoRunFunc func(ctx context.Context, opts UpgradeGoOptions) error

// NewUpgradeGoCmd creates the "upgrade go" subcommand.
func NewUpgradeGoCmd(runFunc UpgradeGoRunFunc) *cobra.Command {
	var opts UpgradeGoOptions

	cmd := &cobra.Command{
		Use:   "go",
		Short: "Upgrade a Go module dependency",
		Long:  "Upgrade a Go module to a new version and fix breaking API changes.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateUpgradeGoFlags(opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFunc(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.Module, "module", "", "Go module path to upgrade (required)")
	cmd.Flags().StringVar(&opts.To, "to", "", "Target version to upgrade to (required)")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Path to the repository (required)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would change without applying")

	cmd.MarkFlagRequired("module")
	cmd.MarkFlagRequired("to")
	cmd.MarkFlagRequired("repo")

	return cmd
}

func validateUpgradeGoFlags(opts UpgradeGoOptions) error {
	if opts.Module == "" {
		return fmt.Errorf("--module is required")
	}
	if opts.To == "" {
		return fmt.Errorf("--to is required")
	}
	if !strings.HasPrefix(opts.To, "v") {
		return fmt.Errorf("--to version must start with 'v' (e.g. v2.3.0)")
	}

	info, err := os.Stat(opts.Repo)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("repo path does not exist: %s", opts.Repo)
		}
		return fmt.Errorf("cannot access repo path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo path is not a directory: %s", opts.Repo)
	}

	return nil
}
