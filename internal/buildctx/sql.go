package buildctx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/cache"
)

// AssembleSQL collects and combines all SQL files for database initialization.
func AssembleSQL(ctx context.Context, bc *BuildContext, c cache.Cache) error {
	sqlDir := filepath.Join(bc.Path, "sql")
	modulesDir := filepath.Join(sqlDir, "modules")

	var combinedSQL strings.Builder

	// 1. Custom migrations from manifest
	for _, mig := range bc.Manifest.Database.Migrations {
		content, err := readSQLFile(mig, filepath.Dir(bc.Path)+"/..")
		if err != nil {
			return fmt.Errorf("read migration %s: %w", mig, err)
		}
		combinedSQL.WriteString("-- Migration: " + mig + "\n")
		combinedSQL.WriteString(content)
		combinedSQL.WriteString("\n\n")
	}

	// 2. Custom seed data from manifest
	for _, seed := range bc.Manifest.Database.Seed {
		content, err := readSQLFile(seed, filepath.Dir(bc.Path)+"/..")
		if err != nil {
			return fmt.Errorf("read seed %s: %w", seed, err)
		}
		combinedSQL.WriteString("-- Seed: " + seed + "\n")
		combinedSQL.WriteString(content)
		combinedSQL.WriteString("\n\n")
	}

	// 3. Module SQL files from cache
	for _, name := range bc.Resolution.BuildOrder {
		mod, ok := bc.Resolution.Modules[name]
		if !ok {
			continue
		}

		srcPath, err := c.GetModule(ctx, name, mod.Version)
		if err != nil {
			// Module might not have SQL files, continue
			continue
		}

		moduleSQLDir := filepath.Join(srcPath, "modules", name, "sql")
		if _, err := os.Stat(moduleSQLDir); err != nil {
			continue // No SQL directory
		}

		// Copy module SQL files to build context modules dir
		modDstDir := filepath.Join(modulesDir, name)
		if err := copyDir(moduleSQLDir, modDstDir); err != nil {
			return fmt.Errorf("copy module %s SQL: %w", name, err)
		}

		// Add to combined SQL
		files, err := os.ReadDir(moduleSQLDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".sql") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(moduleSQLDir, f.Name()))
			if err != nil {
				continue
			}
			combinedSQL.WriteString("-- Module " + name + ": " + f.Name() + "\n")
			combinedSQL.WriteString(string(content))
			combinedSQL.WriteString("\n\n")
		}
	}

	// Write combined SQL file
	combinedPath := filepath.Join(sqlDir, "teeter-combined.sql")
	if err := os.WriteFile(combinedPath, []byte(combinedSQL.String()), 0644); err != nil {
		return fmt.Errorf("write combined SQL: %w", err)
	}

	return nil
}

// readSQLFile reads a SQL file, resolving relative to base directory.
func readSQLFile(path string, baseDir string) (string, error) {
	// Resolve relative paths
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
