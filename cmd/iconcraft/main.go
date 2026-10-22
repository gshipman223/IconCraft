package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
	"github.com/gshipman223/IconCraft/internal/native"
	"github.com/gshipman223/IconCraft/internal/server"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"
)

// errAlreadyRunning means a healthy instance already holds the journal
// lock. Relaunching (e.g. double-clicking while the agent-style app is
// open on another Space) exits quietly — it is not a launch failure and
// must never produce a "couldn't open" dialog.
var errAlreadyRunning = errors.New("another IconCraft instance is still running")

func main() { os.Exit(entry()) }

// A LaunchServices launch has no visible terminal. Report failures instead of
// silently exiting, including panics that occur before the native window exists.
func entry() (code int) {
	defer func() {
		if v := recover(); v != nil {
			reportLaunchFailure(fmt.Errorf("unexpected startup failure: %v\n%s", v, debug.Stack()))
			code = 1
		}
	}()
	if err := run(); err != nil {
		if errors.Is(err, errAlreadyRunning) {
			fmt.Fprintln(os.Stderr, "IconCraft: already running; exiting quietly")
			return 0
		}
		reportLaunchFailure(err)
		return 1
	}
	return 0
}
func run() (result error) {
	self := flag.Bool("self-test", false, "Run macOS-native checks using disposable fixtures only")
	report := flag.String("report", "", "Write native diagnostic report to this path")
	headless := flag.Bool("headless", false, "Run the local backend without a native window (development)")
	reapply := flag.Bool("reapply", false, "Repair reverted icons/names from the journal and exit (keep-applied agent)")
	atLogin := flag.Bool("at-login", false, "Launched by the launch-at-login agent")
	menuOnly := flag.Bool("menu-only", false, "Start in the menu bar without showing the window")
	version := flag.Bool("version", false, "Print build version")
	flag.Parse()
	if *version {
		fmt.Println(core.Version)
		return nil
	}
	// UI language follows the macOS preferred languages (English default).
	// Chosen once per process: the window, the page and notifications all
	// read it from here. Logs and the journal stay English regardless.
	i18n.Set(i18n.Detect())
	if runtime.GOOS != "darwin" && !*headless && !*self {
		return fmt.Errorf("this application requires macOS; current system is %s", runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library/Application Support/IconCraft")
	if runtime.GOOS != "darwin" {
		dir = filepath.Join(home, ".local/share/IconCraft")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	logfile, err := os.OpenFile(filepath.Join(dir, "iconcraft.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logfile.Close()
	log.SetOutput(logfile)
	log.Printf("starting %s on %s/%s (ui language %s)", core.Version, runtime.GOOS, runtime.GOARCH, i18n.Current())
	defer func() {
		if result != nil {
			log.Printf("startup/exit error: %v", result)
		}
		log.SetOutput(os.Stderr)
	}()
	// Advisory lock prevents two copies from racing the backup journal.
	lock, err := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if *reapply {
			// The running instance does its own periodic repair; the agent
			// pass simply defers to it. Not an error, never a dialog.
			log.Printf("reapply deferred: another IconCraft instance is running")
			return nil
		}
		return errAlreadyRunning
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	m, err := native.New(dir)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *self {
		r := m.SelfTest(ctx)
		b, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(b))
		if *report != "" {
			if err = native.WriteReport(*report, r); err != nil {
				return err
			}
		}
		if !r.Passed {
			return fmt.Errorf("native diagnostics did not pass")
		}
		return nil
	}
	e, err := core.Open(dir, home, m)
	if err != nil {
		return err
	}
	if *reapply {
		if _, _, err := e.Scan(ctx); err != nil {
			return err
		}
		res, err := e.Reconcile(ctx)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(res)
		fmt.Println(string(b))
		log.Printf("reapply: checked=%d iconsFixed=%d namesFixed=%d skipped=%v", res.Checked, res.IconFixed, res.NameFixed, res.Skipped)
		return nil
	}
	// Keep skins applied even while the main app is open: apps/updaters that
	// revert a Finder icon or folder name on launch get repaired in the
	// background. When the app is closed, the LaunchAgent (--reapply) does it.
	e.InitKeeperPreference(m.KeepAgentEnabled())
	ensureKeeper(e, m)
	ensureLoginAgent(e, m)
	go keepAppliedLoop(ctx, e, m)
	go autoUpdateCheck(ctx, e, m)
	s := server.New(e)
	listener, err := s.Start()
	if err != nil {
		return err
	}
	defer listener.Close()
	// No developer diagnostic workflow on launch. Every Apply still journals the
	// original metadata, verifies the bundle and rolls back a failed write.
	if *headless {
		fmt.Println(s.URL)
		<-ctx.Done()
	} else {
		exe, _ := os.Executable()
		icon := filepath.Clean(filepath.Join(filepath.Dir(exe), "../Resources/icon.icns"))
		settings := e.Settings()
		startInMenuBar := *menuOnly || (*atLogin && settings.StartInMenuBar)
		if startInMenuBar {
			log.Printf("starting in menu bar (no window until opened)")
		}
		err = m.Window(ctx, s.URL, icon, startInMenuBar)
		if err != nil {
			log.Printf("native window: %v", err)
			if ctx.Err() != nil || errors.Is(err, native.ErrWindowKilled) {
				// Shutting down (signal received) or the window was
				// force-closed/replaced: a normal close, never an
				// error — and never the "couldn't open" dialog.
				err = nil
			}
		}
	}
	// Finish active transactions before exit; interrupted operations retain a prepared recovery record.
	shutdown, stop := context.WithTimeout(context.Background(), 90*time.Second)
	defer stop()
	if se := s.Shutdown(shutdown); err == nil {
		err = se
	}
	return err
}

// ensureKeeper restores the keep-applied LaunchAgent when the user's
// journalled intent is "on" but the agent has gone missing — cleaner
// tools and other external actors sometimes remove launch agents, and
// silently losing skin persistence would be far worse than quietly
// putting it back. The toggle in the UI/menu always wins.
func ensureKeeper(e *core.Engine, m *native.Mac) {
	desired, set := e.KeeperPreference()
	if !set || !desired || m.KeepAgentEnabled() {
		return
	}
	if err := m.SetKeepAgent(true); err != nil {
		log.Printf("keep-applied agent restore: %v", err)
	} else {
		log.Printf("keep-applied agent was missing although enabled; restored it")
	}
}

// ensureLoginAgent reconciles the launch-at-login LaunchAgent with the
// saved preference, the same self-healing approach as the keeper: the
// preference is the intent, the plist is just its current expression.
func ensureLoginAgent(e *core.Engine, m *native.Mac) {
	s := e.Settings()
	if m.LoginAgentEnabled() == s.LaunchAtLogin {
		if !s.LaunchAtLogin {
			return
		}
		// Present and wanted: still rewrite so a changed
		// start-in-menu-bar choice is reflected in the arguments.
	}
	if err := m.SetLoginAgent(s.LaunchAtLogin, s.StartInMenuBar); err != nil {
		log.Printf("launch-at-login agent: %v", err)
	}
}

// autoUpdateCheck checks GitHub for a newer release shortly after
// startup when the user opted in and the last check is stale. It only
// ever informs — updating stays a deliberate download by the user.
func autoUpdateCheck(ctx context.Context, e *core.Engine, m *native.Mac) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(45 * time.Second):
	}
	s := e.Settings()
	if !s.AutoCheckUpdates {
		return
	}
	if s.LastUpdateCheck != "" {
		if t, err := time.Parse(time.RFC3339, s.LastUpdateCheck); err == nil && time.Since(t) < 6*24*time.Hour {
			return
		}
	}
	res, err := e.CheckForUpdate(ctx)
	if err != nil {
		log.Printf("update check: %v", err)
		return
	}
	if res.UpdateAvailable && s.NotifyAttention {
		_ = m.NotifyUser(ctx, i18n.T("IconCraft %s is available", res.Latest), i18n.T("You have %s. Open IconCraft → Settings to get the new version.", res.Current))
	}
}

// keepAppliedLoop periodically repairs skins reverted by app launches or
// updaters while IconCraft itself is running. The first pass is delayed so
// startup scanning settles; failures are logged, never surfaced as dialogs.
func keepAppliedLoop(ctx context.Context, e *core.Engine, m *native.Mac) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
	}
	var lastAttention time.Time
	for {
		ensureKeeper(e, m)
		ensureLoginAgent(e, m)
		if _, _, err := e.Scan(ctx); err != nil {
			log.Printf("keep-applied scan: %v", err)
		} else if res, err := e.Reconcile(ctx); err != nil {
			log.Printf("keep-applied reconcile: %v", err)
		} else if res.IconFixed > 0 || res.NameFixed > 0 {
			log.Printf("keep-applied repaired %d icon(s), %d name(s); skipped: %v", res.IconFixed, res.NameFixed, res.Skipped)
			settings := e.Settings()
			if settings.NotifyRepaired {
				_ = m.NotifyUser(ctx, i18n.T("IconCraft re-applied your skins"), i18n.T("%d icon(s) and %d name(s) had been reverted and are fixed.", res.IconFixed, res.NameFixed))
			}
		} else if len(res.Skipped) > 0 {
			// At most one attention banner every 6 hours: a skipped
			// app (often just a running one) must not nag.
			if e.Settings().NotifyAttention && time.Since(lastAttention) > 6*time.Hour {
				lastAttention = time.Now()
				_ = m.NotifyUser(ctx, i18n.T("Some skins need your attention"), i18n.T("Open IconCraft to review — an app may have updated or be running."))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Minute):
		}
	}
}
