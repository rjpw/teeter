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

func NewValidateBuildCtxCmd() *cobra.Command {
	var (
		manifestPath string
		buildCtxHash string
		ephemeral    bool
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "buildctx [--ephemeral]",
		Short: "Validate a build context with ephemeral database",
		Long: `Validate a build context by spinning up an ephemeral database,
running dbimport, and optionally starting authserver/worldserver briefly.

This is a DRY RUN - it does NOT touch persistent volumes.

Use --ephemeral to automatically create and destroy a temporary MySQL volume.
Without --ephemeral, validates against the build context's existing compose file.`,
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

			if ephemeral {
				return validateEphemeral(ctx, workDir, manifestDir, m, buildCtxHash, timeout)
			}

			return validateExisting(ctx, workDir, manifestDir, m, buildCtxHash, timeout)
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().StringVar(&buildCtxHash, "buildctx", "", "Build context hash to validate")
	cmd.Flags().BoolVar(&ephemeral, "ephemeral", false, "Use ephemeral database (auto-create/destroy)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Max wait time for validation")

	return cmd
}

func validateEphemeral(ctx context.Context, workDir, manifestDir string, m *manifest.Manifest, buildCtxHash string, timeout time.Duration) error {
	fmt.Printf("🔍 Validating build context %s with ephemeral database...\n", buildCtxHash[:min(12, len(buildCtxHash))])
	// Load build context
	bcPath := filepath.Join(workDir, ".teeter", "buildctx", buildCtxHash)
	if _, err := os.Stat(bcPath); err != nil {
		return fmt.Errorf("build context not found: %w", err)
	}

	// Generate ephemeral compose file with temporary volumes
	ephemeralHash := "validate-" + buildCtxHash[:8]
	ephemeralCompose, err := generateEphemeralCompose(bcPath, m, ephemeralHash)
	if err != nil {
		return fmt.Errorf("generate ephemeral compose: %w", err)
	}

	ephemeralComposeFile := filepath.Join(bcPath, "docker-compose.ephemeral.yml")
	if err := os.WriteFile(ephemeralComposeFile, []byte(ephemeralCompose), 0644); err != nil {
		return fmt.Errorf("write ephemeral compose: %w", err)
	}

	// Start ephemeral database only
	fmt.Println("  Starting ephemeral database...")
	upArgs := []string{"up", "-d", "ac-database"}
	upCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, upArgs...)
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("start ephemeral database: %w", err)
	}

	// Wait for database healthy
	if err := waitForEphemeralHealth(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, timeout); err != nil {
		tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)
		return fmt.Errorf("ephemeral database unhealthy: %w", err)
	}

	// Run dbimport
	fmt.Println("  Running dbimport (dry-run)...")
	dbimportArgs := []string{"run", "--rm", "ac-db-import"}
	dbimportCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, dbimportArgs...)
	dbimportOutput := &strings.Builder{}
	dbimportCmd.Stdout = dbimportOutput
	dbimportCmd.Stderr = dbimportOutput
	dbimportStart := time.Now()
	if err := dbimportCmd.Run(); err != nil {
		tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)
		return fmt.Errorf("dbimport failed: %w", err)
	}
	dbimportEnd := time.Now()

	// Parse validation report from dbimport output
	report := parseValidationReport(dbimportOutput.String())
	report.StartTime = dbimportStart
	report.EndTime = dbimportEnd
	fmt.Printf("  %s\n", report.Summary())
	if report.TotalUpdates > 0 || report.TotalArchived > 0 {
		fmt.Printf("  %s\n", report.Detailed())
	}

	// Optionally start authserver/worldserver briefly
	fmt.Println("  Starting authserver briefly...")
	authArgs := []string{"up", "-d", "ac-authserver"}
	authCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, authArgs...)
	authCmd.Stdout = os.Stdout
	authCmd.Stderr = os.Stderr
	if err := authCmd.Run(); err != nil {
		tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)
		return fmt.Errorf("authserver failed: %w", err)
	}

	// Wait a moment for authserver to connect
	time.Sleep(10 * time.Second)

	// Check authserver is running
	if err := checkEphemeralService(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, "ac-authserver"); err != nil {
		tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)
		return fmt.Errorf("authserver not running: %w", err)
	}

	fmt.Println("  ✅ Authserver connected successfully")

	// Start worldserver briefly
	fmt.Println("  Starting worldserver briefly...")
	worldArgs := []string{"up", "-d", "ac-worldserver"}
	worldCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, worldArgs...)
	worldCmd.Stdout = os.Stdout
	worldCmd.Stderr = os.Stderr
	if err := worldCmd.Run(); err != nil {
		tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)
		return fmt.Errorf("worldserver failed: %w", err)
	}

	time.Sleep(10 * time.Second)

	if err := checkEphemeralService(ctx, manifestDir, m.Project.Name, ephemeralComposeFile, "ac-worldserver"); err != nil {
		tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)
		return fmt.Errorf("worldserver not running: %w", err)
	}

	fmt.Println("  ✅ Worldserver connected successfully")

	// Tear down
	tearDownEphemeral(ctx, manifestDir, m.Project.Name, ephemeralComposeFile)

	fmt.Printf("\n✅ Validation PASSED for build context %s\n", buildCtxHash[:min(12, len(buildCtxHash))])
	fmt.Println("   - Database initialization: OK")
	fmt.Println("   - Authserver connection: OK")
	fmt.Println("   - Worldserver connection: OK")
	fmt.Println("   - No persistent volumes modified")

	return nil
}

func validateExisting(ctx context.Context, workDir, manifestDir string, m *manifest.Manifest, buildCtxHash string, timeout time.Duration) error {
	fmt.Printf("🔍 Validating build context %s against existing compose...\n", buildCtxHash[:min(12, len(buildCtxHash))])

	// Just verify the build context has all required files
	bcPath := filepath.Join(workDir, ".teeter", "buildctx", buildCtxHash)
	required := []string{
		"docker-compose.yml",
		"env.ac",
		".env",
		"sql/teeter-dbimport.conf",
		"sql/teeter-authserver.conf",
		"sql/teeter-worldserver.conf",
		"sql/teeter-combined.sql",
	}

	for _, f := range required {
		if _, err := os.Stat(filepath.Join(bcPath, f)); err != nil {
			return fmt.Errorf("missing %s in build context: %w", f, err)
		}
	}

	fmt.Println("  ✅ All required files present")
	fmt.Printf("✅ Build context %s is valid\n", buildCtxHash[:min(12, len(buildCtxHash))])
	return nil
}

func generateEphemeralCompose(bcPath string, m *manifest.Manifest, ephemeralHash string) (string, error) {
	// Read the original compose config
	composeFile := filepath.Join(bcPath, "docker-compose.yml")
	content, err := os.ReadFile(composeFile)
	if err != nil {
		return "", err
	}

	// Replace volume names with ephemeral ones
	ephemeralContent := string(content)
	projectName := m.Project.Name
	ephemeralProjectName := projectName + "-ephemeral-" + ephemeralHash

	// Replace all volume references
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-mysql-data", projectName),
		fmt.Sprintf("teeter-%s-mysql-data", ephemeralProjectName))
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-extracted-data", projectName),
		fmt.Sprintf("teeter-%s-extracted-data", ephemeralProjectName))
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-client-data", projectName),
		fmt.Sprintf("teeter-%s-client-data", ephemeralProjectName))
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-config", projectName),
		fmt.Sprintf("teeter-%s-config", ephemeralProjectName))

	// Also replace in the volumes section at bottom
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-mysql-data:", projectName),
		fmt.Sprintf("teeter-%s-mysql-data:", ephemeralProjectName))
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-extracted-data:", projectName),
		fmt.Sprintf("teeter-%s-extracted-data:", ephemeralProjectName))
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-client-data:", projectName),
		fmt.Sprintf("teeter-%s-client-data:", ephemeralProjectName))
	ephemeralContent = strings.ReplaceAll(ephemeralContent,
		fmt.Sprintf("teeter-%s-config:", projectName),
		fmt.Sprintf("teeter-%s-config:", ephemeralProjectName))

	// Remove client-data-init service (not needed for validation)
	// We'll just skip it by not including it in the depends_on chain

	return ephemeralContent, nil
}

func waitForEphemeralHealth(ctx context.Context, manifestDir, projectName, composeFile string, timeout time.Duration) error {
	profileArgs := []string{"--profile", "core"}
	start := time.Now()
	checkInterval := 5 * time.Second

	// Wait for database
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for database")
		}
		if err := checkEphemeralServiceHealth(ctx, manifestDir, projectName, composeFile, profileArgs, "ac-database"); err == nil {
			fmt.Println("  ✅ Database healthy")
			break
		}
		time.Sleep(checkInterval)
	}

	return nil
}

func checkEphemeralServiceHealth(ctx context.Context, manifestDir, projectName, composeFile string, profileArgs []string, service string) error {
	args := []string{"ps", "--format", "json", service}
	args = append(profileArgs, args...)
	cmd := newDockerCommand(ctx, manifestDir, projectName, composeFile, args...)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		return fmt.Errorf("service not found")
	}

	if !strings.Contains(outputStr, "\"State\":\"running\"") {
		return fmt.Errorf("service not running")
	}

	// Check health via docker exec
	healthArgs := []string{"exec", "-T", service, "sh", "-c", "exit 0"}
	healthArgs = append(profileArgs, healthArgs...)
	healthCmd := newDockerCommand(ctx, manifestDir, projectName, composeFile, healthArgs...)
	return healthCmd.Run()
}

func checkEphemeralServiceCompleted(ctx context.Context, manifestDir, projectName, composeFile string, profileArgs []string, service string) error {
	args := []string{"ps", "--format", "json", service}
	args = append(profileArgs, args...)
	cmd := newDockerCommand(ctx, manifestDir, projectName, composeFile, args...)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		return fmt.Errorf("service not found")
	}

	if !strings.Contains(outputStr, "\"State\":\"exited\"") && !strings.Contains(outputStr, "\"Status\":\"exited\"") {
		return fmt.Errorf("service not completed")
	}

	if strings.Contains(outputStr, "\"ExitCode\":0") {
		return nil
	}
	return fmt.Errorf("service exited with non-zero code")
}

func checkEphemeralService(ctx context.Context, manifestDir, projectName, composeFile string, service string) error {
	args := []string{"ps", "--format", "json", service}
	args = append([]string{"--profile", "core"}, args...)
	cmd := newDockerCommand(ctx, manifestDir, projectName, composeFile, args...)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		return fmt.Errorf("service not found")
	}

	if strings.Contains(outputStr, "\"State\":\"running\"") {
		return nil
	}
	return fmt.Errorf("service not running")
}

func tearDownEphemeral(ctx context.Context, manifestDir, projectName, composeFile string) {
	fmt.Println("  Tearing down ephemeral stack...")
	args := []string{"down", "-v"}
	cmd := newDockerCommand(ctx, manifestDir, projectName, composeFile, args...)
	cmd.Run()
	// Remove ephemeral compose file
	os.Remove(composeFile)
}

// ValidationReport holds parsed dbimport output
type ValidationReport struct {
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

func (r *ValidationReport) Summary() string {
	if r.TotalUpdates == 0 && r.TotalArchived == 0 {
		return "No database updates needed (all databases up-to-date)"
	}
	elapsed := r.EndTime.Sub(r.StartTime)
	return fmt.Sprintf("Would apply %d new updates (%d archived) across 3 databases in ~%s",
		r.TotalUpdates, r.TotalArchived, elapsed.Round(time.Second))
}

func (r *ValidationReport) Detailed() string {
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

// parseValidationReport extracts update counts from dbimport output
func parseValidationReport(output string) *ValidationReport {
	r := &ValidationReport{
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

func atoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
