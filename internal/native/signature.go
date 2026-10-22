package native

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SignatureInfo describes how the running IconCraft copy itself is signed,
// so the Setup & Permissions screen can tell the truth about it: a
// Developer ID identity keeps TCC grants such as App Management across
// updates, while each ad-hoc build is a new app to macOS.
type SignatureInfo struct {
	// Kind is "developer-id", "development" (Apple Development
	// certificate), "adhoc", "unsigned", "unbundled" (not running from an
	// .app) or "unknown" (codesign unavailable or unparseable).
	Kind      string `json:"kind"`
	Notarized bool   `json:"notarized"`
	Team      string `json:"team,omitempty"`
	Authority string `json:"authority,omitempty"`
	// Source records how notarisation was established ("spctl: Notarized
	// Developer ID", "stapled ticket") or what spctl answered instead, so a
	// wrong verdict can be diagnosed from the log.
	Source string `json:"source,omitempty"`
}

// DeveloperID reports whether the copy carries a Developer ID signature.
func (s SignatureInfo) DeveloperID() bool { return s.Kind == "developer-id" }

// notarizationRecheck bounds how often an unconfirmed notarisation is asked
// about again: Gatekeeper may need the network for an unstapled ticket, and
// a slow first answer must not pin "not notarised" for the whole session.
const notarizationRecheck = 5 * time.Minute

var selfSig = struct {
	sync.Mutex
	once      sync.Once
	info      SignatureInfo
	bundle    string
	lastSpctl time.Time
}{}

// SelfSignature inspects the running app bundle (codesign once; spctl for
// the notarisation verdict, re-asked periodically until it confirms) and
// returns the cached picture.
func (m *Mac) SelfSignature(ctx context.Context) SignatureInfo {
	selfSig.once.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			selfSig.info = SignatureInfo{Kind: "unknown"}
			return
		}
		selfSig.bundle = bundlePath(exe)
		if selfSig.bundle == "" {
			selfSig.info = SignatureInfo{Kind: "unbundled"}
			return
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cctx, "/usr/bin/codesign", "-dv", "--verbose=2", selfSig.bundle).CombinedOutput()
		selfSig.info = ParseCodesign(string(out), err)
	})
	selfSig.Lock()
	defer selfSig.Unlock()
	info := selfSig.info
	if !info.DeveloperID() || info.Notarized {
		return info
	}
	if time.Since(selfSig.lastSpctl) < notarizationRecheck {
		return info
	}
	selfSig.lastSpctl = time.Now()
	sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(sctx, "/usr/bin/spctl", "--assess", "--type", "execute", "--verbose=2", selfSig.bundle).CombinedOutput()
	info.Notarized, info.Source = NotarizationVerdict(string(out), hasStapledTicket(selfSig.bundle))
	selfSig.info = info
	return info
}

// NotarizationVerdict combines Gatekeeper's answer with the presence of a
// stapled ticket. spctl is authoritative when it names a notarised source;
// when it is silent or inconclusive (offline, slow, or an older spelling),
// a ticket stapled into the bundle is accepted as evidence, because only
// notarytool-issued tickets are ever stapled. An explicit rejection wins.
func NotarizationVerdict(spctlOut string, stapled bool) (bool, string) {
	if ParseSpctlNotarized(spctlOut) {
		return true, "spctl: Notarized Developer ID"
	}
	source := spctlSource(spctlOut)
	if strings.Contains(spctlOut, ": rejected") {
		return false, "spctl: rejected" + optional(source)
	}
	if stapled {
		return true, "stapled ticket" + optional(source)
	}
	if source == "" {
		return false, "spctl: no verdict"
	}
	return false, "spctl: " + source
}

func optional(s string) string {
	if s == "" {
		return ""
	}
	return " (spctl: " + s + ")"
}

// hasStapledTicket reports whether `xcrun stapler staple` has attached a
// notarisation ticket to the bundle (always Contents/CodeResources).
func hasStapledTicket(bundle string) bool {
	fi, err := os.Stat(filepath.Join(bundle, "Contents", "CodeResources"))
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}

// bundlePath returns the .app enclosing an executable at
// <bundle>/Contents/MacOS/<exe>, or "" when it is not inside one.
func bundlePath(exe string) string {
	exe = filepath.Clean(exe)
	dir := filepath.Dir(exe)
	if filepath.Base(dir) != "MacOS" {
		return ""
	}
	contents := filepath.Dir(dir)
	if filepath.Base(contents) != "Contents" {
		return ""
	}
	app := filepath.Dir(contents)
	if !strings.HasSuffix(strings.ToLower(app), ".app") {
		return ""
	}
	return app
}

// ParseCodesign classifies `codesign -dv --verbose=2` output. The command
// writes its report to stderr and exits non-zero only for unsigned or
// unreadable bundles.
func ParseCodesign(out string, runErr error) SignatureInfo {
	info := SignatureInfo{Kind: "unknown"}
	if strings.Contains(out, "code object is not signed at all") {
		info.Kind = "unsigned"
		return info
	}
	if runErr != nil && strings.TrimSpace(out) == "" {
		return info
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "Signature=adhoc":
			info.Kind = "adhoc"
		case strings.HasPrefix(line, "TeamIdentifier="):
			if v := strings.TrimPrefix(line, "TeamIdentifier="); v != "not set" {
				info.Team = v
			}
		case strings.HasPrefix(line, "Authority="):
			auth := strings.TrimPrefix(line, "Authority=")
			if info.Authority == "" {
				info.Authority = auth
			}
			switch {
			case strings.HasPrefix(auth, "Developer ID Application:"):
				info.Kind = "developer-id"
			case strings.HasPrefix(auth, "Apple Development:") || strings.HasPrefix(auth, "Mac Developer:"):
				if info.Kind == "unknown" {
					info.Kind = "development"
				}
			}
		}
	}
	if info.Kind == "unknown" && strings.Contains(out, "CodeDirectory") && info.Authority == "" {
		// Signed, but with no certificate chain reported: treat as ad-hoc
		// rather than claiming an identity that is not there.
		info.Kind = "adhoc"
	}
	return info
}

// ParseSpctlNotarized reads the Gatekeeper assessment, in its plain
// ("source=Notarized Developer ID") or --raw plist spelling.
func ParseSpctlNotarized(out string) bool {
	return strings.Contains(out, "Notarized Developer ID")
}

// spctlSource extracts the `source=` line of a plain spctl assessment.
func spctlSource(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "source=") {
			return strings.TrimPrefix(line, "source=")
		}
	}
	return ""
}
