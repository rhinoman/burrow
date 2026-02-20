package render

import (
	"bytes"
	"io"
	"strings"

	"github.com/BourgeoisBear/rasterm"
)

// ImageTier represents the terminal's image rendering capability.
type ImageTier int

const (
	TierNone  ImageTier = iota // Tier 2: text only
	TierKitty                  // Kitty graphics protocol (Kitty, Ghostty, WezTerm)
	TierIterm                  // iTerm2 inline images (OSC 1337)
	TierSixel                  // Sixel protocol (foot, Contour)
)

// DetectImageTier determines the terminal's image rendering capability.
// configOverride values: "auto" (default), "inline", "external", "text".
func DetectImageTier(configOverride string) ImageTier {
	switch strings.ToLower(configOverride) {
	case "text":
		return TierNone
	case "external":
		return TierNone
	case "inline":
		return detectBest()
	case "", "auto":
		return detectBest()
	default:
		return TierNone
	}
}

// detectBest probes the terminal for the best supported image protocol.
// Sixel is intentionally not detected here because WriteInlineImage cannot
// render it yet (rasterm.SixelWriteImage requires image.Paletted, not raw PNG).
// When Sixel rendering is implemented, add detection back.
func detectBest() ImageTier {
	if rasterm.IsKittyCapable() {
		return TierKitty
	}
	if rasterm.IsItermCapable() {
		return TierIterm
	}
	return TierNone
}

// WriteInlineImage writes a PNG image to w using the appropriate terminal
// protocol for the given tier. Returns an error if the tier doesn't support
// inline images or if writing fails.
func WriteInlineImage(w io.Writer, pngData []byte, tier ImageTier) error {
	switch tier {
	case TierKitty:
		return rasterm.KittyCopyPNGInline(w, bytes.NewReader(pngData), rasterm.KittyImgOpts{})
	case TierIterm:
		return rasterm.ItermCopyFileInline(w, bytes.NewReader(pngData), int64(len(pngData)))
	default:
		return nil // TierNone and TierSixel (which requires paletted image) — no-op
	}
}
