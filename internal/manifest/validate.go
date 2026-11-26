package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrCode constants for structured error handling
	ErrCodeSchema  = "MANIFEST_SCHEMA"
	ErrCodeVersion = "MANIFEST_VERSION"
	ErrCodeRef     = "MANIFEST_REF"
	ErrCodeCycle   = "MANIFEST_CYCLE"
	ErrCodeIO      = "MANIFEST_IO"
	ErrCodeInvalid = "MANIFEST_INVALID"
)

// ValidationError represents a structured validation error
type ValidationError struct {
	Code    string
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Validate validates the manifest against all rules
func Validate(m *Manifest, manifestDir string) error {
	var errs []*ValidationError

	// Rule 1: Version == "1"
	if m.Version != "1" {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeVersion,
			Field:   "version",
			Message: fmt.Sprintf("unsupported version %q, expected \"1\"", m.Version),
		})
	}

	// Rule 2: Project.Name required, regex ^[a-zA-Z0-9][a-zA-Z0-9_-]*$
	if m.Project.Name == "" {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeInvalid,
			Field:   "project.name",
			Message: "required",
		})
	} else if !projectNameRegex.MatchString(m.Project.Name) {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeInvalid,
			Field:   "project.name",
			Message: "must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$",
		})
	}

	// Rule 3: ACore.Repo valid Git URL; Branch or Commit required
	if m.ACore.Repo == "" {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeInvalid,
			Field:   "acore.repo",
			Message: "required",
		})
	} else if !isValidGitURL(m.ACore.Repo) {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeInvalid,
			Field:   "acore.repo",
			Message: "must be a valid Git URL (http/https/git@)",
		})
	}
	if m.ACore.Branch == "" && m.ACore.Commit == "" {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeInvalid,
			Field:   "acore",
			Message: "either branch or commit must be specified",
		})
	}

	// Rule 4: Module Name unique across Modules
	moduleNames := make(map[string]int)
	for i, mod := range m.Modules {
		if mod.Name == "" {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeInvalid,
				Field:   fmt.Sprintf("modules[%d].name", i),
				Message: "required",
			})
			continue
		}
		if prev, ok := moduleNames[mod.Name]; ok {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeInvalid,
				Field:   fmt.Sprintf("modules[%d].name", i),
				Message: fmt.Sprintf("duplicate module name %q (first at index %d)", mod.Name, prev),
			})
		}
		moduleNames[mod.Name] = i
	}

	// Rule 5: Module Source defaults to catalogue; allowed: catalogue, local, git
	validSources := map[string]bool{"catalogue": true, "local": true, "git": true}
	for i, mod := range m.Modules {
		src := mod.Source
		if src == "" {
			src = "catalogue"
		}
		if !validSources[src] {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeInvalid,
				Field:   fmt.Sprintf("modules[%d].source", i),
				Message: fmt.Sprintf("invalid source %q, allowed: catalogue, local, git", src),
			})
		}
	}

	// Rule 6: DependsOn references existing module names; no cycles
	moduleNameSet := make(map[string]bool)
	for _, mod := range m.Modules {
		moduleNameSet[mod.Name] = true
	}

	// Build adjacency list for cycle detection
	adj := make(map[string][]string)
	for _, mod := range m.Modules {
		for _, dep := range mod.DependsOn {
			if !moduleNameSet[dep] {
				errs = append(errs, &ValidationError{
					Code:    ErrCodeRef,
					Field:   fmt.Sprintf("modules[%s].depends_on", mod.Name),
					Message: fmt.Sprintf("unknown module reference %q", dep),
				})
			}
			adj[mod.Name] = append(adj[mod.Name], dep)
		}
	}

	// Cycle detection using DFS
	if len(errs) == 0 {
		if cycle := detectCycle(adj); cycle != nil {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeCycle,
				Field:   "modules",
				Message: fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")),
			})
		}
	}

	// Rule 7: Database.Migrations/Seed files exist relative to manifestDir
	for i, migration := range m.Database.Migrations {
		path := migration
		if !filepath.IsAbs(path) {
			path = filepath.Join(manifestDir, path)
		}
		if _, err := os.Stat(path); err != nil {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeIO,
				Field:   fmt.Sprintf("database.migrations[%d]", i),
				Message: fmt.Sprintf("file not found: %s", path),
			})
		}
	}
	for i, seed := range m.Database.Seed {
		path := seed
		if !filepath.IsAbs(path) {
			path = filepath.Join(manifestDir, path)
		}
		if _, err := os.Stat(path); err != nil {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeIO,
				Field:   fmt.Sprintf("database.seed[%d]", i),
				Message: fmt.Sprintf("file not found: %s", path),
			})
		}
	}

	// Rule 8: ClientData.Source must be "static" (only supported source)
	validClientSources := map[string]bool{"static": true}
	if !validClientSources[m.ClientData.Source] {
		errs = append(errs, &ValidationError{
			Code:    ErrCodeInvalid,
			Field:   "client_data.source",
			Message: fmt.Sprintf("invalid source %q, only 'static' is supported", m.ClientData.Source),
		})
	} else if m.ClientData.Source == "static" {
		// For static source, validate version if provided
		if m.ClientData.Version != "" && !strings.HasPrefix(m.ClientData.Version, "v") {
			errs = append(errs, &ValidationError{
				Code:    ErrCodeInvalid,
				Field:   "client_data.version",
				Message: "version must start with 'v' (e.g., v20.0)",
			})
		}
	}

	// Rule 9: Compose.Volumes host paths: expand ~, check parent dir exists or creatable
	for hostPath := range m.Compose.Volumes {
		expanded := expandHome(hostPath)
		parent := filepath.Dir(expanded)
		if _, err := os.Stat(parent); err != nil {
			// Check if we can create it (parent of parent exists)
			grandParent := filepath.Dir(parent)
			if _, err := os.Stat(grandParent); err != nil {
				errs = append(errs, &ValidationError{
					Code:    ErrCodeInvalid,
					Field:   fmt.Sprintf("compose.volumes[%s]", hostPath),
					Message: fmt.Sprintf("host path parent not creatable: %s", parent),
				})
			}
		}
	}

	// Rule 10: Environment.Secrets keys must be present in .env (validated at orchestration time)
	// We just note this here; actual validation happens in orchestration

	if len(errs) > 0 {
		errList := make([]error, len(errs))
		for i, e := range errs {
			errList[i] = e
		}
		return errors.Join(errList...)
	}
	return nil
}

var projectNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func isValidGitURL(url string) bool {
	// http/https URLs
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}
	// git@ SSH URLs
	if strings.HasPrefix(url, "git@") && strings.Contains(url, ":") {
		return true
	}
	// git:// URLs
	if strings.HasPrefix(url, "git://") {
		return true
	}
	return false
}

func detectCycle(adj map[string][]string) []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cycle []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		cycle = append(cycle, node)

		for _, neighbor := range adj[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				// Found cycle - extract cycle path
				idx := -1
				for i, n := range cycle {
					if n == neighbor {
						idx = i
						break
					}
				}
				if idx >= 0 {
					cycle = cycle[idx:]
				}
				return true
			}
		}

		recStack[node] = false
		cycle = cycle[:len(cycle)-1]
		return false
	}

	for node := range adj {
		if !visited[node] {
			if dfs(node) {
				return cycle
			}
		}
	}
	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
