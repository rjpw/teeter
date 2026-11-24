package buildctx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// AppliedPatch records a patch that was applied.
type AppliedPatch struct {
	Module  string `json:"module"`  // "acore" or module name
	Patch   string `json:"patch"`   // patch file name
	Target  string `json:"target"`  // target directory
	Applied string `json:"applied"` // timestamp
}

// ApplyPatches applies all patches from the manifest to the build context.
func ApplyPatches(ctx context.Context, bc *BuildContext) error {
	var applied []AppliedPatch

	// Apply AC patches (patches with target "acore" or empty target on AC)
	// For now, we apply module patches to their respective modules

	for _, modSpec := range bc.Manifest.Modules {
		for _, patch := range modSpec.Patches {
			// Determine target path
			var targetPath string
			if patch.Target == "acore" || patch.Target == "" {
				// Target is AC source
				targetPath = bc.ACPath
			} else {
				// Target is a module
				modPath, ok := bc.ModulePaths[patch.Target]
				if !ok {
					return fmt.Errorf("patch target module %s not found", patch.Target)
				}
				targetPath = modPath
			}

			// Resolve patch file path
			patchPath := patch.Path
			if !filepath.IsAbs(patchPath) {
				// Relative to manifest directory (parent of .teeter/buildctx)
				patchPath = filepath.Join(filepath.Dir(bc.Path), "..", patchPath)
			}

			// Apply patch using git apply
			if err := applyPatch(targetPath, patchPath); err != nil {
				return fmt.Errorf("apply patch %s to %s: %w", patchPath, targetPath, err)
			}

			applied = append(applied, AppliedPatch{
				Module:  patch.Target,
				Patch:   filepath.Base(patchPath),
				Target:  targetPath,
				Applied: "now", // TODO: use actual timestamp
			})
		}
	}

	// Record applied patches
	return recordAppliedPatches(bc, applied)
}

// applyPatch applies a patch file using git apply.
func applyPatch(targetDir, patchFile string) error {
	// Use git apply for better handling of git-formatted patches
	cmd := exec.Command("git", "apply", patchFile)
	cmd.Dir = targetDir
	if _, err := cmd.CombinedOutput(); err != nil {
		// Try with patch command as fallback
		return applyPatchWithPatch(targetDir, patchFile)
	}
	return nil
}

// applyPatchWithPatch applies a patch using the patch command.
func applyPatchWithPatch(targetDir, patchFile string) error {
	cmd := exec.Command("patch", "-p1", "-i", patchFile)
	cmd.Dir = targetDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("patch failed: %w: %s", err, out)
	}
	return nil
}

// recordAppliedPatches writes the list of applied patches to a JSON file.
func recordAppliedPatches(bc *BuildContext, applied []AppliedPatch) error {
	teeterDir := filepath.Join(bc.Path, ".teeter")
	if err := os.MkdirAll(teeterDir, 0755); err != nil {
		return err
	}

	patchesFile := filepath.Join(teeterDir, "applied-patches.json")
	data, err := json.MarshalIndent(applied, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(patchesFile, data, 0644)
}
