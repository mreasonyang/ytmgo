package main

import (
	"cmp"
	"context"
	"slices"
	"time"
)

const (
	artworkBrowseWorkers = artworkWorkers - 1
	artworkBrowseLarge   = artworkLargeLimit - 1
	artworkGrace         = 250 * time.Millisecond
	artworkGraceLimit    = 2
)

type artworkLoad struct {
	artDemand
	entry             *artEntry
	ctx               context.Context
	permit            artworkPriority
	started           uint64
	newer             artworkAuthority // accepted response for the same effective URL
	responseWithdrawn bool             // this request itself got a non-reusable response
	graceUntil        time.Time
}

// A worker holds its permit until it exits, even if its entry was already
// removed (for example, when beginFrame observes a different local asset).
type artworkWork struct {
	total, browse, large, browseLarge, secondary int
}

func (w *artworkWork) add(d artDemand, count int) {
	w.total += count
	if d.priority != artPlayer {
		w.browse += count
	}
	if d.edge > 128 {
		w.large += count
		if d.priority != artPlayer {
			w.browseLarge += count
		}
	}
	if d.priority > artPrimary {
		w.secondary += count
	}
}

func (w artworkWork) allows(d artDemand) bool {
	return w.total < artworkWorkers &&
		(d.edge <= 128 || w.large < artworkLargeLimit) &&
		(d.priority == artPlayer || (w.browse < artworkBrowseWorkers && (d.edge <= 128 || w.browseLarge < artworkBrowseLarge))) &&
		(d.priority <= artPrimary || w.secondary < artworkSecondaryLimit)
}

func (job *artworkLoad) reservation() artDemand {
	return artDemand{job.artRequest, job.permit}
}

// Scheduler methods hold c.mu, apart from the decoder acquisition and release
// entry points, which acquire it themselves.
func (c *artworkCache) work(includeCanceled bool) (w artworkWork) {
	for _, job := range c.running {
		if includeCanceled || job.ctx.Err() == nil {
			w.add(job.reservation(), 1)
		}
	}
	return w
}

func (c *artworkCache) abandonLoad(job *artworkLoad) {
	e := job.entry
	e.cancel()
	pool := c.pool(job.artRequest)
	if pool.entries[job.artRequest] == e {
		pool.bytes -= e.bytes
		delete(pool.entries, job.artRequest)
	}
}

func (c *artworkCache) expireGrace(now time.Time) {
	for _, job := range c.running {
		if job.ctx.Err() == nil && !job.graceUntil.IsZero() && !job.graceUntil.After(now) {
			c.abandonLoad(job)
		}
	}
}

func (c *artworkCache) graceLoads() []*artworkLoad {
	var away []*artworkLoad
	for _, job := range c.running {
		if !job.graceUntil.IsZero() && job.ctx.Err() == nil {
			away = append(away, job)
		}
	}
	slices.SortFunc(away, func(a, b *artworkLoad) int {
		if earlier := a.graceUntil.Compare(b.graceUntil); earlier != 0 {
			return earlier
		}
		if tier := cmp.Compare(b.priority, a.priority); tier != 0 {
			return tier
		}
		return cmp.Compare(a.started, b.started)
	})
	return away
}

func (c *artworkCache) updateLoads(wanted map[artRequest]artworkPriority, now time.Time) {
	c.expireGrace(now)
	for _, job := range c.running {
		if job.ctx.Err() != nil {
			continue
		}
		if priority, ok := wanted[job.artRequest]; ok {
			job.priority = priority
			job.graceUntil = time.Time{}
		} else if job.graceUntil.IsZero() {
			job.graceUntil = now.Add(artworkGrace)
		}
	}
	away := c.graceLoads()
	for _, job := range away[:max(0, len(away)-artworkGraceLimit)] {
		c.abandonLoad(job)
	}

	// Apply promotions first. If a demoted job fits only in a reserved tier,
	// retire it before granting a permit for its new tier.
	for _, job := range c.running {
		if job.ctx.Err() == nil && job.graceUntil.IsZero() && job.priority < job.permit {
			job.permit = job.priority
		}
	}
	work := c.work(true)
	for _, job := range c.running {
		if job.ctx.Err() != nil || !job.graceUntil.IsZero() || job.priority <= job.permit {
			continue
		}
		work.add(job.reservation(), -1)
		if work.allows(job.artDemand) {
			job.permit = job.priority
		} else {
			c.abandonLoad(job)
		}
		work.add(job.reservation(), 1)
	}
}

func (c *artworkCache) scheduleGrace(next time.Time) {
	if c.graceTimer != nil {
		if next.Equal(c.graceDeadline) {
			return
		}
		c.graceTimer.Stop()
		c.graceTimer = nil
	}
	c.graceDeadline = next
	if next.IsZero() || c.ctx.Err() != nil {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(time.Until(next), func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.graceTimer == timer {
			c.graceTimer = nil
			c.startQueued()
		}
	})
	c.graceTimer = timer
}

// Canceled workers still block immediate admission. Plan against the slots
// they'll release so a blocked request cancels only the grace jobs it needs.
func (c *artworkCache) reclaimGrace() time.Time {
	away := c.graceLoads()
	if len(away) == 0 {
		return time.Time{}
	}
	work := c.work(false)
	for _, demand := range c.queue {
		if demand.priority == artPrefetch {
			break
		}
		if work.allows(demand) {
			work.add(demand, 1)
			continue
		}
		// There are at most two grace jobs. Try one before displacing both,
		// and only displace jobs if that will unblock this demand.
		for i, job := range away {
			trial := work
			trial.add(job.reservation(), -1)
			if trial.allows(demand) {
				c.abandonLoad(job)
				away = slices.Delete(away, i, i+1)
				work = trial
				break
			}
		}
		if !work.allows(demand) && len(away) == artworkGraceLimit {
			trial := work
			for _, job := range away {
				trial.add(job.reservation(), -1)
			}
			if trial.allows(demand) {
				for _, job := range away {
					c.abandonLoad(job)
				}
				return time.Time{}
			}
		}
		if len(away) == 0 {
			return time.Time{}
		}
		if work.allows(demand) {
			work.add(demand, 1)
		}
	}
	return away[0].graceUntil
}

func (c *artworkCache) startQueued() {
	if c.ctx.Err() != nil {
		return
	}
	c.expireGrace(time.Now())
	covers := c.currentCovers()
	c.queue = slices.DeleteFunc(c.queue, func(d artDemand) bool { return !d.matches(covers) })
	work := c.work(true)
	for len(c.queue) > 0 {
		// A large cover can be blocked while small thumbnails have room.
		index := slices.IndexFunc(c.queue, work.allows)
		if index == -1 {
			break
		}
		d := c.queue[index]
		c.queue = slices.Delete(c.queue, index, index+1)
		pool := c.admissionPool(d)
		e := pool.entries[d.artRequest]
		if e == nil {
			if len(pool.entries) >= pool.limit && !pool.evictOldest(c.visible) {
				continue
			}
			e = &artEntry{}
			pool.entries[d.artRequest] = e
		}
		ctx, cancel := context.WithCancel(c.ctx)
		e.cancel = cancel
		c.clock++
		e.used = c.clock
		c.running[e] = &artworkLoad{artDemand: d, entry: e, ctx: ctx, permit: d.priority, started: c.clock}
		work.add(d, 1)
		c.workers.Go(func() {
			c.finish(d, e, c.load(ctx, d, e))
		})
	}
	c.scheduleGrace(c.reclaimGrace())
	c.startDecoders()
}

type artworkDecodeWaiter struct {
	ctx        context.Context
	job        *artworkLoad
	freshUntil time.Time
	ready      chan struct{}
	granted    bool
}

func (c *artworkCache) takeDecoder(ctx context.Context, e *artEntry, freshUntil time.Time) bool {
	c.mu.Lock()
	waiter := &artworkDecodeWaiter{ctx: ctx, job: c.running[e], freshUntil: freshUntil, ready: make(chan struct{})}
	c.decodeQueue = append(c.decodeQueue, waiter)
	c.startDecoders()
	c.mu.Unlock()
	select {
	case <-waiter.ready:
		return true
	case <-ctx.Done():
	case <-c.ctx.Done():
	}
	c.mu.Lock()
	if waiter.granted {
		c.decoding--
	} else if index := slices.Index(c.decodeQueue, waiter); index >= 0 {
		c.decodeQueue = slices.Delete(c.decodeQueue, index, index+1)
	}
	c.startDecoders()
	c.mu.Unlock()
	return false
}

func (c *artworkCache) releaseDecoder() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decoding--
	c.startDecoders()
}

func (c *artworkCache) startDecoders() {
	if c.ctx.Err() != nil || len(c.decodeQueue) == 0 {
		return
	}
	now := time.Now()
	visibleLoad := len(c.queue) > 0 && c.queue[0].priority < artPrefetch
	for _, job := range c.running {
		if job.ctx.Err() == nil && job.graceUntil.IsZero() && job.priority < artPrefetch {
			visibleLoad = true
			break
		}
	}
	c.decodeQueue = slices.DeleteFunc(c.decodeQueue, func(waiter *artworkDecodeWaiter) bool {
		if waiter.ctx.Err() != nil {
			return true
		}
		job := waiter.job
		if job != nil && !job.graceUntil.IsZero() {
			freshUntil := waiter.freshUntil
			if job.newer.request != 0 {
				freshUntil = job.newer.freshUntil
			}
			if !job.graceUntil.After(now) || (job.asset == nil && !freshUntil.After(now)) {
				c.abandonLoad(job)
				return true
			}
		}
		return false
	})
	for c.decoding < artworkDecoders {
		index, best := -1, artPriorityCount+1
		for i, waiter := range c.decodeQueue {
			priority := artPrimary
			if job := waiter.job; job != nil {
				priority = job.priority
				if !job.graceUntil.IsZero() {
					if visibleLoad {
						continue
					}
					priority = artPriorityCount
				}
			}
			if priority < best {
				index, best = i, priority
			}
		}
		if index < 0 {
			return
		}
		waiter := c.decodeQueue[index]
		c.decodeQueue = slices.Delete(c.decodeQueue, index, index+1)
		waiter.granted = true
		c.decoding++
		close(waiter.ready)
	}
}
