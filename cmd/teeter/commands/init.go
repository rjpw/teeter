package commands

import (
	"fmt"
	"os"
	"path/filepath"

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

			// Default project name to current directory name
			workDir, _ := os.Getwd()
			defaultProjectName := filepath.Base(workDir)

			manifest := defaultManifest(defaultProjectName)
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

func defaultManifest(projectName string) string {
	return fmt.Sprintf(`version: "1"
project:
  name: "%s"
acore:
  repo: "https://github.com/mod-playerbots/azerothcore-wotlk"
  branch: "Playerbot"
modules: []
client_data:
  source: "static"
  version: "v20.0"
environment:
  vars: {}
compose: {}
`, projectName)
}

func printManifest(manifest string) error {
	fmt.Println(manifest)
	return nil
}

func writeManifest(path, manifest string) error {
	return os.WriteFile(path, []byte(manifest), 0644)
}
