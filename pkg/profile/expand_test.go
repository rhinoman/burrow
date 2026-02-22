package profile

import (
	"strings"
	"testing"
	"time"
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

// --- Go template engine tests ---

func nestedProfile() *Profile {
	return &Profile{
		Name: "Test",
		Raw: map[string]interface{}{
			"name": "Test",
			"location": map[string]interface{}{
				"latitude":  "61.22",
				"longitude": "-149.90",
			},
			"coords": "61.22,-149.90",
		},
	}
}

func TestExpandGoTemplate(t *testing.T) {
	result, err := Expand(`Hello {{profile "name"}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello Trivyn" {
		t.Errorf("got %q, want %q", result, "Hello Trivyn")
	}
}

func TestExpandWhitespace(t *testing.T) {
	// Spaces inside delimiters should work via legacy conversion.
	result, err := Expand("Hello {{ profile.name }}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello Trivyn" {
		t.Errorf("got %q, want %q", result, "Hello Trivyn")
	}
}

func TestExpandNested(t *testing.T) {
	result, err := Expand(`lat={{profile "location.latitude"}}`, nestedProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "lat=61.22" {
		t.Errorf("got %q", result)
	}
}

func TestExpandNestedLegacySyntax(t *testing.T) {
	// Legacy dot-notation with nested keys.
	result, err := Expand("lat={{profile.location.latitude}}", nestedProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "lat=61.22" {
		t.Errorf("got %q", result)
	}
}

func TestExpandToday(t *testing.T) {
	result, err := Expand("date={{today}}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if result != "date="+today {
		t.Errorf("got %q, want %q", result, "date="+today)
	}
}

func TestExpandYesterday(t *testing.T) {
	result, err := Expand("date={{yesterday}}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if result != "date="+yesterday {
		t.Errorf("got %q, want %q", result, "date="+yesterday)
	}
}

func TestExpandDateComponents(t *testing.T) {
	result, err := Expand("{{year}}-{{month}}-{{day}}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if result != today {
		t.Errorf("got %q, want %q", result, today)
	}
}

func TestExpandSplitIndex(t *testing.T) {
	result, err := Expand(`{{index (split (profile "coords") ",") 0}}`, nestedProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "61.22" {
		t.Errorf("got %q, want %q", result, "61.22")
	}
}

func TestExpandFallback(t *testing.T) {
	// Malformed Go template syntax — should fall back to legacy expander.
	text := "Hello {{profile.name}} and {{unbalanced"
	result, err := Expand(text, testProfile())
	if err != nil {
		// Legacy expander won't match {{unbalanced so no error expected
		// unless it has unresolved profile refs too.
		t.Fatalf("unexpected error: %v", err)
	}
	// Legacy expander should resolve {{profile.name}} but leave {{unbalanced as-is.
	if !strings.Contains(result, "Trivyn") {
		t.Errorf("expected Trivyn in fallback result, got %q", result)
	}
}

func TestExpandFallbackNested(t *testing.T) {
	// Malformed template with nested profile ref — legacy fallback must
	// still resolve nested dot-notation via the updated regex.
	text := "lat={{profile.location.latitude}} {{unbalanced"
	result, err := Expand(text, nestedProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "61.22") {
		t.Errorf("expected nested ref resolved in fallback, got %q", result)
	}
}

func TestExpandNow(t *testing.T) {
	result, err := Expand("ts={{now}}", testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should parse as RFC3339.
	ts := strings.TrimPrefix(result, "ts=")
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("{{now}} result %q is not valid RFC3339: %v", ts, err)
	}
}

func TestExpandDateFormat(t *testing.T) {
	result, err := Expand(`{{yesterday | date "01/02/2006"}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	yesterday := time.Now().AddDate(0, 0, -1)
	want := yesterday.Format("01/02/2006")
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestExpandDateFormatFromNow(t *testing.T) {
	result, err := Expand(`{{now | date "2006-01-02"}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Now().Format("2006-01-02")
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestExpandDatePassthrough(t *testing.T) {
	// today produces "2006-01-02" format, date reformats to "01/02/2006"
	result, err := Expand(`{{today | date "01/02/2006"}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Now().Format("01/02/2006")
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestExpandDateInvalidInput(t *testing.T) {
	// Unparseable input should pass through unchanged.
	result, err := Expand(`{{profile "name" | date "01/02/2006"}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Trivyn" {
		t.Errorf("got %q, want %q (unparseable input should pass through)", result, "Trivyn")
	}
}

func TestExpandStringFuncs(t *testing.T) {
	result, err := Expand(`{{upper (profile "name")}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "TRIVYN" {
		t.Errorf("got %q, want %q", result, "TRIVYN")
	}

	result, err = Expand(`{{lower (profile "name")}}`, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "trivyn" {
		t.Errorf("got %q, want %q", result, "trivyn")
	}
}
