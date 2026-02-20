package actions

import (
	"strings"
	"testing"
)

func TestBuildMailtoURIBasic(t *testing.T) {
	uri := BuildMailtoURI("user@example.com", "Hello", "Body text")
	if !strings.HasPrefix(uri, "mailto:user@example.com?") {
		t.Errorf("unexpected prefix: %q", uri)
	}
	if !strings.Contains(uri, "subject=Hello") {
		t.Errorf("missing subject: %q", uri)
	}
	if !strings.Contains(uri, "body=Body+text") {
		t.Errorf("missing body: %q", uri)
	}
}

func TestBuildMailtoURIEmpty(t *testing.T) {
	uri := BuildMailtoURI("user@example.com", "", "")
	if uri != "mailto:user@example.com" {
		t.Errorf("expected bare mailto, got %q", uri)
	}
}

func TestBuildMailtoURISpecialChars(t *testing.T) {
	uri := BuildMailtoURI("user@example.com", "RE: Contract #123", "Hello & goodbye")
	if !strings.Contains(uri, "subject=RE%3A+Contract+%23123") {
		t.Errorf("subject not properly encoded: %q", uri)
	}
	if !strings.Contains(uri, "body=Hello+%26+goodbye") {
		t.Errorf("body not properly encoded: %q", uri)
	}
}

func TestBuildMailtoURISubjectOnly(t *testing.T) {
	uri := BuildMailtoURI("user@example.com", "Just a subject", "")
	if !strings.Contains(uri, "subject=") {
		t.Errorf("missing subject: %q", uri)
	}
	if strings.Contains(uri, "body=") {
		t.Errorf("unexpected body param: %q", uri)
	}
}
