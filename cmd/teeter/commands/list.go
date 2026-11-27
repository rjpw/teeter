package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/rjpw/teeter/internal/catalogue"
	"github.com/rjpw/teeter/internal/config"
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	var (
		category string
		search   string
		limit    int
		jsonOut  bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available modules from the catalogue",
		Long:  `List modules from the catalogue index with optional filtering.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Get()

			ctx := cmd.Context()
			idx, err := catalogue.LoadOrFetchIndex(ctx, cfg)
			if err != nil {
				return fmt.Errorf("load catalogue: %w", err)
			}

			modules := idx.ListModules()

			// Filter by category
			if category != "" {
				filtered := modules[:0]
				for _, m := range modules {
					for _, c := range m.Categories {
						if strings.EqualFold(c, category) {
							filtered = append(filtered, m)
							break
						}
					}
				}
				modules = filtered
			}

			// Filter by search term (name, description, topics)
			if search != "" {
				searchLower := strings.ToLower(search)
				filtered := modules[:0]
				for _, m := range modules {
					if strings.Contains(strings.ToLower(m.Name), searchLower) ||
						strings.Contains(strings.ToLower(m.Description), searchLower) ||
						strings.Contains(strings.ToLower(m.FullName), searchLower) {
						filtered = append(filtered, m)
						continue
					}
					for _, t := range m.Topics {
						if strings.Contains(strings.ToLower(t), searchLower) {
							filtered = append(filtered, m)
							break
						}
					}
				}
				modules = filtered
			}

			// Sort by name
			sort.Slice(modules, func(i, j int) bool {
				return modules[i].Name < modules[j].Name
			})

			// Apply limit
			if limit > 0 && len(modules) > limit {
				modules = modules[:limit]
			}

			if jsonOut {
				return outputJSON(modules)
			}

			return outputTable(modules)
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (e.g., module, lua, tool)")
	cmd.Flags().StringVarP(&search, "search", "s", "", "Search term (name, description, topics)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "Limit number of results (0 = no limit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func outputTable(modules []catalogue.ModuleEntry) error {
	if len(modules) == 0 {
		fmt.Println("No modules found")
		return nil
	}

	fmt.Printf("%-30s %-12s %s\n", "NAME", "CATEGORY", "DESCRIPTION")
	fmt.Printf("%-30s %-12s %s\n", strings.Repeat("-", 30), strings.Repeat("-", 12), strings.Repeat("-", 50))

	for _, m := range modules {
		cat := ""
		if len(m.Categories) > 0 {
			cat = m.Categories[0]
		}
		desc := m.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Printf("%-30s %-12s %s\n", truncate(m.Name, 30), cat, desc)
	}
	return nil
}

func outputJSON(modules []catalogue.ModuleEntry) error {
	// Output minimal fields for manifest usage
	type manifestModule struct {
		Name    string `json:"name"`
		Version string `json:"version,omitempty"`
	}
	var out []manifestModule
	for _, m := range modules {
		ver := ""
		if m.LatestRelease != nil {
			ver = m.LatestRelease.TagName
		} else if len(m.Releases) > 0 {
			ver = m.Releases[0].TagName
		} else {
			ver = m.DefaultBranch
		}
		out = append(out, manifestModule{Name: m.Name, Version: ver})
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
