// Package profile handles the user profile (~/.burrow/profile.yaml).
//
// The profile declares user identity, interests, competitors, and other
// domain-specific fields once. These flow into source query params,
// synthesis prompts, ask/interactive context, and draft generation via
// {{profile.field}} template syntax.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile holds user identity and domain context. Three well-known fields
// (Name, Description, Interests) are typed for convenience. Everything
// (including those three) lives in the Raw map for template expansion,
// so arbitrary user-defined fields like "competitors" or "naics_codes"
// work identically.
type Profile struct {
	Name        string   `yaml:"name,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Interests   []string `yaml:"interests,omitempty"`

	// Raw holds every field from the YAML including the typed ones above.
	// This is the source of truth for template expansion.
	Raw map[string]interface{} `yaml:"-"`
}

const filename = "profile.yaml"

// Load reads the profile from burrowDir/profile.yaml.
// Returns (nil, nil) when the file does not exist — the profile is optional.
func Load(burrowDir string) (*Profile, error) {
	path := filepath.Join(burrowDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading profile: %w", err)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile: %w", err)
	}

	// Second pass: unmarshal into raw map so every field is available
	// for template expansion (including user-defined ad-hoc fields).
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing profile raw map: %w", err)
	}
	p.Raw = raw

	return &p, nil
}

// Save writes the profile to burrowDir/profile.yaml. It marshals the
// Raw map to preserve user-defined fields that aren't in the typed struct.
func Save(burrowDir string, p *Profile) error {
	if err := os.MkdirAll(burrowDir, 0o755); err != nil {
		return fmt.Errorf("creating burrow directory: %w", err)
	}

	// Build the raw map from typed fields if Raw is nil (e.g. freshly
	// constructed in the wizard). Otherwise use Raw as the authority.
	raw := p.Raw
	if raw == nil {
		raw = make(map[string]interface{})
	}
	// Sync typed fields into raw map so they're always written.
	if p.Name != "" {
		raw["name"] = p.Name
	}
	if p.Description != "" {
		raw["description"] = p.Description
	}
	if len(p.Interests) > 0 {
		raw["interests"] = p.Interests
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	header := "# Burrow user profile — identity, interests, and domain context\n" +
		"# Referenced in routines via {{profile.field_name}}\n" +
		"# Edit directly or use: gd configure\n\n"

	path := filepath.Join(burrowDir, filename)
	return os.WriteFile(path, []byte(header+string(data)), 0o644)
}

// Get returns a string value for the given key from the Raw map.
// List values are joined with ", ". Returns ("", false) for missing keys.
func (p *Profile) Get(key string) (string, bool) {
	if p == nil || p.Raw == nil {
		return "", false
	}
	val, ok := p.Raw[key]
	if !ok {
		return "", false
	}
	return formatValue(val), true
}

// GetList returns a string slice for the given key. Returns (nil, false)
// for missing keys or non-list values.
func (p *Profile) GetList(key string) ([]string, bool) {
	if p == nil || p.Raw == nil {
		return nil, false
	}
	val, ok := p.Raw[key]
	if !ok {
		return nil, false
	}
	switch v := val.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out, true
	case []string:
		return v, true
	default:
		return nil, false
	}
}

// formatValue converts a raw YAML value to a display string.
// Lists become comma-separated.
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(v, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}
