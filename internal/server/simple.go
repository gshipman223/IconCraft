package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/originals"
)

type pendingIcon struct {
	AppID     string
	CraftID   string
	Candidate originals.Candidate
	Expires   time.Time
}

func (s *Server) ticket(a core.App, c originals.Candidate) string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	key := hex.EncodeToString(b[:])
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for k, v := range s.pendingIcons {
		if time.Now().After(v.Expires) || v.AppID == a.ID {
			delete(s.pendingIcons, k)
		}
	}
	if len(s.pendingIcons) >= 128 {
		for k := range s.pendingIcons {
			delete(s.pendingIcons, k)
			break
		}
	}
	s.pendingIcons[key] = pendingIcon{AppID: a.ID, CraftID: a.CraftID, Candidate: c, Expires: time.Now().Add(5 * time.Minute)}
	return key
}
func (s *Server) simpleAPI(w http.ResponseWriter, r *http.Request, p string) bool {
	ctx := r.Context()
	switch p {
	case "api/rebrand", "api/restore-rebrand":
		// Opt-in internal ("full") skin: sets the name + icon inside a
		// third-party app and re-signs it, so the identity shows while the
		// app runs. Journalled and reversible via RestoreRebrand.
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			AppID     string `json:"appId"`
			AssetID   string `json:"assetId"`
			Name      string `json:"name"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if !b.Confirmed {
			fail(w, errors.New("explicit full-skin confirmation required"))
			return true
		}
		var err error
		if p == "api/rebrand" {
			_, err = s.Engine.Rebrand(ctx, b.AppID, b.AssetID, b.Name)
		} else {
			_, err = s.Engine.RestoreRebrand(ctx, b.AppID)
		}
		if err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]bool{"ok": true})
		}
		return true
	case "api/keepalive":
		// Toggles the background keep-applied LaunchAgent, which repairs
		// icons/names that apps or updaters revert while IconCraft is closed.
		var b struct {
			Enabled bool `json:"enabled"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		agent, ok := s.Engine.Native.(interface {
			SetKeepAgent(bool) error
			KeepAgentEnabled() bool
		})
		if !ok {
			fail(w, errors.New("keep-applied agent is unavailable on this system"))
			return true
		}
		if err := agent.SetKeepAgent(b.Enabled); err != nil {
			fail(w, err)
		} else {
			// Journal the intent: if anything later removes the agent
			// externally, the running app restores it (desired-state).
			_ = s.Engine.SetKeeperPreference(b.Enabled)
			respond(w, 200, map[string]bool{"enabled": agent.KeepAgentEnabled()})
		}
		return true
	case "api/support":
		// Opens a fixed external link (PayPal contributions or the GitHub
		// repo) in the user's real browser via the native bridge. The page
		// can only name a target, never supply a URL.
		var b struct {
			Target string `json:"target"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		url := core.SupportURL
		if b.Target == "github" {
			url = core.RepositoryURL
		} else if b.Target != "paypal" {
			fail(w, errors.New("unknown support link"))
			return true
		}
		opener, ok := s.Engine.Native.(interface {
			OpenURL(context.Context, string) error
		})
		if !ok {
			respond(w, 200, map[string]any{"url": url, "opened": false})
			return true
		}
		if err := opener.OpenURL(ctx, url); err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]any{"url": url, "opened": true})
		}
		return true
	case "api/open-settings":
		// Opens System Settings at App Management so the user can allow
		// IconCraft to change protected apps' icons. Fixed target only.
		var b struct {
			Target string `json:"target"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if b.Target != "appManagement" {
			fail(w, errors.New("unknown settings target"))
			return true
		}
		opener, ok := s.Engine.Native.(interface {
			OpenURL(context.Context, string) error
		})
		if !ok {
			respond(w, 200, map[string]any{"opened": false})
			return true
		}
		if err := opener.OpenURL(ctx, core.AppManagementURL); err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]any{"opened": true})
		}
		return true
	case "api/theme":
		var b struct {
			Theme string `json:"theme"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if err := s.Engine.SetTheme(b.Theme); err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]bool{"ok": true})
		}
		return true
	case "api/rename":
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			AppID     string `json:"appId"`
			Name      string `json:"name"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if !b.Confirmed {
			fail(w, errors.New("rename confirmation required"))
			return true
		}
		a, err := s.Engine.Rename(ctx, b.AppID, b.Name)
		if err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]any{"app": a})
		}
		return true
	case "api/restore-name":
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			AppID     string `json:"appId"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if !b.Confirmed {
			fail(w, errors.New("restore confirmation required"))
			return true
		}
		a, err := s.Engine.RestoreName(ctx, b.AppID)
		if err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]any{"app": a})
		}
		return true
	case "api/donor-choice":
		var b struct {
			AppID   string `json:"appId"`
			DonorID string `json:"donorId"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if _, err := s.Engine.GetApp(b.AppID); err != nil {
			fail(w, err)
			return true
		}
		a, err := s.Engine.CopyDonor(ctx, b.DonorID)
		if err == nil {
			err = s.Engine.RememberChoice(b.AppID, a.ID)
		}
		if err != nil {
			fail(w, err)
		} else {
			respond(w, 200, map[string]any{"asset": a})
		}
		return true

	case "api/original":
		var body struct {
			AppID   string   `json:"appId"`
			Exclude []string `json:"exclude,omitempty"`
		}
		if err := readJSON(w, r, &body); err != nil {
			fail(w, err)
			return true
		}
		a, err := s.Engine.GetApp(body.AppID)
		if err != nil {
			fail(w, err)
			return true
		}
		if a.CraftID == "" {
			fail(w, errors.New("no known counterpart for this application"))
			return true
		}
		if len(body.Exclude) > 6 {
			fail(w, errors.New("too many icon retries"))
			return true
		}
		c, networkErr := s.Resolver.Resolve(ctx, a.CraftID, body.Exclude)
		if networkErr == nil {
			// PNG bytes remain exact. SVG/JPEG artwork is rasterized by the built-in WebKit canvas.
			if c.MIME == "image/png" {
				asset, err := s.Engine.ImportPNG(c.Product.Name, "Publisher original: "+c.Source, c.Data)
				if err == nil {
					err = s.Engine.RememberOriginal(a.CraftID, c.Product.Name, c.Source, asset.ID)
				}
				if err != nil {
					fail(w, err)
					return true
				}
				respond(w, 200, map[string]any{"asset": asset, "product": c.Product.Name, "source": c.Source, "status": "fresh"})
				return true
			}
			key := s.ticket(a, c)
			respond(w, 200, map[string]any{"ticket": key, "dataURL": c.DataURL(), "product": c.Product.Name, "source": c.Source, "status": "render"})
			return true
		}
		// Prefer the last successfully fetched genuine original, then the installed
		// vendor app. Neither fallback is represented as a newly retrieved online icon.
		state := s.Engine.SnapshotState()
		if old, ok := state.Originals[a.CraftID]; ok {
			if asset, exists := state.Assets[old.AssetID]; exists {
				if _, e := s.Engine.AssetPath(asset.ID); e == nil {
					respond(w, 200, map[string]any{"asset": asset, "product": old.Product, "source": old.Source, "status": "cached", "note": "Could not check online; using saved original"})
					return true
				}
			}
		}
		if product, ok := originals.ProductFor(a.CraftID); ok {
			for _, donor := range s.Engine.InstalledApps() {
				// Exact bundle ID, year/variant-suffixed bundle ID, or exact
				// product name — never a lookalike (e.g. AutoCAD LT).
				if !product.MatchesInstalled(donor.Name, donor.BundleID) {
					continue
				}
				asset, err := s.Engine.CopyDonor(ctx, donor.ID)
				if err != nil {
					continue
				}
				if err = s.Engine.RememberOriginal(a.CraftID, product.Name, asset.Source, asset.ID); err != nil {
					fail(w, err)
					return true
				}
				respond(w, 200, map[string]any{"asset": asset, "product": product.Name, "source": asset.Source, "status": "installed", "note": "Using original from installed " + product.Name})
				return true
			}
			fail(w, fmt.Errorf("%w; also checked: saved originals (none) and %d installed apps (no genuine %s installed)", networkErr, len(s.Engine.InstalledApps()), product.Name))
			return true
		}
		fail(w, networkErr)
		return true
	case "api/original/commit":
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			fail(w, errors.New("JSON required"))
			return true
		}
		bytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			fail(w, err)
			return true
		}
		var body struct {
			Ticket string `json:"ticket"`
			PNG    string `json:"png"`
		}
		if err = core.DecodeStrict(bytes, &body); err != nil {
			fail(w, err)
			return true
		}
		s.pendingMu.Lock()
		pending, ok := s.pendingIcons[body.Ticket]
		delete(s.pendingIcons, body.Ticket)
		s.pendingMu.Unlock()
		if !ok || time.Now().After(pending.Expires) {
			fail(w, errors.New("original icon request expired; scan again"))
			return true
		}
		if !strings.HasPrefix(body.PNG, "data:image/png;base64,") {
			fail(w, errors.New("rendered image must be PNG"))
			return true
		}
		png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(body.PNG, "data:image/png;base64,"))
		if err != nil {
			fail(w, err)
			return true
		}
		asset, err := s.Engine.ImportPNG(pending.Candidate.Product.Name, "Publisher original: "+pending.Candidate.Source, png)
		if err == nil {
			err = s.Engine.RememberOriginal(pending.CraftID, pending.Candidate.Product.Name, pending.Candidate.Source, asset.ID)
		}
		if err != nil {
			fail(w, err)
			return true
		}
		respond(w, 200, map[string]any{"asset": asset, "product": pending.Candidate.Product.Name, "source": pending.Candidate.Source, "status": "fresh"})
		return true
	case "api/custom":
		var b struct {
			AppID string `json:"appId"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if _, err := s.Engine.GetApp(b.AppID); err != nil {
			fail(w, err)
			return true
		}
		p, err := s.Engine.Native.Choose(ctx, "icon")
		if err != nil {
			fail(w, err)
			return true
		}
		if p == "" {
			respond(w, 200, map[string]bool{"cancelled": true})
			return true
		}
		asset, err := s.Engine.ImportFile(ctx, p)
		if err == nil {
			err = s.Engine.RememberChoice(b.AppID, asset.ID)
		}
		if err != nil {
			fail(w, err)
			return true
		}
		respond(w, 200, map[string]any{"asset": asset})
		return true
	case "api/choice":
		var b struct {
			AppID   string `json:"appId"`
			AssetID string `json:"assetId"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if err := s.Engine.RememberChoice(b.AppID, b.AssetID); err != nil {
			fail(w, err)
			return true
		}
		respond(w, 200, map[string]bool{"ok": true})
		return true
	case "api/restore-app":
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			AppID     string `json:"appId"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if !b.Confirmed {
			fail(w, errors.New("explicit restore confirmation required"))
			return true
		}
		// Vault restore: strict journalled undo first; if the app changed
		// since skinning (update/move = new identifier), the saved original
		// name and icon are rebuilt onto the current app by bundle ID.
		rep, err := s.Engine.RestoreFromVault(ctx, b.AppID)
		if err != nil && rep.Restored == 0 {
			fail(w, err)
			return true
		}
		out := map[string]any{"restored": rep.Restored, "steps": rep.Steps}
		if err != nil {
			out["error"] = err.Error() // partial restore: report what did get rebuilt
		}
		respond(w, 200, out)
		return true
	case "api/restore-all":
		s.operationMu.Lock()
		defer s.operationMu.Unlock()
		var b struct {
			Confirmed bool `json:"confirmed"`
		}
		if err := readJSON(w, r, &b); err != nil {
			fail(w, err)
			return true
		}
		if !b.Confirmed {
			fail(w, errors.New("explicit restore confirmation required"))
			return true
		}
		restored := 0
		failures := []string{}
		for _, a := range s.Engine.InstalledApps() {
			rep, err := s.Engine.RestoreFromVault(ctx, a.ID)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", a.Name, err))
				continue
			}
			restored += rep.Restored
		}
		respond(w, 200, map[string]any{"restored": restored, "errors": failures})
		return true
	}
	return false
}
