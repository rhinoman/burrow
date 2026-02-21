package rss

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jcadam/burrow/pkg/config"
)

const sampleRSS2 = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Hacker News</title>
    <link>https://news.ycombinator.com</link>
    <description>Links for the curious</description>
    <item>
      <title>First Post</title>
      <link>https://example.com/1</link>
      <description>This is the &lt;b&gt;first&lt;/b&gt; post</description>
      <pubDate>Mon, 20 Jan 2025 12:00:00 +0000</pubDate>
      <dc:creator>alice</dc:creator>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/2</link>
      <description>Second post description</description>
      <pubDate>Tue, 21 Jan 2025 14:30:00 +0000</pubDate>
      <author>bob@example.com</author>
    </item>
  </channel>
</rss>`

const sampleAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom Feed</title>
  <link href="https://example.com" rel="alternate"/>
  <entry>
    <title>Atom Entry One</title>
    <link href="https://example.com/atom/1" rel="alternate"/>
    <summary>Summary of &amp;amp; entry one</summary>
    <updated>2025-01-20T12:00:00Z</updated>
    <author><name>Charlie</name></author>
  </entry>
  <entry>
    <title>Atom Entry Two</title>
    <link href="https://example.com/atom/2" rel="alternate"/>
    <content>Content of entry two</content>
    <updated>2025-01-21T14:30:00Z</updated>
    <author><name>Dana</name></author>
  </entry>
</feed>`

func TestExecuteFeedRSS2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(sampleRSS2))
	}))
	defer srv.Close()

	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: srv.URL,
	}, nil, "")

	result, err := svc.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected error in result: %s", result.Error)
	}

	var feed FeedResult
	if err := json.Unmarshal(result.Data, &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if feed.Feed.Title != "Hacker News" {
		t.Errorf("expected feed title 'Hacker News', got %q", feed.Feed.Title)
	}
	if feed.Feed.Link != "https://news.ycombinator.com" {
		t.Errorf("expected feed link, got %q", feed.Feed.Link)
	}
	if feed.ItemCount != 2 {
		t.Errorf("expected 2 items, got %d", feed.ItemCount)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(feed.Items))
	}
	if feed.Items[0].Title != "First Post" {
		t.Errorf("expected 'First Post', got %q", feed.Items[0].Title)
	}
	if feed.Items[0].Author != "alice" {
		t.Errorf("expected dc:creator 'alice', got %q", feed.Items[0].Author)
	}
	if feed.Items[1].Author != "bob@example.com" {
		t.Errorf("expected author 'bob@example.com', got %q", feed.Items[1].Author)
	}
}

func TestExecuteFeedAtom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(sampleAtom))
	}))
	defer srv.Close()

	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-atom",
		Type:     "rss",
		Endpoint: srv.URL,
	}, nil, "")

	result, err := svc.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected error in result: %s", result.Error)
	}

	var feed FeedResult
	if err := json.Unmarshal(result.Data, &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if feed.Feed.Title != "Example Atom Feed" {
		t.Errorf("expected feed title 'Example Atom Feed', got %q", feed.Feed.Title)
	}
	if feed.Feed.Link != "https://example.com" {
		t.Errorf("expected feed link 'https://example.com', got %q", feed.Feed.Link)
	}
	if feed.ItemCount != 2 {
		t.Errorf("expected 2 items, got %d", feed.ItemCount)
	}
	if feed.Items[0].Author != "Charlie" {
		t.Errorf("expected author 'Charlie', got %q", feed.Items[0].Author)
	}
	// Atom uses content as fallback for summary
	if feed.Items[1].Description != "Content of entry two" {
		t.Errorf("expected content fallback, got %q", feed.Items[1].Description)
	}
}

func TestExecuteUnknownTool(t *testing.T) {
	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: "http://localhost",
	}, nil, "")

	_, err := svc.Execute(context.Background(), "search", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "no tool") {
		t.Errorf("expected 'no tool' in error, got: %v", err)
	}
}

func TestMaxItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleRSS2))
	}))
	defer srv.Close()

	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: srv.URL,
		MaxItems: 1,
	}, nil, "")

	result, err := svc.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var feed FeedResult
	if err := json.Unmarshal(result.Data, &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if feed.ItemCount != 1 {
		t.Errorf("expected 1 item (max_items=1), got %d", feed.ItemCount)
	}
	if len(feed.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(feed.Items))
	}
}

func TestMaxItemsDefault(t *testing.T) {
	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: "http://localhost",
	}, nil, "")

	if svc.maxItems != 20 {
		t.Errorf("expected default maxItems=20, got %d", svc.maxItems)
	}
}

func TestHTMLStripping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<b>bold</b>", "bold"},
		{"<p>Hello &amp; World</p>", "Hello & World"},
		{"no tags", "no tags"},
		{"<a href='x'>link</a> text", "link text"},
		{"", ""},
		{"&lt;not a tag&gt;", "<not a tag>"},
	}
	for _, tt := range tests {
		got := stripHTML(tt.input)
		if got != tt.expected {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDateNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Mon, 20 Jan 2025 12:00:00 +0000", "2025-01-20T12:00:00Z"},
		{"2025-01-20T12:00:00Z", "2025-01-20T12:00:00Z"},
		{"2025-01-20", "2025-01-20T00:00:00Z"},
		{"", ""},
		{"not a date", "not a date"}, // fall back to raw string
	}
	for _, tt := range tests {
		got := normalizeDate(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeDate(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: srv.URL,
	}, nil, "")

	result, err := svc.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute should not return error for HTTP errors: %v", err)
	}
	if !strings.Contains(result.Error, "HTTP 404") {
		t.Errorf("expected error containing 'HTTP 404', got %q", result.Error)
	}
	if result.Data != nil {
		t.Errorf("expected nil Data on HTTP error, got %d bytes", len(result.Data))
	}
}

func TestAuthApplied(t *testing.T) {
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.Write([]byte(sampleRSS2))
	}))
	defer srv.Close()

	// Test bearer auth
	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "bearer", Token: "my-token"},
	}, nil, "")

	_, err := svc.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("no request captured")
	}
	authHeader := capturedReq.Header.Get("Authorization")
	if authHeader != "Bearer my-token" {
		t.Errorf("expected 'Bearer my-token', got %q", authHeader)
	}

	// Test api_key auth
	capturedReq = nil
	svc2 := NewRSSService(config.ServiceConfig{
		Name:     "test-rss2",
		Type:     "rss",
		Endpoint: srv.URL,
		Auth:     config.AuthConfig{Method: "api_key", Key: "secret123"},
	}, nil, "")

	_, err = svc2.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("no request captured")
	}
	if capturedReq.URL.Query().Get("api_key") != "secret123" {
		t.Errorf("expected api_key=secret123, got %q", capturedReq.URL.Query().Get("api_key"))
	}
}

func TestParseFeedBadInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not XML"))
	}))
	defer srv.Close()

	svc := NewRSSService(config.ServiceConfig{
		Name:     "test-rss",
		Type:     "rss",
		Endpoint: srv.URL,
	}, nil, "")

	result, err := svc.Execute(context.Background(), "feed", nil)
	if err != nil {
		t.Fatalf("Execute should not return error for parse failures: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error in result for bad XML")
	}
	if !strings.Contains(result.Error, "parsing feed") {
		t.Errorf("expected 'parsing feed' in error, got %q", result.Error)
	}
}
