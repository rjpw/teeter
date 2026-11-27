package buildctx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rjpw/teeter/internal/cache"
)

// LinkModules links all resolved modules from cache to the build context.
func LinkModules(ctx context.Context, bc *BuildContext, c cache.Cache, strategy cache.LinkStrategy) error {
	for _, name := range bc.Resolution.BuildOrder {
		mod, ok := bc.Resolution.Modules[name]
		if !ok {
			continue
		}

		// Get module source from cache
		srcPath, err := c.GetModule(ctx, name, mod.Version)
		if err != nil {
			return fmt.Errorf("get module %s from cache: %w", name, err)
		}

		// The module source is the worktree returned by GetModule
		moduleSrc := srcPath
		// Link to build context
		dstPath := filepath.Join(bc.Path, "modules", name)
		if err := linkDir(moduleSrc, dstPath, strategy); err != nil {
			return fmt.Errorf("link module %s: %w", name, err)
		}

		// Verify module structure
		if err := verifyModuleStructure(dstPath, name); err != nil {
			return fmt.Errorf("verify module %s: %w", name, err)
		}

		bc.ModulePaths[name] = dstPath
	}
	return nil
}

// verifyModuleStructure checks that the linked module has expected structure.
func verifyModuleStructure(path, moduleName string) error {
	// Check for src/ directory (warn if missing, not fatal - some modules are script-only)
	srcPath := filepath.Join(path, "src")
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: module %s has no src/ directory (script-only or config module)\n", moduleName)
		} else {
			return err
		}
	}

	// Check for CMakeLists.txt or <module>.cmake (warn if missing, not fatal)
	cmakePath := filepath.Join(path, "CMakeLists.txt")
	moduleCmakePath := filepath.Join(path, moduleName+".cmake")
	hasCMake := false
	if _, err := os.Stat(cmakePath); err == nil {
		hasCMake = true
	}
	if _, err := os.Stat(moduleCmakePath); err == nil {
		hasCMake = true
	}
	if !hasCMake {
		// Log warning but don't fail - some modules (scripts, configs) don't need CMake
		fmt.Fprintf(os.Stderr, "Warning: module %s has no CMakeLists.txt or %s.cmake\n", moduleName, moduleName)
	}

	return nil
}
