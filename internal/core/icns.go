package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
)

// makeICNS builds a minimal valid .icns containing PNG renditions of the
// source image at the sizes macOS needs for Finder/Dock/Launchpad. The
// source artwork itself is never redrawn — only resampled.
func makeICNS(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, errors.New("selected icon cannot be decoded for bundle use")
	}
	// OSType -> pixel size (PNG-in-icns renditions).
	renditions := []struct {
		typ  string
		size int
	}{
		{"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic13", 256}, {"ic14", 512},
	}
	var body bytes.Buffer
	for _, r := range renditions {
		scaled := scaleImage(img, r.size)
		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, scaled); err != nil {
			return nil, err
		}
		body.WriteString(r.typ)
		_ = binary.Write(&body, binary.BigEndian, uint32(8+pngBuf.Len()))
		body.Write(pngBuf.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func scaleImage(src image.Image, size int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := b.Min.X + x*w/size
			sy := b.Min.Y + y*h/size
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
