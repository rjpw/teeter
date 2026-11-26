package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewValidateCmd() *cobra.Command {
	var manifestPath string
	var strict bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a teeter manifest",
		Long:  `Validate the manifest file for correctness.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get manifest path from flag or default
			if manifestPath == "" {
				workDir, _ := cmd.Flags().GetString("workdir")
				if workDir == "" {
					workDir = "."
				}
				manifestPath = "teeter.yaml"
				// Check if .teeter/manifest.yaml exists
				teeterManifest := fmt.Sprintf("%s/.teeter/manifest.yaml", workDir)
				if _, err := os.Stat(teeterManifest); err == nil {
					manifestPath = teeterManifest
				}
			}

			// Parse manifest
			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				if jsonOutput {
					printJSONError(err)
				} else {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				}
				return exitCode(2)
			}

			// Validate manifest
			workDir, _ := cmd.Flags().GetString("workdir")
			if workDir == "" {
				workDir = "."
			}
			err = manifest.Validate(m, workDir)
			if err != nil {
				if jsonOutput {
					printJSONError(err)
				} else {
					fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
				}
				return exitCode(2)
			}

			if !jsonOutput {
				fmt.Fprintln(os.Stderr, "Manifest is valid")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail on warnings (not yet implemented)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON instead of human-readable")

	// Add buildctx subcommand
	cmd.AddCommand(NewValidateBuildCtxCmd())

	return cmd
}

func printJSONError(err error) {
	// Simple JSON output for structured errors
	fmt.Printf(`{"error": %q}\n`, err.Error())
}

func exitCode(code int) error {
	return &ExitError{Code: code}
}

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

func isExitError(err error) bool {
	var ee *ExitError
	return errors.As(err, &ee)
}

func getExitCode(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}
