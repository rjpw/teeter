package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/rjpw/teeter/internal/cache/layout"
	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/config"
	"github.com/spf13/cobra"
)

func NewCatalogueCmd() *cobra.Command {
	var (
		token  string
		force  bool
		useAPI bool
	)

	cmd := &cobra.Command{
		Use:   "catalogue",
		Short: "Manage module catalogue",
		Long:  `Commands for managing the module catalogue index.`,
	}

	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Rebuild catalogue index (static by default, GitHub API with --api)",
		Long: `Rebuild the local catalogue index.

By default, fetches the static catalogue from https://raw.githubusercontent.com/azerothcore/azerothcore.github.io/master/data/catalogue.json
(no authentication required, no rate limits).

Use --api to instead query the GitHub API directly (requires GITHUB_TOKEN for practical use).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			cfg := config.Get()

			var idx *catalogue.Index
			var err error

			if useAPI {
				// Get token from flag or environment
				if token == "" {
					token = os.Getenv("GITHUB_TOKEN")
				}
				if token == "" {
					return fmt.Errorf("GitHub token required for --api (set GITHUB_TOKEN or use --token)")
				}

				client := catalogue.NewGitHubClient(token, cfg.CacheDir, 0)
				fmt.Println("Refreshing catalogue from GitHub API...")
				fmt.Println("This may take a minute...")

				idx, err = client.RefreshCatalogue(ctx)
				if err != nil {
					return fmt.Errorf("refresh catalogue from API: %w", err)
				}
			} else {
				fmt.Println("Refreshing catalogue from static source...")
				idx, err = catalogue.FetchStaticCatalogue(ctx)
				if err != nil {
					return fmt.Errorf("refresh catalogue from static: %w", err)
				}
			}

			fmt.Printf("Catalogue refreshed successfully!\n")
			fmt.Printf("  Modules: %d\n", len(idx.Modules))
			fmt.Printf("  Fetched at: %s\n", idx.FetchedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Cache: %s\n", layout.IndexPath())

			return nil
		},
	}

	refreshCmd.Flags().StringVar(&token, "token", "", "GitHub API token (or set GITHUB_TOKEN) - required with --api")
	refreshCmd.Flags().BoolVar(&force, "force", false, "Force refresh even if cache is fresh")
	refreshCmd.Flags().BoolVar(&useAPI, "api", false, "Use GitHub API instead of static catalogue (requires token)")

	cmd.AddCommand(refreshCmd)

	return cmd
}
