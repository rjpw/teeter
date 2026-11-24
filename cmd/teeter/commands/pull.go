package commands

import (
	"github.com/spf13/cobra"
)

func NewPullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch modules and AC source to cache",
		Long:  `Resolve manifest modules and clone them to the local cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement pull/resolution
			return nil
		},
	}
	return cmd
}
