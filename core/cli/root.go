package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the top-level emenda command.
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "emenda",
		Short: "Automated dependency upgrade tool",
		Long:  "Emenda fixes breaking API changes across codebases when upgrading dependencies.",
	}

	cmd.Version = version

	return cmd
}
