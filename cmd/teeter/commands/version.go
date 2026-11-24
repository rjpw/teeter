package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewVersionCmd(version, commit, buildDate string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("teeter %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", buildDate)
			return nil
		},
	}
	return cmd
}
