//go:build darwin

#import <Cocoa/Cocoa.h>

extern void goAgentryTrayStart(void);
// The tray library's application delegate, a global of its Objective-C file.
extern id owner;

void agentryTrayStartOnMain(void) {
  dispatch_async(dispatch_get_main_queue(), ^{ goAgentryTrayStart(); });
}

void agentryTrayActivate(void) {
  dispatch_async(dispatch_get_main_queue(), ^{ [NSApp activateIgnoringOtherApps:YES]; });
}

void agentryTrayFixTarget(void) {
  dispatch_async(dispatch_get_main_queue(), ^{
    NSStatusItem *item = nil;
    @try { item = [owner valueForKey:@"statusItem"]; } @catch (NSException *e) { item = nil; }
    if (item != nil && item.button != nil && item.button.target == nil) {
      item.button.target = owner;
    }
  });
}
