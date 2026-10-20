package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func alphaFill(t *testing.T, img image.Image) float64 {
	t.Helper()
	b, ok := iconAlphaBounds(img)
	if !ok {
		t.Fatal("icon has no opaque pixels")
	}
	return float64(b.Dx()) / float64(img.Bounds().Dx())
}

func paddedPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	red := color.RGBA{200, 30, 40, 255}
	for y := 64; y < 192; y++ {
		for x := 64; x < 192; x++ {
			img.SetRGBA(x, y, red)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func icnsRendition(t *testing.T, b []byte, want string) image.Image {
	t.Helper()
	if len(b) < 8 || string(b[:4]) != "icns" {
		t.Fatal("not an icns container")
	}
	for off := 8; off+8 <= len(b); {
		size := int(binary.BigEndian.Uint32(b[off+4 : off+8]))
		if size < 8 || off+size > len(b) {
			t.Fatal("bad icns chunk")
		}
		if string(b[off:off+4]) == want {
			img, err := png.Decode(bytes.NewReader(b[off+8 : off+size]))
			if err != nil {
				t.Fatal(err)
			}
			return img
		}
		off += size
	}
	t.Fatalf("icns rendition %s not found", want)
	return nil
}

func TestFitIconPNGPlacesFullBleedArtworkOnMacOSGrid(t *testing.T) {
	fitted, changed, err := fitIconPNG(pngBytes(color.RGBA{200, 30, 40, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("full-bleed artwork must be fitted")
	}
	img, err := png.Decode(bytes.NewReader(fitted))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != macOSIconCanvas {
		t.Fatalf("fitted canvas is %d, want %d", img.Bounds().Dx(), macOSIconCanvas)
	}
	if fill := alphaFill(t, img); fill < 0.78 || fill > 0.83 {
		t.Fatalf("fitted artwork fills %.3f of canvas, want the ~0.805 macOS body", fill)
	}
}

func TestFitIconPNGClipsToSquircle(t *testing.T) {
	src := pngBytes(color.RGBA{30, 90, 200, 255}) // full-bleed square artwork
	out, changed, err := fitIconPNG(src)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	alpha := func(x, y int) uint32 {
		_, _, _, a := img.At(x, y).RGBA()
		return a
	}
	// Body occupies ~(100,100)-(924,924) of the 1024 canvas: corners of
	// that square must be transparent (rounded), edge midpoints opaque.
	if alpha(110, 110) != 0 || alpha(913, 913) != 0 {
		t.Fatal("squircle corners should be transparent")
	}
	if alpha(512, 110) == 0 || alpha(110, 512) == 0 || alpha(512, 512) == 0 {
		t.Fatal("squircle edge midpoints and centre should be opaque")
	}
}

func TestFitIconPNGLeavesPaddedArtworkUntouched(t *testing.T) {
	src := paddedPNG(t)
	fitted, changed, err := fitIconPNG(src)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !bytes.Equal(fitted, src) {
		t.Fatal("already-padded artwork must not be shrunk a second time")
	}
}

func TestMakeICNSFitsFullBleedArtworkToNativeScale(t *testing.T) {
	b, err := makeICNS(pngBytes(color.RGBA{200, 30, 40, 255}))
	if err != nil {
		t.Fatal(err)
	}
	img := icnsRendition(t, b, "ic09") // 512px rendition
	if fill := alphaFill(t, img); fill < 0.78 || fill > 0.83 {
		t.Fatalf("bundle icon fills %.3f of its canvas, want ~0.805", fill)
	}
}

func TestMakeICNSPreservesAlreadyPaddedArtwork(t *testing.T) {
	b, err := makeICNS(paddedPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	img := icnsRendition(t, b, "ic09")
	if fill := alphaFill(t, img); fill < 0.47 || fill > 0.53 {
		t.Fatalf("padded artwork changed scale: fills %.3f, want ~0.5", fill)
	}
}

func TestApplyAndReconcileMigrateIconsAtMacOSScale(t *testing.T) {
	e, n, a, asset := fixture(t)
	rec, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.IconFit != IconFitVersion {
		t.Fatal("new skins must record the current icon-fit version")
	}
	applied, err := png.Decode(bytes.NewReader(n.states[a.Path].Attributes["com.apple.ResourceFork"]))
	if err != nil {
		t.Fatal(err)
	}
	if fill := alphaFill(t, applied); fill < 0.78 || fill > 0.83 {
		t.Fatalf("applied Finder icon fills %.3f, want ~0.805", fill)
	}

	// Simulate a pre-fit skin: the raw full-bleed asset is on the app and
	// the record carries no fit version. Reconcile must refit it in place
	// (same record, no new layer).
	assetPath, err := e.AssetPath(asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Set(context.Background(), a.Path, assetPath); err != nil {
		t.Fatal(err)
	}
	snap, err := n.Snapshot(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	for i := range e.State.Records {
		if e.State.Records[i].ID == rec.ID {
			e.State.Records[i].After = &snap
			e.State.Records[i].IconFit = 0
		}
	}
	if err := e.save(); err != nil {
		e.mu.Unlock()
		t.Fatal(err)
	}
	e.mu.Unlock()

	res, err := e.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.IconFixed < 1 {
		t.Fatalf("reconcile did not refit the old skin: %+v", res)
	}
	st := e.SnapshotState()
	if len(st.Records) != 1 {
		t.Fatalf("refit must not stack a new record, got %d", len(st.Records))
	}
	if st.Records[0].IconFit != IconFitVersion {
		t.Fatal("refit record was not marked at the current fit version")
	}
	refitted, err := png.Decode(bytes.NewReader(n.states[a.Path].Attributes["com.apple.ResourceFork"]))
	if err != nil {
		t.Fatal(err)
	}
	if fill := alphaFill(t, refitted); fill < 0.78 || fill > 0.83 {
		t.Fatalf("refitted Finder icon fills %.3f, want ~0.805", fill)
	}
}
