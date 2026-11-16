package core

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// A clone that sets its own Dock icon at launch embeds the original icon
// as a PNG inside the executable. A customisation must leave that binary
// alone: patching it and re-signing is what made updaters miss the app.
func TestRebrandLeavesEmbeddedRuntimeIconUntouched(t *testing.T) {
	e, _, a, asset := fixture(t)
	// Give the fixture bundle an existing .icns whose artwork matches an
	// embedded PNG inside the executable.
	icns, err := makeICNS(pngBytes(color.RGBA{200, 40, 30, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(a.Path, "Contents/Resources"), 0700); err != nil {
		t.Fatal(err)
	}
	iconPath := filepath.Join(a.Path, "Contents/Resources/AppIcon.icns")
	if err := os.WriteFile(iconPath, icns, 0644); err != nil {
		t.Fatal(err)
	}
	origImg, err := icnsImage(icns)
	if err != nil {
		t.Fatal(err)
	}
	var emb bytes.Buffer
	if err := png.Encode(&emb, resizeImage(origImg, 256, 256)); err != nil {
		t.Fatal(err)
	}
	exePath := filepath.Join(a.Path, "Contents/MacOS", a.Executable)
	exe := append([]byte("FAKE-EXE setApplicationIconImage: padding "), emb.Bytes()...)
	exe = append(exe, []byte(" trailing bytes")...)
	if err := os.WriteFile(exePath, exe, 0700); err != nil {
		t.Fatal(err)
	}
	before, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	// Scan again so the inventory carries the same executable identity.
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "external" || rec.RuntimeIcon != "" || len(rec.ExecPatches) != 0 {
		t.Fatalf("rebrand still describes an internal patch: %+v", rec)
	}
	patched, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(patched, exe) {
		t.Fatal("executable bytes changed")
	}
	if after, err := Fingerprint(a); err != nil || after != before {
		t.Fatalf("fingerprint changed: %v", err)
	}
	if len(e.SnapshotState().Rebrands) != 0 {
		t.Fatal("internal skin was journalled")
	}
}

// An embedded PNG that is *not* the app's icon must never be patched,
// and an app that never overrides its running icon reports "none".
func TestPlanExecIconPatchesLeavesOtherImagesAlone(t *testing.T) {
	origImg, err := icnsImage(mustICNS(t, color.RGBA{200, 40, 30, 255}))
	if err != nil {
		t.Fatal(err)
	}
	other := pngBytes(color.RGBA{20, 180, 60, 255})
	exe := append([]byte("plain executable "), other...)
	patches, _, _, setsOwn := planExecIconPatches(exe, origImg, origImg)
	if len(patches) != 0 {
		t.Fatalf("non-icon PNG must not be patched, got %d patches", len(patches))
	}
	if setsOwn {
		t.Fatal("no runtime-icon override present")
	}
}

// Detailed artwork (an engraving-style icon) must still match itself
// across sizes: comparison shrinks by halving, because a single
// point-sampled jump from 1024 to 48 aliases detail into noise — that
// exact failure once made the keeper report a patchable app as
// unpatchable.
func TestIconSimilarityMatchesDetailedArtwork(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			v := uint8((x*31 + y*17 + (x^y)*7) % 251)
			i := big.PixOffset(x, y)
			big.Pix[i+0] = v
			big.Pix[i+1] = uint8((int(v) + x/4) % 256)
			big.Pix[i+2] = uint8((int(v) + y/4) % 256)
			big.Pix[i+3] = 255
		}
	}
	small := resizeImage(big, 256, 256)
	if !iconSimilarity(small, big) {
		t.Fatal("detailed artwork should match itself across sizes")
	}
	other := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for i := range other.Pix {
		other.Pix[i] = 255
	}
	if iconSimilarity(other, big) {
		t.Fatal("blank white must not match detailed artwork")
	}
}

func mustICNS(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	b, err := makeICNS(pngBytes(c))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
