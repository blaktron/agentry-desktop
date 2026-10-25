package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
int agentryNotifyInit(void);
int agentryNotifyShow(const char *title, const char *body, const char *key);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// The answers of the Objective-C side, its enum in notify_darwin.m.
const (
	darwinOK       = 0
	darwinNoBundle = 1
	darwinTooOld   = 2
	darwinNotText  = 3
)

var (
	errNoBundle = errors.New("notifications need the app's bundle: this binary runs outside Agentry Desktop.app")
	errNotText  = errors.New("the notification's text is not UTF-8")
)

// Init makes the app the delegate of its notification center, so a click on
// a notification reaches it. It asks the person nothing: the first
// notification asks.
func Init() error {
	return darwinErr(C.agentryNotifyInit())
}

func show(n Notice) error {
	t, b, k := C.CString(n.Title), C.CString(n.Body), C.CString(n.Key)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(b))
	defer C.free(unsafe.Pointer(k))
	return darwinErr(C.agentryNotifyShow(t, b, k))
}

func darwinErr(rc C.int) error {
	switch rc {
	case darwinOK:
		return nil
	case darwinNoBundle:
		return errNoBundle
	case darwinNotText:
		return errNotText
	}
	return ErrUnsupported
}

//export goAgentryNotifyClicked
func goAgentryNotifyClicked(key *C.char) {
	clicked(C.GoString(key))
}

// withdraw leaves the notification: macOS takes it away itself once seen.
func withdraw(string) error { return nil }
