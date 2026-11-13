package main

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
)

func (a *desktop) navigate(route string) {
	if a.model.glideToTop(route) {
		a.invalidate()
		return
	}
	a.openRoute(route)
}

// Account changes open the destination even when the previous account was
// looking at the same route and its old scroll position is still on screen.
func (a *desktop) openRoute(route string) {
	a.model.Immersive = false
	a.browseParams = ""
	if route == "search" {
		a.model.FocusSearch = true
		a.invalidate()
		return
	}
	a.loadPage(route, "", true)
}

// glideToTop glides the page back to its top when route is the page shown
// and it is scrolled down, as choosing a tab already shown does, and reports
// whether it did; at the top, choosing it loads the page again.
func (m *UIModel) glideToTop(route string) bool {
	if route != m.Route || route == "search" || m.Immersive || m.Loading || m.pageScroll == nil || m.pageScroll.Y <= 0.5 {
		return false
	}
	m.pageScroll.ScrollTo(0, 0, ui.Animation{Duration: tokenMotionSlow, Ease: tokenMotionEase})
	return true
}

func (a *desktop) search(q string) {
	q = strings.TrimSpace(q)
	if q != "" {
		a.model.Immersive = false
		a.loadPage("search", q, true)
	}
}
func (a *desktop) openItem(it catalog.Item) {
	if it.BrowseID != "" && it.Kind != "song" && it.Kind != "video" {
		a.browseParams = it.BrowseParams
		a.loadPage("browse:"+it.BrowseID, "", true)
		// The header shows at once from what the item already says, so its
		// cover can travel there from the item's; the rest comes as it loads.
		a.model.Page = catalog.Page{Title: it.Title, Creator: it.Artist, Artwork: it.Artwork}
	} else if it.ID != "" {
		a.playItem(it)
	}
}
func (a *desktop) reload() {
	location := a.currentPageLocation()
	a.browseParams = location.params
	a.loadPage(location.route, location.query, false)
}

// signedOutLibraryPage leaves the explanation and its action to the library
// card instead of repeating them under the heading.
func signedOutLibraryPage(tr func(string) string) catalog.Page {
	return catalog.Page{Title: tr("shellLibraryTitle")}
}

func (a *desktop) loadPage(route, q string, record bool) {
	if a.pageCancel != nil {
		a.pageCancel()
	}
	a.pageGeneration++
	generation := a.pageGeneration
	a.pageCancel = nil
	a.likedChanges = nil
	a.model.Route = route
	a.model.Query = q
	a.model.Loading = true
	a.model.Error = ""
	a.model.ErrorScope = ""
	a.model.AccountNotice = ""
	a.model.Page = catalog.Page{}
	if route == "downloads" {
		a.model.Loading = false
		a.refreshDownloads()
		a.pushHistory(record)
		a.invalidate()
		return
	}
	if route == "library" {
		if a.model.LibraryLoaded && a.libraryPage != nil {
			a.model.LibraryCached = true
			a.model.Page = a.libraryDisplayPage()
			a.model.Loading = false
		}
		if a.model.SignedIn {
			a.pushHistory(record)
			request := a.loadLibrary(!record)
			request.pageGeneration = generation
		} else if !a.model.LibraryCached {
			a.model.Loading = false
			a.model.Page = signedOutLibraryPage(a.tr)
			a.pushHistory(record)
		} else {
			a.pushHistory(record)
		}
		a.invalidate()
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	if !record {
		ctx = catalog.Refresh(ctx)
	}
	params := a.browseParams
	a.pageCancel = cancel
	// Reserve the destination before it finishes: Retry updates this entry,
	// and Back can still reach the origin after a failed or pending load.
	a.pushHistory(record)
	a.invalidate()
	go func() {
		defer cancel()
		started := time.Now()
		var page catalog.Page
		var err error
		switch {
		case route == "home":
			page, err = a.catalog.Home(ctx)
		case route == "explore":
			page, err = a.catalog.Explore(ctx)
		case route == "search":
			page, err = a.catalog.Search(ctx, q)
		case strings.HasPrefix(route, "browse:"):
			page, err = a.catalog.BrowseWithParams(ctx, strings.TrimPrefix(route, "browse:"), params)
		}
		if ctx.Err() != nil && err == nil {
			return
		}
		a.post(func() {
			if generation != a.pageGeneration {
				return
			}
			recordCatalogDiagnostic(a.diagnosticPath, err, time.Since(started))
			a.model.Loading = false
			if err != nil {
				a.catalogFailure(err)
				return
			}
			a.model.Page = page
			a.reconcileLikedMusic()
			switch route {
			case "home":
				a.model.Page.Title = a.greeting()
			case "explore":
				a.model.Page.Title = a.tr("shellNavExplore")
			case "search":
				a.model.Page.Title = q
			}
			a.pushHistory(false)
		})
	}()
}

// The sidebar keeps loading across navigation. Opening Library during that
// request attaches the current page, so both views receive the same result.
type libraryRequest struct {
	cancel         context.CancelFunc
	pageGeneration uint64
	scope          string
}

func (a *desktop) loadLibrary(force ...bool) *libraryRequest {
	if a.libraryRequest != nil {
		return a.libraryRequest
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	if len(force) > 0 && force[0] {
		ctx = catalog.Refresh(ctx)
	}
	request := &libraryRequest{cancel: cancel, scope: a.libraryScope}
	a.libraryRequest = request
	a.model.LibraryRefreshing = true
	a.model.LibraryErrorKey = ""
	extent := 0
	if a.libraryPage != nil {
		extent = len(a.libraryPage.Items)
	}
	go func() {
		defer cancel()
		page, err := loadLibraryExtent(ctx, a.catalog, extent)
		a.post(func() { a.finishLibrary(request, page, err) })
	}()
	return request
}

func (a *desktop) finishLibrary(request *libraryRequest, page catalog.Page, err error) {
	if a.libraryRequest != request {
		return
	}
	a.libraryRequest = nil
	a.model.LibraryRefreshing = false
	if err == nil {
		a.libraryPage = &page
		a.model.LibraryItems = page.Items
		a.model.LibraryLoaded = true
		a.model.LibraryCached = false
		if request.scope == a.libraryScope {
			a.saveLibrarySnapshot(page)
		}
	} else {
		a.model.LibraryErrorKey = catalogErrorKey(err)
		if a.model.LibraryLoaded {
			a.model.LibraryCached = true
		}
	}
	if a.model.Route != "library" || request.pageGeneration != a.pageGeneration {
		return
	}
	a.model.Loading = false
	if err != nil {
		if a.model.LibraryLoaded {
			a.model.Page = a.libraryDisplayPage()
			return
		}
		a.catalogFailure(err)
		return
	}
	a.model.Page = a.libraryDisplayPage()
	a.pushHistory(false)
}

func (a *desktop) resetLibrary() {
	if a.libraryRequest != nil {
		a.libraryRequest.cancel()
		a.libraryRequest = nil
	}
	a.model.LibraryItems = nil
	a.model.LibraryLoaded = false
	a.model.LibraryErrorKey = ""
	a.model.LibraryCached, a.model.LibraryRefreshing = false, false
}

func (a *desktop) retryLibrary() {
	if !a.model.SignedIn {
		return
	}
	if a.model.Route == "library" {
		a.reload()
		return
	}
	a.loadLibrary(true)
	a.invalidate()
}

func (a *desktop) greeting() string {
	h := time.Now().Hour()
	key := "shellGreetingEvening"
	switch {
	case h < 5:
		key = "shellGreetingNight"
	case h < 12:
		key = "shellGreetingMorning"
	case h < 18:
		key = "shellGreetingAfternoon"
	}
	return a.tr(key)
}

func (a *desktop) pushHistory(record bool) {
	location := pageLocation{a.model.Route, a.model.Query, a.browseParams}
	if !record {
		location = a.currentPageLocation()
	}
	entry := historyEntry{route: location.route, query: location.query, page: a.model.Page, params: location.params,
		openedCover: a.model.openedCover, needsReload: a.model.Loading && a.model.Page.Continuation == "", likedChanges: a.likedChanges}
	if record || a.historyIndex < 0 || a.historyIndex >= len(a.history) {
		end := min(max(a.historyIndex+1, 0), len(a.history))
		a.history = append(a.history[:end], entry)
		if len(a.history) > 24 {
			a.history = a.history[1:]
		}
		a.historyIndex = len(a.history) - 1
	} else if a.historyIndex >= 0 {
		a.history[a.historyIndex] = entry
	}
	a.model.CanBack = a.historyIndex > 0
	a.model.CanForward = a.historyIndex+1 < len(a.history)
}

func (a *desktop) invalidateHistory() {
	for i := range a.history {
		a.history[i].needsReload = true
	}
}

func (a *desktop) travel(delta int) {
	i := a.historyIndex + delta
	if i < 0 || i >= len(a.history) {
		return
	}
	if a.pageCancel != nil {
		a.pageCancel()
	}
	a.pageGeneration++
	a.historyIndex = i
	e := a.history[i]
	a.model.Route = e.route
	a.model.Query = e.query
	a.model.Page = e.page
	a.likedChanges = e.likedChanges
	a.model.openedCover = e.openedCover
	a.browseParams = e.params
	a.model.Loading = false
	a.model.Error = ""
	a.model.ErrorScope = ""
	a.model.Immersive = false
	a.model.CanBack = i > 0
	a.model.CanForward = i+1 < len(a.history)
	if e.route == "downloads" {
		a.refreshDownloads()
	}
	if e.needsReload {
		a.loadPage(e.route, e.query, false)
		return
	}
	// The sidebar keeps refreshing Library while another route is visible.
	// Returning joins that request or shows the result it already delivered.
	if e.route == "library" && a.model.LibraryLoaded && a.libraryPage != nil {
		if a.libraryRequest != nil {
			a.libraryRequest.pageGeneration = a.pageGeneration
		}
		a.model.Page = a.libraryDisplayPage()
	}
	a.invalidate()
}

func (a *desktop) playItem(it catalog.Item) {
	if it.ID == "" {
		a.openItem(it)
		return
	}
	// A list starts at the chosen track and retains its visible ordering.
	items := playableItems(a.model.Page.Items)
	index := -1
	for i, v := range items {
		if v.ID == it.ID {
			index = i
			break
		}
	}
	if index >= 0 {
		a.model.Queue = append([]catalog.Item(nil), items...)
		a.queueIndex = index
	} else {
		a.model.Queue = []catalog.Item{it}
		a.queueIndex = 0
	}
	a.startTrack(it)
	a.queueGeneration++
	queueGeneration := a.queueGeneration
	if index < 0 {
		generation := a.playGeneration
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
			defer cancel()
			queue, err := a.catalog.Queue(ctx, it.ID)
			if err != nil {
				return
			}
			a.post(func() {
				if generation != a.playGeneration || queueGeneration != a.queueGeneration {
					return
				}
				a.model.Queue = []catalog.Item{it}
				for _, q := range queue {
					if q.ID != "" && q.ID != it.ID {
						a.model.Queue = append(a.model.Queue, q)
					}
				}
				a.queueIndex = 0
				a.prepareNext()
			})
		}()
	}
}

func (a *desktop) startTrack(it catalog.Item) {
	a.startTrackAt(it, 0)
}

func (a *desktop) startTrackAt(it catalog.Item, at time.Duration) {
	if a.player == nil {
		a.setError("nativeRuntimeMissing")
		a.model.ErrorScope = ""
		return
	}
	a.cancelResume()
	a.powerPlaybackSuspended = false
	if a.playCancel != nil {
		a.playCancel()
	}
	if a.seekCancel != nil {
		a.seekCancel()
	}
	a.playGeneration++
	a.preparedID = ""
	generation := a.playGeneration
	// A deliberate retry of the same track also restarts its requested video.
	if a.videoID == it.ID {
		a.videoID = ""
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.playCancel = cancel
	a.restoredTrack = false
	a.model.Current = it
	if a.model.AudioQualityNotice == "nativeAudioQualitySaved" {
		a.model.AudioQualityNotice = ""
	}
	a.model.Position = at.Seconds()
	a.model.Duration = it.Duration.Seconds()
	a.model.Playing = false
	a.model.PlaybackLoading = true
	a.loadPending = true
	a.setLyricsNotice("shellLoading")
	a.model.Liked = it.Liked
	a.model.PlaybackError = ""
	a.endedHandled = true
	a.publishPausedTrack()
	a.invalidate()
	if a.model.SignedIn && !it.LikeKnown {
		accountGeneration, likeGeneration := a.accountGeneration, a.likeGeneration
		go func() {
			track, err := a.catalog.Track(ctx, it.ID)
			if err != nil || !track.LikeKnown {
				return
			}
			a.post(func() {
				if generation == a.playGeneration && accountGeneration == a.accountGeneration && likeGeneration == a.likeGeneration {
					a.setLiked(it.ID, track.Liked)
				}
			})
		}()
	}
	go func() {
		err := a.player.LoadAt(ctx, it.ID, at)
		a.post(func() {
			if generation != a.playGeneration {
				return
			}
			a.loadPending = false
			a.model.PlaybackLoading = false
			if errors.Is(err, context.Canceled) {
				// A seek may replace an in-flight load of this same track.
				// Its newer player generation owns the bookmark and loading UI.
				a.model.PlaybackLoading = a.player.Snapshot().Loading
				return
			}
			if err != nil && ctx.Err() == nil {
				// Resolution may fail before the decoder has a position (for
				// example, during account setup). Keep the bookmark for Retry.
				a.restoredTrack = at > 0
				state := a.player.Snapshot()
				recordPlaybackDiagnostic(a.diagnosticPath, "audio", state.Error, state.ErrorDetails)
				a.playbackFailure(state)
			} else if err == nil {
				a.endedHandled = false
				state := a.player.Snapshot()
				a.learnVideo(state)
				recordPlaybackDiagnostic(a.diagnosticPath, "audio", "ready")
				mygo.NowPlaying.Set(mygo.NowPlayingInfo{Title: it.Title, Artist: it.Artist, Album: it.Album, Duration: state.Duration, Position: state.Position, State: mygo.PlaybackPlaying, Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek})
				a.prepareNext()
			}
		})
	}()
	go func() {
		lyrics, err := a.trackLyrics(ctx, it.ID)
		a.post(func() {
			if generation != a.playGeneration || ctx.Err() != nil {
				return
			}
			if err != nil && !errors.Is(err, catalog.ErrNoLyrics) {
				a.setLyricsNotice("nativeLyricsError")
			} else if err != nil || lyrics.Text == "" {
				a.setLyricsNotice("nativeNoLyrics")
			} else {
				a.model.clearLyrics(lyrics.Text)
				a.model.LyricLines = lyrics.Lines
				a.model.LyricsSource = lyrics.Source
				a.model.LyricsAvailable = true
			}
		})
	}()
}
func (a *desktop) playQueue(i int) {
	if i < 0 || i >= len(a.model.Queue) {
		return
	}
	a.queueIndex = i
	a.startTrack(a.model.Queue[i])
}
func (a *desktop) next() {
	if len(a.model.Queue) == 0 {
		return
	}
	i := a.queueIndex + 1
	if i >= len(a.model.Queue) {
		if a.model.Repeat == 1 {
			i = 0
		} else {
			return
		}
	}
	a.playQueue(i)
}

func playableItems(items []catalog.Item) []catalog.Item {
	out := make([]catalog.Item, 0, len(items))
	for _, it := range items {
		if it.ID != "" {
			out = append(out, it)
		}
	}
	return out
}
func (a *desktop) appendQueue(it catalog.Item) {
	if it.ID == "" {
		return
	}
	a.model.Queue = append(a.model.Queue, it)
	a.queueGeneration++
	a.prepareNext()
	a.invalidate()
}

// removeFromQueue takes the songs with ids out of the queue, every time they
// appear, but the playing one stays where it plays.
func (a *desktop) removeFromQueue(ids []string) {
	gone := make(map[string]bool, len(ids))
	for _, id := range ids {
		gone[id] = true
	}
	kept := a.model.Queue[:0:0]
	index := -1
	for i, it := range a.model.Queue {
		if i == a.queueIndex {
			index = len(kept)
		} else if gone[it.ID] {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) == len(a.model.Queue) {
		return
	}
	a.model.Queue, a.queueIndex = kept, index
	a.queueGeneration++
	a.prepareNext()
	a.invalidate()
}

// The queue is already in its current shuffle order. Repeat-one reuses the
// current track via Seek, so it does not reserve a second media stream.
func nextPreparedTrack(queue []catalog.Item, index, repeat int) catalog.Item {
	if repeat == 2 || index < 0 || index >= len(queue) || len(queue) == 0 {
		return catalog.Item{}
	}
	next := index + 1
	if next == len(queue) {
		if repeat != 1 {
			return catalog.Item{}
		}
		next = 0
	}
	return queue[next]
}

func (a *desktop) setAudioQuality(quality audioquality.Quality) {
	if !quality.Valid() || quality == a.model.AudioQuality.Normalize() {
		return
	}
	if err := audioquality.Save(a.audioQualityPath, quality); err != nil {
		a.model.AudioQualityNotice = "nativeAudioQualitySaveError"
	} else {
		a.model.AudioQuality = quality
		a.model.AudioQualityNotice = "nativeAudioQualitySaved"
		if a.player != nil {
			a.player.SetAudioQuality(quality)
			a.prepareNext()
		}
	}
	a.invalidate()
}

func (a *desktop) prepareNext() {
	if a.player == nil {
		return
	}
	id := ""
	if !a.restoredTrack && !a.powerPlaybackSuspended && !a.loadPending && a.model.Current.ID != "" && a.model.PlaybackError == "" {
		id = nextPreparedTrack(a.model.Queue, a.queueIndex, a.model.Repeat).ID
	}
	quality := a.model.AudioQuality.Normalize()
	if id == a.preparedID && (id == "" || quality == a.preparedQuality.Normalize()) {
		return
	}
	if a.prepareCancel != nil {
		a.prepareCancel()
	}
	a.preparedID, a.preparedQuality = id, quality
	a.player.ClearPrepared()
	if id == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.prepareCancel = cancel
	go func() { _ = a.player.Prepare(ctx, id) }()
}

func (a *desktop) repeat() {
	a.model.Repeat = (a.model.Repeat + 1) % 3
	a.prepareNext()
	a.invalidate()
}
func (a *desktop) previous() {
	if a.model.Position > 3 {
		a.seek(0)
		return
	}
	if a.queueIndex > 0 {
		a.playQueue(a.queueIndex - 1)
	} else {
		a.seek(0)
	}
}
func (a *desktop) toggle() {
	if a.player == nil {
		return
	}
	if a.model.Current.ID == "" {
		if len(a.model.Queue) > 0 {
			a.playQueue(0)
		}
		return
	}
	if a.restoredTrack || a.player.Snapshot().Error != "" {
		a.retryPlayback()
		return
	}
	if a.player.Snapshot().Playing {
		a.pause()
	} else {
		a.resume()
	}
	a.invalidate()
}

func (a *desktop) retryPlayback() {
	a.startTrackAt(a.model.Current, time.Duration(a.model.Position*float64(time.Second)))
}

func (a *desktop) cancelResume() {
	if a.resumeCancel != nil {
		a.resumeCancel()
		a.resumeCancel = nil
	}
}

func (a *desktop) pause() {
	a.cancelResume()
	if a.player != nil {
		a.player.Pause()
	}
}

func (a *desktop) resume() {
	if a.player == nil {
		return
	}
	if a.model.Current.ID == "" {
		if len(a.model.Queue) > 0 {
			a.playQueue(0)
		}
		return
	}
	if a.restoredTrack || a.player.Snapshot().Error != "" {
		a.retryPlayback()
		return
	}
	a.cancelResume()
	ctx, cancel := context.WithCancel(a.ctx)
	a.resumeCancel = cancel
	generation := a.playGeneration
	go func() {
		err := a.player.PlayContext(ctx)
		a.post(func() {
			defer cancel()
			if ctx.Err() != nil || generation != a.playGeneration {
				return
			}
			a.resumeCancel = nil
			if errors.Is(err, context.Canceled) {
				return
			} else if err != nil {
				a.playbackFailure(a.player.Snapshot())
			} else {
				a.model.PlaybackError = ""
				recordPlaybackDiagnostic(a.diagnosticPath, "audio", "ready")
				a.endedHandled = false
			}
		})
	}()
}

func (a *desktop) loadMore() {
	if a.model.Loading || a.model.Page.Continuation == "" || nativeContinuationExpired(&a.model) {
		return
	}
	previous := a.model.Page
	location := a.currentPageLocation()
	generation := a.pageGeneration
	a.model.Loading = true
	a.model.Error = ""
	a.model.ErrorScope = ""
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	a.pageCancel = cancel
	go func() {
		defer cancel()
		started := time.Now()
		page, replace, err := continuePage(ctx, a.catalog, location, previous)
		a.post(func() {
			if generation != a.pageGeneration {
				return
			}
			recordContinuationDiagnostic(a.diagnosticPath, err, time.Since(started))
			a.finishPageContinuation(generation, page, replace, err)
		})
	}()
}

func (a *desktop) finishContinuation(generation uint64, page catalog.Page, err error) {
	a.finishPageContinuation(generation, page, false, err)
}

func (a *desktop) finishPageContinuation(generation uint64, page catalog.Page, replace bool, err error) {
	if generation != a.pageGeneration {
		return
	}
	a.model.Loading = false
	if err != nil {
		a.model.setContinuationFailure(err, a.trArgs)
		return
	}
	if replace {
		a.model.Page = page
	} else {
		a.model.Page.Items = append(a.model.Page.Items, page.Items...)
		a.model.Page.Shelves = append(a.model.Page.Shelves, page.Shelves...)
		a.model.Page.Continuation = page.Continuation
	}
	a.reconcileLikedMusic()
	if a.model.Route == "library" && a.model.SignedIn {
		saved := a.model.Page
		a.libraryPage = &saved
		a.model.LibraryItems = append([]catalog.Item(nil), saved.Items...)
		a.saveLibrarySnapshot(saved)
	}
	a.pushHistory(false)
}
func (a *desktop) seek(seconds float64) {
	if a.player == nil || a.model.Current.ID == "" || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return
	}
	a.cancelResume()
	seconds = math.Max(0, math.Min(a.model.Duration, seconds))
	if a.restoredTrack {
		a.model.Position = seconds
		a.publishPausedTrack()
		a.invalidate()
		return
	}
	a.model.PlaybackError = ""
	generation := a.playGeneration
	a.seekGeneration++
	if a.seekCancel != nil {
		a.seekCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.seekCancel = cancel
	if a.prepareCancel != nil {
		a.prepareCancel()
	}
	a.preparedID = ""
	seekGeneration := a.seekGeneration
	go func() {
		defer cancel()
		err := a.player.SeekContext(ctx, time.Duration(seconds*float64(time.Second)))
		a.post(func() {
			if generation == a.playGeneration && seekGeneration == a.seekGeneration {
				if err != nil && !errors.Is(err, context.Canceled) {
					a.playbackFailure(a.player.Snapshot())
				} else if err == nil {
					a.endedHandled = false
					recordPlaybackDiagnostic(a.diagnosticPath, "audio", "ready")
					a.prepareNext()
				}
			}
		})
	}()
}
func (a *desktop) volume(value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	value = math.Max(0, math.Min(1, value))
	a.model.Volume = value
	a.model.Muted = value == 0
	if value > 0 {
		a.unmutedVolume = value
	}
	if a.player != nil {
		a.player.SetVolume(value)
	}
	a.invalidate()
}
func (a *desktop) mute() {
	if a.model.Muted || a.model.Volume == 0 {
		a.volume(a.unmutedVolume)
	} else {
		a.volume(0)
	}
}
func (a *desktop) shuffle() {
	a.queueGeneration++
	a.model.Shuffle = !a.model.Shuffle
	if a.model.Shuffle && a.queueIndex >= 0 && a.queueIndex < len(a.model.Queue) {
		tail := a.model.Queue[a.queueIndex+1:]
		rand.Shuffle(len(tail), func(i, j int) { tail[i], tail[j] = tail[j], tail[i] })
	}
	a.prepareNext()
	a.invalidate()
}
func (a *desktop) like() {
	if a.model.Current.ID == "" {
		return
	}
	if !a.model.SignedIn {
		a.signIn()
		return
	}
	track := a.model.Current
	if a.likeCancel != nil {
		a.likeCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	a.likeCancel = cancel
	accountGeneration := a.accountGeneration
	a.likeGeneration++
	mutationGeneration := a.likeGeneration
	liked := !a.model.Liked
	go func() {
		defer cancel()
		err := a.catalog.Like(ctx, track.ID, liked)
		a.post(func() {
			if accountGeneration != a.accountGeneration || mutationGeneration != a.likeGeneration {
				return
			}
			if err != nil {
				a.setError("nativeAccountError")
				a.model.ErrorScope = ""
			} else {
				a.confirmLike(track, liked)
			}
		})
	}()
}
