package profile

import (
	"testing"
)

func testProfile() *Profile {
	return &Profile{
		Name:        "Trivyn",
		Description: "Small geospatial intelligence firm.",
		Interests:   []string{"knowledge graphs", "geospatial analytics"},
		Raw: map[string]interface{}{
			"name":        "Trivyn",
			"description": "Small geospatial intelligence firm.",
			"interests":   []interface{}{"knowledge graphs", "geospatial analytics"},
			"competitors": []interface{}{"Maxar", "BlackSky"},
			"naics_codes": []interface{}{"541370", "541512"},
		},
	}
}

func TestExpandStringField(t *testing.T) {
	result, err := Expand("Report for {{profile.name}}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Report for Trivyn" {
		t.Errorf("got %q", result)
	}
}

func TestExpandListField(t *testing.T) {
	result, err := Expand("Prioritize: {{profile.interests}}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Prioritize: knowledge graphs, geospatial analytics"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestExpandNilProfile(t *testing.T) {
	text := "Hello {{profile.name}}"
	result, err := Expand(text, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != text {
		t.Errorf("got %q, want unchanged %q", result, text)
	}
}

func TestExpandMissingField(t *testing.T) {
	result, err := Expand("Hello {{profile.unknown}}", testProfile())
	if err == nil {
		t.Fatal("expected error for unresolved field")
	}
	// Text should be unchanged for the missing ref
	if result != "Hello {{profile.unknown}}" {
		t.Errorf("got %q, want unresolved ref left as-is", result)
	}
}

func TestExpandMultiple(t *testing.T) {
	text := "Brief for {{profile.name}}: {{profile.interests}}. Watch: {{profile.competitors}}"
	result, err := Expand(text, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Brief for Trivyn: knowledge graphs, geospatial analytics. Watch: Maxar, BlackSky"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestExpandEmptyText(t *testing.T) {
	result, err := Expand("", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("got %q, want empty", result)
	}
}

func TestExpandNoRefs(t *testing.T) {
	text := "No profile references here."
	result, err := Expand(text, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != text {
		t.Errorf("got %q, want %q", result, text)
	}
}

func TestExpandParams(t *testing.T) {
	params := map[string]string{
		"naics":   "{{profile.naics_codes}}",
		"keyword": "{{profile.interests}}",
		"static":  "unchanged",
	}

	expanded, err := ExpandParams(params, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original map must be untouched
	if params["naics"] != "{{profile.naics_codes}}" {
		t.Error("original params were modified")
	}

	if expanded["naics"] != "541370, 541512" {
		t.Errorf("naics = %q", expanded["naics"])
	}
	if expanded["keyword"] != "knowledge graphs, geospatial analytics" {
		t.Errorf("keyword = %q", expanded["keyword"])
	}
	if expanded["static"] != "unchanged" {
		t.Errorf("static = %q", expanded["static"])
	}
}

func TestExpandParamsNilProfile(t *testing.T) {
	params := map[string]string{"key": "{{profile.name}}"}
	result, err := ExpandParams(params, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "{{profile.name}}" {
		t.Errorf("got %q, want unchanged", result["key"])
	}
}

func TestExpandParamsNilMap(t *testing.T) {
	result, err := ExpandParams(nil, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("got %v, want nil", result)
	}
}

func TestExpandParamsMissingField(t *testing.T) {
	params := map[string]string{"key": "{{profile.missing}}"}
	result, err := ExpandParams(params, testProfile())
	if err == nil {
		t.Fatal("expected error for unresolved field")
	}
	// Value should be left as-is
	if result["key"] != "{{profile.missing}}" {
		t.Errorf("got %q, want unresolved ref left as-is", result["key"])
	}
}
