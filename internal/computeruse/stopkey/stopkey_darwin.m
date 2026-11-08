//go:build darwin

// The stop key on macOS: Control-Shift-Escape, heard by AppKit's global
// event monitor while another application is in front (macOS hands it key
// presses only when Accessibility is allowed) and by a local monitor while
// Agentry Desktop is. The monitors are added and removed on the main thread,
// as AppKit wants.
#import <AppKit/AppKit.h>
#include <stdint.h>

extern void agentryStopKeyPressed(uintptr_t handle);

static BOOL is_stop_key(NSEvent *e) {
	NSEventModifierFlags want = NSEventModifierFlagControl | NSEventModifierFlagShift;
	NSEventModifierFlags held = e.modifierFlags & (NSEventModifierFlagControl | NSEventModifierFlagShift | NSEventModifierFlagOption | NSEventModifierFlagCommand);
	return e.keyCode == 53 && held == want && !e.isARepeat;
}

static void on_main(dispatch_block_t b) {
	if ([NSThread isMainThread]) {
		b();
	} else {
		dispatch_sync(dispatch_get_main_queue(), b);
	}
}

void *agentry_stop_key_bind(uintptr_t handle) {
	__block id global = nil, local = nil;
	on_main(^{
		global = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^(NSEvent *e) {
			if (is_stop_key(e)) {
				agentryStopKeyPressed(handle);
			}
		}];
		local = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^NSEvent *(NSEvent *e) {
			if (is_stop_key(e)) {
				agentryStopKeyPressed(handle);
				return nil;
			}
			return e;
		}];
	});
	NSMutableArray *monitors = [NSMutableArray array];
	if (global != nil) {
		[monitors addObject:global];
	}
	if (local != nil) {
		[monitors addObject:local];
	}
	return (__bridge_retained void *)monitors;
}

void agentry_stop_key_unbind(void *p) {
	NSArray *monitors = (__bridge_transfer NSArray *)p;
	on_main(^{
		for (id m in monitors) {
			[NSEvent removeMonitor:m];
		}
	});
}
