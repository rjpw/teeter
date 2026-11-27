package catalogue

import (
	"time"
)

// Index represents the catalogue index containing all modules.
type Index struct {
	FetchedAt time.Time              `json:"fetched_at"`
	Modules   map[string]ModuleEntry `json:"modules"`
}

// DependencySpec represents a module dependency with version constraint.
type DependencySpec struct {
	Name     string `json:"name"`
	Version  string `json:"version"` // e.g., "*", "^1.0.0", ">=2.0.0"
	Optional bool   `json:"optional"`
	Category string `json:"category"` // "module", "acore", "tool"
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
	Dependencies    []DependencySpec  `json:"dependencies"` // rich dependencies with constraints
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

// ListModules returns all modules as a slice for easy searching/filtering.
func (i *Index) ListModules() []ModuleEntry {
	modules := make([]ModuleEntry, 0, len(i.Modules))
	for _, m := range i.Modules {
		modules = append(modules, m)
	}
	return modules
}

// Release represents a module release/tag.
type Release struct {
	TagName     string  `json:"tag_name"`
	TargetSHA   string  `json:"target_sha"`
	PublishedAt string  `json:"published_at"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"` // downloadable artifacts
}

// Asset represents a release asset (zip, sha256, etc.)
type Asset struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}
