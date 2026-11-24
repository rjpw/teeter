package commands

import (
	"github.com/spf13/cobra"
)

func NewComposeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Generate docker-compose.yml",
		Long:  `Generate the docker-compose.yml from the manifest.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement compose generation
			return nil
		},
	}
	return cmd
}
