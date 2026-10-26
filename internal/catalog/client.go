// Package catalog reads the public, anonymous YouTube Music search catalog.
// The web endpoint is unofficial and may change. No cookies, authentication
// tokens, account data, media streams, or raw response logs are retained.
package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	serviceOrigin     = "https://music.youtube.com"
	maxBootstrapBytes = 2 << 20
	maxSearchBytes    = 4 << 20
	maxQueryRunes     = 200
	requestTimeout    = 18 * time.Second
	userAgent         = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15"
)

var (
	ErrEmptyQuery        = errors.New("enter a search query")
	ErrQueryTooLong      = errors.New("search query exceeds 200 characters")
	ErrUnavailable       = errors.New("YouTube Music search is unavailable")
	ErrInvalidResponse   = errors.New("YouTube Music returned an unsupported response")
	ErrResponseTooLarge  = errors.New("YouTube Music response exceeded the size limit")
	clientVersionPattern = regexp.MustCompile(`"INNERTUBE_CLIENT_VERSION"\s*:\s*"([0-9]+\.[0-9]{8}\.[0-9]+\.[0-9]+)"`)
)

// Track contains public catalog metadata. Missing album/duration data stays
// empty. An ID is a real search watch target, not a claim of regional or embed
// playback availability; the official player still determines that.
type Track struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	Duration  string `json:"duration"`
	Thumbnail string `json:"thumbnail"`
}

// HTTPError includes only status and request stage, never an upstream body.
type HTTPError struct {
	Stage      string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("YouTube Music %s returned HTTP %d", e.Stage, e.StatusCode)
}
func (e *HTTPError) Unwrap() error { return ErrUnavailable }

type Client struct {
	http           *http.Client
	origin         string
	mu             sync.RWMutex
	version        string
	versionExpires time.Time
}

func NewClient() *Client {
	return &Client{
		origin: serviceOrigin,
		http: &http.Client{
			Timeout: requestTimeout,
			// Do not follow consent/login redirects or send requests to other hosts.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.Host != "music.youtube.com" {
					return http.ErrUseLastResponse
				}
				return nil
			},
			// Jar is intentionally nil. Set-Cookie headers are never consumed.
		},
	}
}

// Search returns the first public result page, deduplicated by video ID. A valid
// search with no playable song/video targets returns an empty non-nil slice.
// No login, API key, continuation token, or playback URL is requested.
func (c *Client) Search(ctx context.Context, query string) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) > maxQueryRunes {
		return nil, ErrQueryTooLong
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	version, err := c.clientVersion(ctx)
	if err != nil {
		return nil, err
	}
	payload := struct {
		Context map[string]any `json:"context"`
		Query   string         `json:"query"`
	}{Context: map[string]any{"client": map[string]string{"clientName": "WEB_REMIX", "clientVersion": version, "hl": "en", "gl": "US"}}, Query: query}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalidResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/youtubei/v1/search?prettyPrint=false", bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	req.Header.Set("Referer", c.origin+"/")
	req.Header.Set("X-Youtube-Client-Name", "67")
	req.Header.Set("X-Youtube-Client-Version", version)
	data, err := c.request(req, "search", maxSearchBytes)
	if err != nil {
		return nil, err
	}
	return parseSearch(data)
}

func (c *Client) clientVersion(ctx context.Context) (string, error) {
	c.mu.RLock()
	version, expires := c.version, c.versionExpires
	c.mu.RUnlock()
	if version != "" && time.Now().Before(expires) {
		return version, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+"/", nil)
	if err != nil {
		return "", ErrUnavailable
	}
	data, err := c.request(req, "bootstrap", maxBootstrapBytes)
	if err != nil {
		return "", err
	}
	// Only retain the public client version, never the HTML, full client
	// context, visitor data, API key, or tracking fields.
	match := clientVersionPattern.FindSubmatch(data)
	if len(match) != 2 {
		return "", ErrInvalidResponse
	}
	version = string(match[1])
	c.mu.Lock()
	c.version, c.versionExpires = version, time.Now().Add(time.Hour)
	c.mu.Unlock()
	return version, nil
}

func (c *Client) request(req *http.Request, stage string, maxBytes int64) ([]byte, error) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	response, err := c.http.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		// net/http error strings may include proxy addresses or URL credentials.
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &HTTPError{Stage: stage, StatusCode: response.StatusCode}
	}
	if response.ContentLength > maxBytes {
		return nil, ErrResponseTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		return nil, ErrUnavailable
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

func safeThumbnail(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"ytimg.com", "googleusercontent.com", "ggpht.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return raw
		}
	}
	return ""
}
