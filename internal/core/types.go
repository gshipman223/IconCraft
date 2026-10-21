// Package core implements application discovery and journalled, reversible icon changes.
package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

const Version = "0.13.2"

// SetupVersion is bumped when the first-run permissions review needs to be
// shown again (for example, when a release needs a new macOS permission).
const SetupVersion = 1

// SupportURL is Gavin Shipman's verified PayPal business contribution link.
const SupportURL = "https://www.paypal.com/ncp/payment/E8YDD8BNAKVJA"
const RepositoryURL = "https://github.com/gshipman223/IconCraft"

// AppManagementURL opens System Settings at Privacy & Security → App
// Management, where users can allow IconCraft to change other apps' icons.
const AppManagementURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_AppBundles"
const MaxIconBytes = 16 << 20

type App struct {
	IconStamp    string `json:"iconStamp,omitempty"`
	InfoStamp    string `json:"infoStamp,omitempty"`
	Renamed      bool   `json:"renamed,omitempty"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	BundleID     string `json:"bundleId"`
	Version      string `json:"version"`
	Executable   string `json:"executable"`
	CraftID      string `json:"craftId,omitempty"`
	Equivalent   string `json:"equivalent,omitempty"`
	Protected    bool   `json:"protected"`
	ActiveRecord string `json:"activeRecord,omitempty"`
	AssetID      string `json:"assetId,omitempty"`
}
type Snapshot struct {
	FinderInfo []byte            `json:"finderInfo,omitempty"`
	Exists     bool              `json:"exists"`
	Data       []byte            `json:"data,omitempty"`
	Mode       uint32            `json:"mode,omitempty"`
	Attributes map[string][]byte `json:"attributes,omitempty"`
}

func (s Snapshot) Custom() bool { return len(s.FinderInfo) >= 10 && s.FinderInfo[8]&4 != 0 }

// Equal deliberately ignores unrelated root FinderInfo bits; restoring an icon must not undo tags or other Finder changes.
func (s Snapshot) Equal(t Snapshot) bool {
	return s.Custom() == t.Custom() && s.Exists == t.Exists && bytes.Equal(s.Data, t.Data) && reflect.DeepEqual(s.Attributes, t.Attributes)
}

type Asset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Created string `json:"created"`
}
type Record struct {
	ID          string    `json:"id"`
	App         App       `json:"app"`
	Fingerprint string    `json:"fingerprint"`
	AssetID     string    `json:"assetId"`
	Created     string    `json:"created"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	IconFit     int       `json:"iconFit,omitempty"` // macOS icon-grid version the applied artwork was fitted to
	Before      Snapshot  `json:"before"`
	After       *Snapshot `json:"after,omitempty"`
}
type Preference struct {
	BundleID string `json:"bundleId"`
	CraftID  string `json:"craftId,omitempty"`
	AssetID  string `json:"assetId"`
}

// RebrandRecord journals an opt-in *internal* rebrand: the display name and
// bundle icon file inside a third-party app are replaced (and the bundle
// ad-hoc re-signed) so the chosen identity also shows while the app runs
// (Dock, menu bar, app switcher, Launchpad). Originals are backed up under
// Dir/rebrands/<id>/ and RestoreRebrand puts them back byte-for-byte.
// The bundle ID, executable and all links are never changed.
type RebrandRecord struct {
	ID             string `json:"id"`
	BundleID       string `json:"bundleId"`
	App            App    `json:"app"`
	AssetID        string `json:"assetId"`
	Name           string `json:"name"`
	OriginalName   string `json:"originalName,omitempty"` // internal name before the skin (for restore onto updated apps)
	IconFile       string `json:"iconFile"`
	IconExisted    bool   `json:"iconExisted"`
	BeforePrint    string `json:"beforePrint"`
	AfterPrint     string `json:"afterPrint"`
	BeforeIconHash string `json:"beforeIconHash,omitempty"`
	AfterIconHash  string `json:"afterIconHash,omitempty"`
	Status         string `json:"status"`            // prepared | applied | restored | failed | rollback-required
	IconFit        int    `json:"iconFit,omitempty"` // macOS icon-grid version the written .icns was fitted to
	// RuntimeIcon describes the icon the app presents *while running*:
	// "patched" (an embedded runtime icon inside the executable was
	// replaced with the chosen artwork), "own" (the app sets its own
	// running icon and no replaceable embedded copy was found), "none"
	// (the app uses its bundle icon when running), "" (not yet checked —
	// the keeper checks once and records the outcome).
	RuntimeIcon string          `json:"runtimeIcon,omitempty"`
	ExecPatches []ExecIconPatch `json:"execPatches,omitempty"`
	// ExecIconCheck is the version of the runtime-icon inspection that
	// produced RuntimeIcon, so improved matching re-checks old verdicts.
	ExecIconCheck int    `json:"execIconCheck,omitempty"`
	Created       string `json:"created"`
}

// ExecIconPatch locates one PNG image embedded in an app's executable
// that the app itself loads as its running (Dock/window) icon. The patch
// replaces exactly those bytes (same total length, zero-padded), so no
// file offsets move. The original bytes are backed up next to the
// full-skin backup and restored byte-exactly on unskin.
type ExecIconPatch struct {
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
	SHA256 string `json:"sha256"` // of the original embedded PNG bytes
}

// SavedOriginal is the Originals Vault entry for one app, keyed by its
// stable bundle ID — never by path or app ID, both of which change when the
// folder is renamed or the app is replaced. Before IconCraft first changes
// an app it saves: the original folder/display name, the original internal
// (CFBundleName) name, and the original icon as a checksummed asset. Restore
// pulls from the vault, so the original can be rebuilt even onto an app
// that has since moved, been renamed, or updated (a new identifier).
type SavedOriginal struct {
	BundleID     string `json:"bundleId"`
	AppName      string `json:"appName"`                // original display name
	FolderName   string `json:"folderName"`             // original .app folder name, without ".app"
	InternalName string `json:"internalName,omitempty"` // original CFBundleName, when known
	IconAssetID  string `json:"iconAssetId,omitempty"`  // checksummed PNG of the original icon
	Captured     string `json:"captured"`
}

type Original struct {
	AssetID string `json:"assetId"`
	Product string `json:"product"`
	Source  string `json:"source"`
	Checked string `json:"checked"`
}

// PermissionCheck is the last user-run, app-specific App Management probe.
// macOS does not publish one global "IconCraft may modify apps" answer that
// an ad-hoc app can rely on, so IconCraft records what it actually tested:
// whether creating/removing a harmless temporary file inside one chosen
// installed app was allowed. Status is ready, blocked or unknown.
type PermissionCheck struct {
	AppID     string `json:"appId,omitempty"`
	AppName   string `json:"appName,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
}

// OrganizePrefs is how the user likes the app list ordered. It lives in
// the journal (like the theme) because the UI origin changes every
// launch — a random loopback port — so browser storage cannot persist.
type OrganizePrefs struct {
	Sort  string `json:"sort"`
	Group string `json:"group"`
}

type State struct {
	Theme         string                   `json:"theme,omitempty"`
	Organize      OrganizePrefs            `json:"organize,omitempty"`
	Renames       []RenameRecord           `json:"renames,omitempty"`
	Schema        int                      `json:"schema"`
	Originals     map[string]Original      `json:"originals,omitempty"`
	Choices       map[string]string        `json:"choices,omitempty"`
	Assets        map[string]Asset         `json:"assets"`
	Records       []Record                 `json:"records"`
	Rebrands      []RebrandRecord          `json:"rebrands,omitempty"`
	Vault         map[string]SavedOriginal `json:"vault,omitempty"`
	KeeperDesired bool                     `json:"keeperDesired,omitempty"`
	KeeperSet     bool                     `json:"keeperSet,omitempty"`
	SetupVersion  int                      `json:"setupVersion,omitempty"`
	SetupDoneAt   string                   `json:"setupDoneAt,omitempty"`
	AppAccess     PermissionCheck          `json:"appAccess,omitempty"`
	Settings      Settings                 `json:"settings,omitempty"`
	SettingsSet   bool                     `json:"settingsSet,omitempty"`
	Preferences   map[string]Preference    `json:"preferences"`
	ExtraRoots    []string                 `json:"extraRoots"`
}
type Native interface {
	Inspect(context.Context, []string) ([]App, error)
	Snapshot(string) (Snapshot, error)
	Set(context.Context, string, string) error
	Restore(context.Context, string, Snapshot) error
	ExportIcon(context.Context, string, bool) ([]byte, error)
	Normalize(context.Context, string) ([]byte, error)
	Signature(context.Context, string) (string, error)
	Choose(context.Context, string) (string, error)
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func appID(a App) string   { return hash([]byte(a.Path + "\x00" + a.BundleID))[:32] }
func now() string          { return time.Now().UTC().Format(time.RFC3339Nano) }
func ReadLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	return b, nil
}
func AtomicWrite(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".iconcraft-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	d, e := os.Open(filepath.Dir(path))
	if e == nil {
		defer d.Close()
		_ = d.Sync()
	}
	return nil
}
func SafeApp(path string) error {
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".app" {
		return errors.New("not an absolute .app bundle path")
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return errors.New("application is a symlink or not a directory")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if filepath.Clean(resolved) != filepath.Clean(path) {
		return errors.New("application path contains a symlink; rescan its real location")
	}
	p := filepath.Join(path, "Icon\r")
	if fi, err = os.Lstat(p); err == nil && (!fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0) {
		return errors.New("custom icon file is not a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func Protected(path string) bool {
	for _, p := range []string{"/System/", "/Library/Apple/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
func Fingerprint(a App) (string, error) {
	if err := SafeApp(a.Path); err != nil {
		return "", err
	}
	if a.Executable == "" || filepath.Base(a.Executable) != a.Executable || a.Executable == "." || a.Executable == ".." {
		return "", errors.New("missing or unsafe bundle executable name")
	}
	h := sha256.New()
	for _, rel := range []string{"Contents/Info.plist", "Contents/MacOS/" + a.Executable, "Contents/_CodeSignature/CodeResources"} {
		p := filepath.Join(a.Path, rel)
		real, e := filepath.EvalSymlinks(p)
		if e != nil {
			if strings.Contains(rel, "_CodeSignature") && os.IsNotExist(e) {
				continue
			}
			return "", e
		}
		if !strings.HasPrefix(real, a.Path+string(os.PathSeparator)) {
			return "", errors.New("bundle resource points outside application")
		}
		f, e := os.Open(real)
		if e != nil {
			return "", e
		}
		info, e := f.Stat()
		if e != nil {
			f.Close()
			return "", e
		}
		if !info.Mode().IsRegular() || info.Size() > 2<<30 {
			f.Close()
			return "", errors.New("invalid bundle resource")
		}
		_, _ = h.Write([]byte(rel + "\x00"))
		_, e = io.Copy(h, f)
		_ = f.Close()
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func DecodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
