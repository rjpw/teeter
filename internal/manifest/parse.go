package manifest

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseError represents a structured parsing error
type ParseError struct {
	Path    string
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Message)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// ParseFile parses a manifest from a file path
func ParseFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{Path: path, Message: err.Error()}
	}
	return ParseBytes(data, path)
}

// ParseBytes parses a manifest from byte slice
func ParseBytes(data []byte, path ...string) (*Manifest, error) {
	var m Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		var line int
		// Try to extract line number from error message if available
		p := ""
		if len(path) > 0 {
			p = path[0]
		}
		return nil, &ParseError{Path: p, Line: line, Message: err.Error()}
	}

	// Apply defaults
	if m.ClientData.Source == "" {
		m.ClientData.Source = "local"
	}
	for i := range m.Modules {
		if m.Modules[i].Source == "" {
			m.Modules[i].Source = "catalogue"
		}
	}

	return &m, nil
}
