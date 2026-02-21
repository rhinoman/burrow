package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestProcessHyperlinksNoOp(t *testing.T) {
	input := "Check out https://example.com for details."
	result := processHyperlinks(input, TierNone)
	if result != input {
		t.Errorf("TierNone should be no-op, got %q", result)
	}
}

func TestProcessHyperlinksTier1(t *testing.T) {
	input := "Visit https://example.com today."
	result := processHyperlinks(input, TierKitty)

	expected := "Visit " + ansi.SetHyperlink("https://example.com") + "https://example.com" + ansi.ResetHyperlink() + " today."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestProcessHyperlinksMultipleURLs(t *testing.T) {
	input := "See https://one.com and https://two.com for info."
	result := processHyperlinks(input, TierKitty)

	if !strings.Contains(result, ansi.SetHyperlink("https://one.com")) {
		t.Error("expected first URL to be wrapped")
	}
	if !strings.Contains(result, ansi.SetHyperlink("https://two.com")) {
		t.Error("expected second URL to be wrapped")
	}
}

func TestProcessHyperlinksNoURLs(t *testing.T) {
	input := "No links here, just text."
	result := processHyperlinks(input, TierKitty)
	if result != input {
		t.Errorf("expected unchanged input, got %q", result)
	}
}

func TestProcessHyperlinksHTTP(t *testing.T) {
	input := "See http://insecure.com for more."
	result := processHyperlinks(input, TierIterm)

	if !strings.Contains(result, ansi.SetHyperlink("http://insecure.com")) {
		t.Error("expected http URL to be wrapped")
	}
}

func TestProcessHyperlinksStopsAtClosingParen(t *testing.T) {
	input := "(see https://example.com/path)"
	result := processHyperlinks(input, TierKitty)

	// The URL should not include the closing paren
	if strings.Contains(result, ansi.SetHyperlink("https://example.com/path)")) {
		t.Error("URL should not include trailing closing paren")
	}
	if !strings.Contains(result, ansi.SetHyperlink("https://example.com/path")) {
		t.Error("expected URL without trailing paren to be wrapped")
	}
}

func TestProcessHyperlinksStopsAtANSI(t *testing.T) {
	input := "link: https://example.com\x1b[0m rest"
	result := processHyperlinks(input, TierKitty)

	// Should wrap just the URL, not the ANSI sequence
	if !strings.Contains(result, ansi.SetHyperlink("https://example.com")) {
		t.Error("expected URL to be wrapped")
	}
	if !strings.Contains(result, "\x1b[0m") {
		t.Error("ANSI sequence should be preserved outside the hyperlink")
	}
}
