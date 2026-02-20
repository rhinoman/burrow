package contacts

import (
	"fmt"
	"os"
	"strings"
)

// ParseVCard parses vCard 3.0/4.0 data and returns contacts.
// Extracts FN, EMAIL, ORG, TITLE, TEL, NOTE fields.
func ParseVCard(data []byte) ([]*Contact, []string, error) {
	lines := unfoldLines(string(data))
	var contacts []*Contact
	var warnings []string
	var current *Contact

	for _, line := range lines {
		upper := strings.ToUpper(line)

		if upper == "BEGIN:VCARD" {
			current = &Contact{}
			continue
		}

		if upper == "END:VCARD" {
			if current != nil && current.Name != "" {
				contacts = append(contacts, current)
			} else if current != nil {
				warnings = append(warnings, "skipping vCard with no FN field")
			}
			current = nil
			continue
		}

		if current == nil {
			continue
		}

		prop, value := parseVCardLine(line)
		switch strings.ToUpper(prop) {
		case "FN":
			current.Name = value
		case "EMAIL":
			if current.Email == "" {
				current.Email = value
			}
		case "ORG":
			// ORG can have semicolon-separated components (org;unit;...)
			current.Organization = strings.Split(value, ";")[0]
		case "TITLE":
			current.Title = value
		case "TEL":
			if current.Phone == "" {
				current.Phone = value
			}
		case "NOTE":
			current.Notes = value
		}
	}

	return contacts, warnings, nil
}

// maxVCardSize is the maximum file size accepted for vCard imports (10 MB).
const maxVCardSize = 10 << 20

// ImportVCard reads a .vcf file and adds each contact to the store.
// Returns the number of contacts successfully added and any warnings.
// Rejects files larger than 10 MB.
func (s *Store) ImportVCard(path string) (int, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, nil, fmt.Errorf("reading vCard file: %w", err)
	}
	if info.Size() > maxVCardSize {
		return 0, nil, fmt.Errorf("vCard file too large (%d bytes, max %d)", info.Size(), maxVCardSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, fmt.Errorf("reading vCard file: %w", err)
	}

	return s.ImportVCardData(data)
}

// ImportVCardData parses vCard data and adds each contact to the store.
// Returns the number of contacts successfully added and any warnings.
func (s *Store) ImportVCardData(data []byte) (int, []string, error) {
	contacts, warnings, err := ParseVCard(data)
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

// unfoldLines handles vCard line unfolding: lines starting with a space
// or tab are continuations of the previous line.
func unfoldLines(data string) []string {
	raw := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var lines []string

	for _, line := range raw {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			// Continuation line — append to previous
			if len(lines) > 0 {
				lines[len(lines)-1] += line[1:]
			}
		} else {
			lines = append(lines, line)
		}
	}

	return lines
}

// parseVCardLine extracts the property name (without parameters) and
// the value from a vCard line like "TEL;TYPE=work:+1234567890".
func parseVCardLine(line string) (prop, value string) {
	// Split on first colon to separate property from value
	colonIdx := strings.Index(line, ":")
	if colonIdx < 0 {
		return line, ""
	}

	propPart := line[:colonIdx]
	value = line[colonIdx+1:]

	// Strip parameters (everything after the first semicolon in propPart)
	if semiIdx := strings.Index(propPart, ";"); semiIdx >= 0 {
		prop = propPart[:semiIdx]
	} else {
		prop = propPart
	}

	return prop, value
}
