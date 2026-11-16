package core

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"path/filepath"
	"strings"
)

const (
	// DocIconsOn replaces this app's document icons. DocIconsOff leaves
	// them. Empty follows the setting for every app.
	DocIconsOn  = "on"
	DocIconsOff = "off"

	maxDocIcons = 32

	// docIconCatalogNote is shown when Full icon was asked to change
	// document icons and this app has no replaceable .icns for them.
	// Finder shows the icon of the app that opens the file. Gavin's
	// PDFs open in the customised app, so they use that app's files;
	// a PDF that still opens in Preview keeps Preview's icon.
	docIconCatalogNote = "No replaceable document icon file was found in this app. Finder shows the document icon of the app that opens the file, so a PDF this app opens uses this app's icon, and a PDF that still opens in another app keeps that app's icon. A document icon that lives only in an asset catalog cannot be replaced."
	docIconBinaryNote  = "This app's document icons are listed in a binary Info.plist that could not be read here, so they were left alone. The app icon was still replaced."
)

func canonicalDocIcons(mode string) (string, error) {
	switch mode {
	case "", DocIconsOn, DocIconsOff:
		return mode, nil
	default:
		return "", errors.New("document icons must be on, off, or follow the setting for every app")
	}
}

// documentIconsLocked reports whether this bundle's document icons
// should be replaced. Full icon has to be on; the per-app choice wins.
// Caller holds e.mu.
func (e *Engine) documentIconsLocked(bundle string) bool {
	if e.iconModeLocked(bundle) != IconModeFull {
		return false
	}
	switch e.State.DocIconModes[bundle] {
	case DocIconsOn:
		return true
	case DocIconsOff:
		return false
	}
	return e.State.SettingsSet && e.State.Settings.DocumentIcons
}

// DocumentIcons reports the effective document-icon choice for a bundle.
func (e *Engine) DocumentIcons(bundle string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.documentIconsLocked(bundle)
}

// SetAppDocIcons stores a per-app override. mode "" follows the global
// setting. The caller applies the icon policy afterwards.
func (e *Engine) SetAppDocIcons(appID, mode string) (App, error) {
	mode, err := canonicalDocIcons(mode)
	if err != nil {
		return App{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[appID]
	if !ok {
		return App{}, errors.New("application not found; rescan")
	}
	if e.State.DocIconModes == nil {
		e.State.DocIconModes = map[string]string{}
	}
	if mode == "" {
		delete(e.State.DocIconModes, a.BundleID)
	} else {
		e.State.DocIconModes[a.BundleID] = mode
	}
	a.DocChoice = mode
	a.DocIcons = e.documentIconsLocked(a.BundleID)
	e.Apps[a.ID] = a
	if err = e.save(); err != nil {
		return a, err
	}
	return a, nil
}

// documentIconNames returns the basenames Info.plist gives to document
// icons: CFBundleTypeIconFile, UTTypeIconFile, and the strings inside
// CFBundleTypeIconFiles / UTTypeIconFiles. Badge names, background names
// and icon text are not files. A binary plist yields nothing; the caller
// converts it first. Names that are not a single path element are dropped.
func documentIconNames(plist []byte) []string {
	if len(plist) == 0 {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(plist))
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	type frame struct {
		kind      string
		key       string
		fileArray bool
	}
	var stack []frame
	var capture string
	var buf strings.Builder
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || !safeDocBase(s) {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "key":
				capture = "key"
				buf.Reset()
			case "string":
				capture = "string"
				buf.Reset()
			default:
				parent := ""
				if n := len(stack); n > 0 {
					parent = stack[n-1].key
				}
				stack = append(stack, frame{
					kind:      t.Name.Local,
					fileArray: t.Name.Local == "array" && docIconArrayKey(parent),
				})
				capture = ""
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "key":
				if capture == "key" && len(stack) > 0 && stack[len(stack)-1].kind == "dict" {
					stack[len(stack)-1].key = strings.TrimSpace(buf.String())
				}
			case "string":
				if capture == "string" && len(stack) > 0 {
					top := &stack[len(stack)-1]
					if top.fileArray || docIconFileKey(top.key) {
						add(buf.String())
					}
					if !top.fileArray {
						top.key = ""
					}
				}
			default:
				if len(stack) > 0 && stack[len(stack)-1].kind == t.Name.Local {
					stack = stack[:len(stack)-1]
				}
				if len(stack) > 0 {
					stack[len(stack)-1].key = ""
				}
			}
			capture = ""
		case xml.CharData:
			if capture != "" {
				buf.Write(t)
			}
		}
	}
	return out
}

func docIconFileKey(key string) bool {
	return key == "CFBundleTypeIconFile" || key == "UTTypeIconFile"
}

func docIconArrayKey(key string) bool {
	return key == "CFBundleTypeIconFiles" || key == "UTTypeIconFiles"
}

// safeDocBase accepts a single file name. A slash or ".." would point
// outside Contents/Resources, and those names are ignored.
func safeDocBase(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	return filepath.Base(name) == name
}

func safeIconName(name string) bool {
	return safeDocBase(name) && strings.HasSuffix(strings.ToLower(name), ".icns")
}

// documentPlistXML returns Info.plist as XML. A binary plist is converted
// by the native layer when it can; otherwise nil, and document icons are
// skipped without failing the app-icon replacement.
func (e *Engine) documentPlistXML(ctx context.Context, appPath string, raw []byte) []byte {
	if len(raw) >= 6 && string(raw[:6]) == "bplist" {
		conv, ok := e.Native.(interface {
			PlistXML(context.Context, string) ([]byte, error)
		})
		if !ok {
			return nil
		}
		xmlBytes, err := conv.PlistXML(ctx, appPath)
		if err != nil || len(xmlBytes) == 0 {
			return nil
		}
		return xmlBytes
	}
	return raw
}

// planDocumentIcons reads the document .icns files the plist names.
// The app icon file is not included. Missing files (often an asset
// catalog) are skipped. The returned note is empty when at least one
// file can be replaced.
func (e *Engine) planDocumentIcons(ctx context.Context, app App, plist []byte, mainIcon string) ([]DocIconFile, [][]byte, string) {
	xmlBytes := e.documentPlistXML(ctx, app.Path, plist)
	if xmlBytes == nil {
		return nil, nil, docIconBinaryNote
	}
	names := documentIconNames(xmlBytes)
	if len(names) == 0 {
		return nil, nil, docIconCatalogNote
	}
	var files []DocIconFile
	var originals [][]byte
	seen := map[string]bool{}
	for _, name := range names {
		rel := name
		if !strings.HasSuffix(strings.ToLower(rel), ".icns") {
			rel += ".icns"
		}
		if !safeIconName(rel) || strings.EqualFold(rel, mainIcon) || seen[strings.ToLower(rel)] {
			continue
		}
		seen[strings.ToLower(rel)] = true
		b, err := ReadLimited(filepath.Join(app.Path, "Contents/Resources", rel), MaxIconBytes)
		if err != nil {
			continue
		}
		files = append(files, DocIconFile{File: rel, BeforeHash: hash(b)})
		originals = append(originals, b)
		if len(files) >= maxDocIcons {
			break
		}
	}
	if len(files) == 0 {
		return nil, nil, docIconCatalogNote
	}
	return files, originals, ""
}

// makeDocumentICNS builds a document-style icon: a page with a folded
// corner and the custom icon as a badge, the way macOS draws a document
// that belongs to an app. The page does not fill the canvas, so it is
// not clipped to the app-icon squircle.
func makeDocumentICNS(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, errors.New("selected icon cannot be decoded for a document icon")
	}
	const canvas = 1024
	page := image.NewRGBA(image.Rect(0, 0, canvas, canvas))
	drawDocumentPage(page)
	badge := resizeImage(img, 280, 280)
	// Page is x=180..844, y=80..920. Sit the badge inside the lower half.
	bx, by := 180+(664-280)/2, 920-280-90
	draw.Draw(page, image.Rect(bx, by, bx+280, by+280), badge, badge.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err = png.Encode(&buf, page); err != nil {
		return nil, err
	}
	return makeICNS(buf.Bytes())
}

func drawDocumentPage(dst *image.RGBA) {
	const (
		x0, y0 = 180, 80
		x1, y1 = 844, 920
		fold   = 140
	)
	white := color.RGBA{255, 255, 255, 255}
	flap := color.RGBA{226, 230, 236, 255}
	line := color.RGBA{196, 202, 210, 255}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x < x1-fold || y >= y0+fold {
				dst.SetRGBA(x, y, white)
				continue
			}
			lx := x - (x1 - fold)
			ly := y - y0
			switch {
			case lx+ly < fold-16:
				dst.SetRGBA(x, y, flap)
			case lx+ly < fold:
				dst.SetRGBA(x, y, line)
			default:
				dst.SetRGBA(x, y, white)
			}
		}
	}
}

func writeDocIconBytes(app App, files []DocIconFile, icns []byte) error {
	for _, d := range files {
		if !safeIconName(d.File) {
			return errors.New("document icon name is not a file in Resources")
		}
		p := filepath.Join(app.Path, "Contents/Resources", d.File)
		if err := AtomicWrite(p, icns, 0644); err != nil {
			return fmt.Errorf("could not replace document icon %s: %w", d.File, err)
		}
	}
	return nil
}

// restoreDocIconFiles writes the backed-up document icons back.
// onlyOurs skips a file whose bytes are no longer the ones IconCraft wrote.
func restoreDocIconFiles(app App, rec RebrandRecord, dir string, onlyOurs bool) error {
	for i, d := range rec.DocIcons {
		if !safeIconName(d.File) {
			return errors.New("document icon name is not a file in Resources")
		}
		if onlyOurs && (d.AfterHash == "" || bundleIconHash(app, d.File) != d.AfterHash) {
			continue
		}
		b, err := ReadLimited(filepath.Join(dir, fmt.Sprintf("doc-%d.icns", i)), MaxIconBytes)
		if err != nil {
			return fmt.Errorf("document-icon backup is missing: %w", err)
		}
		if err = AtomicWrite(filepath.Join(app.Path, "Contents/Resources", d.File), b, 0644); err != nil {
			return err
		}
	}
	return nil
}

func docIconsApplied(app App, rec RebrandRecord) bool {
	if len(rec.DocIcons) == 0 {
		return rec.DocNote != ""
	}
	for _, d := range rec.DocIcons {
		if d.AfterHash == "" || bundleIconHash(app, d.File) != d.AfterHash {
			return false
		}
	}
	return true
}

func docIconsStillCustom(app App, rec RebrandRecord) bool {
	for _, d := range rec.DocIcons {
		if d.AfterHash != "" && bundleIconHash(app, d.File) == d.AfterHash {
			return true
		}
	}
	return false
}

// applyDocumentIcons replaces document icons on a Full icon that is
// already applied. The app-icon backup is left alone, so Pause still
// restores the publisher's app icon rather than the custom one.
// The first time, the current document files are backed up. A later
// pass rewrites the same files and does not replace that backup.
// changed is false when there was nothing to replace.
func (e *Engine) applyDocumentIcons(ctx context.Context, app App, rec RebrandRecord) (bool, error) {
	if !e.DocumentIcons(app.BundleID) {
		return false, nil
	}
	t, err := e.tools()
	if err != nil {
		return false, err
	}
	e.mu.Lock()
	assetID := rec.AssetID
	if assetID == "" {
		if p, ok := e.State.Preferences[rec.BundleID]; ok {
			assetID = p.AssetID
		}
	}
	assetPath, err := e.assetPath(assetID)
	e.mu.Unlock()
	if err != nil {
		return false, errors.New("choose an icon first")
	}
	pngBytes, err := ReadLimited(assetPath, MaxIconBytes)
	if err != nil {
		return false, err
	}
	docICNS, err := makeDocumentICNS(pngBytes)
	if err != nil {
		return false, err
	}
	plistPath := filepath.Join(app.Path, "Contents/Info.plist")
	plistBytes, err := ReadLimited(plistPath, 4<<20)
	if err != nil {
		return false, err
	}
	dir := e.rebrandDir(rec.ID)
	files := rec.DocIcons
	if len(files) == 0 {
		var originals [][]byte
		var note string
		files, originals, note = e.planDocumentIcons(ctx, app, plistBytes, rec.IconFile)
		if len(files) == 0 {
			if err = e.setDocIconResult(rec.ID, nil, note, ""); err != nil {
				return false, err
			}
			return false, nil
		}
		for i, b := range originals {
			if err = AtomicWrite(filepath.Join(dir, fmt.Sprintf("doc-%d.icns", i)), b, 0600); err != nil {
				return false, fmt.Errorf("document-icon backup could not be written: %w", err)
			}
		}
	}
	files = append([]DocIconFile(nil), files...)
	if err = writeDocIconBytes(app, files, docICNS); err != nil {
		_ = restoreDocIconFiles(app, RebrandRecord{DocIcons: files}, dir, false)
		return false, err
	}
	for i := range files {
		files[i].AfterHash = hash(docICNS)
	}
	if err = t.Resign(ctx, app.Path); err != nil {
		_ = restoreDocIconFiles(app, RebrandRecord{DocIcons: files}, dir, false)
		_ = t.Resign(ctx, app.Path)
		if fp, er := Fingerprint(app); er == nil {
			_ = e.setDocIconResult(rec.ID, rec.DocIcons, rec.DocNote, fp)
		}
		return false, err
	}
	if status, er := e.Native.Signature(ctx, app.Path); er != nil || status != "valid" {
		return false, errors.New("re-signed bundle did not verify")
	}
	if got, er := ReadLimited(plistPath, 4<<20); er != nil || hash(got) != hash(plistBytes) {
		return false, errors.New("Info.plist changed while replacing document icons; stopped")
	}
	after, err := Fingerprint(app)
	if err != nil {
		return false, err
	}
	if err = e.setDocIconResult(rec.ID, files, "", after); err != nil {
		return true, err
	}
	e.refreshBundleIcons(ctx, app.Path)
	return true, nil
}

// restoreDocumentIconsOnly puts backed-up document icons back and leaves
// the custom app icon in place. It does not take a new backup of the
// custom document bytes.
func (e *Engine) restoreDocumentIconsOnly(ctx context.Context, app App, rec RebrandRecord) error {
	if len(rec.DocIcons) == 0 {
		return nil
	}
	if !docIconsStillCustom(app, rec) {
		return e.setDocIconResult(rec.ID, nil, "", "")
	}
	t, err := e.tools()
	if err != nil {
		return err
	}
	if err = restoreDocIconFiles(app, rec, e.rebrandDir(rec.ID), true); err != nil {
		return err
	}
	if err = t.Resign(ctx, app.Path); err != nil {
		return err
	}
	if status, er := e.Native.Signature(ctx, app.Path); er != nil || status != "valid" {
		return errors.New("re-signed bundle did not verify after restoring document icons")
	}
	after, err := Fingerprint(app)
	if err != nil {
		return err
	}
	if err = e.setDocIconResult(rec.ID, nil, "", after); err != nil {
		return err
	}
	e.refreshBundleIcons(ctx, app.Path)
	return nil
}

// setDocIconResult updates the applied record. An empty afterPrint leaves
// AfterPrint as it was, so a no-op note does not look like a new signature.
func (e *Engine) setDocIconResult(id string, files []DocIconFile, note, afterPrint string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Rebrands {
		if e.State.Rebrands[i].ID != id {
			continue
		}
		e.State.Rebrands[i].DocIcons = files
		e.State.Rebrands[i].DocNote = note
		if afterPrint != "" {
			e.State.Rebrands[i].AfterPrint = afterPrint
		}
		return e.save()
	}
	return errors.New("full icon record is missing")
}

// refreshBundleIcons asks Launch Services, IconServices, Finder and the
// Dock to drop cached icons for these bundles. Mail and other apps that
// are already open keep the icon they loaded until they are relaunched.
func (e *Engine) refreshBundleIcons(ctx context.Context, paths ...string) {
	var clean []string
	for _, p := range paths {
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return
	}
	refresher, ok := e.Native.(interface {
		RefreshIconCache(context.Context, []string) error
	})
	if !ok {
		return
	}
	_ = refresher.RefreshIconCache(ctx, clean)
}

// refreshHeldBundles reloads Finder after Pause has put the original
// icon files and the original Finder tile back.
func (e *Engine) refreshHeldBundles(ctx context.Context, onlyBundle string) {
	e.mu.Lock()
	var paths []string
	seen := map[string]bool{}
	for _, r := range e.State.Rebrands {
		if r.Kind != RebrandKindIcon || !r.Held || r.App.Path == "" {
			continue
		}
		if onlyBundle != "" && r.BundleID != onlyBundle {
			continue
		}
		if seen[r.App.Path] {
			continue
		}
		seen[r.App.Path] = true
		paths = append(paths, r.App.Path)
	}
	e.mu.Unlock()
	e.refreshBundleIcons(ctx, paths...)
}
