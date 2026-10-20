package core

import (
	"context"
	"image/color"
	"testing"
)

func TestKeeperPreferenceJournalAndMigration(t *testing.T) {
	e, _, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	_ = pngBytes(color.RGBA{1, 1, 1, 255})
	if d, set := e.KeeperPreference(); set {
		t.Fatalf("preference should start unset, got desired=%v", d)
	}
	e.InitKeeperPreference(true)
	if d, set := e.KeeperPreference(); !set || !d {
		t.Fatal("migration should seed intent from current agent state")
	}
	e.InitKeeperPreference(false) // must not overwrite journalled intent
	if d, _ := e.KeeperPreference(); !d {
		t.Fatal("migration overwrote existing intent")
	}
	if err := e.SetKeeperPreference(false); err != nil {
		t.Fatal(err)
	}
	if d, set := e.KeeperPreference(); !set || d {
		t.Fatal("toggle off not journalled")
	}
	// Persisted across reopen.
	e2, err := Open(e.Dir, e.Home, e.Native)
	if err != nil {
		t.Fatal(err)
	}
	if d, set := e2.KeeperPreference(); !set || d {
		t.Fatal("intent not persisted across reopen")
	}
}
