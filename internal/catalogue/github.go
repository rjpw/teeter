package catalogue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rjpw/teeter/internal/cache/layout"
)

const (
	githubAPIBase   = "https://api.github.com"
	searchURL       = githubAPIBase + "/search/repositories"
	repoURL         = githubAPIBase + "/repos"
	defaultCacheTTL = 3600 // 1 hour

	// Static catalogue URL from azerothcore.github.io
	staticCatalogueURL = "https://raw.githubusercontent.com/azerothcore/azerothcore.github.io/master/data/catalogue.json"
)

// GitHubClient fetches module data from GitHub API.
type GitHubClient struct {
	token      string
	httpClient *http.Client
	cacheDir   string
	cacheTTL   int
}

// GitHubRepository represents a GitHub repository from the API.
type GitHubRepository struct {
	Name          string   `json:"name"`
	FullName      string   `json:"full_name"`
	Description   string   `json:"description"`
	Homepage      string   `json:"homepage"`
	HTMLURL       string   `json:"html_url"`
	DefaultBranch string   `json:"default_branch"`
	Topics        []string `json:"topics"`
	License       *struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
	StargazersCount int    `json:"stargazers_count"`
	PushedAt        string `json:"pushed_at"`
}

// GitHubRelease represents a GitHub release.
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
	} `json:"assets"`
}

// NewGitHubClient creates a new GitHub API client.
func NewGitHubClient(token, cacheDir string, cacheTTL int) *GitHubClient {
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".teeter", "cache")
	}
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}
	return &GitHubClient{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cacheDir:   cacheDir,
		cacheTTL:   cacheTTL,
	}
}

// RefreshCatalogue rebuilds the catalogue index from GitHub API.
func (c *GitHubClient) RefreshCatalogue(ctx context.Context) (*Index, error) {
	// Search for all module categories
	categories := []string{
		"azerothcore-module",
		"azerothcore-tools",
		"azerothcore-lua",
		"azerothcore-sql",
	}

	allModules := make(map[string]ModuleEntry)

	for _, category := range categories {
		modules, err := c.searchCategory(ctx, category)
		if err != nil {
			return nil, fmt.Errorf("search category %s: %w", category, err)
		}
		for name, entry := range modules {
			// Merge categories if module appears in multiple
			if existing, ok := allModules[name]; ok {
				existing.Categories = mergeCategories(existing.Categories, entry.Categories)
				existing.Topics = mergeTopics(existing.Topics, entry.Topics)
				existing.ACCompatibility = mergeCompat(existing.ACCompatibility, entry.ACCompatibility)
				allModules[name] = existing
			} else {
				allModules[name] = entry
			}
		}
	}

	// Build index
	idx := &Index{
		FetchedAt: time.Now(),
		Modules:   allModules,
	}

	// Save to cache
	if err := c.saveIndex(idx); err != nil {
		return nil, fmt.Errorf("save index: %w", err)
	}

	return idx, nil
}

// searchCategory searches for repositories with a specific topic.
func (c *GitHubClient) searchCategory(ctx context.Context, category string) (map[string]ModuleEntry, error) {
	// Build search query
	q := fmt.Sprintf("topic:%s", category)
	url := fmt.Sprintf("%s?q=%s&sort=stars&order=desc&per_page=100", searchURL, q)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	var resp *http.Response
	var searchResult struct {
		Items []GitHubRepository `json:"items"`
	}

	// Retry with exponential backoff for rate limits
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
				return nil, err
			}
			break
		}

		if resp.StatusCode == 429 {
			// Rate limited - wait and retry
			resetTime := resp.Header.Get("X-RateLimit-Reset")
			wait := 30 * time.Second
			if resetTime != "" {
				if reset, err := strconv.ParseInt(resetTime, 10, 64); err == nil {
					wait = time.Until(time.Unix(reset, 0))
					if wait < 0 {
						wait = 30 * time.Second
					}
				}
			}
			fmt.Fprintf(os.Stderr, "Rate limited, waiting %v before retry...\n", wait)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
				continue
			}
		}

		return nil, fmt.Errorf("search failed: %s", resp.Status)
	}

	if resp == nil {
		return nil, fmt.Errorf("search failed after retries")
	}

	modules := make(map[string]ModuleEntry)
	for _, repo := range searchResult.Items {
		// Get full repo data and releases
		fullRepo, err := c.fetchRepo(ctx, repo.FullName)
		if err != nil {
			continue
		}
		releases, err := c.fetchReleases(ctx, repo.FullName)
		if err != nil {
			releases = []GitHubRelease{}
		}

		entry := c.buildModuleEntry(fullRepo, releases)
		modules[entry.Name] = entry
	}

	return modules, nil
}

// fetchRepo fetches full repository data.
func (c *GitHubClient) fetchRepo(ctx context.Context, fullName string) (GitHubRepository, error) {
	url := fmt.Sprintf("%s/%s", repoURL, fullName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GitHubRepository{}, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return GitHubRepository{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GitHubRepository{}, fmt.Errorf("fetch repo failed: %s", resp.Status)
	}

	var repo GitHubRepository
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return GitHubRepository{}, err
	}
	return repo, nil
}

// fetchReleases fetches repository releases.
func (c *GitHubClient) fetchReleases(ctx context.Context, fullName string) ([]GitHubRelease, error) {
	url := fmt.Sprintf("%s/%s/releases?per_page=50", repoURL, fullName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch releases failed: %s", resp.Status)
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	// Filter out draft releases
	var filtered []GitHubRelease
	for _, r := range releases {
		if !r.Draft {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// buildModuleEntry creates a ModuleEntry from GitHub data.
func (c *GitHubClient) buildModuleEntry(repo GitHubRepository, releases []GitHubRelease) ModuleEntry {
	categories := c.topicsToCategories(repo.Topics)
	acCompat := c.extractACCompatibility(repo.Topics)
	dependencies := c.parseDependencies(repo.Topics)

	var releaseEntries []Release
	var latestRelease *Release
	for _, r := range releases {
		releaseEntries = append(releaseEntries, Release{
			TagName:     r.TagName,
			TargetSHA:   "", // GitHub API doesn't provide target SHA in release list
			PublishedAt: r.PublishedAt.Format(time.RFC3339),
			Prerelease:  r.Prerelease,
		})
	}
	if len(releaseEntries) > 0 {
		latestRelease = &releaseEntries[0]
	}

	license := ""
	if repo.License != nil {
		license = repo.License.SPDXID
	}

	updatedAt := ""
	if repo.PushedAt != "" {
		if t, err := time.Parse(time.RFC3339, strings.Replace(repo.PushedAt, "Z", "+00:00", 1)); err == nil {
			updatedAt = t.Format(time.RFC3339)
		}
	}

	return ModuleEntry{
		Name:            repo.Name,
		FullName:        repo.FullName,
		Description:     repo.Description,
		Homepage:        repo.Homepage,
		RepositoryURL:   repo.HTMLURL,
		Categories:      categories,
		Topics:          repo.Topics,
		LatestRelease:   latestRelease,
		Releases:        releaseEntries,
		DefaultBranch:   repo.DefaultBranch,
		Dependencies:    dependencies,
		Conflicts:       []string{},
		Provides:        []string{},
		CMakeOptions:    map[string]string{},
		SQLFiles:        []string{},
		ConfigSchema:    map[string]any{},
		Stars:           repo.StargazersCount,
		UpdatedAt:       updatedAt,
		License:         license,
		ACCompatibility: acCompat,
	}
}

// topicsToCategories maps GitHub topics to categories.
func (c *GitHubClient) topicsToCategories(topics []string) []string {
	mapping := map[string]string{
		"azerothcore-module": "module",
		"azerothcore-tools":  "tool",
		"azerothcore-lua":    "lua",
		"azerothcore-sql":    "sql",
	}
	var cats []string
	for _, t := range topics {
		if cat, ok := mapping[t]; ok {
			cats = append(cats, cat)
		}
	}
	if len(cats) == 0 {
		cats = []string{"module"}
	}
	return cats
}

// extractACCompatibility extracts AC version compatibility from topics.
func (c *GitHubClient) extractACCompatibility(topics []string) []string {
	var compat []string
	for _, t := range topics {
		if strings.HasPrefix(t, "acore-rev-") {
			compat = append(compat, t)
		}
	}
	return compat
}

// parseDependencies infers dependencies from mod-* topics.
func (c *GitHubClient) parseDependencies(topics []string) []DependencySpec {
	var deps []DependencySpec
	seen := make(map[string]bool)
	for _, t := range topics {
		if strings.HasPrefix(t, "mod-") && !seen[t] &&
			t != "azerothcore-module" && t != "azerothcore-tools" {
			seen[t] = true
			deps = append(deps, DependencySpec{
				Name:     t,
				Version:  "*",
				Optional: false,
				Category: "module",
			})
		}
	}
	return deps
}

// saveIndex saves the index to the cache directory.
func (c *GitHubClient) saveIndex(idx *Index) error {
	indexPath := layout.IndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return err
	}

	cached := CachedIndex{
		Index: idx,
		ETag:  fmt.Sprintf("github-%d", time.Now().Unix()),
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}

// Helper functions for merging
func mergeCategories(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, c := range append(a, b...) {
		if !seen[c] {
			seen[c] = true
			result = append(result, c)
		}
	}
	return result
}

func mergeTopics(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, t := range append(a, b...) {
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

func mergeCompat(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, c := range append(a, b...) {
		if !seen[c] {
			seen[c] = true
			result = append(result, c)
		}
	}
	return result
}

// StaticCatalogue represents the structure of the static catalogue.json
type StaticCatalogue struct {
	GlobalSearch  bool                                 `json:"global_search"`
	Organizations map[string]map[string][]StaticModule `json:"organizations"`
}

// StaticModule represents a module in the static catalogue
type StaticModule struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	StargazersCount int    `json:"stargazers_count"`
	CreatedAt       string `json:"created_at"`
	DefaultBranch   string `json:"default_branch"`
	Owner           struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
	} `json:"owner"`
	HTMLURL       string `json:"html_url"`
	ForksCount    int    `json:"forks_count"`
	WatchersCount int    `json:"watchers_count"`
	License       *struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
	Topics []string `json:"topics"`
}

// FetchStaticCatalogue fetches and parses the static catalogue from azerothcore.github.io
func FetchStaticCatalogue(ctx context.Context) (*Index, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, staticCatalogueURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch static catalogue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch static catalogue failed: %s", resp.Status)
	}

	var static StaticCatalogue
	if err := json.NewDecoder(resp.Body).Decode(&static); err != nil {
		return nil, fmt.Errorf("parse static catalogue: %w", err)
	}

	// Convert static format to our Index format
	modules := make(map[string]ModuleEntry)
	for _, orgModules := range static.Organizations {
		for category, mods := range orgModules {
			for _, m := range mods {
				entry := convertStaticModule(m, category)
				if existing, ok := modules[entry.Name]; ok {
					// Merge categories, topics, compat
					existing.Categories = mergeCategories(existing.Categories, entry.Categories)
					existing.Topics = mergeTopics(existing.Topics, entry.Topics)
					existing.ACCompatibility = mergeCompat(existing.ACCompatibility, entry.ACCompatibility)
					modules[entry.Name] = existing
				} else {
					modules[entry.Name] = entry
				}
			}
		}
	}

	idx := &Index{
		FetchedAt: time.Now(),
		Modules:   modules,
	}

	// Save to cache
	if err := saveIndexToFile(idx); err != nil {
		return nil, fmt.Errorf("save index: %w", err)
	}

	return idx, nil
}

// convertStaticModule converts a StaticModule to ModuleEntry
func convertStaticModule(m StaticModule, category string) ModuleEntry {
	categories := []string{}
	if category == "azerothcore-module" {
		categories = []string{"module"}
	} else if category == "azerothcore-tools" {
		categories = []string{"tool"}
	} else if category == "azerothcore-lua" {
		categories = []string{"lua"}
	} else if category == "azerothcore-sql" {
		categories = []string{"sql"}
	} else {
		categories = []string{"module"}
	}

	acCompat := extractACCompat(m.Topics)
	deps := parseDepsFromTopics(m.Topics)
	license := ""
	if m.License != nil {
		license = m.License.SPDXID
	}

	// Parse updated_at from pushed_at equivalent
	updatedAt := ""
	if m.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, strings.Replace(m.CreatedAt, "Z", "+00:00", 1)); err == nil {
			updatedAt = t.Format(time.RFC3339)
		}
	}

	return ModuleEntry{
		Name:            m.Name,
		FullName:        m.FullName,
		Description:     m.Description,
		Homepage:        "",
		RepositoryURL:   m.HTMLURL,
		Categories:      categories,
		Topics:          m.Topics,
		LatestRelease:   nil, // Static catalogue doesn't have release info
		Releases:        []Release{},
		DefaultBranch:   m.DefaultBranch,
		Dependencies:    deps,
		Conflicts:       []string{},
		Provides:        []string{},
		CMakeOptions:    map[string]string{},
		SQLFiles:        []string{},
		ConfigSchema:    map[string]any{},
		Stars:           m.StargazersCount,
		UpdatedAt:       updatedAt,
		License:         license,
		ACCompatibility: acCompat,
	}
}

// extractACCompat extracts AC compatibility from topics
func extractACCompat(topics []string) []string {
	var compat []string
	for _, t := range topics {
		if strings.HasPrefix(t, "acore-rev-") {
			compat = append(compat, t)
		}
	}
	return compat
}

// parseDepsFromTopics infers dependencies from mod-* topics
func parseDepsFromTopics(topics []string) []DependencySpec {
	var deps []DependencySpec
	seen := make(map[string]bool)
	for _, t := range topics {
		if strings.HasPrefix(t, "mod-") && !seen[t] &&
			t != "azerothcore-module" && t != "azerothcore-tools" &&
			t != "azerothcore-lua" && t != "azerothcore-sql" {
			seen[t] = true
			deps = append(deps, DependencySpec{
				Name:     t,
				Version:  "*",
				Optional: false,
				Category: "module",
			})
		}
	}
	return deps
}

// saveIndexToFile saves the index to the cache file
func saveIndexToFile(idx *Index) error {
	indexPath := layout.IndexPath()
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return err
	}

	cached := CachedIndex{
		Index: idx,
		ETag:  fmt.Sprintf("static-%d", time.Now().Unix()),
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}
