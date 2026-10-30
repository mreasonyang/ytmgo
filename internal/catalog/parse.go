package catalog

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var durationPattern = regexp.MustCompile(`^(?:[0-9]{1,3}:)?[0-9]{1,3}:[0-9]{2}$`)

// Counts are metadata only when the entire segment has a numeric count and a
// known unit. Suffix matching alone would discard creators such as "The Songs".
const metadataNumber = `(?:[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+)(?:\.[0-9]+)?`
const metadataCount = metadataNumber + `[\s\x{00a0}]*(?:[kmb千萬万億亿])?[\s\x{00a0}]*`

var metadataCountPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^` + metadataCount + `(?:views?|plays?|listeners?|songs?|tracks?|subscribers?|episodes?|videos?|首歌曲|首歌|首|集|集节目|集節目|个视频|個影片|部影片|段视频|段影片|位订阅者|位訂閱者|名订阅者|名訂閱者|订阅者|訂閱者|次观看|次觀看|次播放|位听众|位聽眾|名听众|名聽眾|听众|聽眾|曲|本の動画|エピソード|人のチャンネル登録者|回視聴|回再生)$`),
	regexp.MustCompile(`(?i)^(?:观看次数|觀看次數|播放次数|播放次數)[\s：:]*` + metadataCount + `次?$`),
	regexp.MustCompile(`(?i)^チャンネル登録者数[\s：:]*` + metadataCount + `人$`),
	regexp.MustCompile(`(?i)^(?:視聴回数|再生回数)[\s：:]*` + metadataCount + `回$`),
}

var publicVideoArtworkPath = regexp.MustCompile(`^/(?:vi|vi_webp)/[a-zA-Z0-9_-]{11}/[a-zA-Z0-9_-]+\.(?:jpg|webp)$`)

// The parser follows display containers only. Menus, logging payloads and
// recommendation endpoints never become visible items or playable targets.
var containerKeys = []string{
	"singleColumnBrowseResultsRenderer", "twoColumnBrowseResultsRenderer",
	"tabbedSearchResultsRenderer", "sectionListRenderer", "sectionListContinuation",
	"itemSectionRenderer", "secondaryContents", "content", "contents", "items",
	"appendContinuationItemsAction", "continuationItems", "reloadContinuationItemsCommand",
}

type pageCursor struct {
	token string
	query bool
}

func parsePage(response map[string]any) (Page, pageCursor, error) {
	page := Page{Items: []Item{}, Shelves: []Shelf{}}
	contents, hasContents := response["contents"]
	continuationContents, hasContinuation := response["continuationContents"]
	actions, hasActions := response["onResponseReceivedActions"]
	if !hasContents && !hasContinuation && !hasActions {
		return page, pageCursor{}, ErrInvalidResponse
	}
	if hasContents {
		if _, ok := contents.(map[string]any); !ok {
			return page, pageCursor{}, ErrInvalidResponse
		}
	}
	token := pageCursor{}
	seen := make(map[string]bool)
	appendShelf := func(title string, items []Item) {
		if len(items) == 0 {
			return
		}
		// Search item sections are consecutive individual rows. Keep those in
		// one untitled shelf so the native renderer retains the list layout.
		if title == "" && len(page.Shelves) > 0 && page.Shelves[len(page.Shelves)-1].Title == "" {
			last := &page.Shelves[len(page.Shelves)-1]
			last.Items = uniqueItems(append(last.Items, items...))
		} else {
			page.Shelves = append(page.Shelves, Shelf{Title: title, Items: items})
		}
		for _, item := range items {
			key := itemKey(item)
			if !seen[key] && len(page.Items) < maxItems {
				page.Items = append(page.Items, item)
				seen[key] = true
			}
		}
	}
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 40 || len(page.Items) >= maxItems {
			return
		}
		switch node := value.(type) {
		case []any:
			for _, child := range node {
				walk(child, depth+1)
			}
		case map[string]any:
			if token.token == "" {
				token = continuationToken(node)
			}
			for _, key := range []string{"musicResponsiveHeaderRenderer", "musicDetailHeaderRenderer", "musicImmersiveHeaderRenderer", "musicVisualHeaderRenderer"} {
				if header, ok := node[key].(map[string]any); ok {
					applyHeader(&page, header)
					return
				}
			}
			for _, kind := range []string{"musicCarouselShelfRenderer", "musicImmersiveCarouselShelfRenderer", "musicShelfRenderer", "musicPlaylistShelfRenderer", "gridRenderer", "musicShelfContinuation", "musicPlaylistShelfContinuation", "gridContinuation", "musicCardShelfRenderer"} {
				if shelf, ok := node[kind].(map[string]any); ok {
					if token.token == "" {
						token = continuationToken(shelf)
					}
					items := collectItems(shelf)
					title := shelfTitle(shelf)
					if kind == "musicCardShelfRenderer" {
						if card, ok := parseItem(shelf, kind); ok {
							items = uniqueItems(append([]Item{card}, items...))
						}
						// A card's own title names its top result; only the
						// header names the section, such as "Top result".
						title = joinedText(at(shelf, "header", "musicCardShelfHeaderBasicRenderer", "title"))
					}
					appendShelf(title, items)
					return
				}
			}
			if tabs, ok := node["tabs"].([]any); ok {
				// Only the selected tab's content belongs to this page.
				for _, tab := range tabs {
					if selected, _ := at(tab, "tabRenderer", "selected").(bool); selected {
						walk(at(tab, "tabRenderer", "content"), depth+1)
						return
					}
				}
				for _, tab := range tabs {
					if content := at(tab, "tabRenderer", "content"); content != nil {
						walk(content, depth+1)
						break
					}
				}
			}
			if header := node["header"]; header != nil {
				walk(header, depth+1)
			}
			if items := directItem(node); len(items) > 0 {
				appendShelf("", items)
				return
			}
			for _, key := range containerKeys {
				if child, ok := node[key]; ok {
					walk(child, depth+1)
				}
			}
		}
	}
	walk(response["header"], 0)
	walk(contents, 0)
	walk(continuationContents, 0)
	walk(actions, 0)
	return page, token, nil
}

func applyHeader(page *Page, header map[string]any) {
	if title := joinedText(header["title"]); title != "" {
		page.Title = title
	}
	if subtitle := joinedText(header["subtitle"]); subtitle != "" {
		page.Subtitle = subtitle
	}
	// Creator links belong to the header. Never guess an album's artist from
	// its first track, which may be a compilation or a guest performance.
	for _, key := range []string{"straplineTextOne", "subtitle", "secondSubtitle"} {
		var creators []string
		seen := map[string]bool{}
		for _, run := range arrayAt(header[key], "runs") {
			browse := at(run, "navigationEndpoint", "browseEndpoint")
			id := textAt(browse, "browseId")
			if !strings.HasPrefix(id, "UC") && browseKind(browse) != "artist" {
				continue
			}
			name := strings.TrimSpace(textAt(run, "text"))
			if name != "" && !seen[name] {
				seen[name] = true
				creators = append(creators, name)
			}
		}
		if len(creators) > 0 {
			page.Creator = strings.Join(creators, " · ")
			break
		}
	}
	if art := artwork(header); art != "" {
		page.Artwork = art
	}
}

func shelfTitle(shelf map[string]any) string {
	if title := joinedText(shelf["title"]); title != "" {
		return title
	}
	for _, kind := range []string{"musicCarouselShelfBasicHeaderRenderer", "musicCardShelfHeaderBasicRenderer"} {
		if title := joinedText(at(shelf, "header", kind, "title")); title != "" {
			return title
		}
		if title := joinedText(at(shelf, "header", kind, "strapline")); title != "" {
			return title
		}
	}
	return ""
}

func directItem(node map[string]any) []Item {
	for _, kind := range []string{"musicResponsiveListItemRenderer", "musicTwoRowItemRenderer", "playlistPanelVideoRenderer", "musicNavigationButtonRenderer"} {
		if renderer, ok := node[kind].(map[string]any); ok {
			if item, valid := parseItem(renderer, kind); valid {
				return []Item{item}
			}
			return nil
		}
	}
	return nil
}

func collectItems(value any) []Item {
	items := []Item{}
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 30 || len(items) >= maxItems {
			return
		}
		switch node := value.(type) {
		case []any:
			for _, child := range node {
				walk(child, depth+1)
			}
		case map[string]any:
			for _, kind := range []string{"musicResponsiveListItemRenderer", "musicTwoRowItemRenderer", "playlistPanelVideoRenderer", "musicNavigationButtonRenderer"} {
				if renderer, ok := node[kind].(map[string]any); ok {
					if item, valid := parseItem(renderer, kind); valid {
						items = append(items, item)
					}
					return
				}
			}
			if wrapper, ok := node["playlistPanelVideoWrapperRenderer"].(map[string]any); ok {
				walk(wrapper["primaryRenderer"], depth+1)
				return
			}
			for _, key := range []string{"contents", "items", "content", "musicShelfRenderer", "musicPlaylistShelfRenderer", "gridRenderer"} {
				if child, ok := node[key]; ok {
					walk(child, depth+1)
				}
			}
		}
	}
	walk(value, 0)
	return uniqueItems(items)
}

func uniqueItems(items []Item) []Item {
	result := make([]Item, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		key := itemKey(item)
		if !seen[key] {
			result = append(result, item)
			seen[key] = true
		}
	}
	return result
}

func itemKey(item Item) string {
	if item.ID != "" {
		return "video:" + item.ID
	}
	return "browse:" + item.BrowseID + ":" + item.BrowseParams
}

func parseItem(renderer map[string]any, rendererKind string) (Item, bool) {
	item := Item{Kind: "song"}
	if renderer["isPlayable"] == false || renderer["unplayableText"] != nil || strings.Contains(textAt(renderer, "musicItemRendererDisplayPolicy"), "GREY_OUT") {
		return item, false
	}
	title := renderer["title"]
	metadata := []any{renderer["subtitle"]}
	if rendererKind == "musicResponsiveListItemRenderer" {
		columns := arrayAt(renderer, "flexColumns")
		if len(columns) == 0 {
			return item, false
		}
		title = at(columns[0], "musicResponsiveListItemFlexColumnRenderer", "text")
		metadata = nil
		for _, column := range columns[1:] {
			metadata = append(metadata, at(column, "musicResponsiveListItemFlexColumnRenderer", "text"))
		}
		for _, column := range arrayAt(renderer, "fixedColumns") {
			metadata = append(metadata, at(column, "musicResponsiveListItemFixedColumnRenderer", "text"))
		}
	}
	if rendererKind == "playlistPanelVideoRenderer" {
		metadata = []any{renderer["longBylineText"], renderer["lengthText"]}
	}
	if rendererKind == "musicNavigationButtonRenderer" {
		title = renderer["buttonText"]
	}
	item.Title = joinedText(title)
	if item.Title == "" {
		return item, false
	}
	endpoints := []any{renderer["navigationEndpoint"], renderer["clickCommand"]}
	for _, run := range arrayAt(title, "runs") {
		endpoints = append(endpoints, at(run, "navigationEndpoint"))
	}
	// A card's main browse destination takes precedence over the overlay's
	// play command: clicking an album or playlist opens its track list.
	for _, endpoint := range endpoints {
		if browse := at(endpoint, "browseEndpoint"); browse != nil {
			id := textAt(browse, "browseId")
			if browseIDPattern.MatchString(id) {
				item.BrowseID, item.Kind = id, browseKind(browse)
				if params := textAt(browse, "params"); len(params) <= 8192 && browseParamsPattern.MatchString(params) {
					item.BrowseParams = params
				}
				break
			}
		}
	}
	if item.BrowseID == "" {
		for _, endpoint := range endpoints {
			if id := textAt(endpoint, "watchPlaylistEndpoint", "playlistId"); browseIDPattern.MatchString(id) {
				item.BrowseID, item.Kind = "VL"+id, "playlist"
				break
			}
		}
	}
	if item.BrowseID == "" {
		endpoints = append(endpoints,
			at(renderer, "overlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint"),
			at(renderer, "thumbnailOverlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint"))
		for _, endpoint := range endpoints {
			watch := at(endpoint, "watchEndpoint")
			if id := textAt(watch, "videoId"); videoIDPattern.MatchString(id) {
				item.ID = id
				if kind := textAt(watch, "watchEndpointMusicSupportedConfigs", "watchEndpointMusicConfig", "musicVideoType"); kind == "MUSIC_VIDEO_TYPE_OMV" || kind == "MUSIC_VIDEO_TYPE_UGC" {
					item.Kind = "video"
				}
				break
			}
		}
		if item.ID == "" {
			item.ID = textAt(renderer, "playlistItemData", "videoId")
		}
		if rendererKind == "playlistPanelVideoRenderer" {
			item.ID = textAt(renderer, "videoId")
		}
		if !videoIDPattern.MatchString(item.ID) {
			return Item{}, false
		}
	}
	item.Artist, item.Album, item.Duration = parseMetadata(metadata)
	item.Artwork = artwork(renderer)
	if item.ID != "" {
		item.Liked, item.LikeKnown = parseLiked(renderer, item.ID)
	}
	return item, true
}

// Current responsive track renderers expose the actual rating on their top
// level like button. Queue renderers also use a toggle menu whose service
// endpoint describes the NEXT action: INDIFFERENT removes an existing like.
// Only the item's own like control is inspected, never arbitrary nested menus.
func parseLiked(renderer map[string]any, videoID string) (bool, bool) {
	for _, button := range arrayAt(renderer, "menu", "menuRenderer", "topLevelButtons") {
		like := at(button, "likeButtonRenderer")
		if target := textAt(like, "target", "videoId"); target != "" && target != videoID {
			continue
		}
		switch textAt(like, "likeStatus") {
		case "LIKE":
			return true, true
		case "INDIFFERENT", "DISLIKE":
			return false, true
		}
	}
	for _, menuItem := range arrayAt(renderer, "menu", "menuRenderer", "items") {
		toggle := at(menuItem, "toggleMenuServiceItemRenderer")
		icon := textAt(toggle, "defaultIcon", "iconType")
		if icon == "DISLIKE" || icon == "UNDISLIKE" {
			continue
		}
		endpoint := at(toggle, "defaultServiceEndpoint", "likeEndpoint")
		toggledEndpoint := at(toggle, "toggledServiceEndpoint", "likeEndpoint")
		if textAt(toggledEndpoint, "status") == "DISLIKE" {
			continue
		}
		if toggled, _ := at(toggle, "isToggled").(bool); toggled {
			endpoint = toggledEndpoint
		}
		if target := textAt(endpoint, "target", "videoId"); target != "" && target != videoID {
			continue
		}
		switch textAt(endpoint, "status") {
		case "LIKE":
			return false, true
		case "INDIFFERENT":
			return true, true
		}
	}
	return false, false
}

func browseKind(endpoint any) string {
	switch textAt(endpoint, "browseEndpointContextSupportedConfigs", "browseEndpointContextMusicConfig", "pageType") {
	case "MUSIC_PAGE_TYPE_ALBUM":
		return "album"
	case "MUSIC_PAGE_TYPE_ARTIST", "MUSIC_PAGE_TYPE_USER_CHANNEL":
		return "artist"
	case "MUSIC_PAGE_TYPE_PLAYLIST":
		return "playlist"
	case "MUSIC_PAGE_TYPE_PODCAST_SHOW_DETAIL_PAGE":
		return "podcast"
	case "MUSIC_PAGE_TYPE_NON_MUSIC_AUDIO_TRACK_PAGE":
		return "episode"
	}
	id := textAt(endpoint, "browseId")
	switch {
	case strings.HasPrefix(id, "MPRE"):
		return "album"
	case strings.HasPrefix(id, "UC"):
		return "artist"
	case strings.HasPrefix(id, "VL"):
		return "playlist"
	}
	return "browse"
}

func parseMetadata(texts []any) (string, string, time.Duration) {
	var artists []string
	album := ""
	var duration time.Duration
	for _, text := range texts {
		for _, run := range arrayAt(text, "runs") {
			label := cleanText(textAt(run, "text"))
			browse := at(run, "navigationEndpoint", "browseEndpoint")
			switch browseKind(browse) {
			case "album":
				album = label
			case "artist":
				if label != "" && !contains(artists, label) {
					artists = append(artists, label)
				}
			}
			if parsed := parseDuration(label); parsed > 0 {
				duration = parsed
			}
		}
		if parsed := parseDuration(joinedText(text)); parsed > 0 {
			duration = parsed
		}
	}
	if len(artists) == 0 {
		for _, text := range texts {
			for _, segment := range strings.Split(joinedText(text), "•") {
				segment = cleanText(segment)
				if segment == "" || segment == album || parseDuration(segment) > 0 || metadataLabel(segment) {
					continue
				}
				artists = append(artists, segment)
				break
			}
			if len(artists) > 0 {
				break
			}
		}
	}
	return strings.Join(artists, ", "), album, duration
}

func metadataLabel(label string) bool {
	switch strings.ToLower(label) {
	case "song", "video", "album", "single", "ep", "artist", "playlist", "podcast", "episode", "profile",
		"歌曲", "视频", "影片", "音乐视频", "音樂影片", "专辑", "專輯", "单曲", "單曲", "艺人", "藝人", "歌手", "播放列表", "播放清单", "播放清單", "播客", "播客节目", "播客節目", "剧集", "集數", "个人资料", "個人資料",
		"曲", "楽曲", "動画", "ミュージック ビデオ", "アルバム", "シングル", "アーティスト", "プレイリスト", "ポッドキャスト", "エピソード", "プロフィール":
		return true
	}
	if len(label) == 4 {
		if n, err := strconv.Atoi(label); err == nil && n >= 1900 && n <= 2200 {
			return true
		}
	}
	for _, pattern := range metadataCountPatterns {
		if pattern.MatchString(label) {
			return true
		}
	}
	return false
}

func parseDuration(value string) time.Duration {
	if !durationPattern.MatchString(value) {
		return 0
	}
	parts := strings.Split(value, ":")
	seconds := 0
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || (index > 0 && number >= 60) {
			return 0
		}
		seconds = seconds*60 + number
	}
	return time.Duration(seconds) * time.Second
}

func artwork(renderer any) string {
	best := ""
	bestSize := -1.0
	paths := [][]string{
		{"thumbnail", "musicThumbnailRenderer", "thumbnail", "thumbnails"},
		{"thumbnailRenderer", "musicThumbnailRenderer", "thumbnail", "thumbnails"},
		{"thumbnail", "croppedSquareThumbnailRenderer", "thumbnail", "thumbnails"},
		{"thumbnail", "thumbnails"},
		{"foregroundThumbnail", "musicThumbnailRenderer", "thumbnail", "thumbnails"},
	}
	for _, path := range paths {
		for _, thumbnail := range arrayAt(renderer, path...) {
			imageURL := safeArtwork(textAt(thumbnail, "url"))
			if imageURL == "" {
				continue
			}
			width, _ := at(thumbnail, "width").(float64)
			height, _ := at(thumbnail, "height").(float64)
			if size := width * height; size > bestSize {
				best, bestSize = imageURL, size
			}
		}
	}
	return best
}

func safeArtwork(raw string) string {
	if len(raw) > 4096 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if u.RawQuery != "" {
		// Public video thumbnails have a stable path. Remove service-added
		// sizing/tracking query fields instead of retaining opaque URL tokens.
		if (host == "ytimg.com" || strings.HasSuffix(host, ".ytimg.com")) && publicVideoArtworkPath.MatchString(u.Path) {
			u.RawQuery = ""
			return u.String()
		}
		return ""
	}
	for _, domain := range []string{"ytimg.com", "googleusercontent.com", "ggpht.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return raw
		}
	}
	return ""
}

func continuationToken(node map[string]any) pageCursor {
	for _, entry := range arrayAt(node, "continuations") {
		for _, kind := range []string{"nextContinuationData", "nextRadioContinuationData"} {
			if token := textAt(entry, kind, "continuation"); len(token) > 0 && len(token) <= 16<<10 {
				return pageCursor{token: token, query: true}
			}
		}
	}
	var entries []any
	entries = append(entries, arrayAt(node, "contents")...)
	entries = append(entries, arrayAt(node, "items")...)
	entries = append(entries, arrayAt(node, "continuationItems")...)
	if node["continuationItemRenderer"] != nil {
		entries = append(entries, node)
	}
	for _, entry := range entries {
		endpoint := at(entry, "continuationItemRenderer", "continuationEndpoint")
		commands := []any{endpoint}
		commands = append(commands, arrayAt(endpoint, "commandExecutorCommand", "commands")...)
		for _, command := range commands {
			if token := textAt(command, "continuationCommand", "token"); len(token) > 0 && len(token) <= 16<<10 {
				return pageCursor{token: token}
			}
		}
	}
	return pageCursor{}
}

func parseQueue(response map[string]any) ([]Item, string, error) {
	watch := at(response, "contents", "singleColumnMusicWatchNextResultsRenderer", "tabbedRenderer", "watchNextTabbedResultsRenderer")
	if watch == nil {
		return nil, "", ErrInvalidResponse
	}
	items := []Item{}
	lyrics := ""
	found := false
	for _, tab := range arrayAt(watch, "tabs") {
		renderer := at(tab, "tabRenderer")
		if panel := at(renderer, "content", "musicQueueRenderer", "content", "playlistPanelRenderer"); panel != nil {
			items = collectItems(panel)
			found = true
		}
		if disabled, _ := at(renderer, "unselectable").(bool); disabled {
			continue
		}
		endpoint := at(renderer, "endpoint", "browseEndpoint")
		if textAt(endpoint, "browseEndpointContextSupportedConfigs", "browseEndpointContextMusicConfig", "pageType") == "MUSIC_PAGE_TYPE_TRACK_LYRICS" {
			if id := textAt(endpoint, "browseId"); browseIDPattern.MatchString(id) {
				lyrics = id
			}
		}
	}
	if !found {
		return nil, "", ErrInvalidResponse
	}
	return items, lyrics, nil
}

func parseLyrics(response map[string]any) (string, error) {
	result, err := parsePlainLyrics(response)
	return result.Text, err
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
func arrayAt(value any, path ...string) []any { items, _ := at(value, path...).([]any); return items }
func joinedText(value any) string {
	if text := textAt(value, "simpleText"); text != "" {
		return cleanText(text)
	}
	var text strings.Builder
	for _, run := range arrayAt(value, "runs") {
		text.WriteString(textAt(run, "text"))
	}
	return cleanText(text.String())
}
func cleanText(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return string(runes)
}
func contains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}
