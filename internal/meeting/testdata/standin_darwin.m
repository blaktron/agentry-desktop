// A stand-in meeting app for the live Core Audio tests (live_darwin_test.go
// compiles it with clang): it reads the default microphone, waits 4 s,
// plays the WAV file it is given, waits 8 s, plays it again, and goes on
// reading the microphone until its standard input closes, which happens
// when the test that started it ends however it ends. It plays at a
// twentieth of full volume, through the default output, which it holds
// from its start as a meeting app does.
#import <AVFoundation/AVFoundation.h>
#import <AudioToolbox/AudioToolbox.h>
#include <stdio.h>
#include <unistd.h>

static void mic(void *user, AudioQueueRef q, AudioQueueBufferRef b, const AudioTimeStamp *t, UInt32 n, const AudioStreamPacketDescription *d) {
	(void)user;
	(void)t;
	(void)n;
	(void)d;
	AudioQueueEnqueueBuffer(q, b, 0, NULL);
}

int main(int argc, char **argv) {
	@autoreleasepool {
		if (argc != 2) {
			return 2;
		}
		AudioStreamBasicDescription f = {16000, kAudioFormatLinearPCM, kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked, 2, 1, 2, 1, 16, 0};
		AudioQueueRef q = NULL;
		if (AudioQueueNewInput(&f, mic, NULL, NULL, kCFRunLoopCommonModes, 0, &q) != noErr) {
			return 3;
		}
		for (int i = 0; i < 3; i++) {
			AudioQueueBufferRef b = NULL;
			AudioQueueAllocateBuffer(q, 3200, &b);
			AudioQueueEnqueueBuffer(q, b, 0, NULL);
		}
		if (AudioQueueStart(q, NULL) != noErr) {
			return 4;
		}
		NSError *err = nil;
		AVAudioFile *file = [[AVAudioFile alloc] initForReading:[NSURL fileURLWithPath:@(argv[1])] error:&err];
		if (file == nil) {
			return 5;
		}
		AVAudioEngine *engine = [[AVAudioEngine alloc] init];
		AVAudioPlayerNode *player = [[AVAudioPlayerNode alloc] init];
		[engine attachNode:player];
		[engine connect:player to:engine.mainMixerNode format:file.processingFormat];
		engine.mainMixerNode.outputVolume = 0.05f;
		if (![engine startAndReturnError:&err]) {
			return 6;
		}
		[player play];
		dispatch_async(dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
			char buf[256];
			while (read(0, buf, sizeof(buf)) > 0) {
			}
			exit(0);
		});
		double secs = (double)file.length / file.processingFormat.sampleRate;
		printf("ready %.1f\n", secs);
		fflush(stdout);
		sleep(4);
		[player scheduleFile:file atTime:nil completionHandler:nil];
		usleep((useconds_t)((secs + 8) * 1e6));
		[player scheduleFile:file atTime:nil completionHandler:nil];
		for (;;) {
			sleep(60);
		}
	}
}
