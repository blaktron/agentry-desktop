//go:build windows

package stopkey

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// call calls a procedure for its first result alone.
func call(p *windows.LazyProc, args ...uintptr) uintptr {
	r, _, _ := p.Call(args...)
	return r
}

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

const (
	hotKeyID    = 0x4147 // any id the app's thread does not use otherwise
	modControl  = 0x0002
	modShift    = 0x0004
	modNoRepeat = 0x4000
	vkF12       = 0x7b
	wmHotKey    = 0x0312
	wmQuit      = 0x0012
)

// msg is MSG.
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// winBinding is the stop key registered on a thread of its own, which
// waits for its messages until the binding is closed.
type winBinding struct {
	tid  uint32
	done chan struct{}
	once sync.Once
}

func (b *winBinding) Close() error {
	b.once.Do(func() {
		call(procPostThreadMessageW, uintptr(b.tid), wmQuit, 0, 0)
		<-b.done
	})
	return nil
}

func bind(ctx context.Context, stop func()) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := &winBinding{done: make(chan struct{})}
	started := make(chan error, 1)
	go func() {
		// A hot key belongs to the thread that registered it, and only
		// that thread's message loop hears it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(b.done)
		b.tid = windows.GetCurrentThreadId()
		if ok, _, err := procRegisterHotKey.Call(0, hotKeyID, modControl|modShift|modNoRepeat, vkF12); ok == 0 {
			started <- fmt.Errorf("the stop key %s could not be registered with Windows: %v", Name, err)
			return
		}
		defer call(procUnregisterHotKey, 0, hotKeyID)
		started <- nil
		var m msg
		for {
			r := call(procGetMessageW, uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			if m.message == wmHotKey && m.wParam == hotKeyID {
				go stop()
			}
		}
	}()
	if err := <-started; err != nil {
		<-b.done
		return nil, err
	}
	return b, nil
}
