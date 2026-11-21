//go:build darwin && ytmgo_artwork_qa

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

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
