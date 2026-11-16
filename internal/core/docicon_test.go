package core

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useDocumentIcons(t *testing.T, e *Engine) {
	t.Helper()
	s := e.Settings()
	s.IconMode = IconModeFull
	s.DocumentIcons = true
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
}

func writeDocPlist(t *testing.T, app, id string) {
	t.Helper()
	body := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>` + id + `</string>
<key>CFBundleName</key><string>PhotoCraft</string>
<key>CFBundleDisplayName</key><string>PhotoCraft</string>
<key>CFBundleDocumentTypes</key>
<array><dict>
<key>CFBundleTypeName</key><string>Invoice</string>
<key>CFBundleTypeExtensions</key><array><string>pdf</string></array>
<key>CFBundleTypeIconFile</key><string>Document</string>
<key>LSHandlerRank</key><string>Owner</string>
</dict></array>
<key>UTExportedTypeDeclarations</key>
<array><dict>
<key>UTTypeIdentifier</key><string>` + id + `.invoice</string>
<key>UTTypeIconFile</key><string>Export.icns</string>
<key>UTTypeIcons</key><dict>
<key>UTTypeIconBadgeName</key><string>AppIcon</string>
<key>UTTypeIconText</key><string>PDF</string>
</dict>
<key>UTTypeIconFiles</key>
<array><string>Export.icns</string><string>../Secrets.icns</string></array>
</dict></array>
</dict></plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents/Info.plist"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func plantDocumentIcon(t *testing.T, app, name string, body []byte) {
	t.Helper()
	dir := filepath.Join(app, "Contents/Resources")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0644); err != nil {
		t.Fatal(err)
	}
}

func readResource(t *testing.T, app, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app, "Contents/Resources", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDocumentIconNamesSkipBadgesAndPaths(t *testing.T) {
	plist := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleTypeIconFile</key><string>Document</string>
<key>UTTypeIconFile</key><string>Export.icns</string>
<key>UTTypeIconBadgeName</key><string>AppIcon</string>
<key>UTTypeIconText</key><string>PDF</string>
<key>UTTypeIconBackgroundName</key><string>Background</string>
<key>CFBundleTypeIconFiles</key><array><string>Export.icns</string><string>../Secrets.icns</string></array>
</dict></plist>`)
	got := documentIconNames(plist)
	if strings.Join(got, ",") != "Document,Export.icns" {
		t.Fatalf("names = %v", got)
	}
	if documentIconNames([]byte("bplist00not-xml")) != nil {
		t.Fatal("binary plist was treated as icon names")
	}
}

func TestDocumentIconIsAPageNotTheAppIcon(t *testing.T) {
	src := pngBytes(color.RGBA{1, 2, 3, 255})
	app, err := makeICNS(src)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := makeDocumentICNS(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(doc), "icns") {
		t.Fatal("document icon is not an icns")
	}
	if string(doc) == string(app) {
		t.Fatal("document icon is a copy of the app icon")
	}
}

func TestFullIconReplacesDocumentIconsAndPauseRestoresThem(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	before := plistBytes(t, a.Path)
	origApp := plantBundleIcon(t, a.Path)
	origDoc := []byte("original-document-icns")
	plantDocumentIcon(t, a.Path, "Document.icns", origDoc)
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useDocumentIcons(t, e)
	rec, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.DocNote != "" || len(rec.DocIcons) != 2 {
		t.Fatalf("doc record: note=%q files=%+v", rec.DocNote, rec.DocIcons)
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("Info.plist changed")
	}
	if string(readBundleIcon(t, a.Path)) == string(origApp) {
		t.Fatal("app icon was not replaced")
	}
	if string(readResource(t, a.Path, "Document.icns")) == string(origDoc) {
		t.Fatal("document icon was not replaced")
	}
	if string(readResource(t, a.Path, "Export.icns")) == "original-export-icns" {
		t.Fatal("exported type icon was not replaced")
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(readResource(t, a.Path, "Export.icns")) {
		t.Fatal("document icons were not the same page artwork")
	}
	if n.resigns != 1 {
		t.Fatalf("resigns = %d, want 1", n.resigns)
	}
	if n.refreshes < 1 {
		t.Fatal("document icon swap did not refresh Finder")
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher no longer sees the app")
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) != string(origApp) {
		t.Fatal("pause left the custom app icon")
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) {
		t.Fatal("pause left the custom document icon")
	}
	if string(readResource(t, a.Path, "Export.icns")) != "original-export-icns" {
		t.Fatal("pause left the custom export icon")
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("pause changed Info.plist")
	}
	resigns := n.resigns
	if _, err = e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) || n.resigns != resigns {
		t.Fatal("reconcile while paused put the custom document icon back")
	}
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) == string(origApp) {
		t.Fatal("unpause did not put the custom app icon back")
	}
	if string(readResource(t, a.Path, "Document.icns")) == string(origDoc) {
		t.Fatal("unpause did not put the custom document icon back")
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("unpause changed Info.plist")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("document icons duplicated the app")
	}
}

func TestFullIconLeavesDocumentIconsWhenTheToggleIsOff(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	origDoc := []byte("original-document-icns")
	plantBundleIcon(t, a.Path)
	plantDocumentIcon(t, a.Path, "Document.icns", origDoc)
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useFullIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) {
		t.Fatal("document icon changed while the toggle was off")
	}
	if n.resigns != 1 {
		t.Fatalf("resigns = %d, want 1", n.resigns)
	}
}

func TestMissingDocumentIconFileDoesNotFailFullIcon(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	orig := plantBundleIcon(t, a.Path)
	useDocumentIcons(t, e)
	rec, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.DocNote == "" || len(rec.DocIcons) != 0 {
		t.Fatalf("expected a note and no files, got note=%q files=%d", rec.DocNote, len(rec.DocIcons))
	}
	if string(readBundleIcon(t, a.Path)) == string(orig) {
		t.Fatal("app icon was not replaced")
	}
	if n.resigns != 1 {
		t.Fatalf("resigns = %d, want 1", n.resigns)
	}
	if _, err = os.Stat(filepath.Join(a.Path, "Contents/Resources/Document.icns")); !os.IsNotExist(err) {
		t.Fatal("a missing document icon was created")
	}
}

func TestBinaryPlistSkipsDocumentIcons(t *testing.T) {
	e, n, a, asset := fixture(t)
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte("bplist00not-readable"), 0600); err != nil {
		t.Fatal(err)
	}
	orig := plantBundleIcon(t, a.Path)
	useDocumentIcons(t, e)
	rec, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.DocNote == "" {
		t.Fatal("binary plist produced no note")
	}
	if string(readBundleIcon(t, a.Path)) == string(orig) {
		t.Fatal("app icon was not replaced")
	}
	if n.resigns != 1 {
		t.Fatalf("resigns = %d", n.resigns)
	}
}

func TestTurningDocumentIconsOffRestoresOnlyThoseFiles(t *testing.T) {
	e, _, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	origApp := plantBundleIcon(t, a.Path)
	origDoc := []byte("original-document-icns")
	plantDocumentIcon(t, a.Path, "Document.icns", origDoc)
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useDocumentIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	customApp := readBundleIcon(t, a.Path)
	s := e.Settings()
	s.DocumentIcons = false
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) {
		t.Fatal("turning document icons off left the custom document icon")
	}
	if string(readBundleIcon(t, a.Path)) != string(customApp) {
		t.Fatal("turning document icons off changed the app icon")
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) != string(origApp) {
		t.Fatal("pause restored a backup of the custom app icon")
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) {
		t.Fatal("pause put the custom document icon back")
	}
}

func TestTurningDocumentIconsOnDoesNotBackupTheCustomAppIcon(t *testing.T) {
	e, _, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	origApp := plantBundleIcon(t, a.Path)
	origDoc := []byte("original-document-icns")
	plantDocumentIcon(t, a.Path, "Document.icns", origDoc)
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useFullIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	customApp := append([]byte(nil), readBundleIcon(t, a.Path)...)
	s := e.Settings()
	s.DocumentIcons = true
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) != string(customApp) {
		t.Fatal("enabling document icons rewrote the app icon")
	}
	if string(readResource(t, a.Path, "Document.icns")) == string(origDoc) {
		t.Fatal("enabling document icons left the publisher document icon")
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) != string(origApp) {
		t.Fatal("pause did not restore the publisher app icon")
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) {
		t.Fatal("pause did not restore the publisher document icon")
	}
}

func TestUnpauseReappliesDocumentIconsOntoTheCurrentPlist(t *testing.T) {
	e, _, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	plantBundleIcon(t, a.Path)
	plantDocumentIcon(t, a.Path, "Document.icns", []byte("original-document-icns"))
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useDocumentIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	updated := strings.Replace(string(plistBytes(t, a.Path)), "PhotoCraft", "PhotoCraft", 1)
	updated = strings.Replace(updated, "<key>CFBundleName</key><string>PhotoCraft</string>", "<key>CFBundleName</key><string>PhotoCraft</string><key>CFBundleShortVersionString</key><string>9.9</string>", 1)
	if !strings.Contains(updated, "9.9") {
		t.Fatal("plist was not updated")
	}
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	plantDocumentIcon(t, a.Path, "Document.icns", []byte("vendor-document"))
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	got := string(plistBytes(t, a.Path))
	if !strings.Contains(got, "9.9") {
		t.Fatal("unpause wrote the old Info.plist onto the updated bundle")
	}
	if string(readResource(t, a.Path, "Document.icns")) == "vendor-document" || string(readResource(t, a.Path, "Document.icns")) == "original-document-icns" {
		t.Fatal("document icon was not reapplied onto the updated bundle")
	}
}

func TestUnpauseLeavesADocumentIconTheNewPlistDoesNotName(t *testing.T) {
	e, _, a, asset := fixture(t)
	writeDocPlist(t, a.Path, a.BundleID)
	plantBundleIcon(t, a.Path)
	plantDocumentIcon(t, a.Path, "Document.icns", []byte("original-document-icns"))
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useDocumentIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	bare := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>` + a.BundleID + `</string>
<key>CFBundleName</key><string>PhotoCraft</string>
<key>CFBundleShortVersionString</key><string>9.9</string>
</dict></plist>
`
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte(bare), 0600); err != nil {
		t.Fatal(err)
	}
	plantDocumentIcon(t, a.Path, "Document.icns", []byte("vendor-document"))
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if !strings.Contains(string(plistBytes(t, a.Path)), "9.9") {
		t.Fatal("unpause wrote the old Info.plist")
	}
	if string(readResource(t, a.Path, "Document.icns")) != "vendor-document" {
		t.Fatal("a document icon the new plist does not name was overwritten")
	}
}

func TestFullRenameAndDocumentIconsPauseTogether(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.follow = true
	writeDocPlist(t, a.Path, a.BundleID)
	before := plistBytes(t, a.Path)
	origApp := plantBundleIcon(t, a.Path)
	origDoc := []byte("original-document-icns")
	plantDocumentIcon(t, a.Path, "Document.icns", origDoc)
	plantDocumentIcon(t, a.Path, "Export.icns", []byte("original-export-icns"))
	useFullNames(t, e)
	useDocumentIcons(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.ApplyBundleIcon(context.Background(), got.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if string(readResource(t, got.Path, "Document.icns")) == string(origDoc) {
		t.Fatal("renamed folder blocked the document icon swap")
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("pause did not put the folder back")
	}
	if string(readBundleIcon(t, a.Path)) != string(origApp) {
		t.Fatal("pause did not restore the app icon")
	}
	if string(readResource(t, a.Path, "Document.icns")) != string(origDoc) {
		t.Fatal("pause did not restore the document icon")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("pause duplicated the app")
	}
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 {
		t.Fatal(apps, err)
	}
	if filepath.Base(apps[0].Path) != "Photoshop.app" {
		t.Fatalf("unpause folder: %s", apps[0].Path)
	}
	if string(readResource(t, apps[0].Path, "Document.icns")) == string(origDoc) {
		t.Fatal("unpause did not put the custom document icon back")
	}
	if string(plistBytes(t, apps[0].Path)) != string(before) {
		t.Fatal("unpause changed Info.plist")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("unpause duplicated the app")
	}
}
