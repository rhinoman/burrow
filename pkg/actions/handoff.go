package actions

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jcadam/burrow/pkg/config"
)

// Handoff manages system app handoff for opening URLs, files, and mailto links.
type Handoff struct {
	apps config.AppsConfig
}

// NewHandoff creates a Handoff with the given app configuration.
func NewHandoff(apps config.AppsConfig) *Handoff {
	return &Handoff{apps: apps}
}

// OpenURL opens a URL in the configured browser.
func (h *Handoff) OpenURL(rawURL string) error {
	return h.open(h.apps.Browser, rawURL)
}

// OpenFile opens a file in the configured editor.
func (h *Handoff) OpenFile(path string) error {
	return h.open(h.apps.Editor, path)
}

// OpenMailto opens a mailto: URI in the configured email app.
func (h *Handoff) OpenMailto(to, subject, body string) error {
	uri := BuildMailtoURI(to, subject, body)
	return h.open(h.apps.Email, uri)
}

// PlayMedia opens a media file in the configured media player.
func (h *Handoff) PlayMedia(path string) error {
	return h.open(h.apps.Media, path)
}

// BuildMailtoURI constructs a properly encoded mailto: URI.
func BuildMailtoURI(to, subject, body string) string {
	var params []string
	if subject != "" {
		params = append(params, "subject="+url.QueryEscape(subject))
	}
	if body != "" {
		params = append(params, "body="+url.QueryEscape(body))
	}
	uri := "mailto:" + to
	if len(params) > 0 {
		uri += "?" + strings.Join(params, "&")
	}
	return uri
}

// open launches the given target with the configured app or system default.
func (h *Handoff) open(app, target string) error {
	if app == "" || app == "default" {
		app = systemOpener()
	}

	cmd := exec.Command(app, target)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %q with %s: %w", target, app, err)
	}
	// Don't wait — system apps are fire-and-forget
	go cmd.Wait() //nolint:errcheck
	return nil
}

// systemOpener returns the platform default application opener.
func systemOpener() string {
	if runtime.GOOS == "darwin" {
		return "open"
	}
	return "xdg-open"
}
