package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"ytmgo/internal/appearance"
	"ytmgo/internal/catalog"
	"ytmgo/internal/library"
)

// Service is the only native API exposed to ytmgo's own local UI.
// Remote player frames and account pages are never trusted IPC origins.
type Service struct {
	appearancePath string
	appearanceMu   sync.Mutex
	catalog        *catalog.Client
	library        *library.Store
	mu             sync.Mutex
	home           HomeResponse
	homeAt         time.Time
}

func (s *Service) Appearance() string {
	s.appearanceMu.Lock()
	defer s.appearanceMu.Unlock()
	return appearance.Load(s.appearancePath)
}

func (s *Service) SetAppearance(value string) (string, error) {
	s.appearanceMu.Lock()
	defer s.appearanceMu.Unlock()
	if err := appearance.Save(s.appearancePath, value); err != nil {
		return "", errors.New("外观设置未能保存，请稍后重试")
	}
	mygo.Theme.SetSource(mygo.ThemeSource(value))
	return value, nil
}

type SearchResponse struct {
	Tracks []catalog.Track `json:"tracks"`
}
type Section struct {
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle"`
	Tracks   []catalog.Track `json:"tracks"`
}
type HomeResponse struct {
	Sections []Section `json:"sections"`
}

func (s *Service) Search(ctx context.Context, query string) (SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > 160 {
		return SearchResponse{}, errors.New("请输入 1–160 字的歌曲、歌手或专辑名称")
	}
	tracks, err := s.catalog.Search(ctx, query)
	if err != nil {
		return SearchResponse{}, errors.New("暂时无法连接 YouTube Music，请检查网络后重试")
	}
	if tracks == nil {
		tracks = []catalog.Track{}
	}
	return SearchResponse{Tracks: tracks}, nil
}

func (s *Service) Home(ctx context.Context) (HomeResponse, error) {
	s.mu.Lock()
	if time.Since(s.homeAt) < 10*time.Minute && len(s.home.Sections) > 0 {
		result := s.home
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	choices := []struct{ title, subtitle, query string }{
		{"把节奏放慢一点", "CHILL & UNWIND · 轻松听", "chill indie soul songs"},
		{"熟悉的旋律", "MANDOPOP · 华语流行", "华语流行 周杰伦 陈奕迅 孙燕姿"},
		{"留一点时间给自己", "JAZZ & COFFEE · 爵士时光", "jazz bossa nova songs"},
	}
	sections := make([]Section, len(choices))
	var wg sync.WaitGroup
	for i, c := range choices {
		wg.Go(func() {
			tracks, err := s.catalog.Search(ctx, c.query)
			if err != nil {
				return
			}
			if len(tracks) > 8 {
				tracks = tracks[:8]
			}
			if len(tracks) > 0 {
				sections[i] = Section{Title: c.title, Subtitle: c.subtitle, Tracks: tracks}
			}
		})
	}
	wg.Wait()
	result := HomeResponse{Sections: []Section{}}
	for _, section := range sections {
		if len(section.Tracks) > 0 {
			result.Sections = append(result.Sections, section)
		}
	}
	if len(result.Sections) == 0 {
		return result, errors.New("首页暂时无法加载，请检查网络后重试，或搜索一首你想听的歌")
	}
	s.mu.Lock()
	s.home = result
	s.homeAt = time.Now()
	s.mu.Unlock()
	return result, nil
}

func (s *Service) Library() (library.Library, error) { return s.library.Load() }
func (s *Service) SaveLibrary(value library.Library) (library.Library, error) {
	if err := s.library.Save(value); err != nil {
		return library.Library{}, err
	}
	return s.library.Load()
}
