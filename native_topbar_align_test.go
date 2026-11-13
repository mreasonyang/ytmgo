package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The search field spans the player's transport column, right over it:
// one middle column from the top of the window to the bottom, whatever
// the width and whatever the window controls take at a side.
func TestSearchFieldLinesUpWithTheTransport(t *testing.T) {
	for _, controls := range []float32{0, 80} {
		for _, w := range []int{800, 1100, 1440, 1920} {
			t.Run(fmt.Sprint(w, "-", controls), func(t *testing.T) {
				m, a := nativeViewFixture()
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, w, 900)
				if controls > 0 {
					tt.SetTitleBar(ui.TitleBar{Height: nativeTopbarHeight, Left: controls})
				}
				tt.Frame()
				home, explore := rect(t, tt, a.Tr("shellNavHome")), rect(t, tt, a.Tr("shellNavExplore"))
				left, right := home.X+home.W+8, explore.X-8
				// Home and Explore stand either side: the group balances too.
				if mid := (home.X + explore.X + explore.W) / 2; math.Abs(float64(mid-(left+right)/2)) > 0.5 {
					t.Fatalf("Home %v and Explore %v aren't balanced about the field %.1f–%.1f", home, explore, left, right)
				}
				column := rect(t, tt, a.Tr("shellPlayerControls"))
				play := rect(t, tt, a.Tr("shellPause"))
				if math.Abs(float64((left+right)/2-(play.X+play.W/2))) > 0.5 || math.Abs(float64(right-left-nativeCenterColumn(float32(w)))) > 0.5 {
					t.Fatalf("search %.1f–%.1f, play centred at %.1f, column %.0f (player %v)", left, right, play.X+play.W/2, nativeCenterColumn(float32(w)), column)
				}
			})
		}
	}
}

// With the window controls on the right (Windows) and little room, the
// field narrows rather than leaving the middle or pushing the account out.
func TestSearchFieldNarrowsBesideWindowControls(t *testing.T) {
	m, a := nativeViewFixture()
	m.SignedIn = true
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 900, 900)
	tt.SetTitleBar(ui.TitleBar{Height: nativeTopbarHeight, Right: 138})
	tt.Frame()
	home, explore := rect(t, tt, a.Tr("shellNavHome")), rect(t, tt, a.Tr("shellNavExplore"))
	left, right := home.X+home.W+8, explore.X-8
	play := rect(t, tt, a.Tr("shellPause"))
	if math.Abs(float64((left+right)/2-(play.X+play.W/2))) > 0.5 {
		t.Fatalf("search %.1f–%.1f is off the play button's centre %.1f", left, right, play.X+play.W/2)
	}
	account := rect(t, tt, a.Tr("menuAccount")+" · "+nativeAccountStatus(m, a))
	if account.X+account.W > 900-138 {
		t.Fatalf("the account button %v runs under the window controls", account)
	}
}
