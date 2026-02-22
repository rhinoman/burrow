package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &Profile{
		Name:        "Trivyn",
		Description: "Small geospatial intelligence firm.",
		Interests:   []string{"knowledge graphs", "geospatial analytics"},
		Raw: map[string]interface{}{
			"name":        "Trivyn",
			"description": "Small geospatial intelligence firm.",
			"interests":   []interface{}{"knowledge graphs", "geospatial analytics"},
			"competitors": []interface{}{"Maxar", "BlackSky"},
		},
	}

	if err := Save(dir, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.Description != original.Description {
		t.Errorf("Description = %q, want %q", loaded.Description, original.Description)
	}
	if len(loaded.Interests) != len(original.Interests) {
		t.Fatalf("Interests length = %d, want %d", len(loaded.Interests), len(original.Interests))
	}
	for i, v := range loaded.Interests {
		if v != original.Interests[i] {
			t.Errorf("Interests[%d] = %q, want %q", i, v, original.Interests[i])
		}
	}
	// Check ad-hoc field survived round-trip
	val, ok := loaded.Get("competitors")
	if !ok {
		t.Fatal("competitors not found in loaded profile")
	}
	if val != "Maxar, BlackSky" {
		t.Errorf("competitors = %q, want %q", val, "Maxar, BlackSky")
	}
}

func TestLoadMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()

	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p != nil {
		t.Error("expected nil profile for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(path, []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestGet(t *testing.T) {
	p := &Profile{
		Raw: map[string]interface{}{
			"name":        "Trivyn",
			"interests":   []interface{}{"knowledge graphs", "geospatial"},
			"description": "A small firm.",
			"naics_codes": []interface{}{"541370", "541512"},
		},
	}

	// String field
	val, ok := p.Get("name")
	if !ok || val != "Trivyn" {
		t.Errorf("Get(name) = (%q, %v)", val, ok)
	}

	// List field → comma-separated
	val, ok = p.Get("interests")
	if !ok || val != "knowledge graphs, geospatial" {
		t.Errorf("Get(interests) = (%q, %v)", val, ok)
	}

	// Missing field
	_, ok = p.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}

	// Nil profile
	var nilP *Profile
	_, ok = nilP.Get("name")
	if ok {
		t.Error("Get on nil profile should return false")
	}
}

func TestGetList(t *testing.T) {
	p := &Profile{
		Raw: map[string]interface{}{
			"interests": []interface{}{"knowledge graphs", "geospatial"},
			"name":      "Trivyn",
		},
	}

	// List field
	list, ok := p.GetList("interests")
	if !ok {
		t.Fatal("GetList(interests) returned false")
	}
	if len(list) != 2 || list[0] != "knowledge graphs" || list[1] != "geospatial" {
		t.Errorf("GetList(interests) = %v", list)
	}

	// Non-list field
	_, ok = p.GetList("name")
	if ok {
		t.Error("GetList(name) should return false for non-list")
	}

	// Missing field
	_, ok = p.GetList("nonexistent")
	if ok {
		t.Error("GetList(nonexistent) should return false")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	p := &Profile{Name: "Test"}
	if err := Save(dir, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "profile.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("profile.yaml not created: %v", err)
	}
}

func TestGetNestedField(t *testing.T) {
	p := &Profile{
		Raw: map[string]interface{}{
			"location": map[string]interface{}{
				"latitude":  "61.22",
				"longitude": "-149.90",
				"details": map[string]interface{}{
					"city": "Anchorage",
				},
			},
		},
	}

	val, ok := p.Get("location.latitude")
	if !ok || val != "61.22" {
		t.Errorf("Get(location.latitude) = (%q, %v)", val, ok)
	}

	// Three levels deep
	val, ok = p.Get("location.details.city")
	if !ok || val != "Anchorage" {
		t.Errorf("Get(location.details.city) = (%q, %v)", val, ok)
	}
}

func TestGetNestedMissing(t *testing.T) {
	p := &Profile{
		Raw: map[string]interface{}{
			"location": map[string]interface{}{
				"latitude": "61.22",
			},
		},
	}

	// Missing leaf
	_, ok := p.Get("location.nonexistent")
	if ok {
		t.Error("Get(location.nonexistent) should return false")
	}

	// Missing intermediate
	_, ok = p.Get("missing.latitude")
	if ok {
		t.Error("Get(missing.latitude) should return false")
	}

	// Non-map intermediate
	_, ok = p.Get("location.latitude.sub")
	if ok {
		t.Error("Get(location.latitude.sub) should return false for non-map intermediate")
	}
}

func TestSaveFromTypedFieldsOnly(t *testing.T) {
	dir := t.TempDir()

	// Profile built from wizard — has typed fields but no Raw map
	p := &Profile{
		Name:        "Test",
		Description: "A test profile.",
		Interests:   []string{"go", "testing"},
	}

	if err := Save(dir, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != "Test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "Test")
	}
	if len(loaded.Interests) != 2 {
		t.Errorf("Interests length = %d, want 2", len(loaded.Interests))
	}
}
