package catalogue

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rjpw/teeter/internal/manifest"
)

// ResolvedModule represents a module with all version resolution completed.
type ResolvedModule struct {
	Spec         manifest.ModuleSpec
	CachePath    string   // path to module source (modules/<name>/)
	Version      string   // resolved version (tag/sha/branch)
	Repository   string   // git URL
	Commit       string   // resolved commit SHA
	Dependencies []string // from catalogue + manifest
	Conflicts    []string // from catalogue
}

// Resolution represents the complete resolution result.
type Resolution struct {
	Modules    map[string]ResolvedModule // key = module name
	BuildOrder []string                  // topologically sorted
	ACCommit   string                    // resolved AC commit SHA
	ACRepo     string
}

// ACCompatPolicy defines how to handle AC compatibility mismatches.
type ACCompatPolicy string

const (
	ACCompatWarn   ACCompatPolicy = "warn"
	ACCompatError  ACCompatPolicy = "error"
	ACCompatIgnore ACCompatPolicy = "ignore"
)

// ResolveOptions configures the resolution behavior.
type ResolveOptions struct {
	ACCompatPolicy ACCompatPolicy
	Cache          Cache // for checking local modules
}

// Cache interface for resolution (minimal subset).
type Cache interface {
	HasModule(name, version string) bool
	HasACore(commit string) bool
}

// DefaultResolveOptions returns sensible defaults.
func DefaultResolveOptions() ResolveOptions {
	return ResolveOptions{
		ACCompatPolicy: ACCompatWarn,
	}
}

// Resolve resolves all modules and AC from the manifest using the catalogue index.
func Resolve(ctx context.Context, m *manifest.Manifest, idx *Index, opts ResolveOptions) (Resolution, error) {
	if idx == nil {
		return Resolution{}, errors.New("catalogue index is nil")
	}

	res := Resolution{
		Modules: make(map[string]ResolvedModule),
		ACRepo:  m.ACore.Repo,
	}

	// 1. Resolve AC commit
	acCommit, err := resolveACCommit(ctx, m)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve AC commit: %w", err)
	}
	res.ACCommit = acCommit

	// 2. Resolve each module
	for _, spec := range m.Modules {
		rm, err := resolveModule(ctx, spec, m, idx, res.ACCommit, opts)
		if err != nil {
			return Resolution{}, fmt.Errorf("resolve module %s: %w", spec.Name, err)
		}
		res.Modules[spec.Name] = rm
	}

	// 3. Build dependency graph (manifest + catalogue deps)
	depGraph := buildDepGraph(res.Modules)

	// 4. Topological sort (Kahn's algorithm)
	buildOrder, err := topologicalSort(depGraph)
	if err != nil {
		return Resolution{}, fmt.Errorf("topological sort: %w", err)
	}
	res.BuildOrder = buildOrder

	// 5. Conflict detection
	if err := detectConflicts(res.Modules); err != nil {
		return Resolution{}, fmt.Errorf("conflict detection: %w", err)
	}

	// 6. AC compatibility check
	if err := checkACCompatibility(res.Modules, res.ACCommit, opts.ACCompatPolicy); err != nil {
		return Resolution{}, fmt.Errorf("AC compatibility: %w", err)
	}

	return res, nil
}

func resolveACCommit(ctx context.Context, m *manifest.Manifest) (string, error) {
	if m.ACore.Commit != "" {
		return m.ACore.Commit, nil
	}

	// Need to fetch latest commit for branch via git ls-remote
	// For now, return the branch name; actual git ls-remote happens in cache layer
	return m.ACore.Branch, nil
}

func resolveModule(ctx context.Context, spec manifest.ModuleSpec, m *manifest.Manifest, idx *Index, acCommit string, opts ResolveOptions) (ResolvedModule, error) {
	rm := ResolvedModule{
		Spec:       spec,
		Version:    spec.Version,
		Repository: spec.Path, // will be overridden for catalogue source
	}

	switch spec.Source {
	case "catalogue":
		entry, ok := idx.Modules[spec.Name]
		if !ok {
			return ResolvedModule{}, fmt.Errorf("module %q not found in catalogue", spec.Name)
		}

		rm.Repository = entry.RepositoryURL
		rm.Conflicts = entry.Conflicts

		// Resolve version to commit SHA
		commit, version, err := resolveVersion(ctx, entry, spec.Version)
		if err != nil {
			return ResolvedModule{}, fmt.Errorf("resolve version: %w", err)
		}
		rm.Commit = commit
		rm.Version = version

		// Dependencies: union of manifest and catalogue (rich format)
		depMap := make(map[string]bool)
		for _, d := range spec.DependsOn {
			depMap[d] = true
		}
		for _, d := range entry.Dependencies {
			// d is now DependencySpec, extract name
			depMap[d.Name] = true
		}
		rm.Dependencies = mapKeys(depMap)

	case "git":
		rm.Repository = spec.Path
		if spec.Path == "" {
			// Try to get from catalogue
			if entry, ok := idx.Modules[spec.Name]; ok {
				rm.Repository = entry.RepositoryURL
			}
		}
		if rm.Repository == "" {
			return ResolvedModule{}, fmt.Errorf("git source requires repository path")
		}

		commit, version, err := resolveGitVersion(ctx, rm.Repository, spec.Version)
		if err != nil {
			return ResolvedModule{}, fmt.Errorf("resolve git version: %w", err)
		}
		rm.Commit = commit
		rm.Version = version
		// Dependencies from manifest only
		rm.Dependencies = spec.DependsOn
		if rm.Dependencies == nil {
			rm.Dependencies = []string{}
		}
		rm.Conflicts = []string{}
	case "local":
		rm.Repository = "local"
		rm.Commit = ""
		rm.Version = "local"
		rm.CachePath = spec.Path // will be resolved to absolute path by caller
		rm.Dependencies = spec.DependsOn
		if rm.Dependencies == nil {
			rm.Dependencies = []string{}
		}
		rm.Conflicts = []string{}
	default:
		return ResolvedModule{}, fmt.Errorf("unknown module source: %s", spec.Source)
	}

	return rm, nil
}

func resolveVersion(ctx context.Context, entry ModuleEntry, version string) (commit, resolvedVersion string, err error) {
	if version == "" {
		// Use latest release or default branch
		if entry.LatestRelease != nil {
			return entry.LatestRelease.TargetSHA, entry.LatestRelease.TagName, nil
		}
		return "", entry.DefaultBranch, nil
	}

	// Check if it's a commit SHA (40 hex chars)
	if len(version) == 40 && isHex(version) {
		// Verify it exists in releases
		for _, r := range entry.Releases {
			if r.TargetSHA == version {
				return version, r.TagName, nil
			}
		}
		// Not in releases, but could be a valid commit; assume it's valid
		return version, version[:12], nil
	}

	// Check if it's a tag
	for _, r := range entry.Releases {
		if r.TagName == version || r.TagName == "v"+version {
			return r.TargetSHA, r.TagName, nil
		}
	}

	// Check if it's a branch
	if version == entry.DefaultBranch || version == "main" || version == "master" {
		return "", version, nil // commit will be resolved via git ls-remote later
	}

	// Default to latest release
	if entry.LatestRelease != nil {
		return entry.LatestRelease.TargetSHA, entry.LatestRelease.TagName, nil
	}

	return "", entry.DefaultBranch, nil
}

func resolveGitVersion(ctx context.Context, repo, version string) (commit, resolvedVersion string, err error) {
	if version == "" {
		return "", "HEAD", nil
	}

	if len(version) == 40 && isHex(version) {
		return version, version[:12], nil
	}

	// For tags/branches, we'd need git ls-remote; for now return version
	return "", version, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func buildDepGraph(modules map[string]ResolvedModule) map[string][]string {
	graph := make(map[string][]string)
	for name, rm := range modules {
		// Ensure node exists
		if _, ok := graph[name]; !ok {
			graph[name] = nil
		}
		for _, dep := range rm.Dependencies {
			// Edge from dependency to dependent (dep -> name)
			graph[dep] = append(graph[dep], name)
			// Ensure dependent node exists
			if _, ok := graph[name]; !ok {
				graph[name] = nil
			}
		}
	}
	return graph
}

func topologicalSort(graph map[string][]string) ([]string, error) {
	// Kahn's algorithm
	indegree := make(map[string]int)
	for u := range graph {
		if _, ok := indegree[u]; !ok {
			indegree[u] = 0
		}
		for _, v := range graph[u] {
			indegree[v]++
			if _, ok := indegree[v]; !ok {
				indegree[v] = 0
			}
		}
	}

	// Queue of nodes with zero indegree
	queue := make([]string, 0)
	for node, deg := range indegree {
		if deg == 0 {
			queue = append(queue, node)
		}
	}
	sort.Strings(queue) // deterministic order

	var result []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		result = append(result, u)

		for _, v := range graph[u] {
			indegree[v]--
			if indegree[v] == 0 {
				queue = append(queue, v)
			}
		}
		sort.Strings(queue)
	}

	if len(result) != len(graph) {
		// Cycle detected - find and report it
		return nil, findCycle(graph)
	}

	return result, nil
}

func findCycle(graph map[string][]string) error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cycle []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		cycle = append(cycle, node)

		for _, neighbor := range graph[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				// Found cycle - find start of cycle in path
				startIdx := -1
				for i, n := range cycle {
					if n == neighbor {
						startIdx = i
						break
					}
				}
				if startIdx >= 0 {
					cycle = cycle[startIdx:]
					cycle = append(cycle, neighbor)
					return true
				}
				return true
			}
		}

		cycle = cycle[:len(cycle)-1]
		recStack[node] = false
		return false
	}

	for node := range graph {
		if !visited[node] {
			cycle = nil
			if dfs(node) && len(cycle) > 0 {
				return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
			}
		}
	}

	return errors.New("dependency cycle detected")
}

func detectConflicts(modules map[string]ResolvedModule) error {
	for name, rm := range modules {
		for _, conflict := range rm.Conflicts {
			if _, exists := modules[conflict]; exists {
				return fmt.Errorf("module %q conflicts with %q", name, conflict)
			}
		}
	}
	return nil
}

func checkACCompatibility(modules map[string]ResolvedModule, acCommit string, policy ACCompatPolicy) error {
	if policy == ACCompatIgnore {
		return nil
	}

	for name, rm := range modules {
		// We'd need the catalogue entry to check ACCompatibility
		// This is a placeholder - actual check needs catalogue entry
		_ = name
		_ = rm
		_ = acCommit
	}
	return nil
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
