package launchers

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os/exec"
)

// ToPNG decodes a PNG, JPEG, GIF or PNG-compressed ICO into a PNG.
// SVG is rasterised only when rsvg-convert is installed; otherwise it is
// refused rather than guessed.
func ToPNG(raw []byte) ([]byte, error) {
	if len(raw) >= 8 && bytes.Equal(raw[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		if _, err := png.DecodeConfig(bytes.NewReader(raw)); err != nil {
			return nil, err
		}
		return raw, nil
	}
	if isICO(raw) {
		return icoToPNG(raw)
	}
	if len(raw) >= 5 && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("<svg")) || bytes.Contains(raw[:min(256, len(raw))], []byte("<svg")) {
		return svgToPNG(raw)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("icon is not a PNG, JPEG, GIF or ICO")
	}
	return encodePNG(img)
}

// EncodeICO builds a Windows icon containing 16, 32, 48 and 256 pixel PNG
// images. Vista and later accept PNG frames, so the icon does not need a
// BMP encoder and the alpha channel survives.
func EncodeICO(pngBytes []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	sizes := []int{16, 32, 48, 256}
	frames := make([][]byte, len(sizes))
	for i, size := range sizes {
		frame, err := encodePNG(scale(img, size))
		if err != nil {
			return nil, err
		}
		frames[i] = frame
	}
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint16(0))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(len(frames)))
	offset := 6 + 16*len(frames)
	for i, size := range sizes {
		dim := byte(size)
		if size >= 256 {
			dim = 0
		}
		b.WriteByte(dim)
		b.WriteByte(dim)
		b.WriteByte(0)
		b.WriteByte(0)
		_ = binary.Write(&b, binary.LittleEndian, uint16(1))
		_ = binary.Write(&b, binary.LittleEndian, uint16(32))
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(frames[i])))
		_ = binary.Write(&b, binary.LittleEndian, uint32(offset))
		offset += len(frames[i])
	}
	for _, frame := range frames {
		b.Write(frame)
	}
	return b.Bytes(), nil
}

func isICO(b []byte) bool {
	return len(b) > 6 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0
}

func icoToPNG(b []byte) ([]byte, error) {
	if len(b) < 6 {
		return nil, errors.New("icon file is truncated")
	}
	count := int(binary.LittleEndian.Uint16(b[4:6]))
	best := []byte{}
	bestArea := 0
	for i := 0; i < count; i++ {
		off := 6 + 16*i
		if off+16 > len(b) {
			return nil, errors.New("icon directory is truncated")
		}
		size := int(binary.LittleEndian.Uint32(b[off+8 : off+12]))
		at := int(binary.LittleEndian.Uint32(b[off+12 : off+16]))
		if size < 8 || at < 0 || at+size > len(b) {
			continue
		}
		frame := b[at : at+size]
		if !bytes.HasPrefix(frame, []byte{0x89, 'P', 'N', 'G'}) {
			continue
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(frame))
		if err != nil {
			continue
		}
		if cfg.Width*cfg.Height > bestArea {
			best = frame
			bestArea = cfg.Width * cfg.Height
		}
	}
	if best == nil {
		return nil, errors.New("icon file has no PNG image IconCraft can use")
	}
	return best, nil
}

func svgToPNG(raw []byte) ([]byte, error) {
	lower := bytes.ToLower(raw)
	for _, bad := range [][]byte{[]byte("<script"), []byte("onload"), []byte("foreignobject"), []byte("<!entity"), []byte("javascript:")} {
		if bytes.Contains(lower, bad) {
			return nil, errors.New("SVG icon contains active content")
		}
	}
	path, err := exec.LookPath("rsvg-convert")
	if err != nil {
		return nil, errors.New("SVG icons need rsvg-convert, which is not installed")
	}
	cmd := exec.Command(path, "--format", "png")
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, errors.New("could not rasterise the SVG icon")
	}
	if _, err := png.DecodeConfig(bytes.NewReader(out)); err != nil {
		return nil, err
	}
	return out, nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func scale(src image.Image, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sb := src.Bounds()
	if sb.Dx() == size && sb.Dy() == size {
		draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Src)
		return dst
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dst.SetNRGBA(x, y, box(src, x, y, size))
		}
	}
	return dst
}

func box(src image.Image, x, y, size int) color.NRGBA {
	sb := src.Bounds()
	x0 := sb.Min.X + x*sb.Dx()/size
	x1 := sb.Min.X + (x+1)*sb.Dx()/size
	y0 := sb.Min.Y + y*sb.Dy()/size
	y1 := sb.Min.Y + (y+1)*sb.Dy()/size
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	var r, g, b, a, n uint32
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			cr, cg, cb, ca := src.At(xx, yy).RGBA()
			r += cr
			g += cg
			b += cb
			a += ca
			n++
		}
	}
	if n == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(b / n >> 8), uint8(a / n >> 8)}
}
