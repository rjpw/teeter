package commands

import (
	"github.com/spf13/cobra"
)

func NewBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup databases and config",
		Long:  `Create a backup of databases and configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement backup
			return nil
		},
	}
	return cmd
}
