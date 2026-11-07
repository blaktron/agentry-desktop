// The macOS side of computer use. CoreGraphics and AppKit are the app's own
// frameworks already. ScreenCaptureKit (macOS 12.3 and later) and the
// accessibility calls of ApplicationServices are loaded when first used,
// never linked, so the app starts on every macOS it supports whether or not
// they are there: their classes are looked up by name and their functions
// by symbol.
#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>
#include "quartz_darwin.h"

// ---- ScreenCaptureKit, by name -------------------------------------------

static int sck_loaded = -1;

static int load_sck(void) {
	if (sck_loaded < 0) {
		void *h = dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit", RTLD_LAZY | RTLD_LOCAL);
		sck_loaded = (h != NULL && NSClassFromString(@"SCScreenshotManager") != nil) ? 1 : 0;
	}
	return sck_loaded;
}

int qz_capture_kit(void) {
	if (@available(macOS 14.0, *)) {
		return load_sck();
	}
	return 0;
}

// ---- The accessibility calls, by symbol -----------------------------------

typedef const void *ax_ref;
static Boolean (*ax_trusted)(void);
static Boolean (*ax_trusted_options)(CFDictionaryRef);
static ax_ref (*ax_create_app)(pid_t);
static int32_t (*ax_copy_attr)(ax_ref, CFStringRef, CFTypeRef *);
static int32_t (*ax_set_attr)(ax_ref, CFStringRef, CFTypeRef);
static int32_t (*ax_perform)(ax_ref, CFStringRef);
static Boolean (*ax_value_get)(CFTypeRef, uint32_t, void *);

static int ax_loaded = -1;

static int load_ax(void) {
	if (ax_loaded < 0) {
		void *h = dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", RTLD_LAZY | RTLD_LOCAL);
		ax_trusted = h ? dlsym(h, "AXIsProcessTrusted") : NULL;
		ax_trusted_options = h ? dlsym(h, "AXIsProcessTrustedWithOptions") : NULL;
		ax_create_app = h ? dlsym(h, "AXUIElementCreateApplication") : NULL;
		ax_copy_attr = h ? dlsym(h, "AXUIElementCopyAttributeValue") : NULL;
		ax_set_attr = h ? dlsym(h, "AXUIElementSetAttributeValue") : NULL;
		ax_perform = h ? dlsym(h, "AXUIElementPerformAction") : NULL;
		ax_value_get = h ? dlsym(h, "AXValueGetValue") : NULL;
		ax_loaded = (ax_trusted && ax_trusted_options && ax_create_app && ax_copy_attr && ax_set_attr && ax_perform && ax_value_get) ? 1 : 0;
	}
	return ax_loaded;
}

// The AXValue types of a position and a size (kAXValueCGPointType,
// kAXValueCGSizeType).
enum { axPoint = 1, axSize = 2 };

int qz_ax_granted(void) {
	return load_ax() && ax_trusted();
}

void qz_request_ax(void) {
	if (!load_ax()) {
		return;
	}
	NSDictionary *opts = @{@"AXTrustedCheckOptionPrompt": @YES};
	ax_trusted_options((__bridge CFDictionaryRef)opts);
}

int qz_screen_granted(void) {
	return CGPreflightScreenCaptureAccess() ? 1 : 0;
}

void qz_request_screen(void) {
	CGRequestScreenCaptureAccess();
}

// ---- Windows --------------------------------------------------------------

static void copy_str(char *dst, int n, NSString *s) {
	dst[0] = 0;
	const char *u = s != nil ? [s UTF8String] : NULL;
	if (u != NULL) {
		strlcpy(dst, u, n);
	}
}

int qz_windows(qz_win *out, int max) {
	@autoreleasepool {
		CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
		if (list == NULL) {
			return -1;
		}
		NSArray *ws = CFBridgingRelease(list);
		int n = 0;
		NSMutableDictionary *bundles = [NSMutableDictionary dictionary];
		for (NSDictionary *d in ws) {
			if (n < max) {
				qz_win *w = &out[n];
				memset(w, 0, sizeof(*w));
				w->id = [d[(id)kCGWindowNumber] unsignedIntValue];
				w->pid = [d[(id)kCGWindowOwnerPID] intValue];
				w->layer = [d[(id)kCGWindowLayer] intValue];
				NSNumber *alpha = d[(id)kCGWindowAlpha];
				w->alpha = alpha ? [alpha doubleValue] : 1.0;
				CGRect r;
				if (CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)d[(id)kCGWindowBounds], &r)) {
					w->x = r.origin.x;
					w->y = r.origin.y;
					w->w = r.size.width;
					w->h = r.size.height;
				}
				copy_str(w->owner, sizeof(w->owner), d[(id)kCGWindowOwnerName]);
				copy_str(w->title, sizeof(w->title), d[(id)kCGWindowName]);
				NSNumber *pid = @(w->pid);
				NSString *bundle = bundles[pid];
				if (bundle == nil) {
					NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:w->pid];
					bundle = app.bundleIdentifier ?: @"";
					bundles[pid] = bundle;
				}
				copy_str(w->bundle, sizeof(w->bundle), bundle);
			}
			n++;
		}
		return n;
	}
}

int32_t qz_frontmost_pid(void) {
	@autoreleasepool {
		NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
		return app ? app.processIdentifier : -1;
	}
}

// main_height is the main display's height in points: AppKit's screens
// count from the bottom left, Quartz's windows from the top left.
static CGFloat main_height(void) {
	return CGDisplayBounds(CGMainDisplayID()).size.height;
}

double qz_scale(double x, double y, double w, double h) {
	@autoreleasepool {
		NSPoint centre = NSMakePoint(x + w / 2, main_height() - (y + h / 2));
		for (NSScreen *s in [NSScreen screens]) {
			if (NSPointInRect(centre, s.frame)) {
				return s.backingScaleFactor;
			}
		}
		NSScreen *m = [NSScreen mainScreen];
		return m ? m.backingScaleFactor : 1.0;
	}
}

void qz_main_display(double *x, double *y, double *w, double *h) {
	CGRect r = CGDisplayBounds(CGMainDisplayID());
	*x = r.origin.x;
	*y = r.origin.y;
	*w = r.size.width;
	*h = r.size.height;
}

// ---- Capture --------------------------------------------------------------

static void set_err(char *err, int errn, NSString *s) {
	copy_str(err, errn, s);
}

// rgba draws a CGImage into an RGBA buffer the caller frees.
static int rgba(CGImageRef img, uint8_t **out, int *ow, int *oh) {
	size_t w = CGImageGetWidth(img), h = CGImageGetHeight(img);
	if (w == 0 || h == 0) {
		return -1;
	}
	uint8_t *buf = calloc(w * h, 4);
	if (buf == NULL) {
		return -1;
	}
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(buf, w, h, 8, w * 4, cs, kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
	CGColorSpaceRelease(cs);
	if (ctx == NULL) {
		free(buf);
		return -1;
	}
	CGContextDrawImage(ctx, CGRectMake(0, 0, w, h), img);
	CGContextRelease(ctx);
	*out = buf;
	*ow = (int)w;
	*oh = (int)h;
	return 0;
}

// shareable is ScreenCaptureKit's current list of what can be captured.
static SCShareableContent *shareable(NSString **why) API_AVAILABLE(macos(12.3)) {
	__block SCShareableContent *got = nil;
	__block NSError *failed = nil;
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	Class cls = NSClassFromString(@"SCShareableContent");
	[cls getShareableContentExcludingDesktopWindows:YES onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *content, NSError *error) {
		got = content;
		failed = error;
		dispatch_semaphore_signal(done);
	}];
	if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC)) != 0) {
		*why = @"ScreenCaptureKit did not answer within ten seconds";
		return nil;
	}
	if (got == nil) {
		*why = failed ? failed.localizedDescription : @"ScreenCaptureKit listed nothing to capture";
	}
	return got;
}

// screenshot captures a filter at pw by ph pixels through SCScreenshotManager.
static CGImageRef screenshot(SCContentFilter *filter, int pw, int ph, NSString **why) API_AVAILABLE(macos(14.0)) {
	Class cfgCls = NSClassFromString(@"SCStreamConfiguration");
	SCStreamConfiguration *cfg = [[cfgCls alloc] init];
	cfg.width = pw;
	cfg.height = ph;
	cfg.showsCursor = NO;
	cfg.ignoreShadowsSingleWindow = YES;
	// The answer may come after the wait gave up: then the image it
	// retained is released there, never kept.
	__block CGImageRef got = NULL;
	__block NSError *failed = nil;
	__block BOOL abandoned = NO;
	NSObject *lock = [NSObject new];
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	Class mgr = NSClassFromString(@"SCScreenshotManager");
	[mgr captureImageWithFilter:filter configuration:cfg completionHandler:^(CGImageRef image, NSError *error) {
		@synchronized (lock) {
			if (!abandoned && image != NULL) {
				got = CGImageRetain(image);
			}
			failed = error;
		}
		dispatch_semaphore_signal(done);
	}];
	if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC)) != 0) {
		@synchronized (lock) {
			abandoned = YES;
			if (got != NULL) {
				CGImageRelease(got);
				got = NULL;
			}
		}
		*why = @"ScreenCaptureKit did not capture within ten seconds";
		return NULL;
	}
	if (got == NULL) {
		*why = failed ? failed.localizedDescription : @"ScreenCaptureKit captured nothing";
	}
	return got;
}

typedef CGImageRef (*window_image_fn)(CGRect, CGWindowListOption, CGWindowID, CGWindowImageOption);

// window_image is CGWindowListCreateImage, the capture before
// ScreenCaptureKit's screenshots, looked up by symbol.
static window_image_fn window_image(void) {
	static window_image_fn fn = NULL;
	static int looked = 0;
	if (!looked) {
		fn = (window_image_fn)dlsym(RTLD_DEFAULT, "CGWindowListCreateImage");
		looked = 1;
	}
	return fn;
}

int qz_capture_window(uint32_t wid, int pw, int ph, uint8_t **out, int *ow, int *oh, char *err, int errn) {
	@autoreleasepool {
		if (!qz_screen_granted()) {
			set_err(err, errn, @"Screen Recording is not allowed for Agentry Desktop");
			return -1;
		}
		CGImageRef img = NULL;
		NSString *why = nil;
		if (qz_capture_kit()) {
			if (@available(macOS 14.0, *)) {
				SCShareableContent *content = shareable(&why);
				SCWindow *target = nil;
				for (SCWindow *w in content.windows) {
					if (w.windowID == wid) {
						target = w;
						break;
					}
				}
				if (content != nil && target == nil) {
					why = @"the window is no longer on screen";
				}
				if (target != nil) {
					Class filterCls = NSClassFromString(@"SCContentFilter");
					SCContentFilter *filter = [[filterCls alloc] initWithDesktopIndependentWindow:target];
					img = screenshot(filter, pw, ph, &why);
				}
			}
		} else if (window_image() != NULL) {
			img = window_image()(CGRectNull, kCGWindowListOptionIncludingWindow, wid, kCGWindowImageBoundsIgnoreFraming | kCGWindowImageBestResolution);
			if (img == NULL) {
				why = @"macOS returned no image of the window";
			}
		} else {
			why = @"this macOS offers no way to capture a window";
		}
		if (img == NULL) {
			set_err(err, errn, why);
			return -1;
		}
		int rc = rgba(img, out, ow, oh);
		CGImageRelease(img);
		if (rc != 0) {
			set_err(err, errn, @"the window's image could not be read");
		}
		return rc;
	}
}

int qz_capture_display(int pw, int ph, uint8_t **out, int *ow, int *oh, char *err, int errn) {
	@autoreleasepool {
		if (!qz_screen_granted()) {
			set_err(err, errn, @"Screen Recording is not allowed for Agentry Desktop");
			return -1;
		}
		CGImageRef img = NULL;
		NSString *why = nil;
		if (qz_capture_kit()) {
			if (@available(macOS 14.0, *)) {
				SCShareableContent *content = shareable(&why);
				SCDisplay *main = nil;
				for (SCDisplay *d in content.displays) {
					if (d.displayID == CGMainDisplayID()) {
						main = d;
						break;
					}
				}
				if (main != nil) {
					Class filterCls = NSClassFromString(@"SCContentFilter");
					SCContentFilter *filter = [[filterCls alloc] initWithDisplay:main excludingWindows:@[]];
					img = screenshot(filter, pw, ph, &why);
				} else if (content != nil) {
					why = @"ScreenCaptureKit lists no main display";
				}
			}
		} else {
			img = CGDisplayCreateImage(CGMainDisplayID());
			if (img == NULL) {
				why = @"macOS returned no image of the display";
			}
		}
		if (img == NULL) {
			set_err(err, errn, why);
			return -1;
		}
		int rc = rgba(img, out, ow, oh);
		CGImageRelease(img);
		if (rc != 0) {
			set_err(err, errn, @"the display's image could not be read");
		}
		return rc;
	}
}

void qz_free(void *p) { free(p); }

// ---- Focus ----------------------------------------------------------------

int qz_activate(int32_t pid, double x, double y, double w, double h) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		if (app == nil) {
			return -1;
		}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		BOOL activated = [app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
#pragma clang diagnostic pop
		int raised = 0;
		if (!load_ax() || !ax_trusted()) {
			return activated ? 0 : -1;
		}
		ax_ref ax = ax_create_app(pid);
		if (ax == NULL) {
			return activated ? 0 : -1;
		}
		CFTypeRef windows = NULL;
		if (ax_copy_attr(ax, CFSTR("AXWindows"), &windows) == 0 && windows != NULL) {
			NSArray *ws = (__bridge NSArray *)windows;
			for (id win in ws) {
				ax_ref aw = (__bridge ax_ref)win;
				CFTypeRef pos = NULL, size = NULL;
				CGPoint p = CGPointZero;
				CGSize s = CGSizeZero;
				if (ax_copy_attr(aw, CFSTR("AXPosition"), &pos) == 0 && pos != NULL) {
					ax_value_get(pos, axPoint, &p);
					CFRelease(pos);
				}
				if (ax_copy_attr(aw, CFSTR("AXSize"), &size) == 0 && size != NULL) {
					ax_value_get(size, axSize, &s);
					CFRelease(size);
				}
				if (fabs(p.x - x) < 2 && fabs(p.y - y) < 2 && fabs(s.width - w) < 2 && fabs(s.height - h) < 2) {
					ax_set_attr(aw, CFSTR("AXMain"), kCFBooleanTrue);
					raised = ax_perform(aw, CFSTR("AXRaise")) == 0;
					break;
				}
			}
			CFRelease(windows);
		}
		CFRelease(ax);
		return (activated || raised) ? 0 : -1;
	}
}

// ---- Input ----------------------------------------------------------------

int qz_cursor(double *x, double *y) {
	CGEventRef e = CGEventCreate(NULL);
	if (e == NULL) {
		return -1;
	}
	CGPoint p = CGEventGetLocation(e);
	CFRelease(e);
	*x = p.x;
	*y = p.y;
	return 0;
}

static int post(CGEventRef e, uint64_t flags) {
	if (e == NULL) {
		return -1;
	}
	CGEventSetFlags(e, (CGEventFlags)flags);
	CGEventPost(kCGHIDEventTap, e);
	CFRelease(e);
	return 0;
}

// buttons_down tracks the buttons held, so a move while one is down is a
// drag, as the window system expects.
static int buttons_down[3];

int qz_move(double x, double y, uint64_t flags) {
	CGEventType t = kCGEventMouseMoved;
	CGMouseButton b = kCGMouseButtonLeft;
	if (buttons_down[0]) {
		t = kCGEventLeftMouseDragged;
	} else if (buttons_down[1]) {
		t = kCGEventRightMouseDragged;
		b = kCGMouseButtonRight;
	} else if (buttons_down[2]) {
		t = kCGEventOtherMouseDragged;
		b = kCGMouseButtonCenter;
	}
	return post(CGEventCreateMouseEvent(NULL, t, CGPointMake(x, y), b), flags);
}

int qz_button(int button, int down, double x, double y, uint64_t flags, int clicks) {
	CGEventType t;
	CGMouseButton b;
	switch (button) {
	case 1:
		t = down ? kCGEventLeftMouseDown : kCGEventLeftMouseUp;
		b = kCGMouseButtonLeft;
		break;
	case 2:
		t = down ? kCGEventOtherMouseDown : kCGEventOtherMouseUp;
		b = kCGMouseButtonCenter;
		break;
	case 3:
		t = down ? kCGEventRightMouseDown : kCGEventRightMouseUp;
		b = kCGMouseButtonRight;
		break;
	default:
		return -1;
	}
	buttons_down[button == 1 ? 0 : (button == 3 ? 1 : 2)] = down;
	CGEventRef e = CGEventCreateMouseEvent(NULL, t, CGPointMake(x, y), b);
	if (e == NULL) {
		return -1;
	}
	CGEventSetIntegerValueField(e, kCGMouseEventClickState, clicks < 1 ? 1 : clicks);
	return post(e, flags);
}

int qz_scroll(int32_t dy, int32_t dx, double x, double y, uint64_t flags) {
	CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, dy, dx);
	if (e == NULL) {
		return -1;
	}
	CGEventSetLocation(e, CGPointMake(x, y));
	return post(e, flags);
}

int qz_key(uint16_t code, int down, uint64_t flags) {
	return post(CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, down ? true : false), flags);
}

int qz_unicode(uint32_t cp, int down, uint64_t flags) {
	UniChar units[2];
	int n = 1;
	if (cp > 0xffff) {
		cp -= 0x10000;
		units[0] = (UniChar)(0xd800 + (cp >> 10));
		units[1] = (UniChar)(0xdc00 + (cp & 0x3ff));
		n = 2;
	} else {
		units[0] = (UniChar)cp;
	}
	CGEventRef e = CGEventCreateKeyboardEvent(NULL, 0, down ? true : false);
	if (e == NULL) {
		return -1;
	}
	CGEventKeyboardSetUnicodeString(e, n, units);
	return post(e, flags);
}
