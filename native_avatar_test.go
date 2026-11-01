package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeAccountAvatarImageAndMenu(t *testing.T) {
	for _, width := range []int{420, 760, 1100, 1440} {
		for _, appearance := range []string{"dark", "light"} {
			t.Run(fmt.Sprintf("%d-%s", width, appearance), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.SignedIn, m.MusicPremium, m.Appearance = true, true, appearance
				m.AccountAvatar = "synthetic-avatar"
				avatarColor := color.RGBA{215, 61, 163, 255}
				photo := image.NewRGBA(image.Rect(0, 0, 64, 64))
				draw.Draw(photo, photo.Bounds(), image.NewUniform(avatarColor), image.Point{}, draw.Src)
				bitmap := ui.NewBitmap(photo)
				loaded := false
				originalArtwork := a.Artwork
				a.Artwork = func(src string, edge int) ui.ImageSource {
					if src == m.AccountAvatar {
						if edge < 64 {
							t.Error("avatar resolution is too low for Retina")
						}
						if loaded {
							return bitmap
						}
						return nil
					}
					return originalArtwork(src, edge)
				}
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
				label := a.Tr("menuAccount") + " · " + nativeAccountStatus(m, a)
				r := nativeAssertVisible(t, tt, label, width, 900)
				x, y := int(r.X+r.W-24), int(r.Y+24)
				if color.RGBAModel.Convert(tt.Image().At(x, y)) == avatarColor {
					t.Fatal("unloaded avatar did not use the placeholder")
				}
				loaded = true
				tt.Frame()
				if color.RGBAModel.Convert(tt.Image().At(x, y)) != avatarColor {
					t.Fatal("loaded avatar not painted in the top-right account button")
				}
				if color.RGBAModel.Convert(tt.Image().At(x-15, y-15)) == avatarColor {
					t.Fatal("avatar was not clipped to a circle")
				}
				nativeSnapshot(t, tt, fmt.Sprintf("avatar-%d-%s", width, appearance))
				signOuts := 0
				a.SignOut = func() { signOuts++ }
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(a.Tr("shellSignOut")); err != nil || signOuts != 1 {
					t.Fatal("avatar prevented account menu interaction")
				}
			})
		}
	}
}
