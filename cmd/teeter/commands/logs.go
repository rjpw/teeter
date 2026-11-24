package commands

import (
	"github.com/spf13/cobra"
)

func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "View service logs",
		Long:  `View logs from running services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement logs
			return nil
		},
	}
	return cmd
}
