package commands

import (
	"github.com/spf13/cobra"
)

func NewExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <service> <command...>",
		Short: "Execute a command in a service container",
		Long:  `Execute a command in a running service container.`,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement exec
			return nil
		},
	}
	return cmd
}
