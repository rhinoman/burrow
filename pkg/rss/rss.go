// Package rss provides an RSS/Atom feed service adapter for Burrow's service interface.
package rss

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/privacy"
	"github.com/jcadam/burrow/pkg/services"
)

const defaultMaxItems = 20

// RSSService implements services.Service for RSS/Atom feed endpoints.
type RSSService struct {
	name     string
	endpoint string
	auth     config.AuthConfig
	maxItems int
	client   *http.Client
}

// NewRSSService creates an RSS service from config. Each service gets its own
// http.Client to support per-service proxy routing. If privacyCfg is non-nil,
// a privacy transport is applied for referrer stripping, UA rotation, and
// request minimization. proxyURL sets the proxy on the underlying transport
// (empty string means direct connection).
func NewRSSService(cfg config.ServiceConfig, privacyCfg *privacy.Config, proxyURL string) *RSSService {
	baseTransport := &http.Transport{}
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil && parsed != nil {
			baseTransport.Proxy = http.ProxyURL(parsed)
		}
	}
	var transport http.RoundTripper = baseTransport
	if privacyCfg != nil {
		transport = privacy.NewTransport(baseTransport, *privacyCfg)
	}

	maxItems := cfg.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}

	return &RSSService{
		name:     cfg.Name,
		endpoint: cfg.Endpoint,
		auth:     cfg.Auth,
		maxItems: maxItems,
		client:   &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

func (r *RSSService) Name() string { return r.name }

// Execute runs the "feed" tool, which fetches and parses the RSS/Atom feed.
func (r *RSSService) Execute(ctx context.Context, tool string, params map[string]string) (*services.Result, error) {
	if tool != "feed" {
		return nil, fmt.Errorf("service %q has no tool %q (rss services only support \"feed\")", r.name, tool)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	r.applyAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			URL:       r.endpoint,
			Timestamp: time.Now().UTC(),
			Error:     err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if len(body) > 0 {
			errMsg += ": " + string(body)
		}
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			URL:       r.endpoint,
			Timestamp: time.Now().UTC(),
			Error:     errMsg,
		}, nil
	}

	// Limit response to 10MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			URL:       r.endpoint,
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("reading response: %v", err),
		}, nil
	}

	result, err := r.parseFeed(body)
	if err != nil {
		return &services.Result{
			Service:   r.name,
			Tool:      tool,
			URL:       r.endpoint,
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("parsing feed: %v", err),
		}, nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}

	return &services.Result{
		Service:   r.name,
		Tool:      tool,
		Data:      data,
		URL:       r.endpoint,
		Timestamp: time.Now().UTC(),
	}, nil
}

// FeedResult is the JSON output structure for a parsed feed.
type FeedResult struct {
	Feed      FeedMeta   `json:"feed"`
	Items     []FeedItem `json:"items"`
	FetchedAt string     `json:"fetched_at"`
	ItemCount int        `json:"item_count"`
}

// FeedMeta holds feed-level metadata.
type FeedMeta struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
}

// FeedItem holds a single feed entry.
type FeedItem struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
	PubDate     string `json:"pub_date"`
	Author      string `json:"author"`
}

// parseFeed auto-detects RSS 2.0 vs Atom by peeking at the XML root element,
// then normalizes both formats into a unified FeedResult.
func (r *RSSService) parseFeed(data []byte) (*FeedResult, error) {
	// Peek at the root element to determine format.
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid XML: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "rss":
				return r.parseRSS2(data)
			case "feed":
				return r.parseAtom(data)
			default:
				return nil, fmt.Errorf("unrecognized feed format (root element: %q)", se.Name.Local)
			}
		}
	}
}

// RSS 2.0 XML structures
type rss2Feed struct {
	Channel rss2Channel `xml:"channel"`
}

type rss2Channel struct {
	Title       string     `xml:"title"`
	Link        string     `xml:"link"`
	Description string     `xml:"description"`
	Items       []rss2Item `xml:"item"`
}

type rss2Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
}

func (r *RSSService) parseRSS2(data []byte) (*FeedResult, error) {
	var feed rss2Feed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing RSS 2.0: %w", err)
	}

	ch := feed.Channel
	items := make([]FeedItem, 0, len(ch.Items))
	for _, item := range ch.Items {
		if len(items) >= r.maxItems {
			break
		}
		author := item.Author
		if author == "" {
			author = item.Creator
		}
		items = append(items, FeedItem{
			Title:       stripHTML(item.Title),
			Link:        item.Link,
			Description: stripHTML(item.Description),
			PubDate:     normalizeDate(item.PubDate),
			Author:      stripHTML(author),
		})
	}

	return &FeedResult{
		Feed: FeedMeta{
			Title:       stripHTML(ch.Title),
			Link:        ch.Link,
			Description: stripHTML(ch.Description),
		},
		Items:     items,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		ItemCount: len(items),
	}, nil
}

// Atom XML structures
type atomFeed struct {
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Links   []atomLink `xml:"link"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
	Updated string     `xml:"updated"`
	Author  atomAuthor `xml:"author"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func (r *RSSService) parseAtom(data []byte) (*FeedResult, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing Atom: %w", err)
	}

	feedLink := ""
	for _, l := range feed.Links {
		if l.Rel == "" || l.Rel == "alternate" {
			feedLink = l.Href
			break
		}
	}

	items := make([]FeedItem, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		if len(items) >= r.maxItems {
			break
		}
		link := ""
		for _, l := range entry.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		desc := entry.Summary
		if desc == "" {
			desc = entry.Content
		}
		items = append(items, FeedItem{
			Title:       stripHTML(entry.Title),
			Link:        link,
			Description: stripHTML(desc),
			PubDate:     normalizeDate(entry.Updated),
			Author:      entry.Author.Name,
		})
	}

	return &FeedResult{
		Feed: FeedMeta{
			Title: stripHTML(feed.Title),
			Link:  feedLink,
		},
		Items:     items,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		ItemCount: len(items),
	}, nil
}

// stripHTML removes HTML tags using simple rune-level scanning and decodes HTML entities.
func stripHTML(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}

// normalizeDate attempts to parse common date formats and normalize to RFC3339.
// Falls back to the raw string if no format matches.
func normalizeDate(s string) string {
	if s == "" {
		return ""
	}
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		time.RFC3339Nano,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

func (r *RSSService) applyAuth(req *http.Request) {
	switch r.auth.Method {
	case "api_key":
		paramName := r.auth.KeyParam
		if paramName == "" {
			paramName = "api_key"
		}
		q := req.URL.Query()
		q.Set(paramName, r.auth.Key)
		req.URL.RawQuery = q.Encode()
	case "api_key_header":
		headerName := r.auth.KeyParam
		if headerName == "" {
			headerName = "X-API-Key"
		}
		req.Header.Set(headerName, r.auth.Key)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+r.auth.Token)
	case "user_agent":
		req.Header.Set("User-Agent", r.auth.Value)
		req.Header.Set("X-Burrow-Preserve-UA", "true")
	}
}
