package commands

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

func NewVersionCmd(version, commit, buildDate string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Use build-time variables if set, otherwise read from build info
			v := version
			c := commit
			b := buildDate

			if v == "dev" || v == "" {
				if info, ok := debug.ReadBuildInfo(); ok {
					if v == "dev" || v == "" {
						v = info.Main.Version
					}
					if c == "none" || c == "" {
						for _, s := range info.Settings {
							if s.Key == "vcs.revision" {
								c = s.Value
								if len(c) > 12 {
									c = c[:12]
								}
								break
							}
						}
					}
					if b == "unknown" || b == "" {
						for _, s := range info.Settings {
							if s.Key == "vcs.time" {
								b = strings.TrimSuffix(s.Value, "Z")
								break
							}
						}
					}
				}
			}

			fmt.Printf("teeter %s\n", v)
			fmt.Printf("  commit: %s\n", c)
			fmt.Printf("  built:  %s\n", b)
			return nil
		},
	}
}
