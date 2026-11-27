package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rjpw/teeter/internal/buildctx"
	"github.com/rjpw/teeter/internal/cache"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/compose"
	"github.com/rjpw/teeter/internal/config"
	"github.com/rjpw/teeter/internal/manifest"
	"github.com/rjpw/teeter/internal/project"
	"github.com/rjpw/teeter/internal/runtime"
	"github.com/spf13/cobra"
)

func NewUpCmd() *cobra.Command {
	var (
		manifestPath string
		build        bool
		detach       bool
		profiles     string
		wait         bool
		timeout      time.Duration
		noPull       bool
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the server stack",
		Long:  `Build images, initialize databases, and start all services.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Resolve manifest path
			resolvedManifestPath, err := ResolveManifestPath(cmd, manifestPath)
			if err != nil {
				return err
			}
			manifestPath = resolvedManifestPath

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

			// Acquire project lock
			unlock, err := project.AcquireLock(manifestDir)
			if err != nil {
				return fmt.Errorf("project locked: %w", err)
			}
			defer unlock()

			// Initialize config
			cfg := config.Get()

			// Apply CLI flags over manifest/config
			dockerImageTag := m.Compose.BuildStrategy
			if dockerImageTag == "" {
				dockerImageTag = "master"
			}

			if profiles == "" {
				profiles = strings.Join(m.Compose.Profiles, ",")
			}
			if profiles == "" {
				profiles = "core"
			}

			if timeout == 0 {
				timeout = 5 * time.Minute
			}

			// Create cache
			c := cache.NewFSCache(cfg.CacheDir, cache.LinkStrategyFromString(cfg.LinkStrategy))

			// Load or fetch catalogue index
			idx, err := catalogue.LoadOrFetchIndex(ctx, cfg)
			if err != nil {
				return fmt.Errorf("load catalogue index: %w", err)
			}

			// Resolve modules
			res, err := catalogue.Resolve(ctx, m, idx, catalogue.DefaultResolveOptions())
			if err != nil {
				return fmt.Errorf("resolve modules: %w", err)
			}

			bc, err := buildctx.Assemble(ctx, m, &res, c, workDir)
			if err != nil {
				return fmt.Errorf("assemble build context: %w", err)
			}

			// Generate env files
			envFiles, err := compose.GenerateEnvFiles(bc, m)
			if err != nil {
				return fmt.Errorf("generate env files: %w", err)
			}
			if err := writeEnvFiles(bc.Path, envFiles); err != nil {
				return fmt.Errorf("write env files: %w", err)
			}

			// Generate SQL files
			sqlFiles, err := compose.GenerateSQLFiles(ctx, bc, c, m)
			if err != nil {
				return fmt.Errorf("generate SQL files: %w", err)
			}
			if err := writeSQLFiles(bc.Path, sqlFiles); err != nil {
				return fmt.Errorf("write SQL files: %w", err)
			}

			// Generate services
			svcGen := compose.ServiceGenerator{
				BuildContext: bc,
				Manifest:     m,
			}

			services, err := svcGen.GenerateServices()
			if err != nil {
				return fmt.Errorf("generate services: %w", err)
			}

			networks := svcGen.GenerateNetworks()
			volumes := svcGen.GenerateVolumes()

			// Create compose config
			composeConfig := compose.ComposeConfig{
				Services: services,
				Networks: networks,
				Volumes:  volumes,
			}

			yamlData, err := marshalCompose(composeConfig)
			if err != nil {
				return fmt.Errorf("marshal compose: %w", err)
			}

			composeFile := filepath.Join(bc.Path, "docker-compose.yml")
			if err := os.WriteFile(composeFile, yamlData, 0644); err != nil {
				return fmt.Errorf("write compose file: %w", err)
			}

			// Set up thin runtime: create symlink farm pointing to this build context
			rt := &runtime.ThinRuntime{WorkDir: workDir}
			if err := rt.Switch(bc.Hash); err != nil {
				return fmt.Errorf("switch runtime to build context: %w", err)
			}

			// Use thin runtime's compose file for all docker operations
			runtimeComposeFile, err := rt.GetComposeFile()
			if err != nil {
				return fmt.Errorf("get runtime compose file: %w", err)
			}
			composeFile = runtimeComposeFile

			// Build images if requested
			if build {
				fmt.Println("Building images...")
				buildArgs := []string{"build"}
				if build {
					buildArgs = append(buildArgs, "--no-cache")
				}
				buildCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, buildArgs...)
				buildCmd.Stdout = os.Stdout
				buildCmd.Stderr = os.Stderr
				if err := buildCmd.Run(); err != nil {
					return fmt.Errorf("docker compose build: %w", err)
				}
			}

			// Pull upstream images if not --no-pull
			if !noPull {
				fmt.Println("Pulling upstream images...")
				pullArgs := []string{"pull", "--ignore-pull-failures"}
				pullCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, pullArgs...)
				pullCmd.Stdout = os.Stdout
				pullCmd.Stderr = os.Stderr
				if err := pullCmd.Run(); err != nil {
					return fmt.Errorf("docker compose pull: %w", err)
				}
			}

			// Start services in correct order
			fmt.Println("Starting AzerothCore server...")

			// 1. Start database first and wait for healthy
			fmt.Println("  [1/5] Starting database...")
			dbArgs := []string{"up", "-d", "ac-database"}
			dbArgs = append(GetProfileArgs(m), dbArgs...)
			dbCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, dbArgs...)
			dbCmd.Stdout = os.Stdout
			dbCmd.Stderr = os.Stderr
			if err := dbCmd.Run(); err != nil {
				return fmt.Errorf("start database: %w", err)
			}
			if err := waitForServiceHealth(ctx, manifestDir, m.Project.Name, composeFile, timeout, "ac-database"); err != nil {
				return fmt.Errorf("database unhealthy: %w", err)
			}
			fmt.Println("  ✅ Database healthy")

			// 2. Run dbimport (oneshot)
			fmt.Println("  [2/5] Running database import...")
			dbimportArgs := []string{"run", "--rm", "-T", "ac-db-import"}
			dbimportArgs = append(GetProfileArgs(m), dbimportArgs...)
			dbimportCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, dbimportArgs...)
			dbimportCmd.Stdout = os.Stdout
			dbimportCmd.Stderr = os.Stderr
			if err := dbimportCmd.Run(); err != nil {
				return fmt.Errorf("dbimport failed: %w", err)
			}
			fmt.Println("  ✅ Database import completed")

			// 3. Run client-data-init (oneshot)
			fmt.Println("  [3/5] Initializing client data...")
			clientDataArgs := []string{"run", "--rm", "-T", "ac-client-data-init"}
			clientDataArgs = append(GetProfileArgs(m), clientDataArgs...)
			clientDataCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, clientDataArgs...)
			clientDataCmd.Stdout = os.Stdout
			clientDataCmd.Stderr = os.Stderr
			if err := clientDataCmd.Run(); err != nil {
				return fmt.Errorf("client-data-init failed: %w", err)
			}
			fmt.Println("  ✅ Client data initialized")

			// 4. Start authserver
			fmt.Println("  [4/5] Starting authserver...")
			authArgs := []string{"up", "-d", "ac-authserver"}
			authArgs = append(GetProfileArgs(m), authArgs...)
			authCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, authArgs...)
			authCmd.Stdout = os.Stdout
			authCmd.Stderr = os.Stderr
			if err := authCmd.Run(); err != nil {
				return fmt.Errorf("start authserver: %w", err)
			}
			if err := waitForServiceRunning(ctx, manifestDir, m.Project.Name, composeFile, timeout, "ac-authserver"); err != nil {
				return fmt.Errorf("authserver failed: %w", err)
			}
			fmt.Println("  ✅ Authserver running")

			// 5. Start worldserver
			fmt.Println("  [5/5] Starting worldserver...")
			worldArgs := []string{"up", "-d", "ac-worldserver"}
			worldArgs = append(GetProfileArgs(m), worldArgs...)
			worldCmd := newDockerCommand(ctx, manifestDir, m.Project.Name, composeFile, worldArgs...)
			worldCmd.Stdout = os.Stdout
			worldCmd.Stderr = os.Stderr
			if err := worldCmd.Run(); err != nil {
				return fmt.Errorf("start worldserver: %w", err)
			}
			if err := waitForServiceRunning(ctx, manifestDir, m.Project.Name, composeFile, timeout, "ac-worldserver"); err != nil {
				return fmt.Errorf("worldserver failed: %w", err)
			}
			fmt.Println("  ✅ Worldserver running")

			fmt.Println()
			fmt.Println("Server ready!")

			// Print connection info
			printConnectionInfo()

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")
	cmd.Flags().BoolVar(&build, "build", false, "Force rebuild images (--no-cache)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", true, "Run in background")
	cmd.Flags().StringVar(&profiles, "profile", "", "Comma-separated profiles (default: core)")
	cmd.Flags().BoolVar(&wait, "wait", true, "Wait for all services healthy")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Max wait time for health checks")
	cmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip pulling upstream images")

	return cmd
}

func waitForHealth(ctx context.Context, manifestDir, projectName, composeFile string, timeout time.Duration) error {
	fmt.Println("Waiting for services to become healthy...")

	profileArgs := GetProfileArgsFromComposeFile(composeFile)

	start := time.Now()
	checkInterval := 5 * time.Second

	services := []struct {
		name        string
		healthCheck func() error
	}{
		{"ac-database", func() error {
			return checkServiceHealth(ctx, manifestDir, projectName, composeFile, profileArgs, "ac-database")
		}},
		{"ac-db-import", func() error {
			return checkServiceCompleted(ctx, manifestDir, projectName, composeFile, profileArgs, "ac-db-import")
		}},
		{"ac-client-data-init", func() error {
			return checkServiceCompleted(ctx, manifestDir, projectName, composeFile, profileArgs, "ac-client-data-init")
		}},
		{"ac-authserver", func() error {
			return checkServiceRunning(ctx, manifestDir, projectName, composeFile, profileArgs, "ac-authserver")
		}},
		{"ac-worldserver", func() error {
			return checkServiceRunning(ctx, manifestDir, projectName, composeFile, profileArgs, "ac-worldserver")
		}},
	}

	for i, svc := range services {
		fmt.Printf("  [%d/%d] %s: ", i+1, len(services), svc.name)
		for {
			if time.Since(start) > timeout {
				return fmt.Errorf("timeout waiting for %s", svc.name)
			}
			err := svc.healthCheck()
			if err == nil {
				fmt.Println("healthy")
				break
			}
			if !isRetryable(err) {
				return fmt.Errorf("%s failed: %w", svc.name, err)
			}
			time.Sleep(checkInterval)
		}
	}

	fmt.Println()
	fmt.Println("Server ready!")
	return nil
}

func waitForServiceHealth(ctx context.Context, manifestDir, projectName, composeFile string, timeout time.Duration, service string) error {
	profileArgs := GetProfileArgsFromComposeFile(composeFile)
	start := time.Now()
	checkInterval := 5 * time.Second

	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for %s to be healthy", service)
		}
		if err := checkServiceHealth(ctx, manifestDir, projectName, composeFile, profileArgs, service); err == nil {
			return nil
		}
		time.Sleep(checkInterval)
	}
}

func waitForServiceRunning(ctx context.Context, manifestDir, projectName, composeFile string, timeout time.Duration, service string) error {
	profileArgs := GetProfileArgsFromComposeFile(composeFile)
	start := time.Now()
	checkInterval := 5 * time.Second

	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for %s to be running", service)
		}
		if err := checkServiceRunning(ctx, manifestDir, projectName, composeFile, profileArgs, service); err == nil {
			return nil
		}
		time.Sleep(checkInterval)
	}
}

func checkServiceRunning(ctx context.Context, manifestDir, projectName, composeFile string, profileArgs []string, service string) error {
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

	// docker compose ps --format json returns an array of objects
	// Check if any object has State=running
	if strings.Contains(outputStr, "\"State\":\"running\"") {
		return nil
	}
	return fmt.Errorf("service not running: %s", outputStr)
}

func GetProfileArgsFromComposeFile(composeFile string) []string {
	// Read the compose file to get profiles
	// For now, use a default
	return []string{"--profile", "core"}
}

func checkServiceHealth(ctx context.Context, manifestDir, projectName, composeFile string, profileArgs []string, service string) error {
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

	// docker compose ps --format json returns an array of objects
	if !strings.Contains(outputStr, "\"State\":\"running\"") {
		return fmt.Errorf("service not running: %s", outputStr)
	}

	// Check health via docker exec (for services with healthcheck)
	healthArgs := []string{"exec", "-T", service, "sh", "-c", "exit 0"}
	healthArgs = append(profileArgs, healthArgs...)
	healthCmd := newDockerCommand(ctx, manifestDir, projectName, composeFile, healthArgs...)
	return healthCmd.Run()
}

func checkServiceCompleted(ctx context.Context, manifestDir, projectName, composeFile string, profileArgs []string, service string) error {
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

	// docker compose ps --format json returns an array of objects
	if !strings.Contains(outputStr, "\"State\":\"exited\"") && !strings.Contains(outputStr, "\"Status\":\"exited\"") {
		return fmt.Errorf("service not completed: %s", outputStr)
	}

	// Check exit code
	if strings.Contains(outputStr, "\"ExitCode\":0") {
		return nil
	}
	return fmt.Errorf("service exited with non-zero code: %s", outputStr)
}

func isRetryable(err error) bool {
	return err != nil
}

func printConnectionInfo() {
	fmt.Println()
	fmt.Println("Connection info:")
	fmt.Println("  World:  localhost:8085")
	fmt.Println("  Auth:   localhost:3724")
	fmt.Println("  MySQL:  localhost:3306 (user: acore)")
	fmt.Println("  Logs:   teeter logs -f ac-worldserver")
}
