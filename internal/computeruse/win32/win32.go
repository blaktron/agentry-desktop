//go:build windows

// Package win32 is the computer-use backend for Windows. It lists the
// top-level windows of the applications a run may use, captures one window
// with PrintWindow (which draws a window's own content, a covered one's
// included, and leaves the screen alone), and posts input with SendInput.
//
// Windows holds no input to a window: SendInput goes to the foreground
// window, and User Interface Privilege Isolation drops, without a word, input
// aimed at a window of a process with a higher integrity level (an
// application run as administrator). So the backend refuses a window of an
// elevated process when Agentry Desktop is not elevated itself, and the check
// that an allowed window is in the foreground and the point inside it before
// each input is the caller's, with Covered beside it.
//
// Every library is loaded when first called, never linked: user32, gdi32 and
// dwmapi are part of every Windows, and the app starts without reaching them.
package win32

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// screenID is the one window of screen scope.
const screenID = "screen"

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procShowWindow               = user32.NewProc("ShowWindow")
	procGetAncestor              = user32.NewProc("GetAncestor")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procSetCursorPos             = user32.NewProc("SetCursorPos")
	procSendInput                = user32.NewProc("SendInput")
	procWindowFromPoint          = user32.NewProc("WindowFromPoint")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procPrintWindow              = user32.NewProc("PrintWindow")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procBitBlt                 = gdi32.NewProc("BitBlt")

	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	gaRoot              = 2
	swRestore           = 9
	pwRenderFullContent = 2
	dwmwaCloaked        = 14
	srccopy             = 0x00cc0020
	captureblt          = 0x40000000
	smXVirtualScreen    = 76
	smYVirtualScreen    = 77
	smCXVirtualScreen   = 78
	smCYVirtualScreen   = 79

	inputMouse    = 0
	inputKeyboard = 1

	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseMiddleDown = 0x0020
	mouseMiddleUp   = 0x0040
	mouseWheel      = 0x0800
	mouseHWheel     = 0x1000
	wheelDelta      = 120

	keyExtended = 0x0001
	keyUp       = 0x0002
	keyUnicode  = 0x0004
)

// call calls a procedure for its first result alone: the calls here say
// whether they worked in that result, and their error is the thread's last
// error, read only where the result says to.
func call(p *windows.LazyProc, args ...uintptr) uintptr {
	r, _, _ := p.Call(args...)
	return r
}

type rect struct{ Left, Top, Right, Bottom int32 }

type point struct{ X, Y int32 }

// mouseInput and keybdInput are MOUSEINPUT and KEYBDINPUT; each input is
// INPUT, a type and the 32 bytes of its union on 64-bit Windows.
type mouseInput struct {
	dx, dy                int32
	mouseData, flags, tim uint32
	extra                 uintptr
}

type keybdInput struct {
	vk, scan   uint16
	flags, tim uint32
	extra      uintptr
	_          [8]byte
}

type mouseEvent struct {
	typ uint32
	_   uint32
	mi  mouseInput
}

type keyEvent struct {
	typ uint32
	_   uint32
	ki  keybdInput
}

type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}

// Options configures Open.
type Options struct {
	// Scope is computeruse.ScopeApp or computeruse.ScopeScreen.
	Scope string
	// Match says whether a window's process belongs to an application the
	// run may use, and that application's name, from the process's
	// executable file name ("blender.exe"). App scope only.
	Match func(exe string) (app string, ok bool)
}

// Backend implements computeruse.Backend on Windows.
type Backend struct {
	mu  sync.Mutex
	opt Options
	// elevatedSelf is whether Agentry Desktop runs elevated: then no
	// window is out of its input's reach.
	elevatedSelf bool
	// held are the modifiers pressed and not released, which make a
	// character a key of a combination rather than typed text.
	held map[uint32]bool
}

var (
	_ computeruse.Backend = (*Backend)(nil)
	_ computeruse.Coverer = (*Backend)(nil)
)

// Open opens computer use for one binding. In app scope it refuses, with
// computeruse.ErrNotGranted, when a window of an application the run uses
// belongs to an elevated process that Agentry Desktop's input cannot reach.
func Open(ctx context.Context, opt Options) (*Backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opt.Scope != computeruse.ScopeApp && opt.Scope != computeruse.ScopeScreen {
		return nil, fmt.Errorf("win32: unknown scope %q", opt.Scope)
	}
	if opt.Scope == computeruse.ScopeApp && opt.Match == nil {
		return nil, errors.New("win32: app scope needs a Match function")
	}
	if err := user32.Load(); err != nil {
		return nil, fmt.Errorf("%w: user32.dll did not load: %v", computeruse.ErrNotGranted, err)
	}
	if err := gdi32.Load(); err != nil {
		return nil, fmt.Errorf("%w: gdi32.dll did not load: %v", computeruse.ErrNotGranted, err)
	}
	b := &Backend{opt: opt, elevatedSelf: windows.GetCurrentProcessToken().IsElevated(), held: map[uint32]bool{}}
	if opt.Scope == computeruse.ScopeApp {
		for _, w := range b.list() {
			if w.elevated {
				return nil, fmt.Errorf("%w: %s runs as administrator, and Windows does not let the input of an application that is not reach it: run %s without administrator rights", computeruse.ErrNotGranted, w.app, w.app)
			}
		}
	}
	return b, nil
}

// Session names the display.
func (b *Backend) Session() string { return "Windows" }

// Enforced says what Windows holds of the scope.
func (b *Backend) Enforced() string {
	if b.opt.Scope == computeruse.ScopeScreen {
		return "the whole display; Windows holds nothing to a window, and in screen scope nothing is checked"
	}
	return "the capture is of the allowed windows alone, drawn by Windows from each window's own content; Windows holds no input to a window, so Agentry Desktop checks before each input that an allowed window is in the foreground, the point inside it and no other window above it there, and refuses a window of an application run as administrator, which its input cannot reach"
}

type win struct {
	hwnd     uintptr
	app      string
	title    string
	r        rect
	elevated bool
}

func (w win) id() string { return strconv.FormatUint(uint64(w.hwnd), 16) }

// list is the allowed windows, front to back as EnumWindows gives them.
func (b *Backend) list() []win {
	var out []win
	exes := map[uint32]string{}
	elev := map[uint32]bool{}
	cb := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		if ok := call(procIsWindowVisible, hwnd); ok == 0 || cloaked(hwnd) {
			return 1
		}
		var r rect
		if ok := call(procGetWindowRect, hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 || r.Right <= r.Left || r.Bottom <= r.Top {
			return 1
		}
		var pid uint32
		call(procGetWindowThreadProcessID, hwnd, uintptr(unsafe.Pointer(&pid)))
		exe, seen := exes[pid]
		if !seen {
			exe = exeName(pid)
			exes[pid] = exe
			if !b.elevatedSelf {
				elev[pid] = processElevated(pid)
			}
		}
		app, ok := b.opt.Match(exe)
		if !ok {
			return 1
		}
		out = append(out, win{hwnd: hwnd, app: app, title: windowText(hwnd), r: r, elevated: elev[pid]})
		return 1
	})
	call(procEnumWindows, cb, 0)
	return out
}

func cloaked(hwnd uintptr) bool {
	if dwmapi.Load() != nil {
		return false
	}
	var v uint32
	hr := call(procDwmGetWindowAttribute, hwnd, dwmwaCloaked, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return hr == 0 && v != 0
}

func windowText(hwnd uintptr) string {
	n := call(procGetWindowTextLengthW, hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	call(procGetWindowTextW, hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

// exeName is the executable file name of a process, lower-cased; empty
// when it cannot be read.
func exeName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return strings.ToLower(filepath.Base(windows.UTF16ToString(buf[:n])))
}

// processElevated reports a process whose token is elevated, or whose token
// this process may not read, which on a desktop means one of a higher
// integrity level.
func processElevated(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var tok windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok); err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = tok.Close() }()
	return tok.IsElevated()
}

func foreground() uintptr {
	fg := call(procGetForegroundWindow)
	if fg == 0 {
		return 0
	}
	root := call(procGetAncestor, fg, gaRoot)
	if root == 0 {
		return fg
	}
	return root
}

func virtualScreen() rect {
	m := func(i int) int32 {
		v := call(procGetSystemMetrics, uintptr(i))
		return int32(v)
	}
	x, y := m(smXVirtualScreen), m(smYVirtualScreen)
	return rect{Left: x, Top: y, Right: x + m(smCXVirtualScreen), Bottom: y + m(smCYVirtualScreen)}
}

// Windows lists the windows the scope allows.
func (b *Backend) Windows(ctx context.Context) ([]computeruse.Window, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.opt.Scope == computeruse.ScopeScreen {
		r := virtualScreen()
		return []computeruse.Window{{ID: screenID, App: "the screen", Size: image.Pt(int(r.Right-r.Left), int(r.Bottom-r.Top)), Focused: true}}, nil
	}
	fg := foreground()
	var out []computeruse.Window
	for _, w := range b.list() {
		out = append(out, computeruse.Window{ID: w.id(), App: w.app, Title: w.title,
			Size: image.Pt(int(w.r.Right-w.r.Left), int(w.r.Bottom-w.r.Top)), Focused: w.hwnd == fg && !w.elevated})
	}
	return out, nil
}

// find is the allowed window of this id, with its rectangle now.
func (b *Backend) find(id string) (win, error) {
	if b.opt.Scope == computeruse.ScopeScreen {
		if id != screenID {
			return win{}, computeruse.ErrNoWindow
		}
		return win{r: virtualScreen()}, nil
	}
	for _, w := range b.list() {
		if w.id() == id {
			if w.elevated {
				return win{}, fmt.Errorf("%w: %s runs as administrator, which Agentry Desktop's input cannot reach", computeruse.ErrNotGranted, w.app)
			}
			return w, nil
		}
	}
	return win{}, computeruse.ErrNoWindow
}

// Focus brings the window to the foreground. Windows lets a process take
// the foreground only after input it sent, so an Alt press and release goes
// first.
func (b *Backend) Focus(ctx context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	w, err := b.find(id)
	if err != nil || b.opt.Scope == computeruse.ScopeScreen {
		return err
	}
	if ic := call(procIsIconic, w.hwnd); ic != 0 {
		call(procShowWindow, w.hwnd, swRestore)
	}
	_ = sendKeys(keyEvent{typ: inputKeyboard, ki: keybdInput{vk: 0x12}}, keyEvent{typ: inputKeyboard, ki: keybdInput{vk: 0x12, flags: keyUp}})
	call(procBringWindowToTop, w.hwnd)
	call(procSetForegroundWindow, w.hwnd)
	deadline := time.Now().Add(time.Second)
	for foreground() != w.hwnd {
		if time.Now().After(deadline) {
			return fmt.Errorf("Windows did not bring %s's window to the front", w.app)
		}
		if err := sleep(ctx, 50*time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Capture draws the window's own content into a bitmap at its size, or the
// whole virtual screen in screen scope.
func (b *Backend) Capture(ctx context.Context, id string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, err := b.find(id)
	if err != nil {
		return nil, err
	}
	width, height := int(w.r.Right-w.r.Left), int(w.r.Bottom-w.r.Top)
	screen := call(procGetDC, 0)
	if screen == 0 {
		return nil, errors.New("Windows gave no screen to draw into")
	}
	defer call(procReleaseDC, 0, screen)
	mem := call(procCreateCompatibleDC, screen)
	if mem == 0 {
		return nil, errors.New("Windows gave no drawing context")
	}
	defer call(procDeleteDC, mem)
	bmp := call(procCreateCompatibleBitmap, screen, uintptr(width), uintptr(height))
	if bmp == 0 {
		return nil, errors.New("Windows gave no bitmap to draw into")
	}
	defer call(procDeleteObject, bmp)
	old := call(procSelectObject, mem, bmp)
	if id == screenID {
		ok := call(procBitBlt, mem, 0, 0, uintptr(width), uintptr(height), screen, uintptr(w.r.Left), uintptr(w.r.Top), srccopy|captureblt)
		if ok == 0 {
			call(procSelectObject, mem, old)
			return nil, errors.New("Windows did not copy the screen")
		}
	} else if ok := call(procPrintWindow, w.hwnd, mem, pwRenderFullContent); ok == 0 {
		call(procSelectObject, mem, old)
		return nil, fmt.Errorf("Windows did not draw %s's window", w.app)
	}
	pix, err := readBits(mem, bmp, old, width, height)
	if err != nil {
		return nil, err
	}
	if id != screenID && black(pix) && foreground() == w.hwnd {
		// A window drawn by the graphics card (Direct3D, Vulkan) can come
		// back black from PrintWindow. While it is the foreground window
		// nothing covers it, so its area of the screen is the window.
		old = call(procSelectObject, mem, bmp)
		if ok := call(procBitBlt, mem, 0, 0, uintptr(width), uintptr(height), screen, uintptr(w.r.Left), uintptr(w.r.Top), srccopy|captureblt); ok == 0 {
			call(procSelectObject, mem, old)
			return nil, fmt.Errorf("Windows did not copy %s's window from the screen", w.app)
		}
		if pix, err = readBits(mem, bmp, old, width, height); err != nil {
			return nil, err
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i+3 < len(pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pix[i+2], pix[i+1], pix[i], 0xff
	}
	return img, nil
}

// readBits deselects the bitmap from the drawing context and reads its
// pixels, top row first, as BGRA.
func readBits(mem, bmp, old uintptr, width, height int) ([]byte, error) {
	call(procSelectObject, mem, old)
	hdr := bitmapInfoHeader{Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	pix := make([]byte, width*height*4)
	if n := call(procGetDIBits, mem, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&hdr)), 0); n == 0 {
		return nil, errors.New("Windows did not hand back the window's pixels")
	}
	return pix, nil
}

// black reports an image with no lit pixel.
func black(bgra []byte) bool {
	for i := 0; i+2 < len(bgra); i += 4 {
		if bgra[i]|bgra[i+1]|bgra[i+2] != 0 {
			return false
		}
	}
	return true
}

// Cursor is the pointer's position in the window.
func (b *Backend) Cursor(ctx context.Context, id string) (image.Point, bool, error) {
	if err := ctx.Err(); err != nil {
		return image.Point{}, false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, err := b.find(id)
	if err != nil {
		return image.Point{}, false, err
	}
	var p point
	if ok := call(procGetCursorPos, uintptr(unsafe.Pointer(&p))); ok == 0 {
		return image.Point{}, false, errors.New("Windows did not say where the pointer is")
	}
	rel := image.Pt(int(p.X-w.r.Left), int(p.Y-w.r.Top))
	in := rel.X >= 0 && rel.Y >= 0 && rel.X < int(w.r.Right-w.r.Left) && rel.Y < int(w.r.Bottom-w.r.Top)
	return rel, in, nil
}

// Move puts the pointer at a position in the window.
func (b *Backend) Move(ctx context.Context, id string, p image.Point) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, err := b.find(id)
	if err != nil {
		return err
	}
	if p.X < 0 || p.Y < 0 || p.X >= int(w.r.Right-w.r.Left) || p.Y >= int(w.r.Bottom-w.r.Top) {
		return fmt.Errorf("the point %v is outside the window", p)
	}
	if ok := call(procSetCursorPos, uintptr(int(w.r.Left)+p.X), uintptr(int(w.r.Top)+p.Y)); ok == 0 {
		return errors.New("Windows did not move the pointer")
	}
	return nil
}

// Covered says another window is above the allowed one at the point.
func (b *Backend) Covered(ctx context.Context, id string, p image.Point) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, err := b.find(id)
	if err != nil || id == screenID {
		return false, err
	}
	at := point{X: w.r.Left + int32(p.X), Y: w.r.Top + int32(p.Y)}
	hit := call(procWindowFromPoint, uintptr(*(*uint64)(unsafe.Pointer(&at))))
	if hit == 0 {
		return true, nil
	}
	root := call(procGetAncestor, hit, gaRoot)
	return root != w.hwnd, nil
}

// Button presses or releases a button where the pointer is.
func (b *Backend) Button(ctx context.Context, btn computeruse.Button, down bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var flags uint32
	switch {
	case btn == computeruse.ButtonLeft && down:
		flags = mouseLeftDown
	case btn == computeruse.ButtonLeft:
		flags = mouseLeftUp
	case btn == computeruse.ButtonRight && down:
		flags = mouseRightDown
	case btn == computeruse.ButtonRight:
		flags = mouseRightUp
	case btn == computeruse.ButtonMiddle && down:
		flags = mouseMiddleDown
	case btn == computeruse.ButtonMiddle:
		flags = mouseMiddleUp
	default:
		return fmt.Errorf("no such button %d", btn)
	}
	return sendMouse(mouseEvent{typ: inputMouse, mi: mouseInput{flags: flags}})
}

// Scroll scrolls by steps of the wheel where the pointer is.
func (b *Backend) Scroll(ctx context.Context, d computeruse.Direction, steps int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var flags uint32
	var amount int32
	switch d {
	case computeruse.ScrollUp:
		flags, amount = mouseWheel, wheelDelta
	case computeruse.ScrollDown:
		flags, amount = mouseWheel, -wheelDelta
	case computeruse.ScrollRight:
		flags, amount = mouseHWheel, wheelDelta
	case computeruse.ScrollLeft:
		flags, amount = mouseHWheel, -wheelDelta
	default:
		return fmt.Errorf("no such direction %q", d)
	}
	for i := 0; i < steps; i++ {
		if err := sendMouse(mouseEvent{typ: inputMouse, mi: mouseInput{flags: flags, mouseData: uint32(amount)}}); err != nil {
			return err
		}
	}
	return nil
}

// Key presses or releases a key. A named key or a modifier is its virtual
// key; a character is typed as itself, as one or two UTF-16 units, unless
// a modifier is held, when it is the US layout's key for it so the
// combination (ctrl+c, shift+a) reaches the application.
func (b *Backend) Key(ctx context.Context, keysym uint32, down bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	combo := len(b.held) > 0
	if computeruse.IsModifier(keysym) {
		if down {
			b.held[keysym] = true
		} else {
			delete(b.held, keysym)
		}
	}
	b.mu.Unlock()
	var up uint32
	if !down {
		up = keyUp
	}
	if k, ok := computeruse.WindowsKeyOf(keysym, combo); ok {
		flags := up
		if k.Extended {
			flags |= keyExtended
		}
		return sendKeys(keyEvent{typ: inputKeyboard, ki: keybdInput{vk: k.VK, flags: flags}})
	}
	r, ok := computeruse.KeysymRune(keysym)
	if !ok {
		return fmt.Errorf("the key %#x has no key on Windows", keysym)
	}
	var evs []keyEvent
	for _, u := range utf16.Encode([]rune{r}) {
		evs = append(evs, keyEvent{typ: inputKeyboard, ki: keybdInput{scan: u, flags: keyUnicode | up}})
	}
	return sendKeys(evs...)
}

func sendMouse(evs ...mouseEvent) error {
	n, _, err := procSendInput.Call(uintptr(len(evs)), uintptr(unsafe.Pointer(&evs[0])), unsafe.Sizeof(evs[0]))
	if int(n) != len(evs) {
		return fmt.Errorf("Windows took %d of %d inputs: %v", n, len(evs), err)
	}
	return nil
}

func sendKeys(evs ...keyEvent) error {
	n, _, err := procSendInput.Call(uintptr(len(evs)), uintptr(unsafe.Pointer(&evs[0])), unsafe.Sizeof(evs[0]))
	if int(n) != len(evs) {
		return fmt.Errorf("Windows took %d of %d inputs: %v", n, len(evs), err)
	}
	return nil
}

// Close releases every modifier the binding still holds, so a stopped run
// leaves no key down.
func (b *Backend) Close() error {
	b.mu.Lock()
	held := b.held
	b.held = map[uint32]bool{}
	b.mu.Unlock()
	for k := range held {
		if wk, ok := computeruse.WindowsKeyOf(k, false); ok {
			flags := uint32(keyUp)
			if wk.Extended {
				flags |= keyExtended
			}
			_ = sendKeys(keyEvent{typ: inputKeyboard, ki: keybdInput{vk: wk.VK, flags: flags}})
		}
	}
	return nil
}

// structSizes guard the layout SendInput reads: 40 bytes an input on
// 64-bit Windows.
var _ = [1]struct{}{}[unsafe.Sizeof(mouseEvent{})-unsafe.Sizeof(keyEvent{})]
