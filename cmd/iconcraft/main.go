package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
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
	version := flag.Bool("version", false, "Print build version")
	flag.Parse()
	if *version {
		fmt.Println(core.Version)
		return nil
	}
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
	log.Printf("starting %s on %s/%s", core.Version, runtime.GOOS, runtime.GOARCH)
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
		return fmt.Errorf("another IconCraft instance is still running. Quit IconCraft (or end the IconCraft process in Activity Monitor if it has no window), then reopen it. Your icon backups have not been changed")
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
	go keepAppliedLoop(ctx, e)
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
		err = m.Window(ctx, s.URL, icon)
		if err != nil {
			log.Printf("native window: %v", err)
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

// keepAppliedLoop periodically repairs skins reverted by app launches or
// updaters while IconCraft itself is running. The first pass is delayed so
// startup scanning settles; failures are logged, never surfaced as dialogs.
func keepAppliedLoop(ctx context.Context, e *core.Engine) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
	}
	for {
		if _, _, err := e.Scan(ctx); err != nil {
			log.Printf("keep-applied scan: %v", err)
		} else if res, err := e.Reconcile(ctx); err != nil {
			log.Printf("keep-applied reconcile: %v", err)
		} else if res.IconFixed > 0 || res.NameFixed > 0 {
			log.Printf("keep-applied repaired %d icon(s), %d name(s); skipped: %v", res.IconFixed, res.NameFixed, res.Skipped)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Minute):
		}
	}
}
