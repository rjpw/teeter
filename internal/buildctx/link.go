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

		// The module source is at modules/<name>/ inside the cache dir
		moduleSrc := filepath.Join(srcPath, "modules", name)

		// Link to build context
		dstPath := filepath.Join(bc.Path, "modules", name)
		if err := linkDir(moduleSrc, dstPath, strategy); err != nil {
			return fmt.Errorf("link module %s: %w", name, err)
		}

		// Verify module structure
		if err := verifyModuleStructure(dstPath); err != nil {
			return fmt.Errorf("verify module %s: %w", name, err)
		}

		bc.ModulePaths[name] = dstPath
	}
	return nil
}

// verifyModuleStructure checks that the linked module has expected structure.
func verifyModuleStructure(path string) error {
	// Check for CMakeLists.txt
	cmakePath := filepath.Join(path, "CMakeLists.txt")
	if _, err := os.Stat(cmakePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing CMakeLists.txt")
		}
		return err
	}

	// Check for src/ directory
	srcPath := filepath.Join(path, "src")
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing src/ directory")
		}
		return err
	}

	return nil
}
