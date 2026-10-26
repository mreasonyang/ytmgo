// Package library stores only the user's local music collection, never account
// credentials, cookies, or playable media files.
package library

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"ytmgo/internal/catalog"
)

type Playlist struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Tracks []catalog.Track `json:"tracks"`
}
type Library struct {
	Liked     []catalog.Track `json:"liked"`
	Playlists []Playlist      `json:"playlists"`
}
type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store { return &Store{path: path} }
func Empty() Library              { return Library{Liked: []catalog.Track{}, Playlists: []Playlist{}} }

func (s *Store) Load() (Library, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (Library, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return Library{}, errors.New("无法读取本地音乐库")
	}
	defer f.Close()
	if stat, statErr := f.Stat(); statErr != nil || stat.Size() > 4<<20 {
		return Library{}, errors.New("本地音乐库超过大小限制，原文件已保留")
	}
	var value Library
	decoder := json.NewDecoder(io.LimitReader(f, 4<<20))
	if err = decoder.Decode(&value); err != nil {
		return Library{}, errors.New("本地音乐库格式异常，原文件已保留")
	}
	if value.Liked == nil || value.Playlists == nil {
		return Library{}, errors.New("本地音乐库缺少收藏或歌单列表，原文件已保留")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return Library{}, errors.New("本地音乐库格式异常，原文件已保留")
	}
	if err = validate(&value); err != nil {
		return Library{}, err
	}
	return value, nil
}

func (s *Store) Save(value Library) error {
	if err := validate(&value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.New("无法保存音乐库")
	}
	if len(data) > 4<<20 {
		return errors.New("音乐库过大，请减少收藏或歌单数量")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.loadLocked(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return errors.New("无法创建音乐库目录")
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".library-*.tmp")
	if err != nil {
		return errors.New("无法写入音乐库")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.path)
	}
	if err != nil {
		return errors.New("保存音乐库失败，之前的文件已保留")
	}
	return nil
}

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func validate(value *Library) error {
	if value.Liked == nil {
		value.Liked = []catalog.Track{}
	}
	if value.Playlists == nil {
		value.Playlists = []Playlist{}
	}
	if len(value.Liked) > 5000 || len(value.Playlists) > 100 {
		return errors.New("收藏或歌单数量超过上限")
	}
	tracks := append([]catalog.Track{}, value.Liked...)
	seen := map[string]bool{}
	for i := range value.Playlists {
		p := &value.Playlists[i]
		p.Name = strings.TrimSpace(p.Name)
		if p.ID == "" || len(p.ID) > 100 || seen[p.ID] || p.Name == "" || len([]rune(p.Name)) > 80 || len(p.Tracks) > 1000 {
			return errors.New("歌单名称、标识或数量无效")
		}
		seen[p.ID] = true
		if p.Tracks == nil {
			p.Tracks = []catalog.Track{}
		}
		tracks = append(tracks, p.Tracks...)
	}
	for _, track := range tracks {
		if !videoID.MatchString(track.ID) || strings.TrimSpace(track.Title) == "" || len(track.Title) > 1000 || len(track.Artist) > 1000 || len(track.Album) > 1000 {
			return errors.New("歌曲信息无效")
		}
		if track.Thumbnail != "" {
			u, err := url.Parse(track.Thumbnail)
			if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
				return errors.New("封面链接无效")
			}
			h := strings.ToLower(u.Hostname())
			if h != "ytimg.com" && !strings.HasSuffix(h, ".ytimg.com") && h != "googleusercontent.com" && !strings.HasSuffix(h, ".googleusercontent.com") && h != "ggpht.com" && !strings.HasSuffix(h, ".ggpht.com") {
				return errors.New("封面来源无效")
			}
		}
	}
	return nil
}
