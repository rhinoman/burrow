package contacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContactValidation(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Add(&Contact{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}

	err = s.Add(&Contact{Name: "   "})
	if err == nil {
		t.Error("expected error for whitespace-only name")
	}
}

func TestContactYAMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	original := &Contact{
		Name:         "Janet M. Liu",
		Email:        "janet.liu@cisa.dhs.gov",
		Organization: "CISA",
		Title:        "Program Manager",
		Phone:        "+1-555-0100",
		Tags:         []string{"government", "security"},
		Notes:        "Met at RSA Conference 2024",
	}

	if err := s.Add(original); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("janet-m-liu")
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != original.Name {
		t.Errorf("Name: got %q, want %q", got.Name, original.Name)
	}
	if got.Email != original.Email {
		t.Errorf("Email: got %q, want %q", got.Email, original.Email)
	}
	if got.Organization != original.Organization {
		t.Errorf("Organization: got %q, want %q", got.Organization, original.Organization)
	}
	if got.Title != original.Title {
		t.Errorf("Title: got %q, want %q", got.Title, original.Title)
	}
	if got.Phone != original.Phone {
		t.Errorf("Phone: got %q, want %q", got.Phone, original.Phone)
	}
	if len(got.Tags) != len(original.Tags) {
		t.Errorf("Tags length: got %d, want %d", len(got.Tags), len(original.Tags))
	} else {
		for i, tag := range got.Tags {
			if tag != original.Tags[i] {
				t.Errorf("Tags[%d]: got %q, want %q", i, tag, original.Tags[i])
			}
		}
	}
	if got.Notes != original.Notes {
		t.Errorf("Notes: got %q, want %q", got.Notes, original.Notes)
	}
	if got.Added == "" {
		t.Error("Added should be set automatically")
	}
}

func TestStoreAddAndGet(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	c := &Contact{
		Name:  "Alice Smith",
		Email: "alice@example.com",
	}
	if err := s.Add(c); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("alice-smith")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alice Smith" {
		t.Errorf("Name: got %q, want %q", got.Name, "Alice Smith")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("Email: got %q, want %q", got.Email, "alice@example.com")
	}
}

func TestStoreAddCollision(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	c1 := &Contact{Name: "John Smith", Email: "john1@example.com"}
	c2 := &Contact{Name: "John Smith", Email: "john2@example.com"}

	if err := s.Add(c1); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(c2); err != nil {
		t.Fatal(err)
	}

	// First should be at john-smith.yaml
	got1, err := s.Get("john-smith")
	if err != nil {
		t.Fatal(err)
	}
	if got1.Email != "john1@example.com" {
		t.Errorf("first contact Email: got %q, want %q", got1.Email, "john1@example.com")
	}

	// Second should be at john-smith-2.yaml
	got2, err := s.Get("john-smith-2")
	if err != nil {
		t.Fatal(err)
	}
	if got2.Email != "john2@example.com" {
		t.Errorf("second contact Email: got %q, want %q", got2.Email, "john2@example.com")
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	contacts := []*Contact{
		{Name: "Charlie Brown", Email: "charlie@example.com"},
		{Name: "Alice Smith", Email: "alice@example.com"},
		{Name: "Bob Jones", Email: "bob@example.com"},
	}
	for _, c := range contacts {
		if err := s.Add(c); err != nil {
			t.Fatal(err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("List length: got %d, want 3", len(list))
	}

	// Should be alphabetical
	if list[0].Name != "Alice Smith" {
		t.Errorf("list[0].Name: got %q, want %q", list[0].Name, "Alice Smith")
	}
	if list[1].Name != "Bob Jones" {
		t.Errorf("list[1].Name: got %q, want %q", list[1].Name, "Bob Jones")
	}
	if list[2].Name != "Charlie Brown" {
		t.Errorf("list[2].Name: got %q, want %q", list[2].Name, "Charlie Brown")
	}
}

func TestStoreRemove(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Add(&Contact{Name: "To Remove"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Remove("to-remove"); err != nil {
		t.Fatal(err)
	}

	_, err = s.Get("to-remove")
	if err == nil {
		t.Error("expected error after removal")
	}
}

func TestStoreRemoveNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Remove("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent slug")
	}
}

func TestStoreSearch(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(&Contact{Name: "Janet Liu", Email: "janet@cisa.gov", Organization: "CISA", Tags: []string{"security"}})
	s.Add(&Contact{Name: "John Smith", Email: "john@acme.com", Organization: "Acme Corp", Notes: "Met at trade show"})

	tests := []struct {
		query string
		want  int
	}{
		{"Janet", 1},
		{"cisa", 1},
		{"security", 1},
		{"trade show", 1},
		{".com", 1}, // only john@acme.com matches; janet has .gov
		{"acme", 1},
	}

	for _, tt := range tests {
		results, err := s.Search(tt.query)
		if err != nil {
			t.Errorf("Search(%q): %v", tt.query, err)
			continue
		}
		if len(results) != tt.want {
			t.Errorf("Search(%q): got %d results, want %d", tt.query, len(results), tt.want)
		}
	}
}

func TestStoreSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(&Contact{Name: "Janet Liu", Organization: "CISA"})

	results, err := s.Search("JANET")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}

	results, err = s.Search("cisa")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results for 'cisa', want 1", len(results))
	}
}

func TestStoreSearchNoResults(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(&Contact{Name: "Alice"})

	results, err := s.Search("zzzzz")
	if err != nil {
		t.Fatal(err)
	}
	if results == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestStoreLookup(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(&Contact{Name: "Janet M. Liu", Email: "janet@cisa.gov"})
	s.Add(&Contact{Name: "Janet Parker", Email: "janet@example.com"})

	// Exact match
	c := s.Lookup("Janet M. Liu")
	if c == nil || c.Email != "janet@cisa.gov" {
		t.Errorf("exact match failed: got %v", c)
	}

	// Prefix match — "Janet M" should match "Janet M. Liu"
	c = s.Lookup("Janet M")
	if c == nil || c.Email != "janet@cisa.gov" {
		t.Errorf("prefix match failed: got %v", c)
	}

	// Substring match — "Liu" should match "Janet M. Liu"
	c = s.Lookup("Liu")
	if c == nil || c.Email != "janet@cisa.gov" {
		t.Errorf("substring match failed: got %v", c)
	}
}

func TestStoreLookupNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(&Contact{Name: "Alice"})

	c := s.Lookup("Nonexistent Person")
	if c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestStoreForContext(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(&Contact{Name: "Janet M. Liu", Email: "janet.liu@cisa.dhs.gov", Organization: "CISA", Title: "Program Manager"})
	s.Add(&Contact{Name: "John Smith", Email: "john@example.com", Organization: "Acme Corp", Title: "CEO"})

	ctx := s.ForContext()
	if !strings.Contains(ctx, "## Contacts") {
		t.Error("missing header")
	}
	if !strings.Contains(ctx, "Janet M. Liu <janet.liu@cisa.dhs.gov> — CISA, Program Manager") {
		t.Errorf("unexpected format: %s", ctx)
	}
	if !strings.Contains(ctx, "John Smith <john@example.com> — Acme Corp, CEO") {
		t.Errorf("unexpected format: %s", ctx)
	}
}

func TestStoreForContextEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := s.ForContext()
	if ctx != "" {
		t.Errorf("expected empty string, got %q", ctx)
	}
}

func TestStoreCount(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if s.Count() != 0 {
		t.Errorf("empty store count: got %d, want 0", s.Count())
	}

	s.Add(&Contact{Name: "Alice"})
	s.Add(&Contact{Name: "Bob"})

	if s.Count() != 2 {
		t.Errorf("count: got %d, want 2", s.Count())
	}
}

// --- CSV Import Tests ---

func TestImportCSVStandard(t *testing.T) {
	dir := t.TempDir()
	csvFile := filepath.Join(dir, "contacts.csv")
	os.WriteFile(csvFile, []byte("Name,Email,Organization\nJanet Liu,janet@cisa.gov,CISA\nJohn Smith,john@acme.com,Acme Corp\n"), 0o644)

	contacts, warnings, err := ParseCSV(csvFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2", len(contacts))
	}
	if contacts[0].Name != "Janet Liu" {
		t.Errorf("contacts[0].Name: got %q", contacts[0].Name)
	}
	if contacts[0].Email != "janet@cisa.gov" {
		t.Errorf("contacts[0].Email: got %q", contacts[0].Email)
	}
	if contacts[0].Organization != "CISA" {
		t.Errorf("contacts[0].Organization: got %q", contacts[0].Organization)
	}
	_ = warnings
}

func TestImportCSVFirstLastName(t *testing.T) {
	dir := t.TempDir()
	csvFile := filepath.Join(dir, "contacts.csv")
	os.WriteFile(csvFile, []byte("First Name,Last Name,Email\nJanet,Liu,janet@cisa.gov\n"), 0o644)

	contacts, _, err := ParseCSV(csvFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Name != "Janet Liu" {
		t.Errorf("Name: got %q, want %q", contacts[0].Name, "Janet Liu")
	}
}

func TestImportCSVAlternateHeaders(t *testing.T) {
	dir := t.TempDir()
	csvFile := filepath.Join(dir, "contacts.csv")
	os.WriteFile(csvFile, []byte("Name,Email Address,Company,Job Title\nJanet Liu,janet@cisa.gov,CISA,Program Manager\n"), 0o644)

	contacts, _, err := ParseCSV(csvFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Email != "janet@cisa.gov" {
		t.Errorf("Email: got %q", contacts[0].Email)
	}
	if contacts[0].Organization != "CISA" {
		t.Errorf("Organization: got %q", contacts[0].Organization)
	}
	if contacts[0].Title != "Program Manager" {
		t.Errorf("Title: got %q", contacts[0].Title)
	}
}

func TestImportCSVMissingColumns(t *testing.T) {
	dir := t.TempDir()
	csvFile := filepath.Join(dir, "contacts.csv")
	os.WriteFile(csvFile, []byte("Name\nJanet Liu\nJohn Smith\n"), 0o644)

	contacts, _, err := ParseCSV(csvFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2", len(contacts))
	}
	if contacts[0].Email != "" {
		t.Errorf("expected empty Email, got %q", contacts[0].Email)
	}
}

func TestImportCSVEmpty(t *testing.T) {
	dir := t.TempDir()
	csvFile := filepath.Join(dir, "contacts.csv")
	os.WriteFile(csvFile, []byte(""), 0o644)

	contacts, _, err := ParseCSV(csvFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 0 {
		t.Errorf("got %d contacts, want 0", len(contacts))
	}
}

func TestImportCSVTags(t *testing.T) {
	dir := t.TempDir()
	csvFile := filepath.Join(dir, "contacts.csv")
	os.WriteFile(csvFile, []byte("Name,Tags\nJanet Liu,security;government;federal\n"), 0o644)

	contacts, _, err := ParseCSV(csvFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if len(contacts[0].Tags) != 3 {
		t.Errorf("got %d tags, want 3: %v", len(contacts[0].Tags), contacts[0].Tags)
	}
}

// --- vCard Import Tests ---

func TestImportVCardSingle(t *testing.T) {
	data := []byte(`BEGIN:VCARD
VERSION:3.0
FN:Janet M. Liu
EMAIL:janet@cisa.gov
ORG:CISA
TITLE:Program Manager
TEL:+1-555-0100
NOTE:Met at RSA Conference
END:VCARD
`)
	contacts, _, err := ParseVCard(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	c := contacts[0]
	if c.Name != "Janet M. Liu" {
		t.Errorf("Name: got %q", c.Name)
	}
	if c.Email != "janet@cisa.gov" {
		t.Errorf("Email: got %q", c.Email)
	}
	if c.Organization != "CISA" {
		t.Errorf("Organization: got %q", c.Organization)
	}
	if c.Title != "Program Manager" {
		t.Errorf("Title: got %q", c.Title)
	}
	if c.Phone != "+1-555-0100" {
		t.Errorf("Phone: got %q", c.Phone)
	}
	if c.Notes != "Met at RSA Conference" {
		t.Errorf("Notes: got %q", c.Notes)
	}
}

func TestImportVCardMultiple(t *testing.T) {
	data := []byte(`BEGIN:VCARD
VERSION:3.0
FN:Janet Liu
EMAIL:janet@cisa.gov
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:John Smith
EMAIL:john@acme.com
END:VCARD
`)
	contacts, _, err := ParseVCard(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2", len(contacts))
	}
	if contacts[0].Name != "Janet Liu" {
		t.Errorf("contacts[0].Name: got %q", contacts[0].Name)
	}
	if contacts[1].Name != "John Smith" {
		t.Errorf("contacts[1].Name: got %q", contacts[1].Name)
	}
}

func TestImportVCardLineUnfolding(t *testing.T) {
	data := []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Janet \r\n M. Liu\r\nEMAIL:janet@cisa.gov\r\nEND:VCARD\r\n")
	contacts, _, err := ParseVCard(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Name != "Janet M. Liu" {
		t.Errorf("Name after unfolding: got %q, want %q", contacts[0].Name, "Janet M. Liu")
	}
}

func TestImportVCardPropertyParams(t *testing.T) {
	data := []byte(`BEGIN:VCARD
VERSION:3.0
FN:Janet Liu
TEL;TYPE=work:+1-555-0100
EMAIL;TYPE=work;PREF=1:janet@cisa.gov
END:VCARD
`)
	contacts, _, err := ParseVCard(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Phone != "+1-555-0100" {
		t.Errorf("Phone: got %q, want %q", contacts[0].Phone, "+1-555-0100")
	}
	if contacts[0].Email != "janet@cisa.gov" {
		t.Errorf("Email: got %q, want %q", contacts[0].Email, "janet@cisa.gov")
	}
}

func TestImportVCardMinimal(t *testing.T) {
	data := []byte(`BEGIN:VCARD
VERSION:3.0
FN:Janet Liu
END:VCARD
`)
	contacts, _, err := ParseVCard(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}
	if contacts[0].Name != "Janet Liu" {
		t.Errorf("Name: got %q", contacts[0].Name)
	}
	if contacts[0].Email != "" {
		t.Errorf("expected empty Email, got %q", contacts[0].Email)
	}
}
