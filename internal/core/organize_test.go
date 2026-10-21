package core

import "testing"

func TestOrganizePrefsDefaultValidatePersist(t *testing.T) {
	e, _, _, _ := fixture(t)
	if got := e.Organize(); got.Sort != "name-asc" || got.Group != "none" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if err := e.SetOrganize("sideways", "status"); err == nil {
		t.Fatal("invalid sort must be refused")
	}
	if err := e.SetOrganize("name-desc", "everywhere"); err == nil {
		t.Fatal("invalid grouping must be refused")
	}
	if err := e.SetOrganize("name-desc", "status"); err != nil {
		t.Fatal(err)
	}
	if got := e.Organize(); got.Sort != "name-desc" || got.Group != "status" {
		t.Fatalf("not journalled: %+v", got)
	}
	if s := e.Session()["organize"]; s != e.Organize() {
		t.Fatalf("session must carry organize prefs: %v", s)
	}
}
