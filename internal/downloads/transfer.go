package downloads

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"ytmgo/internal/preference"
)

var errTransfer = errors.New("download_transfer")

func (m *Manager) run() {
	defer close(m.done)
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
		}
		for m.ctx.Err() == nil {
			m.mu.Lock()
			var j *job
			if m.online {
				for _, candidate := range m.jobs {
					if candidate.State == Queued {
						j = candidate
						break
					}
				}
			}
			if j == nil {
				m.mu.Unlock()
				break
			}
			ctx, cancel := context.WithCancel(j.life)
			m.active, m.abort = j, cancel
			j.State = Downloading
			scope, dir := m.scope, m.assetDir(j)
			m.activeDir = dir
			m.mu.Unlock()
			signal(m.changes)
			err := m.transfer(ctx, j, scope, dir)
			m.mu.Lock()
			if m.byID[j.Item.ID] == j && j.State == Downloading && ctx.Err() == nil {
				j.State, j.Failure = Complete, ""
				if err != nil {
					j.State = Failed
					switch {
					case errors.Is(err, ErrKey):
						j.Failure = "key"
					case errors.Is(err, ErrStorage):
						j.Failure = "storage"
					case errors.Is(err, ErrMedia):
						j.Failure = "media"
					default:
						j.Failure = "network"
					}
				}
				if m.save() != nil {
					j.State, j.Failure = Failed, "storage"
				}
			}
			if j.State == Complete && j.HasCover && m.byID[j.Item.ID] == j {
				m.byCover[j.Item.Artwork] = filepath.Join(dir, "cover")
			}
			if j.removeAfter != "" {
				_ = os.RemoveAll(j.removeAfter)
			}
			m.active, m.abort, m.activeDir = nil, nil, ""
			m.mu.Unlock()
			cancel()
			signal(m.changes)
		}
	}
}

func (m *Manager) transfer(ctx context.Context, j *job, scope, dir string) error {
	// A URL can expire mid-download. Re-resolve once, and resume only a
	// matching representation. Further failures remain visible for manual retry.
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		remote, resolveErr := m.resolve(ctx, j.Item.ID, j.Quality)
		if resolveErr != nil {
			return resolveErr
		}
		err = m.copyMedia(ctx, j, scope, dir, remote)
		remote.Close()
		if !errors.Is(err, errTransfer) {
			break
		}
	}
	if err != nil {
		return err
	}
	if m.extras != nil && ctx.Err() == nil {
		extraCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		lyrics, cover := m.extras(extraCtx, j.Item)
		cancel()
		m.mu.Lock()
		if ctx.Err() == nil && m.byID[j.Item.ID] == j {
			if lyrics.Text != "" {
				if b, e := json.Marshal(lyrics); e == nil {
					_ = preference.WriteFile(filepath.Join(dir, "lyrics.json"), b)
				}
			}
			if len(cover) > 0 {
				j.HasCover = preference.WriteFile(filepath.Join(dir, "cover"), cover) == nil
			}
		}
		m.mu.Unlock()
	}
	return ctx.Err()
}

func (m *Manager) copyMedia(ctx context.Context, j *job, scope, dir string, remote Remote) error {
	f := remote.Format
	if !f.valid() {
		return ErrMedia
	}
	master, err := m.master(true)
	if err != nil {
		return err
	}
	bind := binding(scope, j.Asset, j.Item.ID, f)
	file, trackKey, offset, err := m.prepareFile(ctx, j, dir, f, master, bind)
	if err != nil {
		return err
	}
	defer file.Close()
	var scratch []byte
	for start := int64(0); start < offset; start += chunkSize {
		if err = ctx.Err(); err != nil {
			return err
		}
		scratch, err = readRecord(file, trackKey, bind, start/chunkSize, f.Length, scratch)
		if err != nil {
			offset = 0
			break
		}
	}
	clear(scratch)
	if err = file.Truncate(diskSize(offset)); err != nil {
		return ErrStorage
	}
	if _, err = file.Seek(0, io.SeekEnd); err != nil {
		return ErrStorage
	}
	m.progress(j, offset)
	if offset < f.Length {
		body, err := remote.Open(ctx, offset)
		if err != nil {
			return errTransfer
		}
		defer body.Close()
		plain := make([]byte, chunkSize)
		defer clear(plain)
		sealed := make([]byte, 0, chunkSize+recordOverhead)
		for offset < f.Length {
			if err = ctx.Err(); err != nil {
				return err
			}
			p := plain[:min(int64(chunkSize), f.Length-offset)]
			if _, err = io.ReadFull(body, p); err != nil {
				return errTransfer
			}
			sealed = trackKey.Seal(sealed[:0], nil, p, blockAAD(bind, offset/chunkSize))
			if _, err = file.Write(sealed); err != nil {
				return ErrStorage
			}
			offset += int64(len(p))
			m.progress(j, offset)
		}
	}
	if file.Sync() != nil {
		return ErrStorage
	}
	return ctx.Err()
}

func (m *Manager) prepareFile(ctx context.Context, j *job, dir string, f Format, master cipher.AEAD, bind [32]byte) (*os.File, cipher.AEAD, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil || m.byID[j.Item.ID] != j {
		return nil, nil, 0, context.Canceled
	}
	if libraryDirectory(dir) != nil {
		return nil, nil, 0, ErrStorage
	}
	file, err := openMedia(dir, os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, nil, 0, ErrStorage
	}
	if j.Format == f && f.ITag > 0 && f.Version != "" && len(j.Wrapped) > 0 {
		key, err := unwrap(master, j.Wrapped, bind)
		if err == nil {
			st, err := file.Stat()
			if err != nil {
				file.Close()
				return nil, nil, 0, ErrStorage
			}
			return file, key, committed(st.Size(), f.Length), nil
		}
	}
	if file.Truncate(0) != nil {
		file.Close()
		return nil, nil, 0, ErrStorage
	}
	plain := make([]byte, 32)
	rand.Read(plain)
	defer clear(plain)
	key, err := cipherFor(plain)
	if err != nil {
		file.Close()
		return nil, nil, 0, err
	}
	j.Format, j.Total, j.Bytes = f, f.Length, 0
	j.Wrapped = master.Seal(nil, nil, plain, bind[:])
	if err = m.save(); err != nil {
		file.Close()
		return nil, nil, 0, err
	}
	return file, key, 0, nil
}

func (m *Manager) progress(j *job, offset int64) {
	m.mu.Lock()
	if m.byID[j.Item.ID] == j {
		j.Bytes = offset
	}
	m.mu.Unlock()
	signal(m.changes)
}
