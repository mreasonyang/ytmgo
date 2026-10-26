package catalog

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var videoIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
var durationPattern = regexp.MustCompile(`^(?:[0-9]{1,3}:)?[0-9]{1,3}:[0-9]{2}$`)

func parseSearch(data []byte) ([]Track, error) {
	var response map[string]any
	if json.Unmarshal(data, &response) != nil {
		return nil, ErrInvalidResponse
	}
	if _, exists := response["error"]; exists {
		return nil, ErrUnavailable
	}
	contents, exists := response["contents"]
	if !exists || contents == nil {
		return nil, ErrInvalidResponse
	}
	if _, ok := contents.(map[string]any); !ok {
		return nil, ErrInvalidResponse
	}
	tracks := make([]Track, 0)
	seen := make(map[string]bool)
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 80 || len(tracks) >= 60 {
			return
		}
		switch node := value.(type) {
		case map[string]any:
			for _, kind := range []string{"musicResponsiveListItemRenderer", "musicTwoRowItemRenderer"} {
				if item, ok := node[kind].(map[string]any); ok {
					track, valid := parseTrack(item, kind)
					if valid && !seen[track.ID] {
						tracks = append(tracks, track)
						seen[track.ID] = true
					}
					return // Never treat menu/recommendation targets as this result.
				}
			}
			keys := make([]string, 0, len(node))
			for key := range node {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(node[key], depth+1)
			}
		case []any:
			for _, child := range node {
				walk(child, depth+1)
			}
		}
	}
	walk(contents, 0)
	return tracks, nil
}

func parseTrack(item map[string]any, kind string) (Track, bool) {
	var track Track
	if item["isPlayable"] == false || strings.Contains(textAt(item, "musicItemRendererDisplayPolicy"), "GREY_OUT") {
		return track, false
	}
	if at(item, "navigationEndpoint", "browseEndpoint") != nil && at(item, "navigationEndpoint", "watchEndpoint") == nil {
		return track, false
	}
	var title any
	var metadata []any
	if kind == "musicResponsiveListItemRenderer" {
		columns := arrayAt(item, "flexColumns")
		if len(columns) == 0 {
			return track, false
		}
		title = at(columns[0], "musicResponsiveListItemFlexColumnRenderer", "text")
		for _, column := range columns[1:] {
			metadata = append(metadata, at(column, "musicResponsiveListItemFlexColumnRenderer", "text"))
		}
		for _, column := range arrayAt(item, "fixedColumns") {
			metadata = append(metadata, at(column, "musicResponsiveListItemFixedColumnRenderer", "text"))
		}
	} else {
		title = item["title"]
		metadata = append(metadata, item["subtitle"])
	}
	track.Title = joinedText(title)
	if track.Title == "" {
		return track, false
	}
	endpoints := []any{
		at(item, "navigationEndpoint", "watchEndpoint"),
		at(item, "overlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint", "watchEndpoint"),
		at(item, "thumbnailOverlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint", "watchEndpoint"),
	}
	for _, run := range arrayAt(title, "runs") {
		endpoints = append(endpoints, at(run, "navigationEndpoint", "watchEndpoint"))
	}
	for _, endpoint := range endpoints {
		musicType := textAt(endpoint, "watchEndpointMusicSupportedConfigs", "watchEndpointMusicConfig", "musicVideoType")
		if musicType != "" && musicType != "MUSIC_VIDEO_TYPE_ATV" && musicType != "MUSIC_VIDEO_TYPE_OMV" && musicType != "MUSIC_VIDEO_TYPE_UGC" {
			return Track{}, false
		}
		if id := textAt(endpoint, "videoId"); videoIDPattern.MatchString(id) && track.ID == "" {
			track.ID = id
		}
	}
	if track.ID == "" {
		track.ID = textAt(item, "playlistItemData", "videoId")
	}
	if !videoIDPattern.MatchString(track.ID) {
		return Track{}, false
	}
	artists := []string{}
	for _, text := range metadata {
		for _, run := range arrayAt(text, "runs") {
			label := cleanText(textAt(run, "text"))
			if label == "" {
				continue
			}
			// English metadata follows the requested hl=en. Do not pull a song
			// from an album/playlist's context menu, or include podcast episodes.
			if at(run, "navigationEndpoint") == nil && (label == "Episode" || label == "Podcast" || label == "Playlist" || label == "Album" || label == "Artist" || label == "Profile" || label == "Single") {
				return Track{}, false
			}
			browseID := textAt(run, "navigationEndpoint", "browseEndpoint", "browseId")
			pageType := textAt(run, "navigationEndpoint", "browseEndpoint", "browseEndpointContextSupportedConfigs", "browseEndpointMusicConfig", "pageType")
			if pageType == "MUSIC_PAGE_TYPE_ALBUM" || strings.HasPrefix(browseID, "MPRE") {
				track.Album = label
			} else if pageType == "MUSIC_PAGE_TYPE_ARTIST" || strings.HasPrefix(browseID, "UC") {
				if !contains(artists, label) {
					artists = append(artists, label)
				}
			}
			if durationPattern.MatchString(label) {
				track.Duration = label
			}
		}
		if track.Duration == "" && durationPattern.MatchString(joinedText(text)) {
			track.Duration = joinedText(text)
		}
	}
	track.Artist = strings.Join(artists, ", ")
	if track.Artist == "" {
		// Some video channel subtitles contain no browse endpoint. Only accept
		// the first plain metadata segment after an optional Song/Video label.
		for _, text := range metadata {
			segments := strings.Split(joinedText(text), "•")
			for _, segment := range segments {
				segment = cleanText(segment)
				if segment == "Song" || segment == "Video" {
					continue
				}
				lower := strings.ToLower(segment)
				if segment != "" && !durationPattern.MatchString(segment) && !strings.Contains(lower, "views") && !strings.Contains(lower, "plays") && !strings.Contains(lower, "listeners") {
					track.Artist = segment
				}
				break
			}
			if track.Artist != "" {
				break
			}
		}
	}
	track.Thumbnail = thumbnail(item)
	return track, true
}

func thumbnail(item map[string]any) string {
	paths := [][]string{
		{"thumbnail", "musicThumbnailRenderer", "thumbnail", "thumbnails"},
		{"thumbnailRenderer", "musicThumbnailRenderer", "thumbnail", "thumbnails"},
		{"thumbnail", "thumbnails"},
	}
	best, bestWidth := "", float64(-1)
	for _, path := range paths {
		for _, entry := range arrayAt(item, path...) {
			if safe := safeThumbnail(textAt(entry, "url")); safe != "" {
				width, _ := at(entry, "width").(float64)
				if width >= bestWidth {
					best, bestWidth = safe, width
				}
			}
		}
	}
	return best
}

func at(value any, path ...string) any {
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}
func textAt(value any, path ...string) string { text, _ := at(value, path...).(string); return text }
func arrayAt(value any, path ...string) []any { array, _ := at(value, path...).([]any); return array }
func joinedText(value any) string {
	if simple := textAt(value, "simpleText"); simple != "" {
		return cleanText(simple)
	}
	var result strings.Builder
	for _, run := range arrayAt(value, "runs") {
		result.WriteString(textAt(run, "text"))
	}
	return cleanText(result.String())
}
func cleanText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return string(runes)
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
