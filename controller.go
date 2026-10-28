package main

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"ytmgo/internal/catalog"
)

func (a *desktop) navigate(route string) {
	a.model.Immersive = false
	a.browseParams = ""
	if route == "search" {
		a.model.FocusSearch = true
		a.invalidate()
		return
	}
	a.loadPage(route, "", true)
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
	} else if it.ID != "" {
		a.playItem(it)
	}
}
func (a *desktop) reload() { a.loadPage(a.model.Route, a.model.Query, false) }

func (a *desktop) loadPage(route, q string, record bool) {
	if a.pageCancel != nil {
		a.pageCancel()
	}
	a.pageGeneration++
	generation := a.pageGeneration
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	params := a.browseParams
	a.pageCancel = cancel
	a.model.Route = route
	a.model.Query = q
	a.model.Loading = true
	a.model.Error = ""
	a.model.ErrorScope = ""
	a.model.Page = catalog.Page{}
	if route == "library" && !a.model.SignedIn {
		cancel()
		a.model.Loading = false
		a.model.Page = catalog.Page{Title: a.tr("shellLibraryTitle"), Subtitle: a.tr("shellLibraryEmptyBody")}
		a.pushHistory(record)
		a.invalidate()
		return
	}
	a.invalidate()
	go func() {
		defer cancel()
		var page catalog.Page
		var err error
		switch {
		case route == "home":
			page, err = a.catalog.Home(ctx)
		case route == "explore":
			page, err = a.catalog.Explore(ctx)
		case route == "library":
			page, err = a.catalog.Library(ctx)
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
			a.model.Loading = false
			if err != nil {
				a.catalogFailure(err)
				return
			}
			a.model.Page = page
			switch route {
			case "home":
				a.model.Page.Title = a.greeting()
			case "explore":
				a.model.Page.Title = a.tr("shellNavExplore")
			case "library":
				a.model.Page.Title = a.tr("shellLibraryTitle")
				a.model.LibraryItems = page.Items
			case "search":
				a.model.Page.Title = q
			}
			a.pushHistory(record)
		})
	}()
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
	entry := historyEntry{route: a.model.Route, query: a.model.Query, page: a.model.Page, params: a.browseParams}
	if record {
		a.history = append(a.history[:a.historyIndex+1], entry)
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
	a.browseParams = e.params
	a.model.Loading = false
	a.model.Error = ""
	a.model.ErrorScope = ""
	a.model.Immersive = false
	a.model.CanBack = i > 0
	a.model.CanForward = i+1 < len(a.history)
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
	if a.player == nil {
		a.model.Error = a.tr("nativeRuntimeMissing")
		a.model.ErrorScope = ""
		return
	}
	a.cancelResume()
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
	a.model.Current = it
	a.model.Position = 0
	a.model.Duration = it.Duration.Seconds()
	a.model.Playing = false
	a.model.PlaybackLoading = true
	a.loadPending = true
	a.model.Lyrics = a.tr("shellLoading")
	a.model.Liked = it.Liked
	a.model.PlaybackError = ""
	a.endedHandled = true
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{Title: it.Title, Artist: it.Artist, Album: it.Album, Duration: it.Duration, State: mygo.PlaybackPaused, Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek})
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
		err := a.player.Load(ctx, it.ID)
		a.post(func() {
			if generation != a.playGeneration {
				return
			}
			a.loadPending = false
			a.model.PlaybackLoading = false
			if err != nil && ctx.Err() == nil {
				state := a.player.Snapshot()
				recordPlaybackDiagnostic(a.diagnosticPath, "audio", state.Error, state.ErrorDetails)
				a.playbackFailure(state)
			} else if err == nil {
				a.endedHandled = false
				state := a.player.Snapshot()
				mygo.NowPlaying.Set(mygo.NowPlayingInfo{Title: it.Title, Artist: it.Artist, Album: it.Album, Duration: state.Duration, Position: state.Position, State: mygo.PlaybackPlaying, Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek})
				a.prepareNext()
			}
		})
	}()
	go func() {
		lyrics, err := a.catalog.Lyrics(ctx, it.ID)
		a.post(func() {
			if generation != a.playGeneration {
				return
			}
			if err != nil || lyrics == "" {
				a.model.Lyrics = a.tr("nativeNoLyrics")
			} else {
				a.model.Lyrics = lyrics
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

// The queue is already in its current shuffle order. Repeat-one reuses the
// current track via Seek, so it does not reserve a second media stream.
func nextPreparedID(queue []catalog.Item, index, repeat int) string {
	if repeat == 2 || index < 0 || index >= len(queue) || len(queue) == 0 {
		return ""
	}
	next := index + 1
	if next == len(queue) {
		if repeat != 1 {
			return ""
		}
		next = 0
	}
	return queue[next].ID
}

func (a *desktop) prepareNext() {
	if a.player == nil {
		return
	}
	id := ""
	if !a.loadPending && a.model.Current.ID != "" && a.model.PlaybackError == "" {
		id = nextPreparedID(a.model.Queue, a.queueIndex, a.model.Repeat)
	}
	if id == a.preparedID {
		return
	}
	if a.prepareCancel != nil {
		a.prepareCancel()
	}
	a.preparedID = id
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
	if a.player.Snapshot().Error != "" {
		a.startTrack(a.model.Current)
		return
	}
	if a.player.Snapshot().Playing {
		a.pause()
	} else {
		a.resume()
	}
	a.invalidate()
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
	if a.player.Snapshot().Error != "" {
		a.startTrack(a.model.Current)
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
				a.endedHandled = false
			}
		})
	}()
}

func (a *desktop) loadMore() {
	if a.model.Loading || a.model.Page.Continuation == "" {
		return
	}
	token := a.model.Page.Continuation
	generation := a.pageGeneration
	a.model.Loading = true
	a.model.Error = ""
	a.model.ErrorScope = ""
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
		defer cancel()
		page, err := a.catalog.Continue(ctx, token)
		a.post(func() {
			if generation != a.pageGeneration {
				return
			}
			a.model.Loading = false
			if err != nil {
				a.catalogFailure(err)
				return
			}
			a.model.Page.Items = append(a.model.Page.Items, page.Items...)
			a.model.Page.Shelves = append(a.model.Page.Shelves, page.Shelves...)
			a.model.Page.Continuation = page.Continuation
			a.pushHistory(false)
		})
	}()
}
func (a *desktop) seek(seconds float64) {
	if a.player == nil || a.model.Current.ID == "" || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return
	}
	a.cancelResume()
	seconds = math.Max(0, math.Min(a.model.Duration, seconds))
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
	id := a.model.Current.ID
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
		err := a.catalog.Like(ctx, id, liked)
		a.post(func() {
			if accountGeneration != a.accountGeneration || mutationGeneration != a.likeGeneration {
				return
			}
			if err != nil {
				a.model.Error = a.tr("nativeAccountError")
				a.model.ErrorScope = ""
			} else {
				a.setLiked(id, liked)
			}
		})
	}()
}

func (a *desktop) setLiked(id string, liked bool) {
	update := func(it *catalog.Item) {
		if it.ID == id {
			it.Liked, it.LikeKnown = liked, true
		}
	}
	update(&a.model.Current)
	if a.model.Current.ID == id {
		a.model.Liked = liked
	}
	for i := range a.model.Queue {
		update(&a.model.Queue[i])
	}
	for i := range a.model.Page.Items {
		update(&a.model.Page.Items[i])
	}
	for s := range a.model.Page.Shelves {
		for i := range a.model.Page.Shelves[s].Items {
			update(&a.model.Page.Shelves[s].Items[i])
		}
	}
}
