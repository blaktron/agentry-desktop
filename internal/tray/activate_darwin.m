//go:build darwin

#import <Cocoa/Cocoa.h>

void agentryTrayActivate(void) {
  dispatch_async(dispatch_get_main_queue(), ^{ [NSApp activateIgnoringOtherApps:YES]; });
}
