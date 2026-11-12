package player

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"ytmgo/internal/audioquality"
	"ytmgo/internal/downloads"
)

// AttachDownloads runs once during startup, before any playback/queue workers.
func (p *Player) AttachDownloads(library *downloads.Manager) {
	p.downloads = library
	p.resolve = p.resolveAudio
}

func (p *Player) resolveAudio(ctx context.Context, track audioTrack) (media, error) {
	source, format, err := p.downloads.OpenTrack(track.id)
	if err != nil {
		return media{}, err
	}
	if source != nil {
		return media{source: source, local: true, Duration: format.Duration, Video: format.Video}, nil
	}
	// A deletion/account switch can race a previously successful local lookup.
	if err = p.lockAccount(ctx); err != nil {
		return media{}, err
	}
	p.mu.Unlock()
	return p.resolver.Resolve(ctx, track)
}

// DownloadSource uses the same range validation, redirect policy, credentials
// and source selection as playback. Only a private loopback URL reaches HTTP.
func (p *Player) DownloadSource(ctx context.Context, id string, q audioquality.Quality) (downloads.Remote, error) {
	if err := p.lockAccount(ctx); err != nil {
		return downloads.Remote{}, err
	}
	p.mu.Unlock()
	resolveCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	res, err := p.resolver.native.ResolveAudio(resolveCtx, id, q)
	cancel()
	if err != nil {
		return downloads.Remote{}, nativeFailure(err)
	}
	bridge, err := newStreamBridge(ctx, media{source: res.Lease})
	if err != nil {
		res.Lease.Close()
		return downloads.Remote{}, err
	}
	return downloads.Remote{
		Format: downloads.Format{ITag: res.ITag, Version: res.LastModified, Type: res.ContentType, Length: res.Length, Duration: res.Duration, Video: res.Video},
		Close:  func() { bridge.Close(); res.Lease.Close() },
		Open: func(ctx context.Context, start int64) (io.ReadCloser, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, bridge.URL, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
			reply, err := bridge.client.Do(req)
			if err != nil {
				return nil, ErrNoTrack
			}
			if reply.StatusCode != http.StatusPartialContent || reply.ContentLength != res.Length-start ||
				reply.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, res.Length-1, res.Length) {
				reply.Body.Close()
				return nil, ErrNoTrack
			}
			return reply.Body, nil
		},
	}, nil
}
