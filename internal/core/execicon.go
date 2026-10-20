package core

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

// restoreExecIconPatches writes the backed-up original embedded-icon
// bytes back into the executable and verifies each region by hash.
func restoreExecIconPatches(exePath string, patches []ExecIconPatch, backupDir string) error {
	if len(patches) == 0 {
		return nil
	}
	raw, err := os.ReadFile(exePath)
	if err != nil {
		return err
	}
	out := append([]byte(nil), raw...)
	for i, p := range patches {
		orig, err := os.ReadFile(filepath.Join(backupDir, fmt.Sprintf("exec-patch-%d.bin", i)))
		if err != nil {
			return fmt.Errorf("runtime-icon backup is missing: %w", err)
		}
		if len(orig) != p.Length || hash(orig) != p.SHA256 {
			return errors.New("runtime-icon backup does not match the recorded patch")
		}
		if p.Offset < 0 || p.Offset+int64(p.Length) > int64(len(out)) {
			return errors.New("recorded icon patch lies outside the executable")
		}
		copy(out[p.Offset:p.Offset+int64(p.Length)], orig)
	}
	info, err := os.Stat(exePath)
	if err != nil {
		return err
	}
	if err := AtomicWrite(exePath, out, info.Mode().Perm()); err != nil {
		return err
	}
	check, err := os.ReadFile(exePath)
	if err != nil {
		return err
	}
	for _, p := range patches {
		if hash(check[p.Offset:p.Offset+int64(p.Length)]) != p.SHA256 {
			return errors.New("restored runtime icon did not verify")
		}
	}
	return nil
}

// Some apps (notably Rust/eframe clones) ignore their bundle .icns while
// running: at startup they call setApplicationIconImage with artwork
// embedded in the executable, so the Dock flips back to the original
// icon on launch no matter what the bundle says. The only safe way to
// skin the *running* icon is to replace that embedded PNG in place —
// same byte length, so nothing else in the binary moves — and re-sign,
// which a full skin already does. The original bytes are backed up and
// restored byte-exactly by RestoreRebrand.
//
// Detection is deliberately conservative: an embedded PNG is only
// treated as the runtime icon when its pixels match the app's original
// bundle icon. Anything else in the binary is never touched.

const execScanLimit = 768 << 20 // executables larger than this are not scanned

// ExecIconCheckVersion is the current runtime-icon inspection version.
// Bump when matching improves so keepers re-check earlier verdicts.
const ExecIconCheckVersion = 3 // v3: refresh patched icons with squircle-clipped artwork

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// setsOwnRuntimeIcon reports whether the executable references the
// macOS call apps use to override their running icon.
func setsOwnRuntimeIcon(exe []byte) bool {
	return bytes.Contains(exe, []byte("setApplicationIconImage"))
}

type embeddedPNG struct {
	offset int
	data   []byte // exact PNG byte stream, IHDR..IEND
	width  int
	height int
}

// scanEmbeddedPNGs finds complete, decodable PNG streams embedded in a
// binary. It parses chunk structure instead of trusting signatures, so
// random byte coincidences are rejected.
func scanEmbeddedPNGs(data []byte) []embeddedPNG {
	var out []embeddedPNG
	for start := 0; start+len(pngSignature) <= len(data); {
		i := bytes.Index(data[start:], pngSignature)
		if i < 0 {
			break
		}
		off := start + i
		start = off + len(pngSignature)
		if png, ok := parseEmbeddedPNG(data, off); ok {
			out = append(out, png)
			start = off + len(png.data)
		}
	}
	return out
}

func parseEmbeddedPNG(data []byte, off int) (embeddedPNG, bool) {
	pos := off + len(pngSignature)
	width, height := 0, 0
	for pos+8 <= len(data) {
		length := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		if length < 0 || pos+12+length > len(data) {
			return embeddedPNG{}, false
		}
		typ := string(data[pos+4 : pos+8])
		if typ == "IHDR" && length >= 8 {
			p := data[pos+8:]
			width = int(p[0])<<24 | int(p[1])<<16 | int(p[2])<<8 | int(p[3])
			height = int(p[4])<<24 | int(p[5])<<16 | int(p[6])<<8 | int(p[7])
		}
		if typ == "IEND" {
			end := pos + 12 + length
			stream := data[off:end]
			img, err := png.Decode(bytes.NewReader(stream))
			if err != nil || width <= 0 || height <= 0 {
				return embeddedPNG{}, false
			}
			_ = img
			return embeddedPNG{offset: off, data: append([]byte(nil), stream...), width: width, height: height}, true
		}
		pos += 12 + length
	}
	return embeddedPNG{}, false
}

// iconSimilarity compares two icons at small size; true when they are
// visibly the same artwork (mean per-channel difference tiny). Alpha is
// compared too so a glyph on transparency cannot match a full-bleed
// photo by colour alone.
// shrinkForCompare reduces an image to comparison size by repeated
// halving, which approximates area averaging. A single bilinear jump
// from 1024 to 48 point-samples instead and aliases detailed artwork
// (engravings, photos) into noise that no longer matches itself.
func shrinkForCompare(img image.Image, size int) *image.RGBA {
	cur := resizeImage(img, img.Bounds().Dx(), img.Bounds().Dy())
	for cur.Bounds().Dx() > size*2 {
		cur = resizeImage(cur, cur.Bounds().Dx()/2, cur.Bounds().Dy()/2)
	}
	return resizeImage(cur, size, size)
}

func iconSimilarity(a, b image.Image) bool {
	const size = 48
	ra := shrinkForCompare(a, size)
	rb := shrinkForCompare(b, size)
	var total uint64
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			ia := ra.PixOffset(x, y)
			ib := rb.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				d := int(ra.Pix[ia+c]) - int(rb.Pix[ib+c])
				if d < 0 {
					d = -d
				}
				total += uint64(d)
			}
		}
	}
	mean := float64(total) / float64(size*size*4)
	return mean <= 10
}

// icnsImage decodes the largest PNG rendition inside .icns bytes.
func icnsImage(icns []byte) (image.Image, error) {
	if len(icns) < 8 || string(icns[:4]) != "icns" {
		return nil, errors.New("not an icns")
	}
	var best []byte
	for pos := 8; pos+8 <= len(icns); {
		length := int(icns[pos+4])<<24 | int(icns[pos+5])<<16 | int(icns[pos+6])<<8 | int(icns[pos+7])
		if length < 8 || pos+length > len(icns) {
			break
		}
		payload := icns[pos+8 : pos+length]
		if bytes.HasPrefix(payload, pngSignature) && len(payload) > len(best) {
			best = payload
		}
		pos += length
	}
	if best == nil {
		return nil, errors.New("icns has no PNG rendition")
	}
	return png.Decode(bytes.NewReader(best))
}

// encodeIconPNG renders asset artwork fitted to the macOS grid at the
// requested pixel size and PNG-encodes it. Callers shrink the size until
// it fits a byte budget; dimensions of the embedded stream may change
// freely because decoders read the PNG header, not a fixed size.
func encodeIconPNG(asset image.Image, size int) ([]byte, error) {
	fitted := fitMacOSIcon(asset)
	var buf bytes.Buffer
	if err := png.Encode(&buf, resizeImage(fitted, size, size)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// planExecIconPatches inspects executable bytes and decides the byte
// replacements that swap the app's runtime icon for the chosen asset.
// It returns the patches, the original byte regions and the replacement
// streams (parallel slices, for backup/rollback), and whether the app
// sets its own running icon at all. Nothing is written here.
func planExecIconPatches(exe []byte, originalIcon, asset image.Image) ([]ExecIconPatch, [][]byte, [][]byte, bool) {
	setsOwn := setsOwnRuntimeIcon(exe)
	if originalIcon == nil || asset == nil {
		return nil, nil, nil, setsOwn
	}
	var patches []ExecIconPatch
	var originals [][]byte
	var replacements [][]byte
	for _, emb := range scanEmbeddedPNGs(exe) {
		if emb.width < 64 || emb.height < 64 || emb.width != emb.height {
			continue // too small or not icon-shaped; leave it alone
		}
		img, err := png.Decode(bytes.NewReader(emb.data))
		if err != nil || !iconSimilarity(img, originalIcon) {
			continue
		}
		replacement := []byte(nil)
		for size := emb.width; size >= 32; size /= 2 {
			candidate, err := encodeIconPNG(asset, size)
			if err == nil && len(candidate) <= len(emb.data) {
				replacement = candidate
				break
			}
		}
		if replacement == nil {
			continue // cannot fit without moving bytes; never force it
		}
		patches = append(patches, ExecIconPatch{Offset: int64(emb.offset), Length: len(emb.data), SHA256: hash(emb.data)})
		originals = append(originals, emb.data)
		replacements = append(replacements, replacement)
	}
	return patches, originals, replacements, setsOwn
}

// applyExecIconPatches writes planned patches into the executable at
// exePath. Replacement streams are zero-padded to the original length
// so the file size and every other offset stay identical. The write is
// atomic (temp file + rename in the same directory).
func applyExecIconPatches(exePath string, exe []byte, patches []ExecIconPatch, replacements [][]byte) error {
	if len(patches) == 0 {
		return nil
	}
	if len(patches) != len(replacements) {
		return errors.New("icon patch plan is inconsistent")
	}
	out := append([]byte(nil), exe...)
	for i, p := range patches {
		if p.Offset < 0 || p.Length <= 0 || p.Offset+int64(p.Length) > int64(len(out)) {
			return errors.New("icon patch lies outside the executable")
		}
		region := out[p.Offset : p.Offset+int64(p.Length)]
		if hash(region) != p.SHA256 {
			return errors.New("executable changed since the icon patch was planned")
		}
		for j := range region {
			region[j] = 0
		}
		copy(region, replacements[i])
	}
	info, err := os.Stat(exePath)
	if err != nil {
		return err
	}
	return AtomicWrite(exePath, out, info.Mode().Perm())
}

// readExecutable loads an app executable for icon scanning/patching.
func readExecutable(app App) (string, []byte, error) {
	exePath := filepath.Join(app.Path, "Contents/MacOS", app.Executable)
	info, err := os.Stat(exePath)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > execScanLimit {
		return "", nil, errors.New("executable too large to scan for its runtime icon")
	}
	raw, err := os.ReadFile(exePath)
	if err != nil {
		return "", nil, err
	}
	return exePath, raw, nil
}

// decodeAssetImage loads a stored icon asset as an image.
func decodeAssetImage(assetPath string) (image.Image, error) {
	raw, err := ReadLimited(assetPath, MaxIconBytes)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}
