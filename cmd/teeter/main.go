package main

import (
	"fmt"
	"os"

	"github.com/rjpw/teeter/cmd/teeter/commands"
	"github.com/rjpw/teeter/internal/config"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "teeter",
		Short: "AzerothCore-WotLK mod setup manager",
		Long:  `teeter assembles and manages AzerothCore-WotLK mod setups from a manifest.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return config.InitConfig(cmd)
		},
	}

	config.AddConfigFlags(rootCmd)
	rootCmd.PersistentFlags().String("manifest", "", "Path to manifest file (default: .teeter/manifest.yaml)")
	rootCmd.PersistentFlags().String("workdir", ".", "Working directory")
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Print actions without executing")
	rootCmd.PersistentFlags().String("config", "", "Path to config file")
	rootCmd.PersistentFlags().Bool("json", false, "Output JSON instead of human-readable")

	rootCmd.AddCommand(
		commands.NewInitCmd(),
		commands.NewValidateCmd(),
		commands.NewComposeCmd(),
		commands.NewUpCmd(),
		commands.NewDownCmd(),
		commands.NewPullCmd(),
		commands.NewCacheCmd(),
		commands.NewListCmd(),
		commands.NewLogsCmd(),
		commands.NewBackupCmd(),
		commands.NewRestoreCmd(),
		commands.NewExecCmd(),
		commands.NewShellCmd(),
		commands.NewClientDataCmd(),
		commands.NewDevCmd(),
		commands.NewVersionCmd(version, commit, buildDate),
	)

	if err := rootCmd.Execute(); err != nil {
		if ee, ok := err.(*commands.ExitError); ok {
			os.Exit(ee.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
