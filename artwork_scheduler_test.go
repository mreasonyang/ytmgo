package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type artworkObservedBody struct {
	io.ReadCloser
	total, threshold int
	reached          func()
}

func (body *artworkObservedBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	body.total += n
	if body.total >= body.threshold {
		body.reached()
	}
	return n, err
}

func artworkLoopback(t *testing.T, c *artworkCache, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	origin, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		upstream := r.Clone(r.Context())
		upstream.URL.Scheme, upstream.URL.Host = origin.Scheme, origin.Host
		return server.Client().Transport.RoundTrip(upstream)
	})
}

func TestArtworkInFlightScrollReusesUpstream(t *testing.T) {
	for _, phase := range []string{"before-headers", "mid-body", "mid-body-no-store"} {
		t.Run(phase, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			c.disk = &artworkDisk{dir: t.TempDir()}
			data := artworkTestPNG(t, 96, 96)
			headers := "public, max-age=3600"
			if phase == "mid-body-no-store" {
				headers = "no-store"
			}
			wantPartial := 0
			if phase != "before-headers" {
				wantPartial = len(data) / 2
			}
			var gets, sent atomic.Int32
			paused, release := make(chan struct{}), make(chan struct{})
			artworkLoopback(t, c, func(w http.ResponseWriter, r *http.Request) {
				attempt := gets.Add(1)
				w.Header().Set("Cache-Control", headers)
				w.Header().Set("Content-Length", fmt.Sprint(len(data)))
				if wantPartial > 0 {
					n, _ := w.Write(data[:wantPartial])
					sent.Add(int32(n))
					w.(http.Flusher).Flush()
				}
				if attempt == 1 {
					close(paused)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					case <-t.Context().Done():
						return
					}
				}
				n, _ := w.Write(data[wantPartial:])
				sent.Add(int32(n))
			})
			bodyRead := make(chan struct{})
			if wantPartial > 0 {
				transport := c.client.Transport
				reached := sync.OnceFunc(func() { close(bodyRead) })
				c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
					res, err := transport.RoundTrip(r)
					if err == nil {
						res.Body = &artworkObservedBody{ReadCloser: res.Body, threshold: wantPartial, reached: reached}
					}
					return res, err
				})
			}
			const raw = "https://i.ytimg.com/continuous"
			requestTestArtwork(c, raw, 128)
			select {
			case <-paused:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream never reached its barrier")
			}
			if wantPartial > 0 {
				select {
				case <-bodyRead:
				case <-time.After(2 * time.Second):
					t.Fatal("client never read the first half of the response")
				}
			}
			if got := sent.Load(); got != int32(wantPartial) {
				t.Fatalf("transferred %d bytes at barrier, want %d", got, wantPartial)
			}
			for i := range 8 {
				empty := c.beginFrame()
				empty.submit()
				priority := artPrimary
				if i%2 == 1 {
					priority = artPlayer
				}
				requestArtworkPriority(c, raw, 128, priority)
			}
			close(release)
			c.workers.Wait()
			if got := requestArtworkPriority(c, raw, 128, artPlayer); got == nil || gets.Load() != 1 || sent.Load() != int32(len(data)) {
				t.Fatalf("revisit bitmap=%t, upstream GETs=%d, bytes=%d, want one complete transfer of %d", got != nil, gets.Load(), sent.Load(), len(data))
			}
		})
	}
}

func TestArtworkOffscreenHTTPCompletion(t *testing.T) {
	for _, policy := range []string{"public, max-age=3600", "public, no-cache", "no-store", "failure"} {
		t.Run(policy, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			c.disk = &artworkDisk{dir: t.TempDir()}
			var redraws, gets, bodyBytes, validated atomic.Int32
			c.redraw = func() { redraws.Add(1) }
			data := artworkTestPNG(t, 128, 128)
			paused, release := make(chan struct{}), make(chan struct{})
			artworkLoopback(t, c, func(w http.ResponseWriter, r *http.Request) {
				attempt := gets.Add(1)
				if policy == "failure" && attempt == 1 {
					close(paused)
					select {
					case <-release:
						w.WriteHeader(http.StatusServiceUnavailable)
					case <-r.Context().Done():
					case <-t.Context().Done():
					}
					return
				}
				w.Header().Set("Cache-Control", policy)
				w.Header().Set("Etag", `"cover"`)
				if r.Header.Get("If-None-Match") == `"cover"` {
					validated.Add(1)
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("Content-Length", fmt.Sprint(len(data)))
				cut := len(data) / 2
				n, _ := w.Write(data[:cut])
				bodyBytes.Add(int32(n))
				w.(http.Flusher).Flush()
				if attempt == 1 {
					close(paused)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					case <-t.Context().Done():
						return
					}
				}
				n, _ = w.Write(data[cut:])
				bodyBytes.Add(int32(n))
			})
			const raw = "https://i.ytimg.com/offscreen-completion"
			requestTestArtwork(c, raw, 128)
			select {
			case <-paused:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream never reached its barrier")
			}
			empty := c.beginFrame()
			empty.submit()
			close(release)
			c.workers.Wait()
			fresh := policy == "public, max-age=3600"
			entry := c.entry(artRequest{url: raw, edge: 128})
			if redraws.Load() != 0 || c.retryTimer != nil {
				t.Fatal("an offscreen completion scheduled a redraw or retry")
			}
			if fresh {
				if entry == nil || entry.bitmap == nil || entry.pixelBytes != artworkPixelCost(128, 128) {
					t.Fatal("idle worker did not fully account for the decoded bitmap")
				}
				idle := c.beginFrame()
				idle.submit()
				if entry.bitmap == nil || entry.pixelBytes != 0 || entry.bytes <= 0 {
					t.Fatal("view did not release offscreen pixels and retain encoded backing")
				}
			} else if entry != nil {
				t.Fatal("an offscreen non-reusable result entered the decoded cache")
			}
			// Use a fresh pool against the same disk to check persistent admission.
			next := artworkTestCache(t, nil)
			next.disk, next.client.Transport = c.disk, c.client.Transport
			requestTestArtwork(next, raw, 128)
			next.workers.Wait()
			if requestTestArtwork(next, raw, 128) == nil {
				t.Fatal("revisited artwork did not produce pixels")
			}
			wantGETs, wantBodies, wantValidated := int32(2), int32(1), int32(0)
			switch policy {
			case "public, max-age=3600":
				wantGETs = 1
			case "public, no-cache":
				wantValidated = 1
			case "no-store":
				wantBodies = 2
			}
			if gets.Load() != wantGETs || bodyBytes.Load() != wantBodies*int32(len(data)) || validated.Load() != wantValidated {
				t.Fatalf("upstream GETs=%d, entity bytes=%d and 304s=%d, want %d, %d and %d", gets.Load(), bodyBytes.Load(), validated.Load(), wantGETs, wantBodies*int32(len(data)), wantValidated)
			}
		})
	}
}

func TestArtworkVisibleCoverReclaimsPartialUpstreamTransfer(t *testing.T) {
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: t.TempDir()}
	data := artworkTestPNG(t, 96, 96)
	cut := len(data) / 2
	paused, canceled := make(chan struct{}), make(chan struct{})
	redraws := make(chan struct{}, 4)
	c.redraw = func() { redraws <- struct{}{} }
	var getsA, bytesA, getsB, bytesB atomic.Int32
	artworkLoopback(t, c, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		switch r.URL.Path {
		case "/partial":
			if getsA.Add(1) == 1 {
				n, _ := w.Write(data[:cut])
				bytesA.Add(int32(n))
				w.(http.Flusher).Flush()
				close(paused)
				select {
				case <-r.Context().Done():
					close(canceled)
				case <-t.Context().Done():
				}
				return
			}
			n, _ := w.Write(data)
			bytesA.Add(int32(n))
		case "/new":
			getsB.Add(1)
			n, _ := w.Write(data)
			bytesB.Add(int32(n))
		default:
			select {
			case <-r.Context().Done():
			case <-t.Context().Done():
			}
		}
	})
	const origin = "https://i.ytimg.com"
	render := func(path string) {
		f := c.beginFrame()
		for i := 1; i < artworkBrowseLarge; i++ {
			f.get(fmt.Sprintf("%s/visible/%d", origin, i), 384, artPrimary)
		}
		f.get(origin+path, 384, artPrimary)
		f.submit()
	}
	await := func(event <-chan struct{}, description string) {
		t.Helper()
		select {
		case <-event:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", description)
		}
	}
	render("/partial")
	await(paused, "first half of the old cover")
	render("/new")
	c.mu.Lock()
	for _, job := range c.running {
		if job.url == origin+"/partial" && job.ctx.Err() == nil {
			t.Error("the old large cover was kept in grace despite the conflicting visible cover")
		}
	}
	c.mu.Unlock()
	await(canceled, "upstream cancellation")
	await(redraws, "new visible cover")
	render("/partial")
	await(redraws, "full transfer on the old cover's return")
	if getsA.Load() != 2 || bytesA.Load() != int32(cut+len(data)) || getsB.Load() != 1 || bytesB.Load() != int32(len(data)) {
		t.Fatalf("partial GETs=%d bytes=%d, new GETs=%d bytes=%d", getsA.Load(), bytesA.Load(), getsB.Load(), bytesB.Load())
	}
}

func TestArtworkGraceExpiresWhileIdleAndIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := artworkTestCache(t, nil)
		var redraws atomic.Int32
		c.redraw = func() { redraws.Add(1) }
		requests := make(chan *http.Request, 5)
		c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
			requests <- r
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		f := c.beginFrame()
		for i := range cap(requests) {
			f.get(fmt.Sprintf("https://i.ytimg.com/grace-%d", i), 128, artPrimary)
		}
		f.submit()
		synctest.Wait()
		before := time.Now()
		empty := c.beginFrame()
		empty.submit()
		synctest.Wait()
		starts, live := 0, 0
		for len(requests) > 0 {
			starts++
			if (<-requests).Context().Err() == nil {
				live++
			}
		}
		c.mu.Lock()
		timer, deadline := c.graceTimer, c.graceDeadline
		c.mu.Unlock()
		if starts != 5 || live != artworkGraceLimit || timer == nil || deadline.Sub(before) != artworkGrace {
			t.Fatalf("started=%d, grace=%d, deadline=%v", starts, live, deadline.Sub(before))
		}
		time.Sleep(artworkGrace - time.Nanosecond)
		empty = c.beginFrame()
		empty.submit()
		synctest.Wait()
		c.mu.Lock()
		sameTimer, running := timer == c.graceTimer, len(c.running)
		c.mu.Unlock()
		if !sameTimer || running != artworkGraceLimit {
			t.Fatal("another empty frame replaced the timer or shortened grace")
		}
		time.Sleep(time.Nanosecond)
		c.workers.Wait()
		synctest.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()
		if time.Since(before) != artworkGrace || c.graceTimer != nil || len(c.running) != 0 || len(c.small.entries) != 0 || redraws.Load() != 0 {
			t.Fatal("idle grace did not expire cleanly")
		}
	})
}

func TestArtworkGraceTimerAdvancesToNextDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := artworkTestCache(t, nil)
		requests := make(chan *http.Request, 2)
		c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
			requests <- r
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		const a, b = "https://i.ytimg.com/first", "https://i.ytimg.com/second"
		f := c.beginFrame()
		f.get(a, 128, artPrimary)
		f.get(b, 128, artPrimary)
		f.submit()
		synctest.Wait()
		first, second := <-requests, <-requests
		if first.URL.String() != a {
			first, second = second, first
		}
		requestTestArtwork(c, b, 128)
		time.Sleep(artworkGrace / 2)
		empty := c.beginFrame()
		empty.submit()
		time.Sleep(artworkGrace / 2)
		synctest.Wait()
		c.mu.Lock()
		timer, until := c.graceTimer, c.graceDeadline
		c.mu.Unlock()
		if first.Context().Err() == nil || second.Context().Err() != nil || timer == nil || time.Until(until) != artworkGrace/2 {
			t.Fatal("first expiry did not retain the second request and advance the timer")
		}
		time.Sleep(artworkGrace / 2)
		c.workers.Wait()
		if second.Context().Err() == nil || time.Now().Compare(until) != 0 {
			t.Fatal("second request did not expire at its own deadline")
		}
	})
}

func TestArtworkPlayerClaimsConflictingSlotAfterCanceledWorkerExits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := artworkTestCache(t, nil)
		type attempt struct {
			req     *http.Request
			release chan struct{}
		}
		started := make(chan attempt, 24)
		c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
			a := attempt{req: r, release: make(chan struct{})}
			started <- a
			<-r.Context().Done()
			if r.URL.Path == "/A" || r.URL.Path == "/B" {
				select {
				case <-a.release:
				case <-c.ctx.Done():
				}
			}
			return nil, r.Context().Err()
		})
		render := func(player string) {
			f := c.beginFrame()
			for i := range artworkBrowseWorkers + 3 {
				edge := 128
				if i < artworkBrowseLarge {
					edge = 384
				}
				f.get(fmt.Sprintf("https://i.ytimg.com/browse/%d", i), edge, artPrimary)
			}
			if player != "" {
				f.get("https://i.ytimg.com/"+player, 384, artPlayer)
			}
			f.submit()
			synctest.Wait()
		}
		work := func() artworkWork {
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.work(true)
		}
		render("")
		if len(started) != artworkBrowseWorkers || work().large != artworkBrowseLarge {
			t.Fatal("browse work consumed the player reservation")
		}
		for range artworkBrowseWorkers {
			<-started
		}
		render("A")
		a := <-started
		if used := work(); a.req.URL.Path != "/A" || used.total != artworkWorkers || used.large != artworkLargeLimit {
			t.Fatal("player did not start against saturated browse work")
		}
		for _, player := range []string{"B", "A", "B", "A"} {
			render(player)
			if a.req.Context().Err() == nil || len(started) != 0 || work().total != artworkWorkers {
				t.Fatal("a new track either waited out grace or reused a still occupied permit")
			}
			close(a.release)
			synctest.Wait()
			if len(started) != 1 {
				t.Fatalf("wanted a single player worker, started %d workers", len(started))
			}
			a = <-started
			if a.req.URL.Path != "/"+player || work().large != artworkLargeLimit {
				t.Fatalf("unexpected replacement: %s", a.req.URL.Path)
			}
		}
	})
}

func TestArtworkDecodeSlotsFollowCurrentPlayerAndVisibleWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := artworkTestCache(t, nil)
		c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		for range artworkDecoders {
			if !c.takeDecoder(c.ctx, nil, time.Time{}) {
				t.Fatal("both decoder slots must be available to browse work")
			}
		}
		const origin = "https://i.ytimg.com/"
		render := func(player string, browse ...string) {
			f := c.beginFrame()
			for _, key := range browse {
				f.get(origin+key, 128, artPrimary)
			}
			f.get(origin+player, 128, artPlayer)
			f.submit()
			synctest.Wait()
		}
		granted := make(chan string, 4)
		release := make(chan struct{})
		wait := func(name string) {
			c.mu.Lock()
			e := c.entry(artRequest{url: origin + name, edge: 128})
			job := c.running[e]
			c.mu.Unlock()
			go func() {
				if c.takeDecoder(job.ctx, e, time.Now().Add(time.Minute)) {
					granted <- name
					select {
					case <-release:
					case <-c.ctx.Done():
					}
					c.releaseDecoder()
				}
			}()
			synctest.Wait()
		}
		render("A", "away", "browse")
		wait("away")
		wait("browse")
		wait("A")
		render("B", "browse")
		wait("B")
		c.mu.Lock()
		waiting := len(c.decodeQueue)
		c.mu.Unlock()
		if len(granted) != 0 || waiting != 4 {
			t.Fatal("a decoder exceeded the two-slot cap")
		}
		for _, want := range []string{"B", "browse"} {
			c.releaseDecoder()
			synctest.Wait()
			if len(granted) != 1 || <-granted != want {
				t.Fatalf("next free decoder did not go to %s", want)
			}
		}
		close(release)
		synctest.Wait()
		c.mu.Lock()
		decoding, waiting := c.decoding, len(c.decodeQueue)
		c.mu.Unlock()
		if decoding != 0 || waiting != 2 || len(granted) != 0 {
			t.Fatal("a grace candidate took a decoder while visible work was outstanding")
		}
		render("A", "browse")
		if len(granted) != 1 || <-granted != "A" {
			t.Fatal("returning player did not reuse its waiting decode")
		}
	})
}
