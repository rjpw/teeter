package commands

import (
	"github.com/spf13/cobra"
)

func NewClientDataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "client-data",
		Short: "Manage client data extraction",
		Long:  `Extract maps, vmaps, mmaps from WoW client.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement client data
			return nil
		},
	}
	return cmd
}
