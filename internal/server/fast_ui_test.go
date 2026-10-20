package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gshipman223/IconCraft/internal/core"
)

type previewNative struct {
	noNative
	mu             sync.Mutex
	calls, exports int
	image          []byte
}

func (n *previewNative) ExportThumbnails(_ context.Context, paths []string) (map[string][]byte, error) {
	n.mu.Lock()
	n.calls++
	n.mu.Unlock()
	out := map[string][]byte{}
	for _, p := range paths {
		out[p] = n.image
	}
	return out, nil
}
func (n *previewNative) ExportIcon(context.Context, string, bool) ([]byte, error) {
	n.mu.Lock()
	n.exports++
	n.mu.Unlock()
	return n.image, nil
}
func (n *previewNative) count() int { n.mu.Lock(); defer n.mu.Unlock(); return n.calls }
func stopPreview(s *Server)         { s.stopOnce.Do(func() { close(s.stop) }) }
func previewApp(t *testing.T, root, name string) core.App {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved // macOS /var -> /private/var; bundles must be real paths
	}
	p := filepath.Join(root, name+".app")
	if e := os.MkdirAll(filepath.Join(p, "Contents/MacOS"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(p, "Contents/Info.plist"), []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(p, "Contents/MacOS/app"), []byte("fixture executable"), 0700); e != nil {
		t.Fatal(e)
	}
	return core.App{ID: name, Name: name, BundleID: "test." + name, Path: p, Executable: "app", CraftID: "photocraft"}
}
func TestThumbnailRequestsCoalesceAndSurviveProcessRestart(t *testing.T) {
	s := testServer(t)
	defer stopPreview(s)
	n := &previewNative{image: originalPNG()}
	s.Engine.Native = n
	a := previewApp(t, t.TempDir(), "PhotoCraft")
	s.Engine.Apps[a.ID] = a
	const readers = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := []error{}
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, e := s.thumbnail(context.Background(), a)
			if e == nil {
				e = core.ValidatePNG(b)
			}
			if e != nil {
				mu.Lock()
				errs = append(errs, e)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 || n.count() != 1 {
		t.Fatal("singleflight", n.count(), errs)
	}
	// A different server has an empty memory cache, but uses the persisted pixels.
	again := New(s.Engine)
	defer stopPreview(again)
	if _, err := again.thumbnail(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if n.count() != 1 {
		t.Fatal("relaunch re-exported an unchanged preview")
	}
}
func TestDifferentThumbnailsAreExportedAsOneBatch(t *testing.T) {
	s := testServer(t)
	defer stopPreview(s)
	n := &previewNative{image: originalPNG()}
	s.Engine.Native = n
	root := t.TempDir()
	a, b := previewApp(t, root, "PhotoCraft"), previewApp(t, root, "VectorCraft")
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, v := range []core.App{a, b} {
		wg.Add(1)
		go func(v core.App) {
			defer wg.Done()
			<-start
			if _, err := s.thumbnail(context.Background(), v); err != nil {
				t.Error(err)
			}
		}(v)
	}
	close(start)
	wg.Wait()
	if n.count() != 1 {
		t.Fatal("expected one native process for two previews", n.count())
	}
}
func TestThumbnailStampChangesWhenIconChanges(t *testing.T) {
	s := testServer(t)
	defer stopPreview(s)
	n := &previewNative{image: originalPNG()}
	s.Engine.Native = n
	a := previewApp(t, t.TempDir(), "PhotoCraft")
	before := thumbKey(a)
	if _, err := s.thumbnail(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Path, "Icon\r"), []byte("replacement marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if thumbKey(a) == before {
		t.Fatal("stale cache key")
	}
	if _, err := s.thumbnail(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if n.count() != 2 {
		t.Fatal("changed icon was not regenerated", n.count())
	}
}
func TestPreviewETagReturns304WithoutNativeWork(t *testing.T) {
	s := testServer(t)
	defer stopPreview(s)
	n := &previewNative{image: originalPNG()}
	s.Engine.Native = n
	a := previewApp(t, t.TempDir(), "PhotoCraft")
	s.Engine.Apps[a.ID] = a
	path := "/" + s.Token + "/api/icon?app=" + a.ID
	w := request(s, "GET", path, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "http://"+s.Host+path, nil)
	r.Host = s.Host
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusNotModified || n.count() != 1 {
		t.Fatal(w.Code, n.count())
	}
}
func TestLightSessionAndThemeHTTP(t *testing.T) {
	s := testServer(t)
	seedOriginalApp(s)
	w := request(s, "GET", "/"+s.Token+"/api/session", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"apps"`) || strings.Contains(w.Body.String(), `"finderInfo"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(s, "POST", "/"+s.Token+"/api/theme", "", `{"theme":"light"}`)
	if w.Code != 200 || s.Engine.SnapshotState().Theme != "light" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(s, "POST", "/"+s.Token+"/api/theme", "", `{"theme":"invalid"}`)
	if w.Code != 400 || s.Engine.SnapshotState().Theme != "light" {
		t.Fatal("bad theme accepted")
	}
}
func TestNewMutationsNeedPOSTAndRenameConfirmation(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"rename", "restore-name", "donor-choice", "theme"} {
		w := request(s, "GET", "/"+s.Token+"/api/"+p, "", "")
		if w.Code != 405 {
			t.Fatal(p, w.Code)
		}
	}
	for _, p := range []string{"rename", "restore-name"} {
		w := request(s, "POST", "/"+s.Token+"/api/"+p, "", `{"appId":"unknown","confirmed":false}`)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "confirmation") {
			t.Fatal(p, w.Code, w.Body.String())
		}
	}
}
func TestDonorChoiceCopiesIconWithoutLaunchOrApply(t *testing.T) {
	s := testServer(t)
	n := &previewNative{image: originalPNG()}
	s.Engine.Native = n
	a := seedOriginalApp(s)
	donor := previewApp(t, t.TempDir(), "InstalledOriginal")
	s.Engine.Apps[donor.ID] = donor
	b, _ := json.Marshal(map[string]string{"appId": a.ID, "donorId": donor.ID})
	w := request(s, "POST", "/"+s.Token+"/api/donor-choice", "", string(b))
	st := s.Engine.SnapshotState()
	if w.Code != 200 || len(st.Records) != 0 || st.Choices[a.BundleID] == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	if n.exports != 1 {
		t.Fatal(n.exports)
	}
}
func TestSessionRespondsWhileSlowProviderIsStillPending(t *testing.T) {
	s := testServer(t)
	seedOriginalApp(s)
	s.Resolver.Client.Transport = originalTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Resolver.Resolve(ctx, "photocraft", nil) }()
	start := time.Now()
	w := request(s, "GET", "/"+s.Token+"/api/session", "", "")
	if w.Code != 200 || time.Since(start) > 500*time.Millisecond {
		t.Fatal("source fetch blocked initial UI")
	}
	cancel()
	<-done
}
func TestRenameHTTPMovesBundleAndRestoreAllRestoresName(t *testing.T) {
	s := testServer(t)
	a := previewApp(t, t.TempDir(), "PhotoCraft")
	s.Engine.Apps[a.ID] = a
	w := request(s, "POST", "/"+s.Token+"/api/rename", "", `{"appId":"PhotoCraft","name":"Photo Editor","confirmed":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	st := s.Engine.Session()
	apps := st["apps"].([]core.App)
	if len(apps) != 1 || apps[0].Name != "Photo Editor" || !apps[0].Renamed {
		t.Fatal(apps)
	}
	w = request(s, "POST", "/"+s.Token+"/api/restore-all", "", `{"confirmed":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"restored":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Stat(a.Path); err != nil {
		t.Fatal(err)
	}
}
