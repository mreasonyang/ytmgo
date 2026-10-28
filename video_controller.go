package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *desktop) setVideo(value bool) { a.model.Video = value; a.syncVideo(); a.invalidate() }

// Video owns resources only while a visible presentation asks for it. Cover
// mode, closing Now Playing, navigation and window hiding all release it.
func (a *desktop) syncVideo() {
	wanted := ""
	if a.windowVisible && a.player != nil && a.model.Video && (a.model.Panel || a.model.Immersive) {
		wanted = a.model.Current.ID
	}
	if wanted == a.videoID {
		return
	}
	a.videoID = wanted
	a.videoGeneration.Add(1)
	a.videoActive.Store(wanted != "")
	a.model.VideoError = ""
	a.model.VideoFrame = nil
	a.videoFrame = nil
	a.videoFrameNumber = 0
	if a.video == nil {
		if wanted != "" {
			a.model.VideoError = a.tr("nativeVideoUnavailable")
		}
		return
	}
	if wanted == "" {
		a.video.Stop()
		a.model.VideoLoading = false
		return
	}
	a.model.VideoLoading = true
	err := a.video.Start(a.ctx, wanted, func() time.Duration { return a.player.Snapshot().Position }, func() bool { return a.player.Snapshot().Playing })
	if err != nil {
		a.model.VideoLoading = false
		a.model.VideoError = a.tr("nativeVideoUnavailable")
	}
}

func (a *desktop) pollVideo() {
	t := time.NewTicker(time.Second / 24)
	defer t.Stop()
	var previousNumber uint64
	var previousLoading bool
	var previousError string
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			if a.video == nil || !a.videoActive.Load() {
				continue
			}
			generation := a.videoGeneration.Load()
			frame, number := a.video.Frame()
			loading, errText := a.video.Loading(), a.video.Error()
			if number == previousNumber && loading == previousLoading && errText == previousError {
				continue
			}
			previousNumber, previousLoading, previousError = number, loading, errText
			// A frame is immutable while held by the UI; Bitmap.Update preserves
			// one GPU texture instead of allocating a new texture for every frame.
			a.post(func() {
				if a.videoID == "" || generation != a.videoGeneration.Load() {
					return
				}
				a.model.VideoLoading = loading
				if errText != "" {
					recordPlaybackDiagnostic(a.diagnosticPath, "video", errText)
					a.model.VideoError = a.tr("nativeVideoUnavailable")
				}
				if frame == nil || number == a.videoFrameNumber {
					return
				}
				a.videoFrameNumber = number
				if a.videoFrame == nil {
					a.videoFrame = ui.NewBitmap(frame)
				} else {
					a.videoFrame.Update(frame)
				}
				a.model.VideoFrame = a.videoFrame
			})
		}
	}
}
