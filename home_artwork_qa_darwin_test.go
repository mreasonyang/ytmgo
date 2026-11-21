//go:build darwin && ytmgo_artwork_qa

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestHomeArtworkQAColdBlockedNativeView(t *testing.T) {
	model, actions := nativeViewFixture()
	model.Loading, model.SignedIn, model.LibraryLoaded = true, true, true
	model.Playing = false
	model.LibraryItems = model.Page.Shelves[1].Items[:1]
	model.Page = catalog.Page{}
	var q homeArtworkQA
	artwork := actions.Artwork
	player, sidebar := 0, 0
	actions.Artwork = func(src string, edge int, priority artworkPriority) ui.ImageSource {
		image := artwork(src, edge, priority)
		if priority == artPlayer && image != nil {
			player++
		}
		return image
	}
	actions.artworkPeek = func(src string, edge int) ui.ImageSource {
		image := artwork(src, edge, artPrimary)
		if src == model.LibraryItems[0].Artwork && edge == 48 && image != nil {
			sidebar++
		}
		return image
	}
	tt := ui.NewTester(func(c *ui.Context) {
		a := actions
		a.homeArtworkQA = &q
		q.prepareFrame(c, func() string {
			if model.Loading {
				return "home_not_ready"
			}
			return ""
		}, 1, 1)
		nativeView(c, model, a)
	}, 1440, 900)
	if player == 0 || sidebar == 0 || q.frameReady || q.paint != nil {
		t.Fatalf("blocked startup: player=%d sidebar=%d ready=%t samples=%d", player, sidebar, q.frameReady, len(q.paint))
	}
	model.Loading = false
	tt.Frame()
	tt.Advance(2 * homeQAFrame)
	if len(q.previousHot) != 1 {
		t.Fatalf("want only the home shortcut counted, got %d samples", len(q.previousHot))
	}
}

func TestHomeArtworkQAIgnoresReplayedAndUnregisteredCovers(t *testing.T) {
	model, actions := nativeViewFixture()
	var q homeArtworkQA
	open, replays, unregistered := true, 0, 0
	art := model.Current.Artwork
	actions.artworkReplayed = func(string, ui.ImageSource) { replays++ }
	tt := ui.NewTester(func(c *ui.Context) {
		q.prepareFrame(c, func() string { return "" }, 1, 1)
		scroll := ui.Scroll(c).Fill()
		a := nativeArtworkViewport(c, actions, scroll)
		a.homeArtworkPageQA = q.trackPage(scroll, a.viewportScroll)
		scroll.Children(func() {
			if open {
				ui.Box(c.Key("leaving")).Size(64, 64).Transition(nativeFadeLong).Children(func() {
					nativeCover(c, art, 64, 8, a, palette(true), artPrimary)
				})
			}
			stray := ui.Box(c.Key("unregistered")).Size(64, 64)
			stray.Draw(func(p *ui.Painter, r ui.Rect) {
				if !stray.Valid() || !stray.InViewAmount(0.6) {
					return
				}
				p.Image(actions.Artwork(art, 64, artPrimary), r, ui.Cover)
				unregistered++
				q.paintedCover(stray, true)
			})
		})
	}, 200, 200)
	tt.Advance(2 * homeQAFrame)
	if len(q.previousHot) != 1 || unregistered == 0 {
		t.Fatalf("registration: samples=%d stray paints=%d", len(q.previousHot), unregistered)
	}
	open = false
	tt.Frame()
	tt.Advance(tokenMotionSlow / 4)
	if replays == 0 || len(q.previousHot) != 0 {
		t.Fatalf("departed cover: replays=%d samples=%d", replays, len(q.previousHot))
	}
}

func TestHomeArtworkQA(t *testing.T) {
	for _, tc := range []struct {
		name, state, reason string
	}{
		{"warm", "pass", "complete"},
		{"note", "fail", "notes_shown"},
		{"manual", "inconclusive", "manual_input"},
		{"short", "inconclusive", "insufficient_scroll"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, actions := nativeViewFixture()
			model.Playing = false
			model.Loading = true
			for i := 0; i < 2 && tc.name != "short"; i++ {
				model.Page.Shelves = append(model.Page.Shelves, model.Page.Shelves[:2]...)
			}
			if tc.name == "short" {
				model.Page.Shelves = model.Page.Shelves[:1]
			}
			var result homeQAResult
			q := &homeArtworkQA{finish: func(r homeQAResult) { result = r }}
			originalArtwork := actions.Artwork
			actions.Artwork = func(src string, edge int, priority artworkPriority) ui.ImageSource {
				if tc.name == "note" && q.phase == homeQABack && priority != artPlayer {
					return nil
				}
				return originalArtwork(src, edge, priority)
			}
			actions.artworkPeek = func(src string, edge int) ui.ImageSource {
				return actions.Artwork(src, edge, artPrimary)
			}
			paginationStarted := false
			if tc.name == "warm" {
				model.Page.Continuation = "fixture"
				actions.LoadMore = func() {
					paginationStarted, model.Loading = true, true
				}
			}
			tt := ui.NewTester(func(c *ui.Context) {
				a := actions
				if !q.ended {
					a.homeArtworkQA = q
					q.prepareFrame(c, func() string {
						if model.Loading && q.phase == homeQAWarm {
							return "home_not_ready"
						}
						return ""
					}, 1, 1)
				}
				nativeView(c, model, a)
			}, 1440, 900)
			tt.Advance(3 * homeQAFrame)
			if q.phase != homeQAWarm || len(q.previousHot) != 0 {
				t.Fatalf("sampled while loading: phase %d", q.phase)
			}
			if tc.name != "short" {
				model.pageScroll.Y = 25
			}
			model.Loading = false
			tt.Frame()
			if tc.name == "manual" {
				tt.Advance(3 * homeQAFrame)
				before := model.pageScroll.Y
				if before <= 25 {
					t.Fatalf("expected a committed move before input, got %g", before)
				}
				tt.Scroll(700, 400, 0, -100)
				tt.Advance(time.Second)
				if model.pageScroll.Y >= before || model.pageScroll.Y <= 25 {
					t.Fatalf("manual scroll lost or overridden: before %g, after %g", before, model.pageScroll.Y)
				}
			} else {
				tt.Advance(3 * time.Second)
			}
			if result.State != tc.state || result.Reason != tc.reason {
				t.Fatalf("unexpected result %+v", result)
			}
			if tc.name == "warm" && !paginationStarted {
				t.Fatal("scroll did not reach the normal pagination trigger")
			}
			if tc.name == "warm" || tc.name == "note" {
				if result.InitialHot < homeQAMinHot || result.Completed != 2 || result.Restored != 1 ||
					result.InitialY != 25 || !homeQANear(result.FinalY, result.InitialY) ||
					!homeQANear(model.pageScroll.Y, result.InitialY) || result.MaxDeltaY <= 1 {
					t.Fatalf("invalid two-round evidence %+v", result)
				}
				for _, r := range result.Rounds {
					if r.Offscreen != result.InitialHot || r.Reentered != r.Offscreen || (r.Notes > 0) != (tc.name == "note") {
						t.Fatalf("invalid round evidence %+v", result)
					}
				}
			}
		})
	}
}

func TestHomeArtworkQAOutput(t *testing.T) {
	path, err := os.MkdirTemp("/private/tmp", "ytmgo-home-qa.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(path) })
	dir := openHomeQADir(path)
	if dir == nil {
		t.Fatal("private output directory rejected")
	}
	defer dir.Close()
	result := homeQAResult{State: "inconclusive", Reason: "home_not_ready"}
	writeHomeQAResult(dir, result)
	output := filepath.Join(path, "result.json")
	data, err := os.ReadFile(output)
	var parsed homeQAResult
	if err != nil || json.Unmarshal(data, &parsed) != nil || parsed != result {
		t.Fatalf("invalid result file: %v", err)
	}
	if info, err := os.Stat(output); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("invalid result mode: %v", err)
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(path, "untouched")
	if err := os.Symlink(target, output); err != nil {
		t.Fatal(err)
	}
	writeHomeQAResult(dir, result)
	if link, err := os.Readlink(output); err != nil || link != target {
		t.Fatalf("result symlink was replaced: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("result followed a symlink")
	}
}
