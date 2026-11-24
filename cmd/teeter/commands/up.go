package commands

import (
	"github.com/spf13/cobra"
)

func NewUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the server stack",
		Long:  `Build images, initialize databases, and start all services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement up orchestration
			return nil
		},
	}
	return cmd
}
