package macquit

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
void agentryInstallQuitHandlers(void);
*/
import "C"

var onQuit func()

func install(quit func()) {
	onQuit = quit
	C.agentryInstallQuitHandlers()
}

//export goAgentryQuitting
func goAgentryQuitting() {
	if onQuit != nil {
		onQuit()
	}
}
