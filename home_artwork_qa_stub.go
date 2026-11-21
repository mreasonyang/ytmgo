//go:build !darwin || !ytmgo_artwork_qa

package main

import "github.com/egoist/mygo/ui"

type homeArtworkQA struct{}

func (*desktop) startHomeArtworkQA()                            {}
func (*desktop) prepareHomeArtworkQA(*ui.Context, *ViewActions) {}
func (*homeArtworkQA) observeInput(ui.Element)                  {}
func (*homeArtworkQA) trackPage(ui.Element, *ui.ScrollState) *homeArtworkQA {
	return nil
}
func (*homeArtworkQA) trackCover(ui.Element)         {}
func (*homeArtworkQA) paintedCover(ui.Element, bool) {}
func (*homeArtworkQA) manualInput()                  {}
