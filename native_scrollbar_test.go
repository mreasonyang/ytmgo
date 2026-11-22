package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

// A scroller's bar shows as the pointer comes over it and as it scrolls,
// fades once the pointer rests on the content, and stays while the
// pointer is on the bar's strip, where it can be grabbed.
func TestScrollbarFadesWhileThePointerRests(t *testing.T) {
	p := palette(true)
	var st ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(p.surface).Children(func() {
			scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&st)
			nativeScrollbar(c, scroll, &st, p, nativeScrollbarCorners{}, nil)
			scroll.Children(func() {
				for i := range 40 {
					ui.Text(c, fmt.Sprint("Row ", i)).Height(24)
				}
			})
		})
	}, 200, 200)
	settle := func(d time.Duration) {
		for elapsed := time.Duration(0); elapsed < d; elapsed += 50 * time.Millisecond {
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
		}
	}
	insets := nativeScrollbarInsets(nativeScrollbarCorners{})
	// The thumb's middle, where MyGo would draw it.
	shown := func() bool {
		bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 200}, &st, false, insets, nativeScrollbarRestWidth)[0]
		c := tt.Image().At(int(2*(bar.X+bar.W/2)), int(2*(bar.Y+bar.H/2)))
		r, _, _, _ := c.RGBA()
		bg, _, _, _ := tt.Image().At(2*100, 2*190).RGBA()
		return r>>8 > bg>>8+8
	}
	tt.SetScale(2)
	tt.Frame()
	if shown() {
		t.Fatal("a bar shows before the pointer comes over the scroller")
	}
	tt.Move(60, 100)
	settle(300 * time.Millisecond)
	if !shown() {
		t.Fatal("the bar does not show as the pointer comes over the scroller")
	}
	settle(2 * time.Second)
	if shown() {
		t.Fatal("the bar stays while the pointer rests on the content")
	}
	tt.Move(194, 100)
	settle(2 * time.Second)
	if !shown() {
		t.Fatal("the bar is not there to grab with the pointer on its strip")
	}
	tt.Move(60, 100)
	settle(2 * time.Second)
	st.Y = 0
	tt.Scroll(60, 100, 0, 120)
	settle(300 * time.Millisecond)
	if st.Y == 0 {
		t.Fatal("the scroller did not scroll")
	}
	if !shown() {
		t.Fatal("the bar does not show while the content scrolls")
	}
	// What shows is what grabs: dragging the drawn thumb scrolls.
	tt.Move(194, 100)
	settle(300 * time.Millisecond)
	before := st.Y
	bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 200}, &st, false, insets, nativeScrollbarWidth)[0]
	tt.Press(bar.X+bar.W/2, bar.Y+bar.H/2)
	tt.Move(bar.X+bar.W/2, bar.Y+bar.H/2+40)
	tt.Release(bar.X+bar.W/2, bar.Y+bar.H/2+40)
	settle(100 * time.Millisecond)
	if st.Y <= before {
		t.Fatalf("dragging the drawn thumb did not scroll (%g -> %g)", before, st.Y)
	}
}

// A grabbed thumb keeps its grabbed colour (muted) for the whole drag,
// wherever the pointer goes, and returns to the track colour once let go.
func TestScrollbarThumbHoldsItsColourWhileDragged(t *testing.T) {
	p := palette(false)
	var st ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().ObserveInput(nativeReleaseScrollGrabs)
		ui.Column(c).Fill().Background(p.surface).Children(func() {
			scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&st)
			nativeScrollbar(c, scroll, &st, p, nativeScrollbarCorners{}, nil)
			scroll.Children(func() {
				for i := range 80 {
					ui.Text(c, fmt.Sprint("Row ", i)).Height(24)
				}
			})
		})
	}, 200, 400)
	tt.SetScale(2)
	insets := nativeScrollbarInsets(nativeScrollbarCorners{})
	thumb := func() color.RGBA {
		tt.Frame()
		bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 400}, &st, false, insets, nativeScrollbarWidth)[0]
		return color.RGBAModel.Convert(tt.Image().At(int(2*(bar.X+bar.W/2)), int(2*(bar.Y+bar.H/2)))).(color.RGBA)
	}
	tt.Move(194, 40)
	tt.Frame()
	bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 400}, &st, false, insets, nativeScrollbarWidth)[0]
	grabbed := thumb()
	tt.Press(bar.X+bar.W/2, bar.Y+bar.H/2)
	tt.Frame()
	for _, to := range [][2]float32{{120, 80}, {20, 140}, {-40, 200}} {
		tt.Move(to[0], to[1])
		if got := thumb(); got != grabbed {
			t.Fatalf("dragged to %v the thumb is %v, want its grabbed %v", to, got, grabbed)
		}
	}
	tt.Release(20, 200)
	tt.Frame()
	if got := thumb(); got == grabbed {
		t.Fatal("the thumb kept its grabbed colour after being let go, off its strip")
	}
}

// Lyrics following playback scroll by themselves: that isn't the reader, so
// the bar doesn't flash on every line.
func TestScrollbarIgnoresLyricsFollowingPlayback(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	p := palette(true)
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, p).Grow(1) }, 360, 340)
	tt.SetScale(2)
	tt.Frame()
	view, ok := tt.Find("Lyrics · Content")
	if !ok {
		t.Fatal("lyrics viewport missing")
	}
	thumbShown := func() bool {
		img := tt.Image()
		_, _, _, _ = img.At(0, 0).RGBA()
		bg, _, _, _ := img.At(int(2*(view.X+view.W-30)), int(2*(view.Y+4))).RGBA()
		for y := view.Y + 4; y < view.Y+view.H-4; y += 2 {
			r, _, _, _ := img.At(int(2*(view.X+view.W-3-nativeScrollbarWidth/2)), int(2*y)).RGBA()
			if r>>8 > bg>>8+8 {
				return true
			}
		}
		return false
	}
	// Each line's glide, sampled on its way and once it has landed.
	for line := 1; line < 5; line++ {
		m.Position = float64(line*5) + 0.5
		tt.Frame()
		for _, step := range []time.Duration{tokenMotionSlow / 2, tokenMotionSlow} {
			tt.Advance(step)
			tt.Frame()
			if thumbShown() {
				t.Fatalf("the bar flashed as the lyrics followed line %d", line)
			}
		}
	}
	requireLyricCentered(t, tt, 4) // they did follow, gliding
}

// The page panel ends as the library beside it does: rows reaching its
// foot fade into the panel while more lies below, never cut flat.
func TestPageFootFadesWhileMoreLiesBelow(t *testing.T) {
	m, a := nativeViewFixture()
	m.Appearance, m.Panel = "light", false
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 720)
	tt.Frame()
	page := rect(t, tt, a.Tr("shellContent"))
	// The album covers run past the foot at this height, 24 in from the
	// page's left edge plus a cover's half.
	x, foot := int(page.X+24+80), int(page.Y+page.H)-1
	img := tt.Image()
	surface := tokenLight.surface
	near := func(a, b uint8) bool { return max(a, b)-min(a, b) < 8 }
	if c := img.RGBAAt(x, foot); !near(c.R, surface.R) || !near(c.G, surface.G) || !near(c.B, surface.B) {
		t.Fatalf("the page's foot is cut, not faded: %v at its last row", c)
	}
	if c := img.RGBAAt(x, foot-nativeFadeBelowSize-4); c.R > 200 && c.G > 200 && c.B > 200 {
		t.Fatalf("no cover above the fade to fade: %v", c)
	}
}

// Scrollbars in rounded panels stop clear of the corner arcs at both ends
// and keep MyGo's grab target aligned with the inset thumb.
func TestScrollbarClearsRoundedCorners(t *testing.T) {
	p := palette(true)
	for _, tc := range []struct {
		name    string
		corners nativeScrollbarCorners
	}{
		{"panel", nativeScrollbarCorners{top: tokenRadiusPanel, bottom: tokenRadiusPanel}},
		{"sheet", nativeScrollbarCorners{bottom: tokenRadiusSheet}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var st ui.ScrollState
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Background(p.surface).Children(func() {
					scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&st)
					nativeScrollbar(c, scroll, &st, p, tc.corners, nil)
					scroll.Children(func() {
						for i := range 60 {
							ui.Text(c, fmt.Sprint("Row ", i)).Height(24)
						}
					})
				})
			}, 240, 300)
			tt.SetScale(2)
			tt.Frame()
			insets := nativeScrollbarInsets(tc.corners)
			rect := ui.Rect{W: 240, H: 300}
			isLit := func(x, y float32) bool {
				r, _, _, _ := tt.Image().At(int(2*x), int(2*y)).RGBA()
				bg, _, _, _ := tt.Image().At(2*100, 2*150).RGBA()
				return r>>8 > bg>>8+8
			}

			// Hover the strip so the bar stays lit at full width.
			tt.Move(233, 150)
			tt.Frame()
			topBar := nativeScrollbarThumbs(rect, &st, false, insets, nativeScrollbarWidth)[0]
			wantTop := insets[0] + 2
			if topBar.Y < wantTop-0.1 {
				t.Fatalf("top thumb starts at %g, want >= %g", topBar.Y, wantTop)
			}
			if tc.corners.top > 0 && isLit(topBar.X+topBar.W/2, 4) {
				t.Fatalf("thumb painted inside top rounded corner (inset %g)", insets[0])
			}
			if !isLit(topBar.X+topBar.W/2, topBar.Y+topBar.H/2) {
				t.Fatal("inset top thumb did not paint at its expected position")
			}
			// Dragging the inset thumb scrolls: MyGo's ScrollbarInsets matches ours.
			before := st.Y
			tt.Press(topBar.X+topBar.W/2, topBar.Y+topBar.H/2)
			tt.Move(topBar.X+topBar.W/2, topBar.Y+topBar.H/2+60)
			tt.Release(topBar.X+topBar.W/2, topBar.Y+topBar.H/2+60)
			tt.Frame()
			if st.Y <= before {
				t.Fatalf("dragging inset thumb did not scroll (%g -> %g)", before, st.Y)
			}

			// Scroll all the way to the foot and verify bottom corner clearance.
			st.Y = st.MaxY
			tt.Move(233, 150)
			tt.Frame()
			botBar := nativeScrollbarThumbs(rect, &st, false, insets, nativeScrollbarWidth)[0]
			wantBot := rect.H - insets[2] - 2
			if botBar.Y+botBar.H > wantBot+0.1 {
				t.Fatalf("bottom thumb ends at %g, want <= %g", botBar.Y+botBar.H, wantBot)
			}
			if tc.corners.bottom > 0 && isLit(botBar.X+botBar.W/2, rect.H-4) {
				t.Fatalf("thumb painted inside bottom rounded corner (inset %g)", insets[2])
			}
		})
	}
}

// A scrollbar rests slim (6 px, 4 px from the edge) while scrolling and
// widens inward to 8 px when the pointer enters its grab strip.
func TestScrollbarSlimAtRestAndExpandsOnHover(t *testing.T) {
	p := palette(true)
	var st ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(p.surface).Children(func() {
			scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&st)
			nativeScrollbar(c, scroll, &st, p, nativeScrollbarCorners{}, nil)
			scroll.Children(func() {
				for i := range 60 {
					ui.Text(c, fmt.Sprint("Row ", i)).Height(24)
				}
			})
		})
	}, 200, 300)
	tt.SetScale(2)
	settle := func(d time.Duration) {
		for elapsed := time.Duration(0); elapsed < d; elapsed += 25 * time.Millisecond {
			tt.Advance(25 * time.Millisecond)
			tt.Frame()
		}
	}
	tt.Frame()
	insets := nativeScrollbarInsets(nativeScrollbarCorners{})
	bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 300}, &st, false, insets, nativeScrollbarWidth)[0]
	midY := bar.Y + bar.H/2
	lit := func(x float32) bool {
		r, _, _, _ := tt.Image().At(int(2*x), int(2*midY)).RGBA()
		bg, _, _, _ := tt.Image().At(2*80, int(2*midY)).RGBA()
		return r>>8 > bg>>8+8
	}

	// Pointer over content shows the slim 6 px thumb anchored 4 px from the right edge (x in [190, 196]).
	tt.Move(60, 150)
	tt.Frame()
	if !lit(193) {
		t.Fatal("slim resting thumb is not painted at x=193")
	}
	if lit(189) {
		t.Fatal("resting thumb expanded inward past 6 px (lit at x=189)")
	}
	if lit(198) {
		t.Fatal("resting thumb sits closer than 4 px to the right edge (lit at x=198)")
	}

	// Moving onto the strip expands it inward to 8 px (x in [188, 196]).
	tt.Move(193, 150)
	settle(tokenMotionFast + 50*time.Millisecond)
	if !lit(189) {
		t.Fatal("hovered thumb did not expand inward to 8 px (unlit at x=189)")
	}
	if lit(198) {
		t.Fatal("expanded thumb moved closer than 4 px to the right edge (lit at x=198)")
	}

	// Moving back to content contracts it smoothly back to 6 px while still visible.
	tt.Move(60, 150)
	settle(tokenMotionFast + 50*time.Millisecond)
	if !lit(193) {
		t.Fatal("thumb disappeared before idle timeout after leaving strip")
	}
	if lit(189) {
		t.Fatal("thumb did not contract back to 6 px after leaving strip (still lit at x=189)")
	}
}

// On the desktop layout, the main page panel's scrollbar clears its 16 px
// rounded top corner instead of overlapping the arc.
func TestDesktopPageScrollbarClearsRoundedCorners(t *testing.T) {
	m, a := nativeViewFixture()
	m.Appearance, m.Panel = "dark", false
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 720)
	tt.SetScale(2)
	tt.Frame()
	page := rect(t, tt, a.Tr("shellContent"))
	// Hover the grab strip inside the inset track so the thumb renders at full width.
	tt.Move(page.X+page.W-8, page.Y+page.H/2)
	tt.Frame()
	img := tt.Image()
	isThumb := func(x, y float32) bool {
		c := img.RGBAAt(int(2*x), int(2*y))
		bg := img.RGBAAt(int(2*(page.X+page.W-30)), int(2*(page.Y+20)))
		return int(c.R)-int(bg.R) > 12 || int(c.G)-int(bg.G) > 12 || int(c.B)-int(bg.B) > 12
	}
	// Right in the 16 px top-right corner (y = 4), no thumb pixel should appear.
	if isThumb(page.X+page.W-8, page.Y+4) {
		t.Fatal("desktop page scrollbar painted inside its rounded top-right corner")
	}
	// Inside the inset track (below inset + 2 = 14), the thumb is visible.
	if !isThumb(page.X+page.W-8, page.Y+20) {
		t.Fatal("desktop page scrollbar thumb missing below the rounded top corner")
	}
}

// End-to-end verification across every rounded-corner scrolling surface in
// the real desktop view (main page top/bottom, library bottom, settings sheet
// bottom in both dark and light appearances), plus compact full-bleed and RTL.
func TestScrollbarRoundedCornersE2E(t *testing.T) {
	shotDir := os.Getenv("YTMGO_SCROLLBAR_E2E_DIR")
	saveCrop := func(name string, img *image.RGBA, x, y, w, h, zoom int) {
		if shotDir == "" {
			return
		}
		if err := os.MkdirAll(shotDir, 0o755); err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		x0 := max(b.Min.X, x)
		y0 := max(b.Min.Y, y)
		x1 := min(b.Max.X, x+w)
		y1 := min(b.Max.Y, y+h)
		out := image.NewRGBA(image.Rect(0, 0, (x1-x0)*zoom, (y1-y0)*zoom))
		for py := y0; py < y1; py++ {
			for px := x0; px < x1; px++ {
				c := img.RGBAAt(px, py)
				for dy := range zoom {
					for dx := range zoom {
						out.SetRGBA((px-x0)*zoom+dx, (py-y0)*zoom+dy, c)
					}
				}
			}
		}
		f, err := os.Create(filepath.Join(shotDir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, out); err != nil {
			t.Fatal(err)
		}
	}
	differs := func(a, b color.RGBA) bool {
		abs := func(x, y uint8) int {
			if x > y {
				return int(x - y)
			}
			return int(y - x)
		}
		return abs(a.R, b.R) > 12 || abs(a.G, b.G) > 12 || abs(a.B, b.B) > 12
	}

	// 1. Desktop main page (top & bottom, drag, corner-zone non-activation) + library bottom + settings sheet bottom.
	for _, appearance := range []string{"dark", "light"} {
		t.Run("desktop-"+appearance, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Appearance, m.Panel = appearance, false
			m.SignedIn, m.LibraryLoaded = true, true
			m.instantMotion = true
			// Populate enough library items so library-scroll overflows noticeably.
			for i := range 28 {
				m.LibraryItems = append(m.LibraryItems, catalog.Item{
					ID: fmt.Sprintf("lib-e2e-%d", i), BrowseID: fmt.Sprintf("VL%d", i),
					Title: fmt.Sprintf("Playlist %02d", i), Artist: "Playlist", Kind: "playlist",
				})
			}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 720)
			tt.SetScale(2)
			tt.Frame()

			// Main page: top corner clearance, drag, and bottom corner clearance.
			page := rect(t, tt, a.Tr("shellContent"))
			tt.Move(page.X+page.W-8, page.Y+page.H/2)
			tt.Frame()
			img := tt.Image()
			bgTop := img.RGBAAt(int(2*(page.X+page.W-30)), int(2*(page.Y+20)))
			if differs(img.RGBAAt(int(2*(page.X+page.W-8)), int(2*(page.Y+6))), bgTop) {
				t.Fatalf("%s page top scrollbar overlapped top-right 16px corner", appearance)
			}
			if !differs(img.RGBAAt(int(2*(page.X+page.W-8)), int(2*(page.Y+22))), bgTop) {
				t.Fatalf("%s page top scrollbar missing below top-right corner inset", appearance)
			}
			saveCrop("page-top-"+appearance, img, int(2*(page.X+page.W-48)), int(2*(page.Y-8)), 112, 144, 3)

			// Dragging the inset page thumb scrolls the page.
			beforePageY := m.pageScroll.Y
			tt.Press(page.X+page.W-8, page.Y+28)
			tt.Move(page.X+page.W-8, page.Y+180)
			tt.Release(page.X+page.W-8, page.Y+180)
			tt.Frame()
			if m.pageScroll.Y <= beforePageY {
				t.Fatalf("%s dragging page scrollbar thumb did not scroll (%g -> %g)", appearance, beforePageY, m.pageScroll.Y)
			}

			// Scroll page to the very bottom and check bottom-right 16px corner clearance.
			m.pageScroll.Y = m.pageScroll.MaxY
			tt.Move(page.X+page.W-8, page.Y+page.H/2)
			tt.Frame()
			img = tt.Image()
			bgBot := img.RGBAAt(int(2*(page.X+page.W-30)), int(2*(page.Y+page.H-20)))
			if differs(img.RGBAAt(int(2*(page.X+page.W-8)), int(2*(page.Y+page.H-6))), bgBot) {
				t.Fatalf("%s page bottom scrollbar overlapped bottom-right 16px corner", appearance)
			}
			if !differs(img.RGBAAt(int(2*(page.X+page.W-8)), int(2*(page.Y+page.H-22))), bgBot) {
				t.Fatalf("%s page bottom scrollbar missing above bottom-right corner inset", appearance)
			}
			saveCrop("page-bottom-"+appearance, img, int(2*(page.X+page.W-48)), int(2*(page.Y+page.H-64)), 112, 144, 3)

			// Library panel: scroll to bottom and check bottom-right 16px corner clearance.
			lib := rect(t, tt, a.Tr("shellLibraryRegion"))
			tt.Scroll(lib.X+lib.W/2, lib.Y+lib.H/2, 0, 4000)
			tt.Move(lib.X+lib.W-8, lib.Y+lib.H/2)
			tt.Frame()
			img = tt.Image()
			libBg := img.RGBAAt(int(2*(lib.X+lib.W-26)), int(2*(lib.Y+lib.H-22)))
			if differs(img.RGBAAt(int(2*(lib.X+lib.W-8)), int(2*(lib.Y+lib.H-6))), libBg) {
				t.Fatalf("%s library bottom scrollbar overlapped bottom-right 16px corner", appearance)
			}
			if !differs(img.RGBAAt(int(2*(lib.X+lib.W-8)), int(2*(lib.Y+lib.H-22))), libBg) {
				t.Fatalf("%s library bottom scrollbar missing above bottom-right corner inset", appearance)
			}
			saveCrop("library-bottom-"+appearance, img, int(2*(lib.X+lib.W-48)), int(2*(lib.Y+lib.H-64)), 112, 144, 3)

			// Settings sheet: open, verify overflow, drag, and check bottom-right 24px corner + border clearance.
			m.Settings = true
			tt.Frame()
			if m.settingsScroll.MaxY <= 0 {
				t.Fatalf("%s settings sheet did not overflow at 1280x720 (MaxY=%g)", appearance, m.settingsScroll.MaxY)
			}
			// Centered desktop sheet: width = nativeSettingsWidth (640), height = min(720-48, 780) = 672,
			// with 1 px border so inner scroller right/bottom edge sits 1 px inside the panel frame.
			sheetW, sheetH := float32(nativeSettingsWidth), min(float32(720-48), float32(780))
			sheet := ui.Rect{X: (1280 - sheetW) / 2, Y: (720 - sheetH) / 2, W: sheetW, H: sheetH}
			m.settingsScroll.Y = m.settingsScroll.MaxY
			tt.Move(sheet.X+sheet.W-9, sheet.Y+sheet.H/2)
			tt.Frame()
			img = tt.Image()
			sheetBg := img.RGBAAt(int(2*(sheet.X+sheet.W-30)), int(2*(sheet.Y+sheet.H-30)))
			// Inside the 24 px corner inset (bottom inset = 20 -> thumb stops at H - 23 inside 1 px border),
			// y = H - 10 must have no thumb pixel.
			if differs(img.RGBAAt(int(2*(sheet.X+sheet.W-9)), int(2*(sheet.Y+sheet.H-10))), sheetBg) {
				t.Fatalf("%s settings sheet bottom scrollbar overlapped bottom-right 24px corner", appearance)
			}
			if !differs(img.RGBAAt(int(2*(sheet.X+sheet.W-9)), int(2*(sheet.Y+sheet.H-34))), sheetBg) {
				t.Fatalf("%s settings sheet bottom scrollbar missing above bottom-right 24px corner inset", appearance)
			}
			saveCrop("settings-bottom-"+appearance, img, int(2*(sheet.X+sheet.W-56)), int(2*(sheet.Y+sheet.H-72)), 128, 160, 3)

			// Dragging the settings sheet's inset thumb upward scrolls back up.
			beforeSheetY := m.settingsScroll.Y
			tt.Press(sheet.X+sheet.W-9, sheet.Y+sheet.H-40)
			tt.Move(sheet.X+sheet.W-9, sheet.Y+sheet.H-180)
			tt.Release(sheet.X+sheet.W-9, sheet.Y+sheet.H-180)
			tt.Frame()
			if m.settingsScroll.Y >= beforeSheetY {
				t.Fatalf("%s dragging settings sheet scrollbar thumb did not scroll (%g -> %g)", appearance, beforeSheetY, m.settingsScroll.Y)
			}
		})
	}

	// 2. Compact layout (full-bleed, no rounded corners): page bar runs all the way to top margin (2 px).
	t.Run("compact-no-corner-inset", func(t *testing.T) {
		m, a := nativeViewFixture()
		m.Appearance, m.Panel = "dark", false
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 420, 720)
		tt.SetScale(2)
		tt.Frame()
		page := rect(t, tt, a.Tr("shellContent"))
		tt.Move(page.X+page.W-8, page.Y+page.H/2)
		tt.Frame()
		img := tt.Image()
		bg := img.RGBAAt(int(2*(page.X+page.W-30)), int(2*(page.Y+8)))
		if !differs(img.RGBAAt(int(2*(page.X+page.W-8)), int(2*(page.Y+6))), bg) {
			t.Fatal("compact full-bleed page unexpectedly applied rounded-corner top inset")
		}
	})

	// 3. RTL layout: vertical bar anchors 4 px from the left edge, clears top/bottom corners,
	// widens inward (to the right), and drags cleanly.
	t.Run("rtl-rounded-corners-and-drag", func(t *testing.T) {
		p := palette(true)
		var st ui.ScrollState
		corners := nativeScrollbarCorners{top: tokenRadiusPanel, bottom: tokenRadiusPanel}
		tt := ui.NewTester(func(c *ui.Context) {
			c.Root().Direction(ui.RightToLeft)
			ui.Column(c).Fill().Background(p.surface).Children(func() {
				scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&st)
				nativeScrollbar(c, scroll, &st, p, corners, nil)
				scroll.Children(func() {
					for i := range 60 {
						ui.Text(c, fmt.Sprint("Row ", i)).Height(24)
					}
				})
			})
		}, 240, 300)
		tt.SetScale(2)
		tt.Frame()
		insets := nativeScrollbarInsets(corners)
		// Resting slim bar on left edge: x in [4, 10].
		tt.Move(120, 150)
		tt.Frame()
		img := tt.Image()
		bg := img.RGBAAt(2*120, 2*150)
		bar := nativeScrollbarThumbs(ui.Rect{W: 240, H: 300}, &st, true, insets, nativeScrollbarRestWidth)[0]
		midY := bar.Y + bar.H/2
		if !differs(img.RGBAAt(2*7, int(2*midY)), bg) || differs(img.RGBAAt(2*11, int(2*midY)), bg) || differs(img.RGBAAt(2*2, int(2*midY)), bg) {
			t.Fatal("RTL resting slim thumb not anchored at x in [4, 10]")
		}
		if differs(img.RGBAAt(2*7, 2*4), bg) {
			t.Fatal("RTL thumb painted inside top-left rounded corner")
		}
		// Hover left grab strip -> widens inward to x in [4, 12], and drag scrolls.
		tt.Move(7, midY)
		for range 8 {
			tt.Advance(25 * time.Millisecond)
			tt.Frame()
		}
		img = tt.Image()
		if !differs(img.RGBAAt(2*11, int(2*midY)), bg) {
			t.Fatal("RTL hovered thumb did not expand inward to the right (x=11)")
		}
		before := st.Y
		tt.Press(8, midY)
		tt.Move(8, midY+60)
		tt.Release(8, midY+60)
		tt.Frame()
		if st.Y <= before {
			t.Fatalf("RTL dragging inset thumb did not scroll (%g -> %g)", before, st.Y)
		}
	})
}
