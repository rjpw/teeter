package commands

import (
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available modules",
		Long:  `List modules from the catalogue or cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement list
			return nil
		},
	}
	return cmd
}
