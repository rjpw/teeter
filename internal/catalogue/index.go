package catalogue

import (
	"time"
)

// Index represents the catalogue index containing all modules.
type Index struct {
	FetchedAt time.Time              `json:"fetched_at"`
	Modules   map[string]ModuleEntry `json:"modules"`
}

// ModuleEntry represents a single module in the catalogue.
type ModuleEntry struct {
	Name            string            `json:"name"`
	FullName        string            `json:"full_name"`
	Description     string            `json:"description"`
	Homepage        string            `json:"homepage"`
	RepositoryURL   string            `json:"repository_url"`
	Categories      []string          `json:"categories"`
	Topics          []string          `json:"topics"`
	LatestRelease   *Release          `json:"latest_release"`
	Releases        []Release         `json:"releases"`
	DefaultBranch   string            `json:"default_branch"`
	Dependencies    []string          `json:"dependencies"`
	Conflicts       []string          `json:"conflicts"`
	Provides        []string          `json:"provides"`
	CMakeOptions    map[string]string `json:"cmake_options"`
	SQLFiles        []string          `json:"sql_files"`
	ConfigSchema    map[string]any    `json:"config_schema"`
	Stars           int               `json:"stars"`
	UpdatedAt       string            `json:"updated_at"`
	License         string            `json:"license"`
	ACCompatibility []string          `json:"ac_compatibility"`
}

// Release represents a module release/tag.
type Release struct {
	TagName     string `json:"tag_name"`
	TargetSHA   string `json:"target_sha"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
}
