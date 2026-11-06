package main

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestNativeContinuationTrackNumbersStayContinuous(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, route := range []string{"browse:VLplaylist", "browse:MPRE_album"} {
			t.Run(fmt.Sprintf("%s/compact=%v", route, compact), func(t *testing.T) {
				m, actions := nativeViewFixture()
				m.Route, m.Current = route, catalog.Item{}
				tracks := make([]catalog.Item, 6)
				for i := range tracks {
					tracks[i] = catalog.Item{ID: fmt.Sprintf("track-%d", i), Title: fmt.Sprintf("Track %d", i+1), Kind: "song"}
				}
				m.Page = catalog.Page{Shelves: []catalog.Shelf{{Items: tracks[:2]}}}
				var played catalog.Item
				actions.Play = func(item catalog.Item) { played = item }
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Column(c).Children(func() { nativePageBody(c, m, actions, palette(true), compact, 1280) })
				}, 1280, 1500)
				assertNumbers := func(want []int) {
					t.Helper()
					var got []int
					for _, text := range tt.Texts() {
						if number, err := strconv.Atoi(text); err == nil {
							got = append(got, number)
						}
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("displayed track numbers %v, want %v", got, want)
					}
				}
				assertNumbers([]int{1, 2})
				// Continuations append shelves; an empty filtered page must not
				// restart or advance the visible row numbering.
				m.Page.Shelves = append(m.Page.Shelves, catalog.Shelf{}, catalog.Shelf{Items: tracks[2:5]}, catalog.Shelf{Items: tracks[5:]})
				for range 2 {
					tt.Frame()
					assertNumbers([]int{1, 2, 3, 4, 5, 6})
				}
				if err := tt.Click(nativeItemLabel(tracks[5])); err != nil || played.ID != tracks[5].ID {
					t.Fatal("continuation row numbering changed the playback target")
				}
				// Recommendations remain an independent, unnumbered shelf;
				// a subsequent table starts its own numbering sequence.
				m.Page.Shelves = append(m.Page.Shelves,
					catalog.Shelf{Title: "Suggestions", Items: []catalog.Item{{ID: "suggestion", Title: "Suggestion", Kind: "song"}}},
					catalog.Shelf{Items: []catalog.Item{{ID: "other", Title: "Other table", Kind: "song"}}})
				tt.Move(-1, -1)
				tt.Frame()
				assertNumbers([]int{1, 2, 3, 4, 5, 6, 1})
			})
		}
	}
}

func TestNativeNumberedTracksFitFourDigits(t *testing.T) {
	for _, compact := range []bool{false, true} {
		m, actions := nativeViewFixture()
		m.Current = catalog.Item{}
		tt := ui.NewTester(func(c *ui.Context) {
			nativeTrackList(c, []catalog.Item{{ID: "four-digits", Title: "Continued track", Kind: "song"}}, m, actions, palette(true), compact, true, 999)
		}, 760, 150)
		number, ok := tt.Find("1000")
		width, height := ui.ShapeText("1000", ui.Font{Size: 15}, 0).Size()
		if !ok || number.W < width || number.H > height+1 {
			t.Fatalf("compact=%v: four-digit row number clipped or wrapped: %v, need %.1fx%.1f", compact, number, width, height)
		}
	}
}
