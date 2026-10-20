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

// A clone that sets its own Dock icon at launch (eframe-style) embeds
// the original icon as a PNG inside the executable. A full skin must
// swap exactly that byte region — same file length — and restore must
// put the original bytes back.
func TestRebrandPatchesEmbeddedRuntimeIcon(t *testing.T) {
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
	if rec.RuntimeIcon != "patched" || len(rec.ExecPatches) != 1 {
		t.Fatalf("runtime icon not patched: %+v", rec)
	}
	patched, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(patched) != len(exe) {
		t.Fatalf("executable size changed: %d -> %d", len(exe), len(patched))
	}
	found := scanEmbeddedPNGs(patched)
	if len(found) != 1 {
		t.Fatalf("patched executable should still contain one icon PNG, got %d", len(found))
	}
	img, err := png.Decode(bytes.NewReader(found[0].data))
	if err != nil {
		t.Fatal(err)
	}
	assetImg, err := decodeAssetImage(filepath.Join(e.Dir, "assets", asset.ID+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if !iconSimilarity(img, fitMacOSIcon(assetImg)) {
		t.Fatal("embedded icon was not replaced with the chosen artwork")
	}
	restored, err := e.RestoreRebrand(context.Background(), a.ID)
	if err != nil || restored.Status != "restored" {
		t.Fatal(restored, err)
	}
	back, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, exe) {
		t.Fatal("executable bytes not restored exactly")
	}
	if after, err := Fingerprint(a); err != nil || after != before {
		t.Fatalf("fingerprint not restored: %v", err)
	}
}

// Re-checking an already-patched skin (e.g. after the inspection
// version bumps) must not flip its verdict to "own": the embedded
// artwork no longer matches the original precisely because we
// replaced it.
func TestRecheckKeepsPatchedVerdict(t *testing.T) {
	e, _, a, asset := fixture(t)
	icns, err := makeICNS(pngBytes(color.RGBA{200, 40, 30, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(a.Path, "Contents/Resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Resources/AppIcon.icns"), icns, 0644); err != nil {
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
	exe := append([]byte("FAKE-EXE setApplicationIconImage: padding "), emb.Bytes()...)
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/MacOS", a.Executable), exe, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	// A record already inspected by the current version is left alone.
	for i := range e.State.Rebrands {
		if e.State.Rebrands[i].ID == rec.ID {
			e.State.Rebrands[i].ExecIconCheck = ExecIconCheckVersion
		}
	}
	changed, err := e.patchRebrandRuntimeIcon(context.Background(), a, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("recheck of a patched app must not patch again")
	}
	got := e.State.Rebrands[0]
	if got.RuntimeIcon != "patched" || got.ExecIconCheck != ExecIconCheckVersion {
		t.Fatalf("verdict flipped on recheck: %+v", got)
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
