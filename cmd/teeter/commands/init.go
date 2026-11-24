package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewInitCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new teeter manifest",
		Long:  `Create a new teeter.yaml manifest file with default values.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			verbose, _ := cmd.Flags().GetBool("verbose")

			manifest := defaultManifest()

			if manifestPath == "" {
				manifestPath = "teeter.yaml"
			}

			if dryRun {
				if verbose {
					fmt.Fprintf(os.Stderr, "Would write manifest to: %s\n", manifestPath)
				}
				return printManifest(manifest)
			}

			if verbose {
				fmt.Fprintf(os.Stderr, "Writing manifest to: %s\n", manifestPath)
			}

			return writeManifest(manifestPath, manifest)
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", "", "Output manifest path (default: teeter.yaml)")
	return cmd
}

func defaultManifest() string {
	return `version: "1"
project:
  name: "my-server"
acore:
  repo: "https://github.com/mod-playerbots/azerothcore-wotlk"
  branch: "Playerbot"
modules: []
client_data:
  source: "local"
  path: "~/wow-client"
environment:
  vars: {}
compose: {}
`
}

func printManifest(manifest string) error {
	fmt.Println(manifest)
	return nil
}

func writeManifest(path, manifest string) error {
	return os.WriteFile(path, []byte(manifest), 0644)
}
