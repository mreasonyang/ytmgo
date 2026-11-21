package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

const artworkHistoryFixtureEnv = "YTMGO_TEST_ARTWORK_HISTORY"
const artworkHistoryPressureCount = 240
const artworkHistoryBodyBytes = 384 << 10

// Catalog completions use MyGo's main-thread dispatcher. Run it in a
// disposable AppKit process and let Tester render entirely in memory.
func runArtworkHistoryFixture(m *testing.M, directory string) int {
	log.SetOutput(io.Discard)
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyProhibited)
	mygo.App.SetName("ytmGO Artwork History Test")
	mygo.App.SetLocale("en")
	for _, kind := range []mygo.PathName{mygo.PathUserData, mygo.PathCache, mygo.PathLogs} {
		mygo.App.SetPath(kind, filepath.Join(directory, string(kind)))
	}
	code := 1
	mygo.App.WhenReady(func() {
		go func() {
			result := m.Run()
			mygo.RunOnMain(func() {
				code = result
				mygo.App.Quit()
			})
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Println("artwork history fixture could not start AppKit")
		return 1
	}
	return code
}

func TestDesktopArtworkHistoryAfterEviction(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the headless MyGo dispatcher fixture uses AppKit")
	}
	dir := os.Getenv(artworkHistoryFixtureEnv)
	if dir == "" {
		dir = t.TempDir()
		ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
		defer cancel()
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.CommandContext(ctx, executable, "-test.run=^TestDesktopArtworkHistoryAfterEviction$", "-test.v")
		child.Env = []string{artworkHistoryFixtureEnv + "=" + dir, "HOME=" + dir, "CFFIXED_USER_HOME=" + dir, "TMPDIR=" + dir, "LANG=en_US.UTF-8"}
		var report bytes.Buffer
		child.Stdout = &report
		err = child.Run()
		// Report only the fixture assertions, not native framework diagnostics.
		for line := range strings.SplitSeq(report.String(), "\n") {
			if strings.Contains(line, "artwork_history_integration_test.go:") {
				t.Log(strings.TrimSpace(line))
			}
		}
		if err != nil {
			t.Fatalf("isolated artwork history test: %v (deadline: %v)", err, ctx.Err())
		}
		return
	}
	testDesktopArtworkHistory(t, dir)
}

type artworkHistoryObject = map[string]any

func artworkHistoryItem(group string, index int) (title, raw, browseID string) {
	title = fmt.Sprintf("%s %03d", group, index)
	raw = fmt.Sprintf("https://lh3.googleusercontent.com/history-%s-%03d=s80", group, index)
	browseID = fmt.Sprintf("VLhistory%s%03d", group, index)
	return
}

func artworkHistoryColor(group string, index int) color.RGBA {
	if group == "Home" {
		return color.RGBA{R: uint8(40 + 3*index), G: uint8(244 - 3*index), B: 213, A: 255}
	}
	return color.RGBA{R: uint8(index + 8), G: 148, B: 51, A: 255}
}

func artworkHistoryDetail(ink color.RGBA) color.RGBA {
	ink.B = 65
	return ink
}

func artworkHistoryHomePNG(t *testing.T, ink color.RGBA) []byte {
	t.Helper()
	picture := tintFill(384, 384, func(x, _ int) color.RGBA {
		// Flat regions survive the preview. Four-pixel stripes between
		// them give the full-resolution raster a separate signature.
		if x%64 >= 32 && x%8 < 4 {
			return artworkHistoryDetail(ink)
		}
		return ink
	})
	var data bytes.Buffer
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func artworkHistoryCatalog(t *testing.T, group string, rows, columns int) []byte {
	t.Helper()
	label := func(text string) artworkHistoryObject { return artworkHistoryObject{"simpleText": text} }
	var shelves []artworkHistoryObject
	for row := range rows {
		var cards []artworkHistoryObject
		for col := range columns {
			title, raw, browseID := artworkHistoryItem(group, row*columns+col)
			cards = append(cards, artworkHistoryObject{"musicTwoRowItemRenderer": artworkHistoryObject{
				"title": label(title),
				"navigationEndpoint": artworkHistoryObject{"browseEndpoint": artworkHistoryObject{
					"browseId": browseID, "browseEndpointContextSupportedConfigs": artworkHistoryObject{
						"browseEndpointContextMusicConfig": artworkHistoryObject{"pageType": "MUSIC_PAGE_TYPE_PLAYLIST"},
					},
				}},
				"thumbnailRenderer": artworkHistoryObject{"musicThumbnailRenderer": artworkHistoryObject{
					"thumbnail": artworkHistoryObject{"thumbnails": []artworkHistoryObject{{"url": raw, "width": 384, "height": 384}}},
				}},
			}})
		}
		shelves = append(shelves, artworkHistoryObject{"musicCarouselShelfRenderer": artworkHistoryObject{
			"title": label(fmt.Sprintf("%s shelf %02d", group, row)), "contents": cards,
		}})
	}
	body, err := json.Marshal(artworkHistoryObject{"contents": artworkHistoryObject{"sectionListRenderer": artworkHistoryObject{"contents": shelves}}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func testDesktopArtworkHistory(t *testing.T, directory string) {
	const homeRows, homeColumns, pressureColumns = 5, 10, 4
	const anchorIndex = homeColumns + 3
	anchor, _, destination := artworkHistoryItem("Home", anchorIndex)
	home := artworkHistoryCatalog(t, "Home", homeRows, homeColumns)
	browse := artworkHistoryCatalog(t, "Browse", artworkHistoryPressureCount/pressureColumns, pressureColumns)
	var catalogRequests, artworkRequests atomic.Int32
	client := catalog.NewWithOptions(catalog.Options{HTTPClient: &http.Client{Transport: artworkTransport(func(r *http.Request) (*http.Response, error) {
		var payload struct{ BrowseID string }
		if r.Method != http.MethodPost || r.URL.Path != "/youtubei/v1/browse" || json.NewDecoder(r.Body).Decode(&payload) != nil {
			return nil, errors.New("unexpected fixture catalog request")
		}
		catalogRequests.Add(1)
		switch payload.BrowseID {
		case "FEmusic_home":
			return artworkHTTPResponse(home, nil), nil
		case destination:
			return artworkHTTPResponse(browse, nil), nil
		default:
			return nil, errors.New("unknown fixture browse ID")
		}
	})}})
	ctx, cancel := context.WithCancel(t.Context())
	cache := newArtworkCache(ctx, nil)
	t.Cleanup(func() { cancel(); cache.workers.Wait() })
	cache.disk = &artworkDisk{dir: filepath.Join(directory, "http-artwork")}
	pictures := make(map[string][]byte)
	padding := make([]byte, artworkHistoryBodyBytes)
	homeColors := make(map[color.RGBA]int)
	detailColors := make(map[color.RGBA]int)
	pressureColors := make(map[color.RGBA]int)
	for _, group := range []struct {
		name   string
		count  int
		colors map[color.RGBA]int
	}{{"Home", homeRows * homeColumns, homeColors}, {"Browse", artworkHistoryPressureCount, pressureColors}} {
		for i := range group.count {
			_, raw, _ := artworkHistoryItem(group.name, i)
			ink := artworkHistoryColor(group.name, i)
			var body []byte
			if group.name == "Home" {
				body = artworkHistoryHomePNG(t, ink)
				detailColors[artworkHistoryDetail(ink)] = i
				// The collection's provisional header may also use the
				// clicked card at a different size during navigation.
				pictures[artworkRequestAt(raw, 128)] = body
				pictures[artworkRequestAt(raw, 768)] = body
			} else {
				body = artworkPreviewFixture(t, 384, 0, ink)
			}
			pictures[artworkRequestAt(raw, 384)] = body
			group.colors[ink] = i
		}
	}
	var requestsMu sync.Mutex
	requested := make(map[string]int)
	cache.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		body := pictures[r.URL.String()]
		if len(body) == 0 || len(body) > artworkHistoryBodyBytes {
			t.Error("unexpected fixture artwork URL, size bucket or encoded weight")
			return nil, errors.New("unknown fixture artwork")
		}
		artworkRequests.Add(1)
		requestsMu.Lock()
		requested[r.URL.String()]++
		requestsMu.Unlock()
		response := artworkHTTPResponse(body, http.Header{"Cache-Control": {"public, max-age=3600"}})
		response.ContentLength = artworkHistoryBodyBytes
		response.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), bytes.NewReader(padding[:artworkHistoryBodyBytes-len(body)])))
		return response, nil
	})
	a := &desktop{ctx: ctx, catalog: client, art: cache, artworkScale: 2, historyIndex: -1, windowVisible: true,
		model: UIModel{Appearance: "dark", Language: "en", instantMotion: true}}
	var tt *ui.Tester
	var paintedReturn *ui.ScrollState
	var watchingReturn bool
	var first artworkHistoryFrame
	var samples, details map[int]image.Point
	mygo.RunOnMain(func() {
		tt = ui.NewTesterWith(func(c *ui.Context) {
			// Tester settles multiple builds per click. Read the previous
			// presentation before it gets overwritten by the next build.
			if paintedReturn != nil && !first.observed {
				first = captureArtworkHistoryFrame(tt, paintedReturn, anchor, samples, details)
			}
			a.view(c)
			if watchingReturn && a.model.Route == "home" && !first.observed {
				scroll := a.model.pageScroll
				// MyGo can rebuild several times within a single frame
				// after a click. Mark only the pass it actually paints.
				c.Root().DrawOver(func(*ui.Painter, ui.Rect) { paintedReturn = scroll })
			}
		}, 1280, 900, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}, Dark: true})
		a.loadPage("home", "", true)
	})
	waitForPage := func(route string, want int) {
		t.Helper()
		var ready bool
		var itemCount int
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
			mygo.RunOnMain(func() {
				itemCount = len(a.model.Page.Items)
				ready = a.model.Route == route && !a.model.Loading && itemCount == want
			})
			if ready {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Fatalf("fixture page %s never completed with %d items (got %d)", route, want, itemCount)
	}
	settle := func() {
		mygo.RunOnMain(tt.Frame)
		cache.workers.Wait()
		mygo.RunOnMain(tt.Frame)
	}
	page := func() ui.Rect {
		r, _ := tt.Find(a.tr("shellContent"))
		return r
	}
	scroll := func(dy float32) {
		mygo.RunOnMain(func() {
			r := page()
			tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, dy)
		})
	}
	waitForPage("home", homeRows*homeColumns)
	settle()
	scroll(350)
	settle()
	var beforeX, afterX ui.Rect
	mygo.RunOnMain(func() {
		beforeX, _ = tt.Find(anchor)
		tt.Scroll(beforeX.X+55, beforeX.Y+70, 288, 0)
	})
	settle()
	var left artworkHistoryFrame
	mygo.RunOnMain(func() {
		afterX, _ = tt.Find(anchor)
		img, viewport := tt.Image(), page()
		samples = artworkHistoryColors(img, viewport, homeColors, 4)
		details = artworkHistoryColors(img, viewport, detailColors, 1)
		left = captureArtworkHistoryFrame(tt, a.model.pageScroll, anchor, samples, details)
	})
	if left.y < 100 || beforeX.X-afterX.X < 200 || len(samples) < 6 || len(left.wrong) != 0 {
		t.Fatalf("fixture did not scroll both axes and display home covers: y=%g anchor=%+v -> %+v samples=%d wrong=%v",
			left.y, beforeX, afterX, len(samples), left.wrong)
	}
	for id := range samples {
		if _, ok := details[id]; !ok {
			t.Fatalf("home cover %d has no full-resolution raster sample", id)
		}
	}
	if len(left.blurred) != 0 {
		t.Fatalf("initial home lost full-resolution detail on covers %v", left.blurred)
	}
	homeKey := func(i int) artRequest {
		_, raw, _ := artworkHistoryItem("Home", i)
		return artRequest{url: artworkRequestAt(raw, 384), edge: 384}
	}
	homeRenditions := func() (resident, full int) {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		for i := range samples {
			e := cache.entry(homeKey(i))
			if e != nil {
				resident++
			}
			if e != nil && e.bitmap != nil {
				if w, h := e.bitmap.Size(); w == 384 && h == 384 {
					full++
				}
			}
		}
		return resident, full
	}
	if _, got := homeRenditions(); got != len(samples) {
		t.Fatalf("initial actual home covers at full resolution: %d/%d", got, len(samples))
	}

	var clickErr error
	mygo.RunOnMain(func() { clickErr = tt.Click(anchor) })
	if clickErr != nil {
		t.Fatal(clickErr)
	}
	waitForPage("browse:"+destination, artworkHistoryPressureCount)
	painted := make(map[int]bool)
	for range 90 {
		settle()
		var atEnd bool
		mygo.RunOnMain(func() {
			r := page()
			for i := range artworkHistoryColors(tt.Image(), r, pressureColors, 4) {
				painted[i] = true
			}
			s := a.model.pageScroll
			atEnd = s.Y >= s.MaxY
		})
		if atEnd {
			break
		}
		var viewport ui.Rect
		mygo.RunOnMain(func() { viewport = page() })
		scroll(viewport.H * 0.6)
	}
	if len(painted) != artworkHistoryPressureCount {
		t.Fatalf("browse pressure painted %d/%d covers", len(painted), artworkHistoryPressureCount)
	}
	requestsMu.Lock()
	pressureLoads := 0
	for i := range artworkHistoryPressureCount {
		_, raw, _ := artworkHistoryItem("Browse", i)
		pressureLoads += requested[artworkRequestAt(raw, 384)]
	}
	requestsMu.Unlock()
	if pressureLoads != artworkHistoryPressureCount {
		t.Fatalf("browse pressure made %d upstream loads, want %d", pressureLoads, artworkHistoryPressureCount)
	}
	if got, _ := homeRenditions(); got != 0 {
		t.Fatalf("browse pressure left %d/%d original 384px renditions in memory", got, len(samples))
	}
	t.Logf("home scrolled to y=%g x=%g, %d painted 384px covers; %d unique 384 KiB browse responses painted, all sampled home renditions evicted",
		left.y, beforeX.X-afterX.X, len(samples), len(painted))

	// Occupy the same worker decode slots that a fresh disk hit uses.
	for range artworkDecoders {
		if !cache.takeDecoder(ctx, nil, time.Time{}) {
			t.Fatal("could not occupy a recovery decoder")
		}
	}
	release := sync.OnceFunc(func() {
		for range artworkDecoders {
			cache.releaseDecoder()
		}
	})
	defer release()
	beforeArt, beforeCatalog := artworkRequests.Load(), catalogRequests.Load()
	var blocked artworkHistoryFrame
	mygo.RunOnMain(func() {
		watchingReturn = true
		clickErr = tt.Click(a.tr("shellBack"))
		tt.Frame()
		blocked = captureArtworkHistoryFrame(tt, a.model.pageScroll, anchor, samples, details)
	})
	if clickErr != nil {
		t.Fatal(clickErr)
	}
	waitForPage("home", homeRows*homeColumns)
	queued := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		cache.mu.Lock()
		queued = len(cache.decodeQueue) > 0
		cache.mu.Unlock()
		if queued {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !queued {
		t.Error("no Back recovery reached the held decoders")
	}
	check := func(stage string, got artworkHistoryFrame) {
		t.Helper()
		if !got.observed || got.y != left.y || got.anchor != left.anchor || len(got.wrong) != 0 {
			t.Errorf("%s: observed=%v y=%g want %g; anchor=%+v want %+v; wrong/missing cover IDs=%v",
				stage, got.observed, got.y, left.y, got.anchor, left.anchor, got.wrong)
		}
	}
	check("first committed Back frame with recovery blocked", first)
	check("settled Back frame with recovery blocked", blocked)
	if _, got := homeRenditions(); got != 0 {
		t.Errorf("%d renditions recovered while the decoders were held", got)
	}
	if len(blocked.blurred) == 0 {
		t.Error("blocked Back already matched all the full-resolution raster samples")
	}
	t.Logf("blocked Back: first-frame color mismatches=%d, settled color mismatches=%d, fine detail missing on %d covers",
		len(first.wrong), len(blocked.wrong), len(blocked.blurred))
	release()
	settle()
	var restored artworkHistoryFrame
	mygo.RunOnMain(func() {
		restored = captureArtworkHistoryFrame(tt, a.model.pageScroll, anchor, samples, details)
	})
	check("Back frame after recovery", restored)
	if len(restored.blurred) != 0 {
		t.Errorf("Back frame still lacks full-resolution detail on covers %v", restored.blurred)
	}
	_, full := homeRenditions()
	upstreamArt, upstreamCatalog := artworkRequests.Load()-beforeArt, catalogRequests.Load()-beforeCatalog
	if full != len(samples) || upstreamArt != 0 || upstreamCatalog != 0 {
		t.Errorf("after Back: 384px=%d/%d upstream artwork=%d catalog=%d, want all 384px and zero new upstream requests",
			full, len(samples), upstreamArt, upstreamCatalog)
	} else {
		t.Logf("after Back: %d/%d covers restored to 384px; upstream artwork=%d catalog=%d", full, len(samples), upstreamArt, upstreamCatalog)
	}
}

// Sample the actual first-pass raster. The fixture's flat colours uniquely
// identify each cover, while the viewport crop excludes other UI surfaces.
func artworkHistoryColors(im *image.RGBA, r ui.Rect, colors map[color.RGBA]int, step int) map[int]image.Point {
	area := image.Rect(int(r.X)+4, int(r.Y)+4, int(r.X+r.W)-4, int(r.Y+r.H)-24).Intersect(im.Bounds())
	points := make(map[int]image.Point)
	for y := area.Min.Y; y+step < area.Max.Y; y += step {
		for x := area.Min.X; x+step < area.Max.X; x += step {
			ink := im.RGBAAt(x, y)
			id, ok := colors[ink]
			if _, found := points[id]; !ok || found {
				continue
			}
			if im.RGBAAt(x, y+step) == ink && im.RGBAAt(x+step/2, y+step/2) == ink {
				points[id] = image.Pt(x+step/2, y+step/2)
			}
		}
	}
	return points
}

type artworkHistoryFrame struct {
	observed bool
	y        float32
	anchor   ui.Rect
	wrong    []int
	blurred  []int
}

func captureArtworkHistoryFrame(tt *ui.Tester, scroll *ui.ScrollState, anchor string, samples, details map[int]image.Point) artworkHistoryFrame {
	if scroll == nil {
		return artworkHistoryFrame{}
	}
	f := artworkHistoryFrame{observed: true, y: scroll.Y}
	f.anchor, _ = tt.Find(anchor)
	im := tt.Image()
	for id, p := range samples {
		ink := artworkHistoryColor("Home", id)
		if im.RGBAAt(p.X, p.Y) != ink {
			f.wrong = append(f.wrong, id)
		}
		if d, ok := details[id]; ok && im.RGBAAt(d.X, d.Y) != artworkHistoryDetail(ink) {
			f.blurred = append(f.blurred, id)
		}
	}
	slices.Sort(f.wrong)
	slices.Sort(f.blurred)
	return f
}
