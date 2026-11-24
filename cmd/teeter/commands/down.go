package commands

import (
	"github.com/spf13/cobra"
)

func NewDownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop the server stack",
		Long:  `Stop and remove all services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement down orchestration
			return nil
		},
	}
	return cmd
}
