package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// urlPattern matches HTTP(S) URLs, stopping before whitespace, ANSI escapes,
// closing parens/brackets/angles, or trailing punctuation.
var urlPattern = regexp.MustCompile(`https?://[^\s\x1b)\]>]+`)

// wrapURLsForView wraps URLs in viewport output with both BubbleZone marks
// (for click detection) and OSC 8 hyperlinks (for terminal hover/click).
// When zones are nil (e.g. in tests or non-interactive use), falls back to
// processHyperlinks.
//
// Glamour may word-wrap long URLs across lines. The regex matches only the
// first-line fragment, so we resolve each match against full URLs extracted
// from the raw markdown to avoid opening truncated paths.
func (v Viewer) wrapURLsForView(vpOutput string) string {
	if v.zones == nil {
		return processHyperlinks(vpOutput, v.imageTier)
	}

	urls := make(map[string]string)
	counter := 0

	result := urlPattern.ReplaceAllStringFunc(vpOutput, func(matched string) string {
		// Resolve fragment to full URL when Glamour has split it across lines.
		url := v.resolveFullURL(matched)

		zoneID := fmt.Sprintf("url-%d", counter)
		counter++
		urls[zoneID] = url

		display := matched // display the visible fragment, not the full URL
		if v.imageTier != TierNone {
			display = ansi.SetHyperlink(url) + matched + ansi.ResetHyperlink()
		}
		return v.zones.Mark(zoneID, display)
	})

	v.zoneState.urls = urls
	return result
}

// resolveFullURL checks if matched is a prefix of any known URL from the raw
// markdown. Returns the longest matching URL to avoid ambiguity when one URL
// is a prefix of another (e.g. "https://example.com/a" vs "https://example.com/abc").
func (v Viewer) resolveFullURL(matched string) string {
	best := matched
	for _, link := range v.links {
		if strings.HasPrefix(link.url, matched) && len(link.url) > len(best) {
			best = link.url
		}
	}
	return best
}

// processHyperlinks wraps URLs in the rendered output with OSC 8 hyperlink
// escape sequences so they become clickable in supporting terminals.
// On Tier 2 (TierNone) this is a no-op.
func processHyperlinks(rendered string, tier ImageTier) string {
	if tier == TierNone {
		return rendered
	}

	return urlPattern.ReplaceAllStringFunc(rendered, func(url string) string {
		return ansi.SetHyperlink(url) + url + ansi.ResetHyperlink()
	})
}
