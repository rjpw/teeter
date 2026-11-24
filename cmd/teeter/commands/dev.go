package commands

import (
	"github.com/spf13/cobra"
)

func NewDevCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Development mode commands",
		Long:  `Commands for live module development.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement dev mode
			return nil
		},
	}
	return cmd
}
