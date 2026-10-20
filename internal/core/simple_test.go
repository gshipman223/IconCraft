package core

import (
	"context"
	"image/color"
	"testing"
)

func TestRememberOriginalAndCustomArePreviewsNotWrites(t *testing.T) {
	e, n, a, asset := fixture(t)
	if err := e.RememberOriginal(a.CraftID, "Adobe Photoshop", "https://www.adobe.com/icon.svg", asset.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.RememberChoice(a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if n.sets != 0 || len(e.SnapshotState().Records) != 0 {
		t.Fatal("preview changed app")
	}
	loaded, err := Open(e.Dir, e.Home, n)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State.Originals[a.CraftID].AssetID != asset.ID || loaded.State.Choices[a.BundleID] != asset.ID {
		t.Fatal("preferences not persisted")
	}
	if err = e.RememberChoice(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	if e.SnapshotState().Choices[a.BundleID] != "" {
		t.Fatal("custom not cleared")
	}
}
func TestOriginalRejectsMissingAssetAndUnsafeIdentity(t *testing.T) {
	e, _, _, asset := fixture(t)
	if e.RememberOriginal("../../x", "x", "x", asset.ID) == nil {
		t.Fatal("bad identity")
	}
	if e.RememberOriginal("photocraft", "x", "x", "missing") == nil {
		t.Fatal("missing asset")
	}
}
func TestSingleRestoreUnwindsEveryReplacement(t *testing.T) {
	e, n, a, first := fixture(t)
	before := Snapshot{FinderInfo: make([]byte, 32), Exists: true, Data: []byte("old custom"), Attributes: map[string][]byte{"com.apple.ResourceFork": []byte("existing artwork")}}
	before.FinderInfo[8] = 4
	n.states[a.Path] = before
	second, err := e.ImportPNG("second", "test", pngBytes(color.RGBA{9, 8, 7, 255}))
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []Asset{first, second, first} {
		if _, err = e.Apply(context.Background(), a.ID, asset.ID); err != nil {
			t.Fatal(err)
		}
	}
	count, err := e.RestoreOriginal(context.Background(), a.ID)
	if err != nil || count != 3 {
		t.Fatal(count, err)
	}
	if !n.states[a.Path].Equal(before) {
		t.Fatal("did not restore pre-IconCraft custom icon")
	}
	for _, r := range e.SnapshotState().Records {
		if r.Status != "restored" {
			t.Fatal(r.Status)
		}
	}
}
func TestSimpleRestorePreservesOutsideChanges(t *testing.T) {
	e, n, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	changed := Snapshot{Exists: true, Data: []byte("changed elsewhere")}
	n.states[a.Path] = changed
	if _, err := e.RestoreOriginal(context.Background(), a.ID); err == nil {
		t.Fatal("overwrote external change")
	}
	if !n.states[a.Path].Equal(changed) {
		t.Fatal("outside icon lost")
	}
}
func TestRestoreNothingIsHarmless(t *testing.T) {
	e, _, a, _ := fixture(t)
	n, err := e.RestoreOriginal(context.Background(), a.ID)
	if n != 0 || err != nil {
		t.Fatal(n, err)
	}
}
