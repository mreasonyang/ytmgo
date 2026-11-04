package catalog

import (
	"context"
	"errors"
	"time"
)

type freshKey struct{}

// Refresh bypasses completed responses, but still joins a network request
// already in flight. Use only for an explicit user refresh/retry.
func Refresh(ctx context.Context) context.Context { return context.WithValue(ctx, freshKey{}, true) }

type cachedResponse[T any] struct {
	value   T
	err     error
	expires time.Time
	used    uint64
	bytes   int
}
type responseFlight[T any] struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	value   T
	err     error
}

// All fields are protected by Client.mu. Results are private in-memory metadata,
// never HTTP bodies or credentials. Callers clone mutable slices before use.
type responseCache[T any] struct {
	entries           map[string]*cachedResponse[T]
	flights           map[string]*responseFlight[T]
	generation, clock uint64
	bytes             int
}

func (m *responseCache[T]) reset() {
	m.generation++
	clear(m.entries)
	m.bytes = 0
	for _, f := range m.flights {
		f.cancel()
	}
	clear(m.flights)
}

func (m *responseCache[T]) remove(key string) {
	if e := m.entries[key]; e != nil {
		m.bytes -= e.bytes
		delete(m.entries, key)
	}
}

func memoized[T any](c *Client, ctx context.Context, m *responseCache[T], key string, ttl time.Duration, budget int, size func(T) int, load func(context.Context) (T, error), cacheable ...func(T) bool) (T, error) {
	var zero T
	ctx, scope, err := c.operation(ctx)
	if err != nil {
		return zero, err
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return zero, err
	}
	if err := c.scopeErrorLocked(scope); err != nil {
		c.mu.Unlock()
		return zero, err
	}
	if m.entries == nil {
		m.entries = make(map[string]*cachedResponse[T])
		m.flights = make(map[string]*responseFlight[T])
	}
	for k, e := range m.entries {
		if !time.Now().Before(e.expires) {
			m.remove(k)
		}
	}
	if fresh, _ := ctx.Value(freshKey{}).(bool); fresh {
		m.remove(key)
	}
	if e := m.entries[key]; e != nil {
		m.clock++
		e.used = m.clock
		value, err := e.value, e.err
		c.mu.Unlock()
		return value, err
	}
	generation := m.generation
	f := m.flights[key]
	if f == nil {
		if len(m.flights) >= 32 {
			c.mu.Unlock()
			return zero, ErrUnavailable
		}
		// Preserve the originating identity while giving each waiter independent
		// cancellation. The last departing waiter cancels the shared operation.
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		f = &responseFlight[T]{done: make(chan struct{}), cancel: cancel}
		m.flights[key] = f
		go func() {
			value, err := load(work)
			if work.Err() != nil {
				err = work.Err()
			}
			cancel()
			c.mu.Lock()
			defer c.mu.Unlock()
			if scopeErr := c.scopeErrorLocked(scope); scopeErr != nil {
				err = scopeErr
			}
			if generation != m.generation {
				err = ErrSessionChanged
			}
			if m.flights[key] == f {
				delete(m.flights, key)
				retain := err == nil || errors.Is(err, ErrNoLyrics)
				for _, accept := range cacheable {
					retain = retain && accept(value)
				}
				if retain {
					weight, lifetime := size(value)+len(key)+128, ttl
					if err != nil {
						lifetime = 30 * time.Second
					}
					if weight <= budget {
						for len(m.entries) >= 64 || m.bytes+weight > budget {
							oldest := ""
							var oldestUse uint64 = ^uint64(0)
							for k, e := range m.entries {
								if e.used < oldestUse {
									oldest, oldestUse = k, e.used
								}
							}
							m.remove(oldest)
						}
						m.clock++
						m.entries[key] = &cachedResponse[T]{value: value, err: err, expires: time.Now().Add(lifetime), used: m.clock, bytes: weight}
						m.bytes += weight
					}
				}
			}
			f.value, f.err = value, err
			close(f.done)
		}()
	}
	f.waiters++
	c.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-f.done:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f.waiters--
	if f.waiters == 0 && m.flights[key] == f {
		delete(m.flights, key)
		f.cancel()
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := c.scopeErrorLocked(scope); err != nil {
		return zero, err
	}
	if generation != m.generation {
		return zero, ErrSessionChanged
	}
	return f.value, f.err
}

type queueResult struct {
	items  []Item
	lyrics string
}

func itemBytes(it Item) int {
	return 160 + len(it.ID) + len(it.Title) + len(it.Artist) + len(it.Album) + len(it.Artwork) + len(it.Kind) + len(it.BrowseID) + len(it.BrowseParams)
}
func itemsBytes(items []Item) int {
	n := 0
	for _, it := range items {
		n += itemBytes(it)
	}
	return n
}
func pageBytes(p Page) int {
	n := 128 + len(p.Title) + len(p.Creator) + len(p.Subtitle) + len(p.Artwork) + len(p.Continuation) + itemsBytes(p.Items)
	for _, shelf := range p.Shelves {
		n += 64 + len(shelf.Title) + itemsBytes(shelf.Items)
	}
	return n
}
func clonePage(p Page) Page {
	p.Items = append([]Item(nil), p.Items...)
	p.Shelves = append([]Shelf(nil), p.Shelves...)
	for i := range p.Shelves {
		p.Shelves[i].Items = append([]Item(nil), p.Shelves[i].Items...)
	}
	return p
}
func lyricsBytes(r LyricsResult) int {
	n := 64 + len(r.Text) + len(r.Source)
	for _, line := range r.Lines {
		n += 48 + len(line.Text)
	}
	return n
}
func cloneLyrics(r LyricsResult) LyricsResult {
	r.Lines = append([]LyricLine(nil), r.Lines...)
	return r
}

func (c *Client) resetResponsesLocked() {
	c.pages.reset()
	c.queues.reset()
	c.lyricResults.reset()
}
