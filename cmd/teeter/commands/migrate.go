package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

func NewMigrateCmd() *cobra.Command {
	var (
		manifestPath string
		buildCtxHash string
		force        bool
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "migrate --buildctx <hash>",
		Short: "Apply database migrations from a build context to persistent database",
		Long: `Apply database migrations from a validated build context to the persistent database.

This runs dbimport against the PERSISTENT database volume (not ephemeral).
It is idempotent - only new updates are applied.

Run 'teeter validate buildctx --ephemeral' first to test safely.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			if buildCtxHash == "" {
				return fmt.Errorf("--buildctx required")
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

			return runMigrate(ctx, workDir, m, buildCtxHash, timeout, force)
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVar(&buildCtxHash, "buildctx", "", "Build context hash to migrate from")
	cmd.Flags().BoolVar(&force, "force", false, "Skip validation warning")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Max wait time for migration")

	return cmd
}

func runMigrate(ctx context.Context, workDir string, m *manifest.Manifest, buildCtxHash string, timeout time.Duration, force bool) error {
	fmt.Printf("📦 Migrating persistent database using build context %s...\n", buildCtxHash[:min(12, len(buildCtxHash))])

	// Load build context
	bcPath := filepath.Join(workDir, ".teeter", "buildctx", buildCtxHash)
	if _, err := os.Stat(bcPath); err != nil {
		return fmt.Errorf("build context not found: %w", err)
	}

	// Check if runtime is pointing to this build context
	runtimeDir := filepath.Join(workDir, ".teeter", "runtime")
	currentHashFile := filepath.Join(runtimeDir, ".current-hash")
	if data, err := os.ReadFile(currentHashFile); err == nil {
		current := strings.TrimSpace(string(data))
		if current != buildCtxHash {
			if !force {
				return fmt.Errorf("runtime is at %s, not %s. Use --force to migrate anyway, or run 'teeter switch --buildctx %s' first", current[:min(12, len(current))], buildCtxHash[:min(12, len(buildCtxHash))], buildCtxHash)
			}
			fmt.Printf("⚠️  Runtime is at %s, migrating anyway (--force)\n", current[:min(12, len(current))])
		}
	}

	// Verify required files exist
	required := []string{
		"sql/teeter-dbimport.conf",
		"sql/teeter-combined.sql",
	}
	for _, f := range required {
		if _, err := os.Stat(filepath.Join(bcPath, f)); err != nil {
			return fmt.Errorf("missing %s in build context: %w", f, err)
		}
	}

	// Get the persistent compose file (from runtime)
	composeFile := filepath.Join(runtimeDir, "docker-compose.yml")
	if _, err := os.Stat(composeFile); err != nil {
		// Fallback to build context compose file
		composeFile = filepath.Join(bcPath, "docker-compose.yml")
	}

	// Ensure database is running
	fmt.Println("  Checking database...")
	if err := ensureDatabaseRunning(ctx, workDir, composeFile); err != nil {
		return fmt.Errorf("database not running: %w", err)
	}

	// Run dbimport against persistent database
	fmt.Println("  Running dbimport against persistent database...")
	migrateStart := time.Now()
	dbimportArgs := []string{"compose", "-f", composeFile, "--profile", "core", "run", "--rm", "ac-db-import"}
	dbimportCmd := newDockerCommand(ctx, workDir, dbimportArgs...)
	dbimportOutput := &strings.Builder{}
	dbimportCmd.Stdout = dbimportOutput
	dbimportCmd.Stderr = dbimportOutput
	if err := dbimportCmd.Run(); err != nil {
		return fmt.Errorf("dbimport failed: %w", err)
	}
	migrateEnd := time.Now()

	// Parse migration report
	report := parseMigrationReport(dbimportOutput.String())
	report.StartTime = migrateStart
	report.EndTime = migrateEnd

	fmt.Printf("  %s\n", report.Summary())
	if report.TotalUpdates > 0 || report.TotalArchived > 0 {
		fmt.Printf("  %s\n", report.Detailed())
	}

	if report.TotalUpdates == 0 && report.TotalArchived == 0 {
		fmt.Println("  ✅ Database already up-to-date")
	} else {
		fmt.Printf("  ✅ Migration completed in %s\n", migrateEnd.Sub(migrateStart).Round(time.Second))
	}

	return nil
}

// MigrationReport holds parsed dbimport output for migration
type MigrationReport struct {
	AuthUpdates   int
	AuthArchived  int
	CharUpdates   int
	CharArchived  int
	WorldUpdates  int
	WorldArchived int
	TotalUpdates  int
	TotalArchived int
	StartTime     time.Time
	EndTime       time.Time
	Errors        []string
	Warnings      []string
}

func (r *MigrationReport) Summary() string {
	if r.TotalUpdates == 0 && r.TotalArchived == 0 {
		return "No database updates needed (all databases up-to-date)"
	}
	elapsed := r.EndTime.Sub(r.StartTime)
	return fmt.Sprintf("Applied %d new updates (%d archived) across 3 databases in %s",
		r.TotalUpdates, r.TotalArchived, elapsed.Round(time.Second))
}

func (r *MigrationReport) Detailed() string {
	var b strings.Builder
	b.WriteString("Database update summary:\n")
	if r.AuthUpdates > 0 || r.AuthArchived > 0 {
		b.WriteString(fmt.Sprintf("  Auth:     %d new, %d archived\n", r.AuthUpdates, r.AuthArchived))
	}
	if r.CharUpdates > 0 || r.CharArchived > 0 {
		b.WriteString(fmt.Sprintf("  Character: %d new, %d archived\n", r.CharUpdates, r.CharArchived))
	}
	if r.WorldUpdates > 0 || r.WorldArchived > 0 {
		b.WriteString(fmt.Sprintf("  World:    %d new, %d archived\n", r.WorldUpdates, r.WorldArchived))
	}
	b.WriteString(fmt.Sprintf("  Total:    %d new, %d archived\n", r.TotalUpdates, r.TotalArchived))
	if len(r.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, w := range r.Warnings {
			b.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}
	if len(r.Errors) > 0 {
		b.WriteString("Errors:\n")
		for _, e := range r.Errors {
			b.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}
	return b.String()
}

// parseMigrationReport extracts update counts from dbimport output
func parseMigrationReport(output string) *MigrationReport {
	r := &MigrationReport{
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	// Regex patterns for dbimport output
	authRe := regexp.MustCompile(`(?i)Updating Auth database\.\.\.\n>> Auth database is up-to-date! Containing (\d+) new and (\d+) archived updates`)
	charRe := regexp.MustCompile(`(?i)Updating Character database\.\.\.\n>> Character database is up-to-date! Containing (\d+) new and (\d+) archived updates`)
	worldRe := regexp.MustCompile(`(?i)Updating World database\.\.\.\n>> World database is up-to-date! Containing (\d+) new and (\d+) archived updates`)

	// Also handle case where databases are created fresh
	authCreateRe := regexp.MustCompile(`(?i)Creating database "acore_auth"`)
	charCreateRe := regexp.MustCompile(`(?i)Creating database "acore_characters"`)
	worldCreateRe := regexp.MustCompile(`(?i)Creating database "acore_world"`)

	// Try to match update lines
	if m := authRe.FindStringSubmatch(output); m != nil {
		r.AuthUpdates = atoi(m[1])
		r.AuthArchived = atoi(m[2])
	} else if authCreateRe.MatchString(output) {
		r.AuthUpdates = -1 // Unknown, fresh database
	}

	if m := charRe.FindStringSubmatch(output); m != nil {
		r.CharUpdates = atoi(m[1])
		r.CharArchived = atoi(m[2])
	} else if charCreateRe.MatchString(output) {
		r.CharUpdates = -1
	}

	if m := worldRe.FindStringSubmatch(output); m != nil {
		r.WorldUpdates = atoi(m[1])
		r.WorldArchived = atoi(m[2])
	} else if worldCreateRe.MatchString(output) {
		r.WorldUpdates = -1
	}

	// Calculate totals (ignore -1 for fresh DBs)
	totalNew := 0
	totalArch := 0
	for _, v := range []int{r.AuthUpdates, r.CharUpdates, r.WorldUpdates} {
		if v > 0 {
			totalNew += v
		}
	}
	for _, v := range []int{r.AuthArchived, r.CharArchived, r.WorldArchived} {
		if v > 0 {
			totalArch += v
		}
	}
	r.TotalUpdates = totalNew
	r.TotalArchived = totalArch

	// Look for errors/warnings
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") && !strings.Contains(lower, "error_log") {
			r.Errors = append(r.Errors, strings.TrimSpace(line))
		} else if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
			r.Warnings = append(r.Warnings, strings.TrimSpace(line))
		}
	}

	return r
}
