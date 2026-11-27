package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/rjpw/teeter/internal/runtime"
	"github.com/spf13/cobra"
)

func NewSwitchCmd() *cobra.Command {
	var (
		manifestPath string
		buildCtxHash string
		rollback     bool
		list         bool
	)

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Switch active build context (atomic cutover)",
		Long: `Switch the thin runtime to point to a different build context.

This performs an atomic symlink flip in .teeter/runtime/buildctx and updates
.current-hash. The running stack can then be restarted with 'teeter up'
to pick up the new build context.

Use --rollback to switch back to the previous build context.
Use --list to show all available build contexts.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			// Load manifest to get work directory
			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				return err
			}

			manifestDir := filepath.Dir(manifestPath)
			if manifestDir == "." {
				manifestDir, _ = os.Getwd()
			}
			if err := manifest.Validate(m, manifestDir); err != nil {
				return err
			}

			workDir := filepath.Dir(manifestDir)
			rt := &runtime.ThinRuntime{WorkDir: workDir}

			if list {
				hashes, err := rt.ListBuildContexts()
				if err != nil {
					return fmt.Errorf("list build contexts: %w", err)
				}
				current, _ := rt.CurrentHash()
				fmt.Println("Available build contexts:")
				for _, h := range hashes {
					marker := ""
					if h == current {
						marker = " (current)"
					}
					fmt.Printf("  %s%s\n", h[:min(12, len(h))], marker)
				}
				return nil
			}

			if rollback {
				return switchRollback(rt, m)
			}

			if buildCtxHash == "" {
				return fmt.Errorf("--buildctx required (or use --rollback or --list)")
			}

			// Validate build context exists and has required files
			if err := rt.ValidateBuildContext(buildCtxHash); err != nil {
				return fmt.Errorf("invalid build context: %w", err)
			}

			// Switch
			fmt.Printf("Switching to build context %s...\n", buildCtxHash[:min(12, len(buildCtxHash))])
			if err := rt.Switch(buildCtxHash); err != nil {
				return fmt.Errorf("switch: %w", err)
			}

			fmt.Println("Switch complete. Run 'teeter up' to restart services with new build context.")
			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVar(&buildCtxHash, "buildctx", "", "Build context hash to switch to")
	cmd.Flags().BoolVar(&rollback, "rollback", false, "Switch back to previous build context")
	cmd.Flags().BoolVar(&list, "list", false, "List available build contexts")

	return cmd
}

func switchRollback(rt *runtime.ThinRuntime, m *manifest.Manifest) error {
	current, err := rt.CurrentHash()
	if err != nil {
		return fmt.Errorf("get current hash: %w", err)
	}
	if current == "" {
		return fmt.Errorf("no current build context to rollback from")
	}

	hashes, err := rt.ListBuildContexts()
	if err != nil {
		return fmt.Errorf("list build contexts: %w", err)
	}

	// Find previous hash (the one before current in list)
	var prevHash string
	foundCurrent := false
	for i := len(hashes) - 1; i >= 0; i-- {
		if hashes[i] == current {
			foundCurrent = true
			continue
		}
		if foundCurrent {
			prevHash = hashes[i]
			break
		}
	}

	if prevHash == "" {
		return fmt.Errorf("no previous build context found for rollback")
	}

	// Validate it
	if err := rt.ValidateBuildContext(prevHash); err != nil {
		return fmt.Errorf("previous build context invalid: %w", err)
	}

	fmt.Printf("Rolling back from %s to %s...\n", current[:min(12, len(current))], prevHash[:min(12, len(prevHash))])
	if err := rt.Switch(prevHash); err != nil {
		return fmt.Errorf("rollback switch: %w", err)
	}

	fmt.Println("Rollback complete. Run 'teeter up' to restart services.")
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
