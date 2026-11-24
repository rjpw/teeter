package commands

import (
	"github.com/spf13/cobra"
)

func NewShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell <service>",
		Short: "Open a shell in a service container",
		Long:  `Open an interactive shell in a running service container.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement shell
			return nil
		},
	}
	return cmd
}
