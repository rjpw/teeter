package commands

import (
	"github.com/spf13/cobra"
)

func NewRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore from backup",
		Long:  `Restore databases and configuration from a backup.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement restore
			return nil
		},
	}
	return cmd
}
