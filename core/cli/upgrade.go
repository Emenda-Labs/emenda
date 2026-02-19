package cli

import (
	"github.com/spf13/cobra"
)

// NewUpgradeCmd creates the "upgrade" parent command.
func NewUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade a dependency and fix breaking changes",
		Long:  "Upgrade a dependency to a new version and automatically fix breaking API changes in the codebase.",
	}

	return cmd
}
