package privacy

import (
	"fmt"
	"net/url"
)

// RouteEntry maps a service name to a proxy URL. This mirrors config.RouteConfig
// but lives in the privacy package to avoid an import cycle.
type RouteEntry struct {
	Service string
	Proxy   string
}

// ResolveProxy returns the proxy URL for a given service. It checks routes for
// an explicit match, then falls back to defaultProxy, then normalizes shorthands.
// An empty return means direct (no proxy).
func ResolveProxy(serviceName, defaultProxy string, routes []RouteEntry) string {
	for _, r := range routes {
		if r.Service == serviceName {
			return normalizeProxy(r.Proxy)
		}
	}
	return normalizeProxy(defaultProxy)
}

// ValidateProxyURL checks that a proxy value is a recognized shorthand or a
// well-formed URL with an allowed scheme. Returns nil for valid values.
func ValidateProxyURL(raw string) error {
	switch raw {
	case "", "none", "direct", "tor":
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid proxy URL %q: %w", raw, err)
	}

	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		// valid
	default:
		return fmt.Errorf("invalid proxy URL %q: scheme must be http, https, socks5, or socks5h", raw)
	}

	if u.Host == "" {
		return fmt.Errorf("invalid proxy URL %q: missing host", raw)
	}

	return nil
}

// normalizeProxy expands shorthand values to their full proxy URLs.
func normalizeProxy(value string) string {
	switch value {
	case "", "none", "direct":
		return ""
	case "tor":
		return "socks5h://127.0.0.1:9050"
	default:
		return value
	}
}
