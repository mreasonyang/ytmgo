// Command login-icon converts the companion login artwork to native icon formats.
// Run from the repository root after updating the helper's artwork/icon.png. Conversion
// happens at development time; the helper needs no image decoder at runtime.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

func main() {
	if err := convert(); err != nil {
		panic(err)
	}
}

func convert() error {
	f, err := os.Open("native/account-login/artwork/icon.png")
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return err
	}
	resize := func(size int) *image.NRGBA {
		dst := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		return dst
	}
	raw := "native/account-login/assets/icon.rgba"
	if err := os.MkdirAll(filepath.Dir(raw), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(raw, resize(128).Pix, 0644); err != nil {
		return err
	}
	// PNG-backed ICNS entries, including Retina sizes. A 512px maximum is ample
	// for the transient Dock icon and avoids shipping the full source artwork.
	var body bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	for _, entry := range []struct {
		kind string
		size int
	}{{"ic11", 32}, {"ic12", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}} {
		var data bytes.Buffer
		if err := encoder.Encode(&data, resize(entry.size)); err != nil {
			return err
		}
		body.WriteString(entry.kind)
		_ = binary.Write(&body, binary.BigEndian, uint32(data.Len()+8))
		body.Write(data.Bytes())
	}
	var icon bytes.Buffer
	icon.WriteString("icns")
	_ = binary.Write(&icon, binary.BigEndian, uint32(body.Len()+8))
	icon.Write(body.Bytes())
	return os.WriteFile("scripts/login/macos/AppIcon.icns", icon.Bytes(), 0644)
}
