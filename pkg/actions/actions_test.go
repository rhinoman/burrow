package actions

import (
	"testing"
)

func TestParseActionsEmpty(t *testing.T) {
	actions := ParseActions("")
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestParseActionsNoMarkers(t *testing.T) {
	md := "# Report\n\nSome analysis here.\n\n- Item 1\n- Item 2\n"
	actions := ParseActions(md)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestParseActionsDraft(t *testing.T) {
	md := "- [Draft] Send follow-up email to vendor\n"
	actions := ParseActions(md)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != ActionDraft {
		t.Errorf("expected ActionDraft, got %v", actions[0].Type)
	}
	if actions[0].Description != "Send follow-up email to vendor" {
		t.Errorf("unexpected description: %q", actions[0].Description)
	}
}

func TestParseActionsOpen(t *testing.T) {
	md := "- [Open] View full filing (https://sec.gov/filing/123)\n"
	actions := ParseActions(md)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != ActionOpen {
		t.Errorf("expected ActionOpen, got %v", actions[0].Type)
	}
	if actions[0].Target != "https://sec.gov/filing/123" {
		t.Errorf("unexpected target: %q", actions[0].Target)
	}
}

func TestParseActionsConfigure(t *testing.T) {
	md := "- [Configure] Add NAICS code 541511\n"
	actions := ParseActions(md)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != ActionConfigure {
		t.Errorf("expected ActionConfigure, got %v", actions[0].Type)
	}
}

func TestParseActionsCaseInsensitive(t *testing.T) {
	md := "- [DRAFT] uppercase\n- [draft] lowercase\n- [Draft] mixed\n"
	actions := ParseActions(md)
	if len(actions) != 3 {
		t.Errorf("expected 3 actions, got %d", len(actions))
	}
	for _, a := range actions {
		if a.Type != ActionDraft {
			t.Errorf("expected ActionDraft, got %v", a.Type)
		}
	}
}

func TestParseActionsMultiple(t *testing.T) {
	md := `## Suggested Actions

- [Draft] Email to procurement team
- [Open] Review contract (https://example.com/contract)
- [Configure] Add monitoring for NAICS 541370
`
	actions := ParseActions(md)
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(actions))
	}
	if actions[0].Type != ActionDraft {
		t.Errorf("action 0: expected draft, got %v", actions[0].Type)
	}
	if actions[1].Type != ActionOpen {
		t.Errorf("action 1: expected open, got %v", actions[1].Type)
	}
	if actions[2].Type != ActionConfigure {
		t.Errorf("action 2: expected configure, got %v", actions[2].Type)
	}
}

func TestParseDraftStructured(t *testing.T) {
	raw := "To: vendor@example.com\nSubject: Follow-up on proposal\n\nDear Vendor,\n\nThank you for the proposal.\n\nBest regards"
	d := parseDraft(raw)
	if d.To != "vendor@example.com" {
		t.Errorf("To: got %q", d.To)
	}
	if d.Subject != "Follow-up on proposal" {
		t.Errorf("Subject: got %q", d.Subject)
	}
	if d.Body == "" {
		t.Error("Body is empty")
	}
	if d.Raw != raw {
		t.Error("Raw not preserved")
	}
}

func TestParseDraftBodyWithColons(t *testing.T) {
	raw := "To: vendor@example.com\nSubject: Re: proposal details\n\nDear Sir: Thank you for reaching out.\n\nThe deadline is: Friday.\n\nBest regards"
	d := parseDraft(raw)
	if d.To != "vendor@example.com" {
		t.Errorf("To: got %q", d.To)
	}
	if d.Subject != "Re: proposal details" {
		t.Errorf("Subject: got %q", d.Subject)
	}
	if d.Body != "Dear Sir: Thank you for reaching out.\n\nThe deadline is: Friday.\n\nBest regards" {
		t.Errorf("Body should preserve colons: got %q", d.Body)
	}
}

func TestParseDraftUnstructured(t *testing.T) {
	raw := "Here is a simple response without headers."
	d := parseDraft(raw)
	if d.Body == "" {
		t.Error("Body is empty for unstructured input")
	}
	if d.Raw != raw {
		t.Error("Raw not preserved")
	}
}
