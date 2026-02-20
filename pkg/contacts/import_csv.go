package contacts

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// ParseCSV reads a CSV file and returns parsed contacts and any warnings
// (e.g., unmapped columns). Recognized headers (case-insensitive):
// Name, First Name, Last Name, Email, Email Address, Organization,
// Company, Title, Job Title, Phone, Phone Number, Tags, Notes.
func ParseCSV(path string) ([]*Contact, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening CSV: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CSV: %w", err)
	}

	if len(records) < 1 {
		return nil, nil, nil
	}

	// Map header names to column indices
	header := records[0]
	colMap := make(map[string]int)
	var warnings []string

	for i, h := range header {
		normalized := strings.ToLower(strings.TrimSpace(h))
		switch normalized {
		case "name":
			colMap["name"] = i
		case "first name", "firstname":
			colMap["first_name"] = i
		case "last name", "lastname":
			colMap["last_name"] = i
		case "email", "email address", "e-mail":
			colMap["email"] = i
		case "organization", "company", "org":
			colMap["organization"] = i
		case "title", "job title", "jobtitle":
			colMap["title"] = i
		case "phone", "phone number", "telephone":
			colMap["phone"] = i
		case "tags", "labels", "categories":
			colMap["tags"] = i
		case "notes", "note":
			colMap["notes"] = i
		default:
			if normalized != "" {
				warnings = append(warnings, fmt.Sprintf("unmapped column: %q", h))
			}
		}
	}

	var contacts []*Contact
	for _, row := range records[1:] {
		c := &Contact{}

		if idx, ok := colMap["name"]; ok && idx < len(row) {
			c.Name = strings.TrimSpace(row[idx])
		}

		// Combine first + last name if "name" column wasn't present or is empty
		if c.Name == "" {
			var parts []string
			if idx, ok := colMap["first_name"]; ok && idx < len(row) {
				if v := strings.TrimSpace(row[idx]); v != "" {
					parts = append(parts, v)
				}
			}
			if idx, ok := colMap["last_name"]; ok && idx < len(row) {
				if v := strings.TrimSpace(row[idx]); v != "" {
					parts = append(parts, v)
				}
			}
			c.Name = strings.Join(parts, " ")
		}

		if idx, ok := colMap["email"]; ok && idx < len(row) {
			c.Email = strings.TrimSpace(row[idx])
		}
		if idx, ok := colMap["organization"]; ok && idx < len(row) {
			c.Organization = strings.TrimSpace(row[idx])
		}
		if idx, ok := colMap["title"]; ok && idx < len(row) {
			c.Title = strings.TrimSpace(row[idx])
		}
		if idx, ok := colMap["phone"]; ok && idx < len(row) {
			c.Phone = strings.TrimSpace(row[idx])
		}
		if idx, ok := colMap["tags"]; ok && idx < len(row) {
			raw := strings.TrimSpace(row[idx])
			if raw != "" {
				for _, t := range strings.Split(raw, ";") {
					t = strings.TrimSpace(t)
					if t != "" {
						c.Tags = append(c.Tags, t)
					}
				}
			}
		}
		if idx, ok := colMap["notes"]; ok && idx < len(row) {
			c.Notes = strings.TrimSpace(row[idx])
		}

		// Skip rows with no name
		if c.Name == "" {
			continue
		}

		contacts = append(contacts, c)
	}

	return contacts, warnings, nil
}

// ImportCSV parses a CSV file and adds each contact to the store.
// Returns the number of contacts successfully added and any warnings.
func (s *Store) ImportCSV(path string) (int, []string, error) {
	contacts, warnings, err := ParseCSV(path)
	if err != nil {
		return 0, warnings, err
	}

	added := 0
	for _, c := range contacts {
		if err := s.Add(c); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %q: %v", c.Name, err))
			continue
		}
		added++
	}

	return added, warnings, nil
}
