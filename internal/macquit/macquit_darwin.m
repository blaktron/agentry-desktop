//go:build darwin

#import <Cocoa/Cocoa.h>

extern void goAgentryQuitting(void);

// AgentryQuitter marks a quit as the person's before it goes ahead.
@interface AgentryQuitter : NSObject
- (void)quit:(id)sender;
- (void)handleQuitEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply;
@end

@implementation AgentryQuitter
- (void)quit:(id)sender {
  goAgentryQuitting();
  [NSApp terminate:sender];
}
- (void)handleQuitEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
  goAgentryQuitting();
  [NSApp terminate:nil];
}
@end

static AgentryQuitter *quitter;

// retarget points every menu item whose action is terminate: at the quitter.
static void retarget(NSMenu *menu) {
  for (NSMenuItem *item in [menu itemArray]) {
    if ([item action] == @selector(terminate:)) {
      [item setTarget:quitter];
      [item setAction:@selector(quit:)];
    }
    if ([item hasSubmenu]) retarget([item submenu]);
  }
}

void agentryInstallQuitHandlers(void) {
  dispatch_async(dispatch_get_main_queue(), ^{
    if (quitter == nil) quitter = [AgentryQuitter new];
    retarget([NSApp mainMenu]);
    [[NSAppleEventManager sharedAppleEventManager] setEventHandler:quitter
                                                       andSelector:@selector(handleQuitEvent:withReplyEvent:)
                                                     forEventClass:kCoreEventClass
                                                        andEventID:kAEQuitApplication];
  });
}
