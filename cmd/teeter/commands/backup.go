package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewBackupCmd() *cobra.Command {
	var (
		manifestPath string
		output       string
		compress     bool
	)

	cmd := &cobra.Command{
		Use:   "backup [--output backup.sql.gz]",
		Short: "Backup all databases and client data metadata",
		Long: `Backup all three AzerothCore databases (auth, world, characters) and client data metadata.

Creates a compressed SQL dump that can be restored with 'teeter restore'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

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

			// Default output filename with timestamp
			if output == "" {
				timestamp := time.Now().Format("20060102-150405")
				ext := ".sql"
				if compress {
					ext += ".gz"
				}
				output = fmt.Sprintf("teeter-backup-%s-%s%s", m.Project.Name, timestamp, ext)
			}

			return runBackup(ctx, workDir, m, output, compress)
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: teeter-backup-<project>-<timestamp>.sql[.gz])")
	cmd.Flags().BoolVar(&compress, "compress", true, "Compress output with gzip")

	return cmd
}

func runBackup(ctx context.Context, workDir string, m *manifest.Manifest, outputPath string, compress bool) error {
	fmt.Printf("💾 Backing up databases for project %s...\n", m.Project.Name)

	// Get runtime compose file
	runtimeDir := filepath.Join(workDir, ".teeter", "runtime")
	composeFile := filepath.Join(runtimeDir, "docker-compose.yml")
	if _, err := os.Stat(composeFile); err != nil {
		// Fallback to build context
		hashes, err := listBuildContexts(workDir)
		if err != nil || len(hashes) == 0 {
			return fmt.Errorf("no build context found: %w", err)
		}
		composeFile = filepath.Join(workDir, ".teeter", "buildctx", hashes[len(hashes)-1], "docker-compose.yml")
	}

	// Ensure database is running
	fmt.Println("  Checking database...")
	if err := ensureDatabaseRunning(ctx, workDir, composeFile); err != nil {
		return fmt.Errorf("database not running: %w", err)
	}

	// Get MySQL root password from .env
	envFile := filepath.Join(runtimeDir, ".env")
	if _, err := os.Stat(envFile); err != nil {
		envFile = filepath.Join(filepath.Dir(composeFile), ".env")
	}
	mysqlRootPassword, err := readMySQLRootPassword(envFile)
	if err != nil {
		return fmt.Errorf("read MySQL password: %w", err)
	}

	// Dump databases
	databases := []string{"acore_auth", "acore_world", "acore_characters"}
	var allSQL strings.Builder

	fmt.Println("  Dumping databases...")
	for _, db := range databases {
		fmt.Printf("    Dumping %s...\n", db)
		sql, err := dumpDatabase(ctx, workDir, composeFile, mysqlRootPassword, db)
		if err != nil {
			return fmt.Errorf("dump %s: %w", db, err)
		}
		allSQL.WriteString(fmt.Sprintf("-- Database: %s\n", db))
		allSQL.WriteString(sql)
		allSQL.WriteString("\n\n")
	}

	// Add client data metadata
	clientDataMeta, err := getClientDataMetadata(workDir, m)
	if err != nil {
		fmt.Printf("  Warning: could not read client data metadata: %v\n", err)
	} else {
		allSQL.WriteString("-- Client Data Metadata\n")
		allSQL.WriteString(clientDataMeta)
		allSQL.WriteString("\n")
	}

	// Add backup metadata header
	header := fmt.Sprintf(`-- teeter backup
-- Project: %s
-- Date: %s
-- Databases: %s
-- Compressed: %v

`, m.Project.Name, time.Now().Format(time.RFC3339), strings.Join(databases, ", "), compress)

	fullOutput := header + allSQL.String()

	// Write output
	if compress {
		if err := writeCompressed(outputPath, fullOutput); err != nil {
			return fmt.Errorf("write compressed backup: %w", err)
		}
	} else {
		if err := os.WriteFile(outputPath, []byte(fullOutput), 0644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}

	info, _ := os.Stat(outputPath)
	fmt.Printf("  ✅ Backup written to %s (%s)\n", outputPath, formatBytes(info.Size()))
	return nil
}

func dumpDatabase(ctx context.Context, workDir, composeFile, password, database string) (string, error) {
	args := []string{
		"compose", "-f", composeFile, "--profile", "core",
		"exec", "-T", "ac-database",
		"mysqldump",
		"-u", "root",
		"-p" + password,
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--no-create-db",
		database,
	}
	cmd := newDockerCommand(ctx, workDir, args...)
	output, err := cmd.Output()
	if err != nil {
		// Try without --no-create-db for older MySQL
		args = []string{
			"compose", "-f", composeFile, "--profile", "core",
			"exec", "-T", "ac-database",
			"mysqldump",
			"-u", "root",
			"-p" + password,
			"--single-transaction",
			"--routines",
			"--triggers",
			"--events",
			"--hex-blob",
			database,
		}
		cmd = newDockerCommand(ctx, workDir, args...)
		output, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(output), nil
}
