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
// source image at the sizes macOS needs for Finder/Dock/Launchpad.
// Full-bleed artwork is first placed on the standard macOS icon grid
// (fitMacOSIcon), so a skinned app sits at the same visual scale as a
// native one in Finder, the Dock and Launchpad; already-padded artwork
// passes through untouched apart from resampling.
func makeICNS(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, errors.New("selected icon cannot be decoded for bundle use")
	}
	img = fitMacOSIcon(img)
	// OSType -> pixel size (PNG-in-icns renditions).
	renditions := []struct {
		typ  string
		size int
	}{
		{"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic13", 256}, {"ic14", 512},
	}
	var body bytes.Buffer
	encoded := map[int][]byte{}
	for _, r := range renditions {
		pngBytes, ok := encoded[r.size]
		if !ok {
			scaled := resizeImage(img, r.size, r.size)
			var pngBuf bytes.Buffer
			if err := png.Encode(&pngBuf, scaled); err != nil {
				return nil, err
			}
			pngBytes = pngBuf.Bytes()
			encoded[r.size] = pngBytes
		}
		body.WriteString(r.typ)
		_ = binary.Write(&body, binary.BigEndian, uint32(8+len(pngBytes)))
		body.Write(pngBytes)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}
