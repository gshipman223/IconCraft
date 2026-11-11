//go:build windows

package native

import (
	"context"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32  = syscall.NewLazyDLL("user32.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")

	procRegisterClass = user32.NewProc("RegisterClassExW")
	procCreateWindow  = user32.NewProc("CreateWindowExW")
	procDefWindowProc = user32.NewProc("DefWindowProcW")
	procGetMessage    = user32.NewProc("GetMessageW")
	procTranslate     = user32.NewProc("TranslateMessage")
	procDispatch      = user32.NewProc("DispatchMessageW")
	procPostQuit      = user32.NewProc("PostQuitMessage")
	procDestroyWindow = user32.NewProc("DestroyWindow")
	procLoadIcon      = user32.NewProc("LoadIconW")
	procCreateMenu    = user32.NewProc("CreatePopupMenu")
	procAppendMenu    = user32.NewProc("AppendMenuW")
	procTrackMenu     = user32.NewProc("TrackPopupMenu")
	procSetForeground = user32.NewProc("SetForegroundWindow")
	procGetCursorPos  = user32.NewProc("GetCursorPos")
	procShellNotify   = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy    = 0x0002
	wmCommand    = 0x0111
	wmLButtonUp  = 0x0202
	wmRButtonUp  = 0x0205
	wmUser       = 0x0400
	nimAdd       = 0
	nimModify    = 1
	nimDelete    = 2
	nifMessage   = 0x1
	nifIcon      = 0x2
	nifTip       = 0x4
	idiApp       = 32512
	mfString     = 0
	tpmReturnCmd = 0x0100
	idOpen       = 1
	idQuit       = 2
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type point struct{ X, Y int32 }

type msg struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIcon struct {
	Size      uint32
	Hwnd      syscall.Handle
	ID        uint32
	Flags     uint32
	Callback  uint32
	Icon      syscall.Handle
	Tip       [128]uint16
	State     uint32
	StateMask uint32
	Info      [256]uint16
	Timeout   uint32
	InfoTitle [64]uint16
	InfoFlags uint32
	Guid      [16]byte
	Balloon   syscall.Handle
}

type trayState struct {
	hwnd   syscall.Handle
	open   func()
	cancel context.CancelFunc
}

var (
	trayWndProc = syscall.NewCallback(trayProc)
	activeTray  trayState
)

func trayProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	if message == wmUser+1 {
		switch lparam {
		case wmLButtonUp:
			if activeTray.open != nil {
				activeTray.open()
			}
		case wmRButtonUp:
			cmd := trackTrayMenu(syscall.Handle(hwnd))
			if cmd == idQuit && activeTray.cancel != nil {
				activeTray.cancel()
			}
			if cmd == idOpen && activeTray.open != nil {
				activeTray.open()
			}
		}
		return 0
	}
	if message == wmDestroy {
		procPostQuit.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wparam, lparam)
	return r
}

func trackTrayMenu(hwnd syscall.Handle) uintptr {
	menu, _, _ := procCreateMenu.Call()
	if menu == 0 {
		return 0
	}
	open, _ := syscall.UTF16PtrFromString("Open IconCraft")
	quit, _ := syscall.UTF16PtrFromString("Quit")
	procAppendMenu.Call(menu, mfString, idOpen, uintptr(unsafe.Pointer(open)))
	procAppendMenu.Call(menu, mfString, idQuit, uintptr(unsafe.Pointer(quit)))
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForeground.Call(uintptr(hwnd))
	cmd, _, _ := procTrackMenu.Call(menu, tpmReturnCmd, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
	return cmd
}

func (w *Windows) trayLoop(ctx context.Context, cancel context.CancelFunc, _ string, open func()) (err error) {
	defer func() {
		if recover() != nil {
			err = syscall.EINVAL
		}
	}()
	runtime.LockOSThread()
	class, err := syscall.UTF16PtrFromString("IconCraftTray")
	if err != nil {
		return err
	}
	wc := wndClassEx{WndProc: trayWndProc, ClassName: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := syscall.UTF16PtrFromString("IconCraft")
	hwnd, _, _ := procCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		return syscall.EINVAL
	}
	h := syscall.Handle(hwnd)
	icon, _, _ := procLoadIcon.Call(0, uintptr(idiApp))
	nid := notifyIcon{Hwnd: h, ID: 1, Flags: nifMessage | nifIcon | nifTip, Callback: wmUser + 1, Icon: syscall.Handle(icon)}
	nid.Size = uint32(unsafe.Sizeof(nid))
	copy(nid.Tip[:], syscall.StringToUTF16("IconCraft"))
	activeTray = trayState{hwnd: h, open: open, cancel: cancel}
	procShellNotify.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	defer procShellNotify.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	go func() {
		<-ctx.Done()
		procDestroyWindow.Call(hwnd)
	}()
	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil
		}
		procTranslate.Call(uintptr(unsafe.Pointer(&m)))
		procDispatch.Call(uintptr(unsafe.Pointer(&m)))
	}
}
