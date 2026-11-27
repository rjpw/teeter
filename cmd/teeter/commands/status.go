package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rjpw/teeter/internal/manifest"
	"github.com/spf13/cobra"
)

type composeServiceStatus struct {
	Name       string `json:"Name"`
	State      string `json:"State"`
	Status     string `json:"Status"`
	Health     string `json:"Health"`
	ExitCode   int    `json:"ExitCode"`
	CreatedAt  string `json:"CreatedAt"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
}

func NewStatusCmd() *cobra.Command {
	var (
		manifestPath string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show service health status",
		Long:  `Show formatted health status of all services.`,
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

			// Find build context
			finder := &BuildContextFinder{
				ManifestDir: manifestDir,
				Manifest:    m,
			}
			bc, err := finder.Find()
			if err != nil {
				return fmt.Errorf("find build context: %w", err)
			}

			composeFile := GetComposeFile(bc)
			if _, err := os.Stat(composeFile); os.IsNotExist(err) {
				return fmt.Errorf("compose file not found: %s", composeFile)
			}

			// Get profiles from manifest
			profileArgs := GetProfileArgs(m)

			// Get service status from docker compose
			dockerArgs := []string{"compose", "-f", composeFile}
			dockerArgs = append(dockerArgs, profileArgs...)
			dockerArgs = append(dockerArgs, "ps", "--format", "json")

			statusCmd := newDockerCommand(ctx, bc.Path, dockerArgs...)
			var output []byte
			output, err = statusCmd.Output()
			if err != nil {
				return fmt.Errorf("docker compose ps: %w", err)
			}

			// Parse JSON lines (one per service)
			var services []composeServiceStatus
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				var svc composeServiceStatus
				if err := json.Unmarshal([]byte(line), &svc); err != nil {
					continue
				}
				services = append(services, svc)
			}

			// Print formatted status
			fmt.Printf("Project: %s\n", m.Project.Name)
			fmt.Printf("Status:  %s\n", overallStatus(services))
			fmt.Println()

			if len(services) == 0 {
				fmt.Println("No services found.")
				return nil
			}

			// Print table
			fmt.Printf("%-25s %-12s %-20s %-10s %s\n", "SERVICE", "STATE", "HEALTH", "EXIT CODE", "UPTIME")
			fmt.Println(strings.Repeat("-", 85))

			for _, svc := range services {
				uptime := calculateUptime(svc)
				health := formatHealth(svc)
				fmt.Printf("%-25s %-12s %-20s %-10d %s\n",
					truncate(svc.Name, 24),
					svc.State,
					health,
					svc.ExitCode,
					uptime,
				)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest file (default: teeter.yaml or .teeter/manifest.yaml)")

	return cmd
}

func overallStatus(services []composeServiceStatus) string {
	if len(services) == 0 {
		return "Stopped"
	}

	allHealthy := true
	anyRunning := false

	for _, svc := range services {
		if svc.State == "running" {
			anyRunning = true
			if !strings.Contains(strings.ToLower(svc.Health), "healthy") && svc.Health != "" {
				allHealthy = false
			}
		}
		if svc.State == "exited" && svc.ExitCode != 0 {
			return "Failed"
		}
	}

	if !anyRunning {
		return "Stopped"
	}
	if allHealthy {
		return "Running (healthy)"
	}
	return "Running (degraded)"
}

func formatHealth(svc composeServiceStatus) string {
	if svc.Health != "" {
		return svc.Health
	}
	if svc.State == "exited" {
		if svc.ExitCode == 0 {
			return "completed"
		}
		return "failed"
	}
	if svc.State == "running" {
		return "unknown"
	}
	return svc.State
}

func calculateUptime(svc composeServiceStatus) string {
	if svc.StartedAt == "" {
		return "-"
	}
	start, err := time.Parse(time.RFC3339Nano, svc.StartedAt)
	if err != nil {
		// Try without nano
		start, err = time.Parse(time.RFC3339, svc.StartedAt)
		if err != nil {
			return "-"
		}
	}

	var end time.Time
	if svc.State == "exited" && svc.FinishedAt != "" {
		end, _ = time.Parse(time.RFC3339Nano, svc.FinishedAt)
		if end.IsZero() {
			end, _ = time.Parse(time.RFC3339, svc.FinishedAt)
		}
	} else {
		end = time.Now()
	}

	duration := end.Sub(start)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd", int(duration.Hours()/24))
}
