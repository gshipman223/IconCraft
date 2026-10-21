package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/native"
	"github.com/gshipman223/IconCraft/internal/originals"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed ui/*
var ui embed.FS

type Server struct {
	thumbMu    sync.Mutex
	thumbOnce  sync.Once
	stopOnce   sync.Once
	stop       chan struct{}
	thumbQueue chan thumbRequest
	thumbWait  map[string]chan struct{}

	Resolver     *originals.Resolver
	pendingMu    sync.Mutex
	pendingIcons map[string]pendingIcon
	operationMu  sync.Mutex
	Engine       *core.Engine
	HTTP         *http.Server
	Host         string
	Token        string
	URL          string
	mu           sync.RWMutex
	Report       native.Report
	Testing      bool
	started      bool
	cacheMu      sync.Mutex
	iconCache    map[string][]byte
	iconOrder    []string
	iconBytes    int
}

func New(e *core.Engine) *Server {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return &Server{stop: make(chan struct{}), thumbQueue: make(chan thumbRequest, 256), thumbWait: map[string]chan struct{}{}, Resolver: originals.New(), pendingIcons: map[string]pendingIcon{}, Engine: e, Token: hex.EncodeToString(b[:]), Report: native.Report{Version: core.Version, Checks: []native.Check{}}, Testing: false, iconCache: map[string][]byte{}}
}
func (s *Server) Start() (net.Listener, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.Host = l.Addr().String()
	s.URL = "http://" + s.Host + "/" + s.Token + "/"
	s.HTTP = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10}
	s.started = true
	go func() { _ = s.HTTP.Serve(l) }()
	return l, nil
}
func (s *Server) SetReport(r native.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Report = r
	s.Testing = false
}
func (s *Server) report() (native.Report, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Report, s.Testing
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) { respond(w, 400, map[string]string{"error": err.Error()}) }
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("JSON content type required")
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return err
	}
	return core.DecodeStrict(b, v)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	if r.Host != s.Host {
		http.Error(w, "Invalid host", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.Host {
		http.Error(w, "Invalid origin", 403)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) != 2 || subtle.ConstantTimeCompare([]byte(parts[0]), []byte(s.Token)) != 1 {
		http.NotFound(w, r)
		return
	}
	p := parts[1]
	if !strings.HasPrefix(p, "api/") {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		if p == "" {
			p = "index.html"
		}
		if p != "index.html" && p != "app.js" && p != "style.css" && p != "logo.svg" {
			http.NotFound(w, r)
			return
		}
		f, _ := fs.Sub(ui, "ui")
		b, err := fs.ReadFile(f, p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(p, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		} else if strings.HasSuffix(p, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		} else if strings.HasSuffix(p, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(b)
		return
	}
	expected := "POST"
	if p == "api/session" || p == "api/status" || p == "api/permissions" || p == "api/icon" || p == "api/profile" || p == "api/report" || p == "api/defaults" {
		expected = "GET"
	}
	if r.Method != expected {
		w.Header().Set("Allow", expected)
		w.WriteHeader(405)
		return
	}
	ctx := r.Context()
	if s.simpleAPI(w, r, p) {
		return
	}
	switch p {
	case "api/session":
		sess := s.Engine.Session()
		sess["keepAlive"] = false
		if p, ok := s.Engine.Native.(interface{ KeepAgentEnabled() bool }); ok {
			sess["keepAlive"] = p.KeepAgentEnabled()
		}
		sess["settings"] = s.Engine.Settings()
		sess["loginAgent"] = false
		if p, ok := s.Engine.Native.(interface{ LoginAgentEnabled() bool }); ok {
			sess["loginAgent"] = p.LoginAgentEnabled()
		}
		respond(w, 200, sess)
	case "api/settings":
		var b struct {
			Settings core.Settings `json:"settings"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return
		}
		saved, err := s.Engine.SaveSettings(b.Settings)
		if err != nil {
			fail(w, err)
			return
		}
		out := map[string]any{"settings": saved, "loginAgent": saved.LaunchAtLogin, "version": core.Version}
		if agent, ok := s.Engine.Native.(interface {
			SetLoginAgent(bool, bool) error
			LoginAgentEnabled() bool
		}); ok {
			if err := agent.SetLoginAgent(saved.LaunchAtLogin, saved.StartInMenuBar); err != nil {
				out["warning"] = "Preference saved, but macOS could not update the launch-at-login item: " + err.Error()
			}
			out["loginAgent"] = agent.LoginAgentEnabled()
		}
		respond(w, 200, out)
	case "api/update-check":
		res, err := s.Engine.CheckForUpdate(ctx)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, res)
	case "api/refresh-icon-cache":
		refresher, ok := s.Engine.Native.(interface {
			RefreshIconCache(context.Context, []string) error
		})
		if !ok {
			fail(w, errors.New("icon cache refresh requires the macOS native layer"))
			return
		}
		state := s.Engine.SnapshotState()
		seen := map[string]bool{}
		var paths []string
		add := func(p string) {
			if p != "" && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
		for _, rec := range state.Records {
			if rec.Status == "applied" {
				add(rec.App.Path)
			}
		}
		for _, rec := range state.Rebrands {
			if rec.Status == "applied" {
				add(rec.App.Path)
			}
		}
		if err := refresher.RefreshIconCache(ctx, paths); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]any{"refreshed": len(paths)})
	case "api/open-data-folder":
		opener, ok := s.Engine.Native.(interface {
			OpenFolder(context.Context, string) error
		})
		if !ok {
			fail(w, errors.New("opening folders requires the macOS native layer"))
			return
		}
		if err := opener.OpenFolder(ctx, s.Engine.Dir); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]bool{"opened": true})
	case "api/defaults":
		provider, ok := s.Engine.Native.(interface {
			DefaultApps(context.Context) ([]native.DefaultCategoryState, error)
		})
		if !ok {
			fail(w, errors.New("default apps require the macOS native layer"))
			return
		}
		cats, err := provider.DefaultApps(ctx)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]any{"categories": s.defaultsPayload(cats)})
	case "api/defaults/set":
		var b struct {
			Category string `json:"category"`
			BundleID string `json:"bundleId"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return
		}
		setter, ok := s.Engine.Native.(interface {
			SetDefaultApp(context.Context, string, string) (native.DefaultCategoryState, error)
		})
		if !ok {
			fail(w, errors.New("default apps require the macOS native layer"))
			return
		}
		updated, err := setter.SetDefaultApp(ctx, b.Category, b.BundleID)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]any{"category": s.defaultsPayload([]native.DefaultCategoryState{updated})[0]})
	case "api/test-notification":
		notifier, ok := s.Engine.Native.(interface {
			NotifyUser(context.Context, string, string) error
		})
		if !ok {
			fail(w, errors.New("notifications require the macOS native layer"))
			return
		}
		if err := notifier.NotifyUser(ctx, "IconCraft notifications are on", "Skin repairs and items needing attention will arrive like this."); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]bool{"sent": true})
	case "api/status":
		rep, pending := s.report()
		state := s.Engine.SnapshotState()
		respond(w, 200, map[string]any{"version": core.Version, "assets": state.Assets, "records": summaries(state.Records), "extraRoots": state.ExtraRoots, "originals": state.Originals, "choices": state.Choices, "catalogue": s.Engine.Catalogue(), "report": rep, "testing": pending})
	case "api/permissions":
		payload, err := s.permissionsPayload(ctx, false, "")
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, payload)
	case "api/scan":
		apps, warnings, err := s.Engine.Scan(ctx)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]any{"apps": apps, "warnings": warnings})
	case "api/refresh":
		err := s.Engine.Refresh(ctx)
		if err != nil {
			fail(w, fmt.Errorf("using cached catalogue: %w", err))
			return
		}
		respond(w, 200, s.Engine.Catalogue())
	case "api/import":
		p, err := s.Engine.Native.Choose(ctx, "icon")
		if err != nil {
			fail(w, err)
			return
		}
		if p == "" {
			respond(w, 200, map[string]bool{"cancelled": true})
			return
		}
		a, err := s.Engine.ImportFile(ctx, p)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, a)
	case "api/donor":
		var b struct {
			AppID string `json:"appId"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return
		}
		a, err := s.Engine.CopyDonor(ctx, b.AppID)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, a)
	case "api/root":
		p, err := s.Engine.Native.Choose(ctx, "folder")
		if err != nil {
			fail(w, err)
			return
		}
		if p == "" {
			respond(w, 200, map[string]bool{"cancelled": true})
			return
		}
		if err = s.Engine.AddRoot(p); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]string{"path": p})
	case "api/apply":
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			AppID     string `json:"appId"`
			AssetID   string `json:"assetId"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return
		}
		if !b.Confirmed {
			fail(w, errors.New("explicit change confirmation required"))
			return
		}
		rec, err := s.Engine.Apply(ctx, b.AppID, b.AssetID)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]string{"recordId": rec.ID, "status": rec.Status})
	case "api/restore":
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			RecordID  string `json:"recordId"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return
		}
		if !b.Confirmed {
			fail(w, errors.New("explicit restore confirmation required"))
			return
		}
		rec, err := s.Engine.Restore(ctx, b.RecordID)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]string{"recordId": rec.ID, "status": rec.Status})
	case "api/icon":
		if asset := r.URL.Query().Get("asset"); asset != "" {
			p, err := s.Engine.AssetPath(asset)
			if err != nil {
				fail(w, err)
				return
			}
			if cacheHeaders(w, r, asset, true) {
				return
			}
			b, err := core.ReadLimited(p, core.MaxIconBytes)
			if err != nil {
				fail(w, err)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(b)
			return
		}
		a, err := s.Engine.GetApp(r.URL.Query().Get("app"))
		if err != nil {
			fail(w, err)
			return
		}
		if cacheHeaders(w, r, thumbKey(a), false) {
			return
		}
		b, err := s.thumbnail(ctx, a)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(b)

	case "api/profile":
		b, err := s.Engine.ExportProfile()
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=IconCraft-profile.json")
		_, _ = w.Write(b)
	case "api/profile-pick":
		p, err := s.Engine.Native.Choose(ctx, "profile")
		if err != nil {
			fail(w, err)
			return
		}
		if p == "" {
			respond(w, 200, map[string]bool{"cancelled": true})
			return
		}
		if !strings.EqualFold(filepath.Ext(p), ".json") {
			fail(w, errors.New("select a JSON profile"))
			return
		}
		b, err := core.ReadLimited(p, 64<<20)
		if err != nil {
			fail(w, err)
			return
		}
		if err = s.Engine.ImportProfile(b); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case "api/profile-import":
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
		if err != nil {
			fail(w, err)
			return
		}
		if err = s.Engine.ImportProfile(b); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case "api/report":
		rep, _ := s.report()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=IconCraft-mac-diagnostics.json")
		_ = json.NewEncoder(w).Encode(rep)
	case "api/export":
		var b struct {
			Kind string `json:"kind"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return
		}
		var data []byte
		var err error
		name := "IconCraft-profile.json"
		if b.Kind == "report" {
			name = "IconCraft-mac-diagnostics.json"
			rep, _ := s.report()
			data, err = json.MarshalIndent(rep, "", "  ")
		} else if b.Kind == "profile" {
			data, err = s.Engine.ExportProfile()
		} else {
			fail(w, errors.New("unknown export type"))
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		dir, err := s.Engine.Native.Choose(ctx, "folder")
		if err != nil {
			fail(w, err)
			return
		}
		if dir == "" {
			respond(w, 200, map[string]bool{"cancelled": true})
			return
		}
		name = strings.TrimSuffix(name, ".json") + "-" + time.Now().Format("20060102-150405") + ".json"
		p := filepath.Join(dir, name)
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fail(w, err)
			return
		}
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err == nil {
			err = ce
		}
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]string{"path": p})
	default:
		http.NotFound(w, r)
	}
}

// defaultsPayload enriches LaunchServices categories with scanned-app
// IDs so the UI can render real icons for candidates it already knows.
func (s *Server) defaultsPayload(cats []native.DefaultCategoryState) []map[string]any {
	byPath := map[string]string{}
	for _, a := range s.Engine.Apps {
		byPath[a.Path] = a.ID
	}
	candidate := func(c native.DefaultCandidate) map[string]any {
		return map[string]any{"bundleId": c.BundleID, "name": c.Name, "path": c.Path, "appId": byPath[c.Path]}
	}
	out := []map[string]any{}
	for _, cat := range cats {
		list := []map[string]any{}
		var current map[string]any
		for _, c := range cat.Candidates {
			m := candidate(c)
			list = append(list, m)
			if c.BundleID == cat.Current {
				current = m
			}
		}
		if current == nil {
			current = map[string]any{"bundleId": cat.Current, "name": cat.Current, "path": "", "appId": ""}
		}
		out = append(out, map[string]any{"id": cat.ID, "label": cat.Label, "detail": cat.Detail, "current": current, "candidates": list})
	}
	return out
}
func summaries(records []core.Record) []map[string]any {
	out := []map[string]any{}
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		out = append(out, map[string]any{"id": r.ID, "appId": r.App.ID, "app": r.App.Name, "version": r.App.Version, "created": r.Created, "status": r.Status, "error": r.Error})
	}
	return out
}
func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	if s.HTTP != nil {
		err = s.HTTP.Shutdown(ctx)
	}
	s.stopOnce.Do(func() { close(s.stop) })
	return err
}

func (s *Server) clearIcons() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.iconCache = map[string][]byte{}
	s.iconOrder = nil
	s.iconBytes = 0
}
func (s *Server) cacheIcon(id string, b []byte) {
	if len(b) > 2<<20 {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if _, ok := s.iconCache[id]; ok {
		return
	}
	for len(s.iconOrder) > 0 && (len(s.iconOrder) >= 64 || s.iconBytes+len(b) > 32<<20) {
		key := s.iconOrder[0]
		s.iconOrder = s.iconOrder[1:]
		s.iconBytes -= len(s.iconCache[key])
		delete(s.iconCache, key)
	}
	s.iconCache[id] = b
	s.iconOrder = append(s.iconOrder, id)
	s.iconBytes += len(b)
}
