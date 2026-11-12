// Package downloads stores account-scoped, authenticated offline audio.
package downloads

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"ytmgo/internal/accountsession"
)

const chunkSize = 256 << 10
const recordOverhead = 12 + 16 // random GCM nonce and authentication tag
const masterKeyName = "offline-media-master-v1"

var ErrStorage = errors.New("download_storage")
var ErrKey = errors.New("download_key_unavailable")
var ErrMedia = errors.New("download_media_invalid")

type Format struct {
	ITag     int
	Version  string // Service media modification identifier, never a signed URL.
	Type     string
	Length   int64
	Duration time.Duration
	Video    bool
}

func (f Format) valid() bool {
	return (f.Type == "audio/mp4" || f.Type == "audio/webm") && f.Length > 0 && f.Length <= 64<<30 && f.Duration > 0 && f.Duration <= 24*time.Hour && len(f.Version) <= 32
}

func cipherFor(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrKey
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// Bind each wrapped track key to its library, asset and complete media description.
func binding(scope, asset, id string, format Format) [32]byte {
	b, _ := json.Marshal([]any{"ytmgo-offline-v1", scope, asset, id, format})
	return sha256.Sum256(b)
}

func blockAAD(bind [32]byte, index int64) []byte {
	return binary.BigEndian.AppendUint64(bind[:], uint64(index))
}

func (m *Manager) master(create bool) (cipher.AEAD, error) {
	m.keyMu.Lock()
	defer m.keyMu.Unlock()
	if m.key != nil {
		return m.key, nil
	}
	key, err := m.secrets.Get(masterKeyName, accountsession.AllowPrompt)
	if errors.Is(err, accountsession.ErrNotFound) && create {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err == nil {
			err = m.secrets.Set(masterKeyName, key)
		}
	}
	if err != nil {
		return nil, ErrKey
	}
	defer clear(key)
	m.key, err = cipherFor(key)
	return m.key, err
}

func unwrap(master cipher.AEAD, wrapped []byte, bind [32]byte) (cipher.AEAD, error) {
	if len(wrapped) != 32+recordOverhead {
		return nil, ErrMedia
	}
	key, err := master.Open(nil, nil, wrapped, bind[:])
	if err != nil {
		return nil, ErrKey
	}
	defer clear(key)
	return cipherFor(key)
}

func diskSize(plain int64) int64 {
	return plain + ((plain+chunkSize-1)/chunkSize)*recordOverhead
}

// Only whole authenticated records count as resumable. An interrupted write
// may have left part of the next record on disk.
func committed(size, total int64) int64 {
	if total > 0 && size == diskSize(total) {
		return total
	}
	if size < 0 || size > diskSize(total) {
		return 0
	}
	return min(total, size/(chunkSize+recordOverhead)*chunkSize)
}

type Source struct {
	life   context.Context
	dir    string
	format Format
	aead   cipher.AEAD
	bind   [32]byte
	bad    atomic.Bool
	damage func()
}

func (s *Source) damaged() {
	if s.bad.CompareAndSwap(false, true) && s.damage != nil {
		s.damage()
	}
}

func (s *Source) Err() error {
	if s.bad.Load() {
		return ErrMedia
	}
	return s.life.Err()
}

// Keep media reads and resumable writes inside the allocated track directory.
func openMedia(dir string, flags int) (*os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	st, err := root.Lstat("media")
	if err == nil && !st.Mode().IsRegular() {
		return nil, ErrStorage
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return root.OpenFile("media", flags, 0600)
}

// Each HTTP range has its own cursor and one decrypted record. Seek and
// overlapping FFmpeg requests never create a plaintext file.
func (s *Source) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if s.life.Err() != nil {
		http.Error(w, "", http.StatusGone)
		return
	}
	f, err := openMedia(s.dir, os.O_RDONLY)
	if err != nil {
		s.damaged()
		http.Error(w, "", http.StatusGone)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() != diskSize(s.format.Length) {
		s.damaged()
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	done := make(chan struct{})
	stop := context.AfterFunc(s.life, func() { _ = f.Close(); _ = http.NewResponseController(w).SetWriteDeadline(time.Now()); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	w.Header().Set("Content-Type", s.format.Type)
	w.Header().Set("Cache-Control", "no-store")
	r := &reader{source: s, file: f, request: req.Context(), index: -1}
	defer func() { clear(r.block) }()
	http.ServeContent(w, req, "audio", time.Time{}, io.NewSectionReader(r, 0, s.format.Length))
}

type reader struct {
	source  *Source
	file    *os.File
	request context.Context
	index   int64
	block   []byte
}

func (r *reader) ReadAt(dst []byte, pos int64) (int, error) {
	s := r.source
	if pos < 0 {
		return 0, ErrMedia
	}
	n := 0
	for len(dst) > 0 && pos < s.format.Length {
		if err := r.request.Err(); err != nil {
			return n, err
		}
		if err := s.life.Err(); err != nil {
			return n, err
		}
		index := pos / chunkSize
		if r.index != index {
			block, err := readRecord(r.file, s.aead, s.bind, index, s.format.Length, r.block)
			if err != nil {
				if s.life.Err() == nil {
					s.damaged()
				}
				return n, ErrMedia
			}
			r.block, r.index = block, index
		}
		k := copy(dst, r.block[pos%chunkSize:])
		dst, pos, n = dst[k:], pos+int64(k), n+k
	}
	if len(dst) > 0 {
		return n, io.EOF
	}
	return n, nil
}

func readRecord(f *os.File, aead cipher.AEAD, bind [32]byte, index, total int64, buf []byte) ([]byte, error) {
	size := int(min(int64(chunkSize), total-index*chunkSize)) + recordOverhead
	if cap(buf) < size {
		buf = make([]byte, size)
	} else {
		buf = buf[:size]
	}
	if _, err := f.ReadAt(buf, index*(chunkSize+recordOverhead)); err != nil {
		return nil, err
	}
	return aead.Open(buf[:0], nil, buf, blockAAD(bind, index))
}
