//go:build darwin

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

extern void goAgentryNotifyClicked(char *key);

// The answers to Go (notify_darwin.go): ready, no bundle, a macOS without
// UserNotifications, a value that is not text.
enum { agentryNotifyOK = 0, agentryNotifyNoBundle = 1, agentryNotifyTooOld = 2, agentryNotifyNotText = 3 };

// The notification center's delegate: a notification shows while the app is
// in front too (the Go side decides when not to raise one), and a click on
// one hands its key to Go.
API_AVAILABLE(macos(10.14))
@interface AgentryNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation AgentryNotifyDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
  if (@available(macOS 11.0, *)) {
    done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
  } else {
    done(UNNotificationPresentationOptionAlert | UNNotificationPresentationOptionSound);
  }
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))done {
  if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
    id key = response.notification.request.content.userInfo[@"key"];
    if ([key isKindOfClass:[NSString class]]) {
      goAgentryNotifyClicked((char *)[(NSString *)key UTF8String]);
    }
  }
  done();
}
@end

static id agentryNotifyDelegate;

// A bare binary has no bundle identifier, and the notification center
// raises an exception for a process without one.
static int agentryNotifyReady(void) {
  if (@available(macOS 10.14, *)) {
    if ([[NSBundle mainBundle] bundleIdentifier] == nil) {
      return agentryNotifyNoBundle;
    }
    static dispatch_once_t once;
    dispatch_once(&once, ^{
      agentryNotifyDelegate = [AgentryNotifyDelegate new];
      [UNUserNotificationCenter currentNotificationCenter].delegate = agentryNotifyDelegate;
    });
    return agentryNotifyOK;
  }
  return agentryNotifyTooOld;
}

int agentryNotifyInit(void) {
  return agentryNotifyReady();
}

int agentryNotifyShow(const char *title, const char *body, const char *key) {
  int rc = agentryNotifyReady();
  if (rc != agentryNotifyOK) {
    return rc;
  }
  if (@available(macOS 10.14, *)) {
    NSString *t = [NSString stringWithUTF8String:title];
    NSString *b = [NSString stringWithUTF8String:body];
    NSString *k = [NSString stringWithUTF8String:key];
    if (t == nil || b == nil || k == nil) {
      return agentryNotifyNotText;
    }
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    // The first time, the system asks the person whether Agentry may notify
    // them; after that it answers at once with what they chose.
    [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                          completionHandler:^(BOOL granted, NSError *error) {
      if (!granted) {
        NSLog(@"Agentry: a task's notification was not shown: notifications for Agentry are off in System Settings");
        return;
      }
      UNMutableNotificationContent *content = [UNMutableNotificationContent new];
      content.title = t;
      content.body = b;
      content.userInfo = @{@"key" : k};
      content.sound = [UNNotificationSound defaultSound];
      UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
                                                                            content:content
                                                                            trigger:nil];
      [center addNotificationRequest:request withCompletionHandler:nil];
    }];
    return agentryNotifyOK;
  }
  return agentryNotifyTooOld;
}
