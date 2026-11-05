package main

import (
	"image"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/videoquality"
)

func (a *desktop) setVideoQuality(quality int) {
	if a.changeVideoQuality(quality) {
		a.setVideoError("nativeVideoUnavailable")
	}
	a.invalidate()
}

// Report a rejected video restart separately from localization, as
// syncVideoSession does, so this transition also runs without a native window.
func (a *desktop) changeVideoQuality(quality int) bool {
	if !videoquality.Valid(quality) || quality == videoquality.Normalize(a.model.VideoQuality) {
		return false
	}
	if err := videoquality.Save(a.videoQualityPath, quality); err != nil {
		a.model.VideoQualityError = "nativeVideoQualitySaveError"
		return false
	}
	a.model.VideoQuality = quality
	a.model.VideoQualityError = ""
	// Force the same track through video resolution again. The audio player
	// keeps its current lease, clock, volume and playing/paused state.
	return a.syncVideoSession(true)
}

func (a *desktop) setVideo(value bool) {
	retry := value && a.model.VideoError != ""
	a.model.Video = value
	a.syncVideoState(retry)
	a.invalidate()
}

// Video owns resources only while a visible presentation asks for it. Cover
// mode, closing Now Playing, navigation and window hiding all release it.
func (a *desktop) syncVideo() {
	a.syncVideoState(false)
}

func (a *desktop) syncVideoState(retry bool) {
	if a.syncVideoSession(retry) {
		a.setVideoError("nativeVideoUnavailable")
	}
}

// syncVideoSession changes only video ownership and presentation state. It
// reports startup rejection for the UI to localize, without touching audio.
func (a *desktop) syncVideoSession(retry bool) bool {
	wanted := ""
	if !a.powerPlaybackSuspended && a.windowVisible && a.player != nil && a.model.Video && (a.model.Panel || a.model.Immersive) {
		wanted = a.model.Current.ID
	}
	if wanted == a.videoID && !(retry && wanted != "") {
		return false
	}
	a.videoID = wanted
	a.videoGeneration.Add(1)
	a.videoActive.Store(wanted != "")
	a.model.VideoError = ""
	a.model.VideoLoading = false
	a.model.VideoFrame = nil
	a.videoFrame = nil
	a.videoFrameNumber = 0
	if a.video == nil {
		a.videoActive.Store(false)
		return wanted != ""
	}
	if wanted == "" {
		a.video.Stop()
		a.model.VideoLoading = false
		return false
	}
	a.model.VideoLoading = true
	err := a.video.StartWithQuality(a.ctx, wanted, a.model.VideoQuality, func() time.Duration { return a.player.Snapshot().Position }, func() bool { return a.player.Snapshot().Playing })
	if err != nil {
		a.videoActive.Store(false)
		a.model.VideoLoading = false
		return true
	}
	return false
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
			details := a.video.ErrorDetails()
			if number == previousNumber && loading == previousLoading && errText == previousError {
				continue
			}
			previousNumber, previousLoading, previousError = number, loading, errText
			// A frame is immutable while held by the UI; Bitmap.Update preserves
			// one GPU texture instead of allocating a new texture for every frame.
			a.post(func() {
				if a.videoID == "" || generation != a.videoGeneration.Load() || !a.videoActive.Load() {
					return
				}
				message := ""
				if errText != "" {
					recordPlaybackDiagnostic(a.diagnosticPath, "video", errText, details)
					message = a.tr("nativeVideoUnavailable")
				}
				a.applyVideoFrame(generation, frame, number, loading, message)
				if errText != "" {
					a.setVideoError("nativeVideoUnavailable")
				}
			})
		}
	}
}

func (a *desktop) applyVideoFrame(generation uint64, frame image.Image, number uint64, loading bool, errText string) {
	if a.videoID == "" || generation != a.videoGeneration.Load() || !a.videoActive.Load() {
		return
	}
	if errText != "" {
		a.model.VideoError = errText
		a.model.VideoLoading = false
		a.model.VideoFrame, a.videoFrame = nil, nil
		a.videoFrameNumber = 0
		return
	}
	a.model.VideoError = ""
	a.model.VideoLoading = loading
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
}
