// Package privacy provides HTTP transport middleware for privacy hardening.
package privacy

import (
	"net/http"
	"sync/atomic"
)

// sentinelPreserveUA is set by service auth to prevent UA rotation from
// overwriting an auth-required User-Agent.
const sentinelPreserveUA = "X-Burrow-Preserve-UA"

// userAgents is a pool of common browser user-agent strings for rotation.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
}

// Config controls which privacy hardening features are active.
type Config struct {
	StripReferrers     bool
	RandomizeUserAgent bool
	MinimizeRequests   bool
}

// Transport is an http.RoundTripper that applies privacy hardening to outbound requests.
type Transport struct {
	base    http.RoundTripper
	config  Config
	uaIndex atomic.Uint64
}

// NewTransport wraps a base transport with privacy hardening. If base is nil,
// a fresh http.Transport is created to avoid sharing http.DefaultTransport.
func NewTransport(base http.RoundTripper, cfg Config) *Transport {
	if base == nil {
		base = &http.Transport{}
	}
	return &Transport{base: base, config: cfg}
}

// RoundTrip applies privacy modifications and delegates to the base transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the caller's request.
	r := req.Clone(req.Context())

	if t.config.StripReferrers {
		r.Header.Del("Referer")
		r.Header.Del("Origin")
	}

	if t.config.RandomizeUserAgent {
		// If the sentinel header is set, the auth layer needs this specific UA.
		if r.Header.Get(sentinelPreserveUA) != "" {
			r.Header.Del(sentinelPreserveUA)
		} else {
			idx := t.uaIndex.Add(1) - 1
			r.Header.Set("User-Agent", userAgents[idx%uint64(len(userAgents))])
		}
	} else {
		// Always strip sentinel even if UA rotation is off.
		r.Header.Del(sentinelPreserveUA)
	}

	if t.config.MinimizeRequests {
		r.Header.Del("X-Requested-With")
		r.Header.Del("DNT")
		r.Header.Set("Accept", "*/*")
	}

	return t.base.RoundTrip(r)
}
