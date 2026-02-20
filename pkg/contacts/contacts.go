// Package contacts provides a YAML-file-per-contact store with CRUD,
// search, and lookup. Contact data never leaves the local machine.
package contacts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/slug"
	"gopkg.in/yaml.v3"
)

// Contact represents a single person in the user's contact store.
type Contact struct {
	Name         string   `yaml:"name"`
	Email        string   `yaml:"email,omitempty"`
	Organization string   `yaml:"organization,omitempty"`
	Title        string   `yaml:"title,omitempty"`
	Phone        string   `yaml:"phone,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
	Notes        string   `yaml:"notes,omitempty"`
	Added        string   `yaml:"added,omitempty"` // YYYY-MM-DD
}

// Store manages contacts as individual YAML files in a directory.
type Store struct {
	dir string
}

// NewStore creates a Store rooted at dir, creating the directory if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating contacts directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Add writes a contact to disk. The filename is derived from the contact's
// name via slug.Sanitize. If a file with that name already exists, an
// incrementing suffix (-2, -3, ...) is appended.
func (s *Store) Add(c *Contact) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("contact name is required")
	}

	if c.Added == "" {
		c.Added = time.Now().Format("2006-01-02")
	}

	base := slug.Sanitize(c.Name)
	filename := base + ".yaml"
	path := filepath.Join(s.dir, filename)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		filename = fmt.Sprintf("%s-%d.yaml", base, i)
		path = filepath.Join(s.dir, filename)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling contact: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// Get reads a contact by its slug (filename without .yaml extension).
func (s *Store) Get(slugName string) (*Contact, error) {
	path := filepath.Join(s.dir, slugName+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading contact %q: %w", slugName, err)
	}
	var c Contact
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing contact %q: %w", slugName, err)
	}
	return &c, nil
}

// List returns all contacts sorted alphabetically by name.
func (s *Store) List() ([]*Contact, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing contacts: %w", err)
	}

	var contacts []*Contact
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var c Contact
		if err := yaml.Unmarshal(data, &c); err != nil {
			continue
		}
		contacts = append(contacts, &c)
	}

	sort.Slice(contacts, func(i, j int) bool {
		return strings.ToLower(contacts[i].Name) < strings.ToLower(contacts[j].Name)
	})

	return contacts, nil
}

// Remove deletes a contact file by slug.
func (s *Store) Remove(slugName string) error {
	path := filepath.Join(s.dir, slugName+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("contact %q not found", slugName)
	}
	return os.Remove(path)
}

// Search returns contacts matching query (case-insensitive substring)
// across all text fields.
func (s *Store) Search(query string) ([]*Contact, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(query)
	var results []*Contact
	for _, c := range all {
		if contactMatches(c, q) {
			results = append(results, c)
		}
	}
	if results == nil {
		results = []*Contact{}
	}
	return results, nil
}

// Lookup finds the best-matching contact by name. It tries exact match,
// then case-insensitive exact, then prefix, then substring. Returns nil
// if no match found.
func (s *Store) Lookup(name string) *Contact {
	all, err := s.List()
	if err != nil || len(all) == 0 {
		return nil
	}

	// Exact match
	for _, c := range all {
		if c.Name == name {
			return c
		}
	}

	lower := strings.ToLower(name)

	// Case-insensitive exact
	for _, c := range all {
		if strings.ToLower(c.Name) == lower {
			return c
		}
	}

	// Case-insensitive prefix
	for _, c := range all {
		if strings.HasPrefix(strings.ToLower(c.Name), lower) {
			return c
		}
	}

	// Case-insensitive substring
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.Name), lower) {
			return c
		}
	}

	return nil
}

// ForContext formats all contacts as a compact text block suitable for
// LLM context injection. Returns empty string if no contacts.
func (s *Store) ForContext() string {
	all, err := s.List()
	if err != nil || len(all) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Contacts\n\n")
	for _, c := range all {
		b.WriteString("- ")
		b.WriteString(c.Name)
		if c.Email != "" {
			b.WriteString(" <")
			b.WriteString(c.Email)
			b.WriteString(">")
		}
		if c.Organization != "" || c.Title != "" {
			b.WriteString(" — ")
			parts := []string{}
			if c.Organization != "" {
				parts = append(parts, c.Organization)
			}
			if c.Title != "" {
				parts = append(parts, c.Title)
			}
			b.WriteString(strings.Join(parts, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Count returns the number of contacts in the store.
func (s *Store) Count() int {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			count++
		}
	}
	return count
}

// SlugFor returns the slug for a contact name.
func SlugFor(name string) string {
	return slug.Sanitize(name)
}

// contactMatches checks if a contact matches a lowercase query string
// across all searchable fields.
func contactMatches(c *Contact, q string) bool {
	if strings.Contains(strings.ToLower(c.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(c.Email), q) {
		return true
	}
	if strings.Contains(strings.ToLower(c.Organization), q) {
		return true
	}
	if strings.Contains(strings.ToLower(c.Title), q) {
		return true
	}
	if strings.Contains(strings.ToLower(c.Notes), q) {
		return true
	}
	if strings.Contains(strings.ToLower(strings.Join(c.Tags, " ")), q) {
		return true
	}
	return false
}
