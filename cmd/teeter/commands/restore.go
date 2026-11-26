package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewRestoreCmd() *cobra.Command {
	var (
		manifestPath string
		input        string
		buildCtxHash string
		force        bool
	)

	cmd := &cobra.Command{
		Use:   "restore --input backup.sql.gz [--buildctx <hash>]",
		Short: "Restore databases from a backup file",
		Long: `Restore all three AzerothCore databases (auth, world, characters) from a backup file.

The backup is applied to the persistent database volume. The target build context
determines which compose file to use (runtime or build context).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			if input == "" {
				return fmt.Errorf("--input required")
			}

			// Load manifest
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

			return runRestore(ctx, workDir, m, input, buildCtxHash, force)
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVarP(&input, "input", "i", "", "Input backup file path (required)")
	cmd.Flags().StringVar(&buildCtxHash, "buildctx", "", "Build context hash to restore into (default: current runtime)")
	cmd.Flags().BoolVar(&force, "force", false, "Force restore even if database has data")

	return cmd
}

func runRestore(ctx context.Context, workDir string, m *manifest.Manifest, inputPath, buildCtxHash string, force bool) error {
	fmt.Printf("♻️  Restoring databases for project %s from %s...\n", m.Project.Name, inputPath)

	// Check input file exists
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Read backup file
	content, err := readBackupFile(inputPath)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	// Determine target compose file
	var composeFile string
	if buildCtxHash != "" {
		composeFile = filepath.Join(workDir, ".teeter", "buildctx", buildCtxHash, "docker-compose.yml")
	} else {
		runtimeDir := filepath.Join(workDir, ".teeter", "runtime")
		composeFile = filepath.Join(runtimeDir, "docker-compose.yml")
		if _, err := os.Stat(composeFile); err != nil {
			// Fallback to latest build context
			hashes, err := listBuildContexts(workDir)
			if err != nil || len(hashes) == 0 {
				return fmt.Errorf("no build context found: %w", err)
			}
			composeFile = filepath.Join(workDir, ".teeter", "buildctx", hashes[len(hashes)-1], "docker-compose.yml")
		}
	}

	// Ensure database is running
	fmt.Println("  Checking database...")
	if err := ensureDatabaseRunning(ctx, workDir, composeFile); err != nil {
		return fmt.Errorf("database not running: %w", err)
	}

	// Get MySQL root password
	envFile := filepath.Join(filepath.Dir(composeFile), ".env")
	mysqlRootPassword, err := readMySQLRootPassword(envFile)
	if err != nil {
		return fmt.Errorf("read MySQL password: %w", err)
	}

	// Parse backup to extract individual database dumps
	dumps := parseBackupContent(content)

	// Restore each database
	databases := []string{"acore_auth", "acore_world", "acore_characters"}
	for _, db := range databases {
		dump := dumps[db]
		if dump == "" {
			fmt.Printf("  Warning: no dump found for %s, skipping\n", db)
			continue
		}

		if !force {
			// Check if database has data
			hasData, err := databaseHasData(ctx, workDir, composeFile, mysqlRootPassword, db)
			if err != nil {
				return fmt.Errorf("check database %s: %w", db, err)
			}
			if hasData {
				return fmt.Errorf("database %s has data, use --force to overwrite", db)
			}
		}

		fmt.Printf("  Restoring %s...\n", db)
		if err := restoreDatabase(ctx, workDir, composeFile, mysqlRootPassword, db, dump); err != nil {
			return fmt.Errorf("restore %s: %w", db, err)
		}
		fmt.Printf("  ✅ %s restored\n", db)
	}

	fmt.Println("  ✅ Restore completed")
	return nil
}

func parseBackupContent(content string) map[string]string {
	dumps := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentDB string
	var currentDump strings.Builder
	inDump := false

	for _, line := range lines {
		// Detect database header
		if strings.HasPrefix(line, "-- Database: ") {
			// Save previous dump
			if currentDB != "" {
				dumps[currentDB] = currentDump.String()
			}
			// Start new dump
			currentDB = strings.TrimSpace(strings.TrimPrefix(line, "-- Database: "))
			currentDump.Reset()
			inDump = true
			continue
		}

		// Detect metadata section (end of database dumps)
		if strings.HasPrefix(line, "-- Client Data Metadata") {
			if currentDB != "" {
				dumps[currentDB] = currentDump.String()
			}
			inDump = false
			currentDB = ""
			continue
		}

		if inDump {
			currentDump.WriteString(line)
			currentDump.WriteString("\n")
		}
	}

	// Save last dump (in case no metadata section)
	if currentDB != "" {
		dumps[currentDB] = currentDump.String()
	}

	return dumps
}

func databaseHasData(ctx context.Context, workDir, composeFile, password, database string) (bool, error) {
	args := []string{
		"compose", "-f", composeFile, "--profile", "core",
		"exec", "-T", "ac-database",
		"mysql", "-u", "root", "-p" + password,
		"-e", fmt.Sprintf("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='%s'", database),
	}
	cmd := newDockerCommand(ctx, workDir, args...)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	outputStr := strings.TrimSpace(string(output))
	// Output format: COUNT(*)\n123\n
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "COUNT(*)" {
			var count int
			fmt.Sscanf(line, "%d", &count)
			return count > 0, nil
		}
	}
	return false, nil
}

func restoreDatabase(ctx context.Context, workDir, composeFile, password, database, dump string) error {
	// Use mysql command to restore with increased max_allowed_packet
	args := []string{
		"compose", "-f", composeFile, "--profile", "core",
		"exec", "-T", "ac-database",
		"mysql", "-u", "root", "-p" + password,
		"--max-allowed-packet=1G",
		"--net-buffer-length=1M",
		database,
	}
	cmd := newDockerCommand(ctx, workDir, args...)
	cmd.Stdin = strings.NewReader(dump)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("mysql restore failed: %w, stderr: %s", err, stderr.String())
	}
	return nil
}

func readBackupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// For now, assume uncompressed. Could add gzip detection.
	return string(data), nil
}
