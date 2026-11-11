package main

// This opt-in harness opens the production native view in a real MyGo window.
// It never starts the application controller, account store, playback or HTTP
// network transport. Its only inputs and outputs are in the supplied directory.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestMain(m *testing.M) {
	directory := os.Getenv("YTMGO_ARTWORK_DESKTOP_ACCEPTANCE_DIR")
	// A copied QA bundle can be reopened by native automation after quitting.
	// Keep that restart bound to the same isolated directory without relying
	// on launch-time environment inheritance. Ordinary test binaries have no marker.
	if directory == "" {
		if executable, err := os.Executable(); err == nil {
			marker := filepath.Join(filepath.Dir(executable), "..", "Resources", "artwork-acceptance-directory")
			if data, err := os.ReadFile(marker); err == nil {
				directory = strings.TrimSpace(string(data))
			}
		}
	}
	if directory != "" {
		os.Exit(runArtworkDesktopAcceptance(directory))
	}
	os.Exit(m.Run())
}

type artworkDesktopStats struct {
	mu                             sync.Mutex
	Frames, GPUFrames, OtherLogs   int
	WarmRequests, WarmPlaceholders int
	BitmapChanges, HDWarmRequests  int
	Durations                      []float64
	Bodies                         atomic.Int64
	observed                       map[artRequest]uintptr
}

var artworkFrameLogPattern = regexp.MustCompile(`^mygo: frame [0-9]+ took ([0-9.]+) ms:`)

// Only numeric frame measurements leave the logger; raw diagnostic strings
// are neither written to a file nor returned by the harness.
func (s *artworkDesktopStats) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if parts := artworkFrameLogPattern.FindSubmatch(p); len(parts) == 2 {
		ms, _ := strconv.ParseFloat(string(parts[1]), 64)
		s.Durations = append(s.Durations, ms)
		s.Frames++
		if bytes.Contains(p, []byte("drawn on the GPU")) {
			s.GPUFrames++
		}
	} else {
		s.OtherLogs++
	}
	return len(p), nil
}

func artworkDesktopPNG(edge int) []byte {
	im := image.NewRGBA(image.Rect(0, 0, edge, edge))
	for y := range edge {
		for x := range edge {
			// Fine detail distinguishes the requested rendition from a preview.
			im.SetRGBA(x, y, color.RGBA{uint8(45 + (x/7+y/7)%2*170), uint8(40 + x*170/edge), uint8(50 + y*170/edge), 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return b.Bytes()
}

func artworkDesktopPages() (catalog.Page, catalog.Page, []catalog.Item) {
	item := func(i int, kind string) catalog.Item {
		return catalog.Item{ID: fmt.Sprintf("fixture-%03d", i), BrowseID: fmt.Sprintf("fixture-%03d", i), Title: fmt.Sprintf("Artwork fixture %03d", i), Artist: "Isolated acceptance", Kind: kind, Artwork: fmt.Sprintf("https://lh3.googleusercontent.com/artwork-fixture-%03d=s80", i), Duration: 3 * time.Minute}
	}
	home := catalog.Page{Title: "Artwork cache acceptance"}
	for i := range 12 {
		shelf := catalog.Shelf{Title: fmt.Sprintf("Scroll shelf %02d", i+1)}
		for j := range 5 {
			shelf.Items = append(shelf.Items, item(i*5+j, "album"))
		}
		home.Shelves = append(home.Shelves, shelf)
	}
	list := catalog.Page{Title: "Long list acceptance"}
	for i := range 240 {
		list.Items = append(list.Items, item(100+i, "song"))
	}
	return home, list, []catalog.Item{item(1, "song"), item(2, "song"), item(3, "song")}
}

func runArtworkDesktopAcceptance(directory string) int {
	if !filepath.IsAbs(directory) || os.MkdirAll(directory, 0700) != nil {
		return 2
	}
	stats := &artworkDesktopStats{observed: make(map[artRequest]uintptr)}
	log.SetFlags(0)
	log.SetOutput(stats)
	mygo.App.SetName("ytmGO Artwork QA")
	for _, kind := range []mygo.PathName{mygo.PathUserData, mygo.PathCache, mygo.PathLogs} {
		mygo.App.SetPath(kind, filepath.Join(directory, fmt.Sprintf("storage-%v", kind)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := newArtworkCache(ctx, nil)
	cache.disk = &artworkDisk{dir: filepath.Join(directory, "http-cache")}
	bodies := map[int][]byte{128: artworkDesktopPNG(128), 384: artworkDesktopPNG(384), 768: artworkDesktopPNG(768)}
	cache.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		edge := 768
		for _, candidate := range []int{128, 384} {
			if strings.HasSuffix(req.URL.Path, fmt.Sprintf("=w%d-h%d", candidate, candidate)) {
				edge = candidate
			}
		}
		stats.Bodies.Add(1)
		return &http.Response{StatusCode: 200, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{"Cache-Control": {"public, max-age=86400"}}, Body: io.NopCloser(bytes.NewReader(bodies[edge])), ContentLength: int64(len(bodies[edge]))}, nil
	})
	home, list, queue := artworkDesktopPages()
	model, actions := nativeViewFixture()
	model.Current, model.Queue, model.Page, model.LibraryItems = queue[0], queue, home, nil
	model.Playing, model.Panel, model.Immersive, model.SignedIn = false, false, false, false
	model.Lyrics, model.Route = "Isolated artwork acceptance. No playback or account.", "home"
	var window *mygo.Window
	var sequence, current int
	resetStats := func() {
		stats.mu.Lock()
		stats.Frames, stats.GPUFrames, stats.WarmRequests, stats.HDWarmRequests, stats.WarmPlaceholders, stats.BitmapChanges, stats.Durations = 0, 0, 0, 0, 0, 0, nil
		stats.mu.Unlock()
	}
	writeStatus := func() {
		stats.mu.Lock()
		durations := append([]float64(nil), stats.Durations...)
		sort.Float64s(durations)
		percentile := func(p float64) float64 {
			if len(durations) == 0 {
				return 0
			}
			return durations[min(len(durations)-1, int(float64(len(durations)-1)*p))]
		}
		status := map[string]any{"sequence": sequence, "frames": stats.Frames, "gpu_frames": stats.GPUFrames, "other_log_count": stats.OtherLogs, "warm_requests": stats.WarmRequests, "hd_warm_requests": stats.HDWarmRequests, "warm_placeholders": stats.WarmPlaceholders, "bitmap_changes": stats.BitmapChanges, "body_downloads": stats.Bodies.Load(), "frame_p50_ms": percentile(.5), "frame_p95_ms": percentile(.95), "frame_p99_ms": percentile(.99), "frame_max_ms": percentile(1), "route": model.Route, "panel": model.Panel, "immersive": model.Immersive}
		stats.mu.Unlock()
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		status["go_heap_alloc_bytes"], status["go_heap_inuse_bytes"], status["go_total_alloc_bytes"] = memory.HeapAlloc, memory.HeapInuse, memory.TotalAlloc
		status["display_scale"] = actions.artworkScale
		cache.mu.Lock()
		for name, pool := range map[string]*artworkPool{"thumbnail": &cache.small, "browse": &cache.browse, "player": &cache.player} {
			pixels := 0
			for _, entry := range pool.entries {
				pixels += entry.pixelBytes
			}
			status[name+"_entries"], status[name+"_bytes"], status[name+"_unreleased_pixels"] = len(pool.entries), pool.bytes, pixels
		}
		cache.mu.Unlock()
		if model.pageScroll != nil {
			status["scroll_y"], status["scroll_max_y"] = model.pageScroll.Y, model.pageScroll.MaxY
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		_ = os.WriteFile(filepath.Join(directory, "status.json"), append(data, '\n'), 0600)
	}
	mygo.App.WhenReady(func() {
		cache.redraw = func() {
			if window != nil {
				window.Invalidate()
			}
		}
		actions.ArtworkTint = cache.tint
		actions.artworkPeek = cache.peek
		actions.Navigate = func(route string) {
			model.Route = route
			if route == "home" {
				model.Page = home
			} else {
				model.Route, model.Page = "browse:fixture", list
			}
		}
		actions.Open = func(catalog.Item) { model.Route, model.Page = "browse:fixture", list }
		actions.Back = func() { model.Route, model.Page = "home", home }
		actions.Next = func() { current = (current + 1) % len(queue); model.Current = queue[current] }
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "Artwork QA", Submenu: []*mygo.MenuItem{
				{Label: "Reset measurements", Click: func(*mygo.MenuItem, *mygo.Window) { resetStats() }},
				{Label: "Desktop size", Click: func(*mygo.MenuItem, *mygo.Window) { window.SetSize(1440, 940) }},
				{Label: "Narrow size", Click: func(*mygo.MenuItem, *mygo.Window) { window.SetSize(760, 740) }},
				{Label: "Compact size", Click: func(*mygo.MenuItem, *mygo.Window) { window.SetSize(420, 740) }},
				{Role: mygo.RoleQuit},
			}},
		}))
		window = mygo.NewWindow(mygo.WindowOptions{Title: "ytmGO Artwork QA — isolated fixture", Width: 1440, Height: 940, MinWidth: 380, MinHeight: 640, Content: ui.View(func(c *ui.Context) {
			frame := artworkFrame{cache: cache}
			actions.Artwork = func(raw string, requested int, priority artworkPriority) ui.ImageSource {
				got := frame.get(raw, requested, priority)
				key := artRequest{artworkRequestAt(raw, artworkBucket(requested)), artworkBucket(requested)}
				stats.mu.Lock()
				if previous := stats.observed[key]; previous != 0 {
					stats.WarmRequests++
					if key.edge >= 384 {
						stats.HDWarmRequests++
					}
					if got == nil {
						stats.WarmPlaceholders++
					} else if reflect.ValueOf(got).Pointer() != previous {
						stats.BitmapChanges++
					}
				} else if bitmap, ok := got.(*ui.Bitmap); ok {
					w, h := bitmap.Size()
					if max(w, h) == key.edge {
						stats.observed[key] = reflect.ValueOf(bitmap).Pointer()
					}
				}
				stats.mu.Unlock()
				return got
			}
			nativeView(c, model, actions)
			if frame.submit() {
				c.Invalidate()
			}
		})})
		window.OnClose(func(*mygo.CloseEvent) { mygo.App.Quit() })
		actions.artworkScale = mygo.Screen.DisplayMatching(window.Bounds()).ScaleFactor
		window.Invalidate()
		window.Show()
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					mygo.RunOnMain(func() {
						var command struct {
							Sequence      int
							Action        string
							Y             float32
							Width, Height int
						}
						data, err := os.ReadFile(filepath.Join(directory, "command.json"))
						if err == nil && json.Unmarshal(data, &command) == nil && command.Sequence > sequence {
							sequence = command.Sequence
							switch command.Action {
							case "home":
								model.Route, model.Page, model.Immersive = "home", home, false
							case "list":
								model.Route, model.Page, model.Immersive = "browse:fixture", list, false
							case "scroll":
								if model.pageScroll != nil {
									model.pageScroll.ScrollTo(0, command.Y, ui.Animation{Duration: time.Millisecond})
								}
							case "panel":
								model.Panel, model.Immersive = !model.Panel, false
							case "immersive":
								model.Immersive = !model.Immersive
							case "next":
								current = (current + 1) % len(queue)
								model.Current = queue[current]
							case "resize":
								window.SetSize(command.Width, command.Height)
							case "reset":
								resetStats()
							case "quit":
								writeStatus()
								cancel()
								mygo.App.Quit()
								return
							}
							window.Invalidate()
						}
						writeStatus()
					})
				}
			}
		}()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { cancel() })
	if mygo.App.Run() != nil {
		return 3
	}
	cache.workers.Wait()
	return 0
}
