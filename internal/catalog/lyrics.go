package catalog

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// The mobile lyrics profile and response shape follow ytmusicapi's get_lyrics
// and as_mobile implementation. The override belongs to one browse request;
// concurrent catalog requests and the selected account are never changed.
// https://github.com/sigma67/ytmusicapi/blob/main/ytmusicapi/ytmusic.py
// https://github.com/sigma67/ytmusicapi/blob/main/ytmusicapi/mixins/browsing.py
const lyricsClientVersion = "7.21.50"

const (
	maxLyricsBytes  = 128 << 10
	maxLyricsLines  = 4096
	maxLyricsSource = 4096
	maxLyricMillis  = int64(24 * time.Hour / time.Millisecond)
)

// LyricLine uses the service's real cue interval [Start, End). Missing or
// invalid timing never becomes an estimated timestamp.
type LyricLine struct {
	Text       string
	Start, End time.Duration
}

// LyricsResult retains readable text even when line timing is unavailable.
// Source is the provider attribution supplied by the lyrics response.
type LyricsResult struct {
	Text, Source string
	Lines        []LyricLine
}

func (c *Client) LyricsWithTiming(ctx context.Context, videoID string) (LyricsResult, error) {
	result, err := memoized(c, ctx, &c.lyricResults, "timed:"+videoID, 10*time.Minute, 2<<20, lyricsBytes, func(work context.Context) (LyricsResult, error) { return c.loadLyricsWithTiming(work, videoID) })
	return cloneLyrics(result), err
}

func (c *Client) loadLyricsWithTiming(ctx context.Context, videoID string) (LyricsResult, error) {
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return LyricsResult{}, err
	}
	result, err := c.lyricsResult(ctx, videoID, true)
	if err != nil || scope.session == nil || len(result.Lines) != 0 {
		return result, err
	}

	// A signed-in browse can supply plain lyrics even when YouTube Music
	// publishes cues for the same recording. Start from the video ID so no
	// account headers, browse references or visitor data enter the public lookup.
	c.mu.RLock()
	public := NewWithOptions(Options{HTTPClient: c.http, Language: c.language, Region: c.region})
	c.mu.RUnlock()
	lookup, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	items, browseID, lookupErr := public.watchQueue(lookup, videoID)
	if lookupErr == nil && len(items) > 0 && items[0].ID == videoID && browseID != "" && c.scopeError(scope) == nil {
		if synced, err := public.timedLyrics(lookup, browseID); err == nil && len(synced.Lines) > 0 {
			result = synced
		}
	}
	if err := ctx.Err(); err != nil {
		return LyricsResult{}, err
	}
	if err := c.scopeError(scope); err != nil {
		return LyricsResult{}, err
	}
	return result, nil
}

func (c *Client) lyricsResult(ctx context.Context, videoID string, timed bool) (LyricsResult, error) {
	if !videoIDPattern.MatchString(videoID) {
		return LyricsResult{}, ErrInvalidID
	}
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return LyricsResult{}, err
	}
	c.mu.RLock()
	reference, ok := c.lyrics[videoID]
	c.mu.RUnlock()
	if !ok || time.Now().After(reference.expires) {
		if _, err := c.Queue(ctx, videoID); err != nil {
			return LyricsResult{}, err
		}
		c.mu.RLock()
		reference = c.lyrics[videoID]
		c.mu.RUnlock()
	}
	if err := ctx.Err(); err != nil {
		return LyricsResult{}, err
	}
	if err := c.scopeError(scope); err != nil {
		return LyricsResult{}, err
	}
	if reference.browseID == "" {
		return LyricsResult{}, ErrNoLyrics
	}
	if timed {
		result, requestErr := c.timedLyrics(ctx, reference.browseID)
		if err := ctx.Err(); err != nil {
			return LyricsResult{}, err
		}
		if err := c.scopeError(scope); err != nil {
			return LyricsResult{}, err
		}
		if requestErr == nil {
			return result, nil
		}
		// The mobile profile or timed response can be unavailable for a track.
		// Retry the established plain-text path within the same account scope.
	}
	response, err := c.api(ctx, "browse", map[string]any{"browseId": reference.browseID})
	if err != nil {
		return LyricsResult{}, err
	}
	result, err := parsePlainLyrics(response)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return LyricsResult{}, ctxErr
	}
	if scopeErr := c.scopeError(scope); scopeErr != nil {
		return LyricsResult{}, scopeErr
	}
	return result, err
}

func (c *Client) timedLyrics(ctx context.Context, browseID string) (LyricsResult, error) {
	response, err := c.apiRequestWithOptions(ctx, "browse", map[string]any{"browseId": browseID}, requestOptions{lyricsMobile: true})
	if err != nil {
		return LyricsResult{}, err
	}
	return parseTimedLyrics(response)
}

func parseTimedLyrics(response map[string]any) (LyricsResult, error) {
	data := at(response, "contents", "elementRenderer", "newElement", "type", "componentType", "model", "timedLyricsModel", "lyricsData")
	if data == nil {
		return parsePlainLyrics(response)
	}
	rows := arrayAt(data, "timedLyricsData")
	if len(rows) == 0 {
		return LyricsResult{}, ErrNoLyrics
	}
	if len(rows) > maxLyricsLines {
		return LyricsResult{}, ErrResponseTooLarge
	}
	source := textAt(data, "sourceMessage")
	if len(source) > maxLyricsSource {
		return LyricsResult{}, ErrResponseTooLarge
	}
	result := LyricsResult{Source: strings.TrimSpace(source)}
	var text strings.Builder
	lines := make([]LyricLine, 0, len(rows))
	validTiming := true
	var previous time.Duration
	for index, row := range rows {
		line, ok := at(row, "lyricLine").(string)
		if !ok {
			return LyricsResult{}, ErrInvalidResponse
		}
		separator := 0
		if index > 0 {
			separator = 1
		}
		if len(line) > maxLyricsBytes-text.Len()-separator {
			return LyricsResult{}, ErrResponseTooLarge
		}
		if separator != 0 {
			text.WriteByte('\n')
		}
		text.WriteString(line)
		start, startOK := lyricTime(at(row, "cueRange", "startTimeMilliseconds"))
		end, endOK := lyricTime(at(row, "cueRange", "endTimeMilliseconds"))
		if !startOK || !endOK || end <= start || index > 0 && start < previous {
			validTiming = false
		}
		lines = append(lines, LyricLine{Text: line, Start: start, End: end})
		previous = start
	}
	result.Text = strings.TrimSpace(text.String())
	if result.Text == "" {
		return LyricsResult{}, ErrNoLyrics
	}
	if validTiming {
		result.Lines = lines
	}
	return result, nil
}

func lyricTime(value any) (time.Duration, bool) {
	var millis int64
	switch v := value.(type) {
	case string:
		if len(v) == 0 || len(v) > 12 {
			return 0, false
		}
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				return 0, false
			}
		}
		var err error
		millis, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > float64(maxLyricMillis) || math.Trunc(v) != v {
			return 0, false
		}
		millis = int64(v)
	case json.Number:
		return lyricTime(string(v))
	default:
		return 0, false
	}
	if millis < 0 || millis > maxLyricMillis {
		return 0, false
	}
	return time.Duration(millis) * time.Millisecond, true
}

func parsePlainLyrics(response map[string]any) (LyricsResult, error) {
	for _, content := range arrayAt(response, "contents", "sectionListRenderer", "contents") {
		if renderer := at(content, "musicDescriptionShelfRenderer"); renderer != nil {
			text, err := lyricText(at(renderer, "description"), maxLyricsBytes)
			if err != nil {
				return LyricsResult{}, err
			}
			if text == "" {
				continue
			}
			source, err := lyricText(at(renderer, "footer"), maxLyricsSource)
			if err != nil {
				return LyricsResult{}, err
			}
			if source == "" {
				source, err = lyricText(renderer, maxLyricsSource)
				if err != nil {
					return LyricsResult{}, err
				}
			}
			return LyricsResult{Text: text, Source: source}, nil
		}
	}
	return LyricsResult{}, ErrNoLyrics
}

func lyricText(value any, limit int) (string, error) {
	if simple := textAt(value, "simpleText"); simple != "" {
		if len(simple) > limit {
			return "", ErrResponseTooLarge
		}
		return strings.TrimSpace(simple), nil
	}
	var text strings.Builder
	for _, run := range arrayAt(value, "runs") {
		part := textAt(run, "text")
		if len(part) > limit-text.Len() {
			return "", ErrResponseTooLarge
		}
		text.WriteString(part)
	}
	return strings.TrimSpace(text.String()), nil
}
