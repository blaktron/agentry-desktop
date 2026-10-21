//go:build darwin

#import <Cocoa/Cocoa.h>

extern void goAgentryQuitting(void);

// AgentryQuitter marks a quit as the person's before it goes ahead: each
// quit menu item it took over keeps its original target and action, and the
// quit Apple event goes on to terminate the app as AppKit's own handler does.
@interface AgentryQuitter : NSObject
@property (strong) NSMapTable<NSMenuItem *, id> *targets;
@property (strong) NSMapTable<NSMenuItem *, NSString *> *actions;
- (void)quit:(NSMenuItem *)sender;
- (void)handleQuitEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply;
@end

@implementation AgentryQuitter
- (instancetype)init {
  if ((self = [super init])) {
    _targets = [NSMapTable weakToStrongObjectsMapTable];
    _actions = [NSMapTable weakToStrongObjectsMapTable];
  }
  return self;
}
- (void)quit:(NSMenuItem *)sender {
  goAgentryQuitting();
  id target = [self.targets objectForKey:sender];
  if (target == [NSNull null]) target = nil;
  NSString *action = [self.actions objectForKey:sender];
  if (action != nil) {
    [NSApp sendAction:NSSelectorFromString(action) to:target from:sender];
  } else {
    [NSApp terminate:sender];
  }
}
- (void)handleQuitEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)reply {
  goAgentryQuitting();
  [NSApp terminate:nil];
}
@end

static AgentryQuitter *quitter;

// isQuit reports whether item quits the app: AppKit's terminate:, or the
// window library's own Quit action.
static BOOL isQuit(NSMenuItem *item) {
  SEL a = [item action];
  return a == @selector(terminate:) || a == NSSelectorFromString(@"Quit");
}

// retarget points every quit item at the quitter, remembering where it went.
static void retarget(NSMenu *menu) {
  for (NSMenuItem *item in [menu itemArray]) {
    if (isQuit(item) && [item target] != quitter) {
      [quitter.targets setObject:([item target] ?: [NSNull null]) forKey:item];
      [quitter.actions setObject:NSStringFromSelector([item action]) forKey:item];
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
