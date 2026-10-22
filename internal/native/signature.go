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
}

// DeveloperID reports whether the copy carries a Developer ID signature.
func (s SignatureInfo) DeveloperID() bool { return s.Kind == "developer-id" }

var (
	selfSigOnce sync.Once
	selfSig     SignatureInfo
)

// SelfSignature inspects the running app bundle once (codesign, then spctl
// for the notarisation verdict) and caches the result for the process.
func (m *Mac) SelfSignature(ctx context.Context) SignatureInfo {
	selfSigOnce.Do(func() {
		selfSig = inspectSelf(ctx)
	})
	return selfSig
}

func inspectSelf(ctx context.Context) SignatureInfo {
	exe, err := os.Executable()
	if err != nil {
		return SignatureInfo{Kind: "unknown"}
	}
	bundle := bundlePath(exe)
	if bundle == "" {
		return SignatureInfo{Kind: "unbundled"}
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "/usr/bin/codesign", "-dv", "--verbose=2", bundle).CombinedOutput()
	info := ParseCodesign(string(out), err)
	if info.DeveloperID() {
		sctx, scancel := context.WithTimeout(ctx, 30*time.Second)
		defer scancel()
		sout, _ := exec.CommandContext(sctx, "/usr/bin/spctl", "--assess", "--type", "execute", "--verbose=2", bundle).CombinedOutput()
		info.Notarized = ParseSpctlNotarized(string(sout))
	}
	return info
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

// ParseSpctlNotarized reads the Gatekeeper assessment: only a stapled or
// online-verified notarisation ticket yields "source=Notarized Developer ID".
func ParseSpctlNotarized(out string) bool {
	return strings.Contains(out, "source=Notarized Developer ID")
}
