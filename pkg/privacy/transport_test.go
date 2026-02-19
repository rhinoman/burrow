package privacy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripReferrers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "" {
			t.Error("expected Referer stripped")
		}
		if r.Header.Get("Origin") != "" {
			t.Error("expected Origin stripped")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tr := NewTransport(http.DefaultTransport, Config{StripReferrers: true})
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Referer", "https://evil.example.com")
	req.Header.Set("Origin", "https://evil.example.com")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
}

func TestUserAgentRotation(t *testing.T) {
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get("User-Agent"))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tr := NewTransport(http.DefaultTransport, Config{RandomizeUserAgent: true})
	client := &http.Client{Transport: tr}

	for i := 0; i < len(userAgents)+1; i++ {
		req, _ := http.NewRequest("GET", srv.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	if len(received) != len(userAgents)+1 {
		t.Fatalf("expected %d requests, got %d", len(userAgents)+1, len(received))
	}

	// Verify round-robin: first N requests should each use a different UA
	for i := 0; i < len(userAgents); i++ {
		if received[i] != userAgents[i] {
			t.Errorf("request %d: expected UA %q, got %q", i, userAgents[i], received[i])
		}
	}
	// Request N+1 wraps back to first UA
	if received[len(userAgents)] != userAgents[0] {
		t.Errorf("expected wrap-around to UA[0], got %q", received[len(userAgents)])
	}
}

func TestPreserveAuthUserAgent(t *testing.T) {
	var receivedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		// Sentinel header must NOT reach the server
		if r.Header.Get(sentinelPreserveUA) != "" {
			t.Error("sentinel header leaked to server")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tr := NewTransport(http.DefaultTransport, Config{RandomizeUserAgent: true})
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("User-Agent", "burrow/1.0 contact@example.com")
	req.Header.Set(sentinelPreserveUA, "true")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if receivedUA != "burrow/1.0 contact@example.com" {
		t.Errorf("expected preserved UA, got %q", receivedUA)
	}
}

func TestMinimizeHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") != "" {
			t.Error("expected X-Requested-With stripped")
		}
		if r.Header.Get("DNT") != "" {
			t.Error("expected DNT stripped")
		}
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Errorf("expected Accept */* , got %q", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tr := NewTransport(http.DefaultTransport, Config{MinimizeRequests: true})
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("DNT", "1")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
}

func TestNoOpWhenDisabled(t *testing.T) {
	var receivedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		if r.Header.Get("Referer") != "https://example.com" {
			t.Error("expected Referer preserved when not stripping")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tr := NewTransport(http.DefaultTransport, Config{}) // all disabled
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("User-Agent", "custom-ua")
	req.Header.Set("Referer", "https://example.com")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if receivedUA != "custom-ua" {
		t.Errorf("expected original UA preserved, got %q", receivedUA)
	}
}
