package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// IconFitVersion marks icon records whose artwork has been placed on the
// standard macOS icon grid. Older records are migrated once by Reconcile.
const IconFitVersion = 1

const (
	macOSIconCanvas = 1024
	macOSIconBody   = 824 // visible body of a native macOS app icon
)

// iconAlphaBounds returns the bounding box of meaningfully opaque pixels.
func iconAlphaBounds(img image.Image) (image.Rectangle, bool) {
	b := img.Bounds()
	if b.Empty() {
		return image.Rectangle{}, false
	}
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a <= 0x0800 {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
		}
	}
	if !found {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX, maxY), true
}

// needsMacOSFit identifies a *finished, full-bleed* icon: publisher/App
// Store artwork that runs essentially edge-to-edge. On macOS that renders
// about 20% larger than native app icons, whose visible body occupies
// ~824 of the 1024 icon canvas. Artwork that already carries transparent
// padding (a genuine macOS icon, or a glyph on transparency) is left
// exactly as designed — shrinking it again would make it too small.
func needsMacOSFit(img image.Image) bool {
	bounds, ok := iconAlphaBounds(img)
	if !ok {
		return false
	}
	full := img.Bounds()
	fillW := float64(bounds.Dx()) / float64(full.Dx())
	fillH := float64(bounds.Dy()) / float64(full.Dy())
	return fillW >= 0.93 && fillH >= 0.90
}

// fitMacOSIcon places a full-bleed icon onto the native macOS icon grid:
// the whole artwork is scaled (aspect preserved) into an 824px box centred
// on a transparent 1024px canvas. Icons that are not full-bleed pass
// through unchanged.
func fitMacOSIcon(img image.Image) image.Image {
	if !needsMacOSFit(img) {
		return img
	}
	b := img.Bounds()
	scale := math.Min(float64(macOSIconBody)/float64(b.Dx()), float64(macOSIconBody)/float64(b.Dy()))
	dw := max(1, int(math.Round(float64(b.Dx())*scale)))
	dh := max(1, int(math.Round(float64(b.Dy())*scale)))
	canvas := image.NewRGBA(image.Rect(0, 0, macOSIconCanvas, macOSIconCanvas))
	scaled := resizeImage(img, dw, dh)
	drawImageAt(canvas, scaled, (macOSIconCanvas-dw)/2, (macOSIconCanvas-dh)/2)
	return canvas
}

// fitIconPNG returns a macOS-grid PNG for a full-bleed source. The second
// result reports whether the bytes differ from the input.
func fitIconPNG(src []byte) ([]byte, bool, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, false, err
	}
	if !needsMacOSFit(img) {
		return src, false, nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, fitMacOSIcon(img)); err != nil {
		return nil, false, err
	}
	return out.Bytes(), true, nil
}

// fittedIconTemp materialises the icon file that should actually be handed
// to macOS for an asset. Stored assets keep their original artwork; only
// the applied copy is placed on the macOS grid.
func fittedIconTemp(dir, assetPath string) (string, func(), error) {
	raw, err := ReadLimited(assetPath, MaxIconBytes)
	if err != nil {
		return "", nil, err
	}
	fitted, changed, err := fitIconPNG(raw)
	if err != nil {
		return "", nil, fmt.Errorf("selected icon cannot be prepared at macOS scale: %w", err)
	}
	if !changed {
		return assetPath, func() {}, nil
	}
	f, err := os.CreateTemp(dir, ".iconcraft-fitted-*.png")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err = f.Write(fitted); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return name, cleanup, nil
}

func (e *Engine) assetNeedsFit(assetID string) (bool, error) {
	p, err := e.AssetPath(assetID)
	if err != nil {
		return false, err
	}
	raw, err := ReadLimited(p, MaxIconBytes)
	if err != nil {
		return false, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return false, err
	}
	return needsMacOSFit(img), nil
}

func (e *Engine) markIconFit(recordID string, fullSkin bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if fullSkin {
		for i := range e.State.Rebrands {
			if e.State.Rebrands[i].ID == recordID {
				e.State.Rebrands[i].IconFit = IconFitVersion
				return e.save()
			}
		}
		return errors.New("full-skin record not found")
	}
	for i := range e.State.Records {
		if e.State.Records[i].ID == recordID {
			e.State.Records[i].IconFit = IconFitVersion
			return e.save()
		}
	}
	return errors.New("icon record not found")
}

// refitFinderRecord rewrites one already-applied Finder skin at macOS
// scale, in place: no new journal layer is stacked, and the write only
// happens when the current icon is still byte-identical to the skin this
// record applied (a custom icon set elsewhere is never stomped).
func (e *Engine) refitFinderRecord(ctx context.Context, app App, recordID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx := -1
	for i := range e.State.Records {
		if e.State.Records[i].ID == recordID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("icon record not found")
	}
	rec := e.State.Records[idx]
	if !isLive(rec.Status) || rec.After == nil || rec.App.Path != app.Path {
		return errors.New("icon record is not the live skin for this app")
	}
	if fp, err := Fingerprint(app); err != nil || fp != rec.Fingerprint {
		return errors.New("app changed since the skin was applied")
	}
	current, err := e.Native.Snapshot(app.Path)
	if err != nil {
		return err
	}
	if !current.Equal(*rec.After) {
		return errors.New("current icon no longer matches this skin; leaving it alone")
	}
	if err := e.probeWritable(app); err != nil {
		return err
	}
	assetPath, err := e.assetPath(rec.AssetID)
	if err != nil {
		return err
	}
	fitPath, cleanup, err := fittedIconTemp(e.Dir, assetPath)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := e.Native.Set(ctx, app.Path, fitPath); err != nil {
		return err
	}
	after, err := e.Native.Snapshot(app.Path)
	if err != nil {
		return err
	}
	if !after.Custom() {
		return errors.New("macOS did not record the resized icon")
	}
	oldAfter, oldFit := rec.After, rec.IconFit
	e.State.Records[idx].After = &after
	e.State.Records[idx].IconFit = IconFitVersion
	if err := e.save(); err != nil {
		e.State.Records[idx].After, e.State.Records[idx].IconFit = oldAfter, oldFit
		_ = e.Native.Restore(context.Background(), app.Path, *oldAfter)
		return err
	}
	return nil
}

// refitRebrandRecord replaces one full skin's bundle icon with the same
// artwork at macOS scale and re-signs, updating the existing record. It
// refuses running or changed apps exactly like a fresh full skin.
func (e *Engine) refitRebrandRecord(ctx context.Context, app App, recordID string) error {
	t, err := e.tools()
	if err != nil {
		return err
	}
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	idx := -1
	for i := range e.State.Rebrands {
		if e.State.Rebrands[i].ID == recordID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("full-skin record not found")
	}
	rec := e.State.Rebrands[idx]
	if rec.Status != "applied" || rec.App.Path != app.Path {
		return errors.New("full-skin record is not the live skin for this app")
	}
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, err := runner.Running(ctx, app.Path)
		if err != nil {
			return err
		}
		if running {
			return errors.New("app is running; its icon will be resized after it quits")
		}
	}
	if fp, err := Fingerprint(app); err != nil || fp != rec.AfterPrint {
		return errors.New("app changed since the full skin was applied")
	}
	if err := e.probeWritable(app); err != nil {
		return err
	}
	iconRel := rec.IconFile
	if iconRel == "" || iconRel != filepath.Base(iconRel) {
		return errors.New("recorded bundle icon path is unsafe")
	}
	assetPath, err := e.assetPath(rec.AssetID)
	if err != nil {
		return err
	}
	pngBytes, err := ReadLimited(assetPath, MaxIconBytes)
	if err != nil {
		return err
	}
	icns, err := makeICNS(pngBytes)
	if err != nil {
		return err
	}
	iconPath := filepath.Join(app.Path, "Contents/Resources", iconRel)
	oldIcon, err := ReadLimited(iconPath, MaxIconBytes)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = AtomicWrite(iconPath, oldIcon, 0644)
		_ = t.Resign(ctx, app.Path)
		return cause
	}
	if err := AtomicWrite(iconPath, icns, 0644); err != nil {
		return fmt.Errorf("could not replace the bundle icon at macOS scale: %w", err)
	}
	if err := t.Resign(ctx, app.Path); err != nil {
		return rollback(err)
	}
	if status, err := e.Native.Signature(ctx, app.Path); err != nil || status != "valid" {
		return rollback(errors.New("re-signed bundle did not verify after resizing its icon"))
	}
	after, err := Fingerprint(app)
	if err != nil {
		return rollback(err)
	}
	current, err := ReadLimited(iconPath, MaxIconBytes)
	if err != nil {
		return rollback(err)
	}
	e.State.Rebrands[idx].AfterPrint = after
	e.State.Rebrands[idx].AfterIconHash = hash(current)
	e.State.Rebrands[idx].IconFit = IconFitVersion
	if err := e.save(); err != nil {
		e.State.Rebrands[idx].AfterPrint = rec.AfterPrint
		e.State.Rebrands[idx].AfterIconHash = rec.AfterIconHash
		e.State.Rebrands[idx].IconFit = rec.IconFit
		return rollback(err)
	}
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, app.Path)
	}
	return nil
}

func drawImageAt(dst *image.RGBA, src image.Image, offsetX, offsetY int) {
	b := src.Bounds()
	draw.Draw(dst, image.Rect(offsetX, offsetY, offsetX+b.Dx(), offsetY+b.Dy()), src, b.Min, draw.Src)
}

// resizeImage resamples with bilinear interpolation on premultiplied
// channels (no transparent-edge halos). It replaces nearest-neighbour
// scaling for every icon rendition IconCraft generates.
func resizeImage(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	b := src.Bounds()
	if b.Empty() || width <= 0 || height <= 0 {
		return dst
	}
	// Materialise once so sampling reads pixels directly instead of
	// paying an interface conversion for every tap.
	materialised := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(materialised, materialised.Bounds(), src, b.Min, draw.Src)
	sw, sh := b.Dx(), b.Dy()
	sample := func(x, y int) (uint8, uint8, uint8, uint8) {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= sw {
			x = sw - 1
		}
		if y >= sh {
			y = sh - 1
		}
		i := materialised.PixOffset(x, y)
		p := materialised.Pix[i:]
		return p[0], p[1], p[2], p[3]
	}
	for dy := 0; dy < height; dy++ {
		sy := (float64(dy)+0.5)*float64(sh)/float64(height) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		for dx := 0; dx < width; dx++ {
			sx := (float64(dx)+0.5)*float64(sw)/float64(width) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			r00, g00, b00, a00 := sample(x0, y0)
			r10, g10, b10, a10 := sample(x0+1, y0)
			r01, g01, b01, a01 := sample(x0, y0+1)
			r11, g11, b11, a11 := sample(x0+1, y0+1)
			mix := func(v00, v10, v01, v11 uint8) uint8 {
				top := float64(v00)*(1-fx) + float64(v10)*fx
				bottom := float64(v01)*(1-fx) + float64(v11)*fx
				return uint8(math.Round(top*(1-fy) + bottom*fy))
			}
			i := dst.PixOffset(dx, dy)
			dst.Pix[i+0] = mix(r00, r10, r01, r11)
			dst.Pix[i+1] = mix(g00, g10, g01, g11)
			dst.Pix[i+2] = mix(b00, b10, b01, b11)
			dst.Pix[i+3] = mix(a00, a10, a01, a11)
		}
	}
	return dst
}
