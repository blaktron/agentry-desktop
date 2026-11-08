//go:build darwin

package stopkey

/*
#cgo CFLAGS: -fobjc-arc -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdint.h>
void *agentry_stop_key_bind(uintptr_t handle);
void agentry_stop_key_unbind(void *monitors);
*/
import "C"

import (
	"context"
	"runtime/cgo"
	"sync"
	"unsafe"
)

// macBinding is the stop key as AppKit's event monitors: one for key
// presses in other applications, which needs Accessibility, and one for
// those in Agentry Desktop's own windows.
type macBinding struct {
	monitors unsafe.Pointer
	handle   cgo.Handle
	once     sync.Once
}

func (b *macBinding) Close() error {
	b.once.Do(func() {
		C.agentry_stop_key_unbind(b.monitors)
		b.handle.Delete()
	})
	return nil
}

//export agentryStopKeyPressed
func agentryStopKeyPressed(h C.uintptr_t) {
	if stop, ok := cgo.Handle(h).Value().(func()); ok {
		go stop()
	}
}

func bind(ctx context.Context, stop func()) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h := cgo.NewHandle(stop)
	m := C.agentry_stop_key_bind(C.uintptr_t(h))
	return &macBinding{monitors: m, handle: h}, nil
}
