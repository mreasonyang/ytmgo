// Package catalog provides the native client's YouTube Music catalog.
// Requests use the current WEB_REMIX protocol. Responses are reduced to public
// display metadata; upstream bodies, cookies and playback URLs are never logged.
package catalog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"ytmgo/internal/netproxy"
)

var defaultTransport = sync.OnceValue(func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = netproxy.Proxy
	return transport
})

const (
	serviceOrigin = "https://music.youtube.com"
	// Matches the bundled youtubei.js YTMUSIC WEB_REMIX client contract.
	musicClientVersion       = "1.20250219.01.00"
	requestTimeout           = 20 * time.Second
	maxResponseBytes   int64 = 8 << 20
	maxItems                 = 500
	maxCacheEntries          = 128
)

var (
	ErrEmptyQuery               = errors.New("enter a search query")
	ErrQueryTooLong             = errors.New("search query exceeds 200 characters")
	ErrInvalidID                = errors.New("invalid YouTube Music item")
	ErrUnavailable              = errors.New("YouTube Music is unavailable")
	ErrInvalidResponse          = errors.New("YouTube Music returned an unsupported response")
	ErrResponseTooLarge         = errors.New("YouTube Music response exceeded the size limit")
	ErrNoLyrics                 = errors.New("lyrics are not available for this track")
	ErrExpiredContinuation      = errors.New("this page has expired; refresh to load more")
	ErrAuthenticationRequired   = errors.New("sign in to YouTube Music to use your library")
	ErrInvalidSession           = errors.New("YouTube Music sign-in is incomplete")
	ErrSessionChanged           = errors.New("catalog_session_changed")
	ErrSessionExpired           = errors.New("catalog_session_expired")
	ErrAccountStatusUnavailable = errors.New("catalog_account_status_unavailable")
	videoIDPattern              = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	browseIDPattern             = regexp.MustCompile(`^[a-zA-Z0-9_.=-]{1,512}$`)
	browseParamsPattern         = regexp.MustCompile(`^[a-zA-Z0-9_+=%/.-]*$`)
	languagePattern             = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z]{2,8})?$`)
	regionPattern               = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Item is either playable (ID is a video ID) or browseable (BrowseID is set).
// Kind is song, video, album, artist, playlist, or browse.
type Item struct {
	ID           string        `json:"id,omitempty"`
	Title        string        `json:"title"`
	Artist       string        `json:"artist,omitempty"`
	Album        string        `json:"album,omitempty"`
	Artwork      string        `json:"artwork,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	Kind         string        `json:"kind"`
	BrowseID     string        `json:"browseId,omitempty"`
	BrowseParams string        `json:"browseParams,omitempty"`
	Liked        bool          `json:"liked,omitempty"`
	LikeKnown    bool          `json:"likeKnown,omitempty"`
}

type Shelf struct {
	Title string `json:"title"`
	Items []Item `json:"items"`
}

// Items contains the flattened, deduplicated page contents. Shelves preserves
// the service's grouping and item order. Continuation is an opaque local handle,
// valid only on the originating Client for 30 minutes; it is not an API token.
type Page struct {
	Title        string  `json:"title"`
	Creator      string  `json:"creator,omitempty"`
	Subtitle     string  `json:"subtitle,omitempty"`
	Artwork      string  `json:"artwork,omitempty"`
	Shelves      []Shelf `json:"shelves"`
	Items        []Item  `json:"items"`
	Continuation string  `json:"continuation,omitempty"`
}

// HTTPError contains only the operation, status and a fixed redirect category,
// never upstream response text, headers or URLs.
type HTTPError struct {
	Stage      string
	StatusCode int
	Redirect   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("YouTube Music %s returned HTTP %d", e.Stage, e.StatusCode)
}
func (e *HTTPError) Unwrap() error {
	if e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden {
		return ErrAuthenticationRequired
	}
	return ErrUnavailable
}

// Options permits an explicitly supplied transport (for example an application
// proxy) and locale. The client is copied; its cookie jar and redirects are
// disabled. Account cookies must be supplied explicitly with SetSession.
type Options struct {
	HTTPClient *http.Client
	Language   string
	Region     string
}

type continuation struct {
	endpoint, title string
	cursor          pageCursor
	visitorData     string
	payload         map[string]any
	expires         time.Time
}
type lyricsReference struct {
	browseID string
	expires  time.Time
}

type Client struct {
	http                     *http.Client
	origin, language, region string
	mu                       sync.RWMutex
	continuations            map[string]continuation
	lyrics                   map[string]lyricsReference
	session                  *accountSession
	sessionGeneration        uint64
}

func New() *Client { return NewWithOptions(Options{}) }

// SetLanguage changes the locale for future requests without replacing the
// account session or interrupting playback. In-flight requests keep their locale.
func (c *Client) SetLanguage(language string) {
	if !languagePattern.MatchString(language) {
		language = "en"
	}
	c.mu.Lock()
	c.language = language
	clear(c.continuations)
	c.mu.Unlock()
}

func NewWithOptions(options Options) *Client {
	client := &http.Client{}
	if options.HTTPClient != nil {
		*client = *options.HTTPClient
	} else {
		client.Transport = defaultTransport()
	}
	if client.Timeout <= 0 || client.Timeout > requestTimeout {
		client.Timeout = requestTimeout
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	language, region := options.Language, options.Region
	if !languagePattern.MatchString(language) {
		language = "en"
	}
	if !regionPattern.MatchString(region) {
		region = "US"
	}
	return &Client{http: client, origin: serviceOrigin, language: language, region: region,
		continuations: make(map[string]continuation), lyrics: make(map[string]lyricsReference)}
}

func (c *Client) Search(ctx context.Context, query string) (Page, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Page{}, ErrEmptyQuery
	}
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 {
		return Page{}, ErrQueryTooLong
	}
	return c.page(ctx, "search", map[string]any{"query": query}, query)
}

func (c *Client) Home(ctx context.Context) (Page, error) {
	return c.page(ctx, "browse", map[string]any{"browseId": "FEmusic_home"}, "Home")
}
func (c *Client) Explore(ctx context.Context) (Page, error) {
	return c.page(ctx, "browse", map[string]any{"browseId": "FEmusic_explore"}, "Explore")
}

func (c *Client) Browse(ctx context.Context, id string) (Page, error) {
	return c.BrowseWithParams(ctx, id, "")
}

func (c *Client) BrowseWithParams(ctx context.Context, id, params string) (Page, error) {
	if !browseIDPattern.MatchString(id) {
		return Page{}, ErrInvalidID
	}
	if len(params) > 8192 || !browseParamsPattern.MatchString(params) {
		return Page{}, ErrInvalidID
	}
	payload := map[string]any{"browseId": id}
	if params != "" {
		payload["params"] = params
	}
	return c.page(ctx, "browse", payload, "")
}

func (c *Client) Library(ctx context.Context) (Page, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return Page{}, err
	}
	if scope.session == nil {
		return Page{}, ErrAuthenticationRequired
	}
	return c.page(ctx, "browse", map[string]any{"browseId": "FEmusic_library_landing"}, "Library")
}

func (c *Client) Continue(ctx context.Context, handle string) (Page, error) {
	ctx, _, err := c.operation(ctx)
	if err != nil {
		return Page{}, err
	}
	c.mu.RLock()
	entry, ok := c.continuations[handle]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return Page{}, ErrExpiredContinuation
	}
	if entry.cursor.query {
		return c.loadPage(ctx, entry.endpoint, entry.payload, entry.title, entry.cursor.token, entry.visitorData)
	}
	return c.loadPage(ctx, entry.endpoint, map[string]any{"continuation": entry.cursor.token}, entry.title, "", entry.visitorData)
}

func (c *Client) page(ctx context.Context, endpoint string, payload map[string]any, title string) (Page, error) {
	return c.loadPage(ctx, endpoint, payload, title, "", "")
}

func (c *Client) loadPage(ctx context.Context, endpoint string, payload map[string]any, title, queryContinuation, visitorData string) (Page, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return Page{}, err
	}
	response, err := c.apiRequestWithOptions(ctx, endpoint, payload, requestOptions{queryContinuation: queryContinuation, visitorData: visitorData})
	if err != nil {
		return Page{}, err
	}
	page, token, err := parsePage(response)
	if err != nil {
		return Page{}, err
	}
	if page.Title == "" {
		page.Title = title
	}
	if token.token != "" {
		// Continuations are bound to the visitor that produced the page. Keep
		// this opaque value with its local handle, not in global client state:
		// concurrent Home/Search requests can return different visitor IDs.
		if visitor := textAt(response, "responseContext", "visitorData"); validVisitorData(visitor) {
			visitorData = visitor
		}
		page.Continuation, err = c.rememberContinuation(scope, endpoint, token, page.Title, payload, visitorData)
		if err != nil {
			return Page{}, err
		}
	}
	if err := c.scopeError(scope); err != nil {
		return Page{}, err
	}
	return page, nil
}

func (c *Client) Queue(ctx context.Context, videoID string) ([]Item, error) {
	if !videoIDPattern.MatchString(videoID) {
		return nil, ErrInvalidID
	}
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return nil, err
	}
	response, err := c.api(ctx, "next", map[string]any{
		"videoId": videoID, "playlistId": "RDAMVM" + videoID,
		"enablePersistentPlaylistPanel": true, "isAudioOnly": true,
		"tunerSettingValue": "AUTOMIX_SETTING_NORMAL",
		"watchEndpointMusicSupportedConfigs": map[string]any{"watchEndpointMusicConfig": map[string]any{
			"hasPersistentPlaylistPanel": true, "musicVideoType": "MUSIC_VIDEO_TYPE_ATV",
		}},
	})
	if err != nil {
		return nil, err
	}
	items, lyrics, err := parseQueue(response)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if err := c.scopeErrorLocked(scope); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if len(c.lyrics) >= maxCacheEntries {
		clear(c.lyrics)
	}
	c.lyrics[videoID] = lyricsReference{browseID: lyrics, expires: time.Now().Add(10 * time.Minute)}
	c.mu.Unlock()
	return items, nil
}

// Track refreshes a selected track from the service's real watch queue. Search
// and browse cards may omit rating state; LikeKnown remains false when the
// service also omits it from next, rather than inventing an unliked status.
func (c *Client) Track(ctx context.Context, videoID string) (Item, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return Item{}, err
	}
	items, err := c.Queue(ctx, videoID)
	if err != nil {
		return Item{}, err
	}
	for _, item := range items {
		if item.ID == videoID {
			if err := c.scopeError(scope); err != nil {
				return Item{}, err
			}
			return item, nil
		}
	}
	return Item{}, ErrInvalidResponse
}

func (c *Client) Lyrics(ctx context.Context, videoID string) (string, error) {
	result, err := c.lyricsResult(ctx, videoID, false)
	return result.Text, err
}

// Like changes only the explicitly selected track's account rating. The caller
// should invoke it in response to the user's like/unlike control.
func (c *Client) Like(ctx context.Context, videoID string, liked bool) error {
	if !videoIDPattern.MatchString(videoID) {
		return ErrInvalidID
	}
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return err
	}
	if scope.session == nil {
		return ErrAuthenticationRequired
	}
	endpoint := "like/removelike"
	if liked {
		endpoint = "like/like"
	}
	response, err := c.api(ctx, endpoint, map[string]any{"target": map[string]string{"videoId": videoID}})
	if err != nil {
		return err
	}
	if _, ok := response["responseContext"].(map[string]any); !ok {
		return ErrInvalidResponse
	}
	return c.scopeError(scope)
}

func (c *Client) api(ctx context.Context, endpoint string, payload map[string]any) (map[string]any, error) {
	return c.apiRequest(ctx, endpoint, payload, "")
}

func (c *Client) apiRequest(ctx context.Context, endpoint string, payload map[string]any, queryContinuation string) (map[string]any, error) {
	return c.apiRequestWithOptions(ctx, endpoint, payload, requestOptions{queryContinuation: queryContinuation})
}

type requestOptions struct {
	queryContinuation string
	visitorData       string
	allowLoggedOut    bool
	lyricsMobile      bool
}

// Visitor data remains in memory and is never exposed in the public Page.
func validVisitorData(value string) bool {
	if len(value) == 0 || len(value) > 4096 {
		return false
	}
	for _, ch := range value {
		if ch < '!' || ch > '~' {
			return false
		}
	}
	return true
}

func (c *Client) apiRequestWithOptions(ctx context.Context, endpoint string, payload map[string]any, options requestOptions) (map[string]any, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := c.scopeError(scope); err != nil {
		return nil, err
	}
	requestPayload := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		requestPayload[key] = value
	}
	c.mu.RLock()
	language, region := c.language, c.region
	c.mu.RUnlock()
	clientName, clientVersion, clientNumber := "WEB_REMIX", musicClientVersion, "67"
	if options.lyricsMobile {
		clientName, clientVersion, clientNumber = "ANDROID_MUSIC", lyricsClientVersion, "21"
	}
	clientContext := map[string]string{
		"clientName": clientName, "clientVersion": clientVersion, "hl": language, "gl": region,
	}
	if validVisitorData(options.visitorData) {
		clientContext["visitorData"] = options.visitorData
	}
	requestContext := map[string]any{"client": clientContext}
	if scope.session != nil && scope.session.onBehalfOfUser != "" {
		requestContext["user"] = map[string]string{"onBehalfOfUser": scope.session.onBehalfOfUser}
	}
	requestPayload["context"] = requestContext
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, ErrInvalidResponse
	}
	parameters := url.Values{"prettyPrint": {"false"}}
	if options.queryContinuation != "" {
		parameters.Set("ctoken", options.queryContinuation)
		parameters.Set("continuation", options.queryContinuation)
		parameters.Set("type", "next")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/youtubei/v1/"+endpoint+"?"+parameters.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", serviceOrigin)
	req.Header.Set("Referer", serviceOrigin+"/")
	req.Header.Set("X-Youtube-Client-Name", clientNumber)
	req.Header.Set("X-Youtube-Client-Version", clientVersion)
	req.Header.Set("Accept-Language", language)
	if validVisitorData(options.visitorData) {
		req.Header.Set("X-Goog-Visitor-Id", options.visitorData)
	}
	authorize(req, scope.session)
	data, err := c.request(req, endpoint, maxResponseBytes)
	if scopeErr := c.scopeError(scope); scopeErr != nil {
		return nil, scopeErr
	}
	if err != nil {
		return nil, err
	}
	var response map[string]any
	if json.Unmarshal(data, &response) != nil || response == nil {
		return nil, ErrInvalidResponse
	}
	if _, ok := response["error"]; ok {
		return nil, ErrUnavailable
	}
	if !options.allowLoggedOut && req.Header.Get("Authorization") != "" {
		if loggedOut, _ := at(response, "responseContext", "mainAppWebResponseContext", "loggedOut").(bool); loggedOut {
			return nil, ErrAuthenticationRequired
		}
	}
	if err := c.scopeError(scope); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) request(req *http.Request, stage string, limit int64) ([]byte, error) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15")
	if req.Header.Get("Accept-Language") == "" {
		c.mu.RLock()
		req.Header.Set("Accept-Language", c.language)
		c.mu.RUnlock()
	}
	response, err := c.http.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		return nil, ErrUnavailable // net/http errors can contain credentials and signed URLs.
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		failure := &HTTPError{Stage: stage, StatusCode: response.StatusCode}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			failure.Redirect = redirectCategory(response.Header.Get("Location"), req.URL)
		}
		return nil, failure
	}
	if response.ContentLength > limit {
		return nil, ErrResponseTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		return nil, ErrUnavailable
	}
	if int64(len(data)) > limit {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

func (c *Client) rememberContinuation(scope sessionScope, endpoint string, cursor pageCursor, title string, payload map[string]any, visitorData string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", nil
	}
	handle := hex.EncodeToString(random[:])
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.scopeErrorLocked(scope); err != nil {
		return "", err
	}
	for key, value := range c.continuations {
		if time.Now().After(value.expires) {
			delete(c.continuations, key)
		}
	}
	if len(c.continuations) >= maxCacheEntries {
		oldestKey := ""
		var oldest time.Time
		for key, value := range c.continuations {
			if oldestKey == "" || value.expires.Before(oldest) {
				oldestKey, oldest = key, value.expires
			}
		}
		delete(c.continuations, oldestKey)
	}
	c.continuations[handle] = continuation{endpoint: endpoint, cursor: cursor, visitorData: visitorData, title: title, payload: payload, expires: time.Now().Add(30 * time.Minute)}
	return handle, nil
}
