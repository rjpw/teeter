package commands

import (
	"github.com/spf13/cobra"
)

func NewCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage module cache",
		Long:  `Inspect and manage the local module cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement cache management
			return nil
		},
	}
	return cmd
}
