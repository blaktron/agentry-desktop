#import <AVFoundation/AVFoundation.h>
#import <AudioToolbox/AudioToolbox.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <CoreAudio/CoreAudio.h>
#import <Foundation/Foundation.h>
#include <libproc.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "coreaudio_darwin.h"

int ca_supported(void) {
	if (@available(macOS 14.4, *)) {
		return 1;
	}
	return 0;
}

static AudioObjectPropertyAddress global(AudioObjectPropertySelector sel) {
	AudioObjectPropertyAddress a = {sel, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
	return a;
}

static UInt32 prop_u32(AudioObjectID obj, AudioObjectPropertySelector sel, UInt32 def) {
	AudioObjectPropertyAddress a = global(sel);
	UInt32 v = def, size = sizeof(v);
	if (AudioObjectGetPropertyData(obj, &a, 0, NULL, &size, &v) != noErr) {
		return def;
	}
	return v;
}

int ca_processes(ca_proc *out, int max) {
	if (!ca_supported()) {
		return -1;
	}
	AudioObjectPropertyAddress a = global(kAudioHardwarePropertyProcessObjectList);
	UInt32 size = 0;
	if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &a, 0, NULL, &size) != noErr) {
		return -1;
	}
	int n = (int)(size / sizeof(AudioObjectID));
	if (n == 0) {
		return 0;
	}
	AudioObjectID *ids = calloc((size_t)n, sizeof(AudioObjectID));
	if (ids == NULL) {
		return -1;
	}
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, ids) != noErr) {
		free(ids);
		return -1;
	}
	n = (int)(size / sizeof(AudioObjectID));
	for (int i = 0; i < n && i < max; i++) {
		ca_proc *p = &out[i];
		memset(p, 0, sizeof(*p));
		p->object = ids[i];
		AudioObjectPropertyAddress pa = global(kAudioProcessPropertyPID);
		pid_t pid = -1;
		UInt32 psize = sizeof(pid);
		if (AudioObjectGetPropertyData(ids[i], &pa, 0, NULL, &psize, &pid) != noErr) {
			pid = -1;
		}
		p->pid = pid;
		p->input = prop_u32(ids[i], kAudioProcessPropertyIsRunningInput, 0) != 0;
		p->output = prop_u32(ids[i], kAudioProcessPropertyIsRunningOutput, 0) != 0;
		if (pid > 0 && proc_pidpath(pid, p->path, sizeof(p->path)) <= 0) {
			p->path[0] = 0;
		}
	}
	free(ids);
	return n;
}

uint32_t ca_default_device(int input) {
	return prop_u32(kAudioObjectSystemObject, input ? kAudioHardwarePropertyDefaultInputDevice : kAudioHardwarePropertyDefaultOutputDevice, kAudioObjectUnknown);
}

uint32_t ca_self_object(void) {
	if (!ca_supported()) {
		return 0;
	}
	AudioObjectPropertyAddress a = global(kAudioHardwarePropertyTranslatePIDToProcessObject);
	pid_t pid = getpid();
	AudioObjectID obj = kAudioObjectUnknown;
	UInt32 size = sizeof(obj);
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, sizeof(pid), &pid, &size, &obj) != noErr) {
		return 0;
	}
	return obj;
}

int ca_mic_authorization(void) {
	return (int)[AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
}

void ca_mic_ask(void) {
	if (ca_mic_authorization() != CA_AUTH_UNDETERMINED) {
		return;
	}
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio
	                         completionHandler:^(BOOL granted) {
		                         (void)granted;
	                         }];
}

// --- captures ----------------------------------------------------------------

// RING_SECONDS is how much audio a ring holds: Go drains it every few tens of
// milliseconds, so this is room for a long stall.
#define RING_SECONDS 20

struct ca_capture {
	double rate;
	// The ring: Core Audio's thread writes at w, Go reads at r; each only
	// moves its own index.
	float *ring;
	uint64_t cap;
	_Atomic uint64_t w, r;
	_Atomic uint64_t frames, dropped, heard;
	// A tap.
	AudioObjectID tap, agg;
	AudioDeviceIOProcID proc;
	int interleaved;
	// A microphone.
	AudioQueueRef queue;
};

static ca_capture *capture_new(double rate) {
	ca_capture *c = calloc(1, sizeof(*c));
	if (c == NULL) {
		return NULL;
	}
	c->rate = rate;
	c->cap = (uint64_t)(rate * RING_SECONDS);
	c->ring = calloc((size_t)c->cap, sizeof(float));
	if (c->ring == NULL) {
		free(c);
		return NULL;
	}
	c->tap = kAudioObjectUnknown;
	c->agg = kAudioObjectUnknown;
	return c;
}

static void capture_free(ca_capture *c) {
	free(c->ring);
	free(c);
}

// push puts one sample in the ring, or counts it dropped when the ring is
// full. Core Audio's thread only.
static inline void push(ca_capture *c, float v) {
	uint64_t w = atomic_load_explicit(&c->w, memory_order_relaxed);
	uint64_t r = atomic_load_explicit(&c->r, memory_order_acquire);
	if (w - r >= c->cap) {
		atomic_fetch_add_explicit(&c->dropped, 1, memory_order_relaxed);
		return;
	}
	c->ring[w % c->cap] = v;
	atomic_store_explicit(&c->w, w + 1, memory_order_release);
}

static void deliver(ca_capture *c, const AudioBufferList *in, UInt32 frames) {
	uint64_t heard = 0;
	if (in->mNumberBuffers == 0 || frames == 0) {
		return;
	}
	if (c->interleaved) {
		const float *s = in->mBuffers[0].mData;
		UInt32 ch = in->mBuffers[0].mNumberChannels ? in->mBuffers[0].mNumberChannels : 1;
		UInt32 avail = in->mBuffers[0].mDataByteSize / (UInt32)(sizeof(float) * ch);
		if (s == NULL) {
			return;
		}
		if (frames > avail) {
			frames = avail;
		}
		for (UInt32 i = 0; i < frames; i++) {
			float sum = 0;
			for (UInt32 k = 0; k < ch; k++) {
				sum += s[i * ch + k];
			}
			float v = sum / (float)ch;
			if (v != 0) {
				heard++;
			}
			push(c, v);
		}
	} else {
		UInt32 nb = in->mNumberBuffers;
		for (UInt32 i = 0; i < frames; i++) {
			float sum = 0;
			UInt32 used = 0;
			for (UInt32 b = 0; b < nb; b++) {
				const float *s = in->mBuffers[b].mData;
				if (s != NULL && i < in->mBuffers[b].mDataByteSize / sizeof(float)) {
					sum += s[i];
					used++;
				}
			}
			float v = used ? sum / (float)used : 0;
			if (v != 0) {
				heard++;
			}
			push(c, v);
		}
	}
	atomic_fetch_add_explicit(&c->frames, frames, memory_order_relaxed);
	atomic_fetch_add_explicit(&c->heard, heard, memory_order_relaxed);
}

static void fail_status(char *err, int errlen, const char *what, OSStatus st) {
	char code[8] = {0};
	UInt32 be = CFSwapInt32HostToBig((UInt32)st);
	memcpy(code, &be, 4);
	int printable = 1;
	for (int i = 0; i < 4; i++) {
		if (code[i] < 32 || code[i] > 126) {
			printable = 0;
		}
	}
	if (printable) {
		snprintf(err, (size_t)errlen, "macOS could not %s (Core Audio error '%s')", what, code);
	} else {
		snprintf(err, (size_t)errlen, "macOS could not %s (Core Audio error %d)", what, (int)st);
	}
}

// TAP_DEVICE_NAME names the aggregate device a tap is read through, which
// this process sees among its input devices while it records.
#define TAP_DEVICE_NAME @"Agentry Meeting notes"

static NSString *device_uid(AudioObjectID dev) {
	AudioObjectPropertyAddress a = global(kAudioDevicePropertyDeviceUID);
	CFStringRef uid = NULL;
	UInt32 size = sizeof(uid);
	if (AudioObjectGetPropertyData(dev, &a, 0, NULL, &size, &uid) != noErr || uid == NULL) {
		return nil;
	}
	return (__bridge_transfer NSString *)uid;
}

ca_capture *ca_tap_start(const uint32_t *objects, int n, int global_tap, char *err, int errlen) {
	if (@available(macOS 14.4, *)) {
		@autoreleasepool {
			NSMutableArray<NSNumber *> *list = [NSMutableArray arrayWithCapacity:(NSUInteger)n];
			for (int i = 0; i < n; i++) {
				[list addObject:@(objects[i])];
			}
			CATapDescription *desc = global_tap ? [[CATapDescription alloc] initMonoGlobalTapButExcludeProcesses:list]
			                                    : [[CATapDescription alloc] initMonoMixdownOfProcesses:list];
			desc.name = TAP_DEVICE_NAME;
			desc.privateTap = YES;
			desc.muteBehavior = CATapUnmuted;
			AudioObjectID tap = kAudioObjectUnknown;
			OSStatus st = AudioHardwareCreateProcessTap(desc, &tap);
			if (st != noErr) {
				fail_status(err, errlen, "start recording the meeting's sound", st);
				return NULL;
			}
			AudioStreamBasicDescription fmt = {0};
			AudioObjectPropertyAddress fa = global(kAudioTapPropertyFormat);
			UInt32 fsize = sizeof(fmt);
			st = AudioObjectGetPropertyData(tap, &fa, 0, NULL, &fsize, &fmt);
			if (st != noErr || fmt.mFormatID != kAudioFormatLinearPCM || !(fmt.mFormatFlags & kAudioFormatFlagIsFloat) || fmt.mBitsPerChannel != 32 || fmt.mSampleRate <= 0) {
				AudioHardwareDestroyProcessTap(tap);
				snprintf(err, (size_t)errlen, "macOS offered the meeting's sound in a form Agentry does not read");
				return NULL;
			}
			NSString *out = device_uid(ca_default_device(0));
			if (out == nil) {
				AudioHardwareDestroyProcessTap(tap);
				snprintf(err, (size_t)errlen, "this Mac has no sound output to record");
				return NULL;
			}
			NSDictionary *agg = @{
				@kAudioAggregateDeviceNameKey : TAP_DEVICE_NAME,
				@kAudioAggregateDeviceUIDKey : [[NSUUID UUID] UUIDString],
				@kAudioAggregateDeviceMainSubDeviceKey : out,
				@kAudioAggregateDeviceIsPrivateKey : @YES,
				@kAudioAggregateDeviceIsStackedKey : @NO,
				@kAudioAggregateDeviceTapAutoStartKey : @YES,
				@kAudioAggregateDeviceSubDeviceListKey : @[],
				@kAudioAggregateDeviceTapListKey : @[ @{@kAudioSubTapDriftCompensationKey : @YES, @kAudioSubTapUIDKey : desc.UUID.UUIDString} ],
			};
			AudioObjectID aggID = kAudioObjectUnknown;
			st = AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)agg, &aggID);
			if (st != noErr) {
				AudioHardwareDestroyProcessTap(tap);
				fail_status(err, errlen, "set up the device that records the meeting's sound", st);
				return NULL;
			}
			ca_capture *c = capture_new(fmt.mSampleRate);
			if (c == NULL) {
				AudioHardwareDestroyAggregateDevice(aggID);
				AudioHardwareDestroyProcessTap(tap);
				snprintf(err, (size_t)errlen, "out of memory");
				return NULL;
			}
			c->tap = tap;
			c->agg = aggID;
			c->interleaved = !(fmt.mFormatFlags & kAudioFormatFlagIsNonInterleaved);
			st = AudioDeviceCreateIOProcIDWithBlock(&c->proc, aggID, NULL,
			                                        ^(const AudioTimeStamp *now, const AudioBufferList *in, const AudioTimeStamp *inTime,
			                                          AudioBufferList *outData, const AudioTimeStamp *outTime) {
				                                        (void)now;
				                                        (void)inTime;
				                                        (void)outData;
				                                        (void)outTime;
				                                        UInt32 ch = in->mNumberBuffers ? in->mBuffers[0].mNumberChannels : 1;
				                                        if (ch == 0) {
					                                        ch = 1;
				                                        }
				                                        UInt32 frames = in->mNumberBuffers ? in->mBuffers[0].mDataByteSize / (UInt32)(sizeof(float) * (c->interleaved ? ch : 1)) : 0;
				                                        deliver(c, in, frames);
			                                        });
			if (st == noErr) {
				st = AudioDeviceStart(aggID, c->proc);
				if (st != noErr) {
					fail_status(err, errlen, "start the device that records the meeting's sound", st);
				}
			} else {
				fail_status(err, errlen, "read the device that records the meeting's sound", st);
			}
			if (st != noErr) {
				ca_stop(c);
				return NULL;
			}
			return c;
		}
	}
	snprintf(err, (size_t)errlen, "recording needs macOS 14.4 or later");
	return NULL;
}

// The microphone is read at the transcriber's own rate, one channel: the
// queue converts from whatever the device delivers.
#define MIC_RATE 16000.0
#define MIC_BUFFERS 3
#define MIC_FRAMES 1600

static void mic_input(void *user, AudioQueueRef q, AudioQueueBufferRef buf, const AudioTimeStamp *start, UInt32 packets,
                      const AudioStreamPacketDescription *desc) {
	(void)start;
	(void)desc;
	ca_capture *c = user;
	AudioBufferList bl = {1, {{1, buf->mAudioDataByteSize, buf->mAudioData}}};
	UInt32 frames = packets ? packets : buf->mAudioDataByteSize / sizeof(float);
	deliver(c, &bl, frames);
	AudioQueueEnqueueBuffer(q, buf, 0, NULL);
}

static void copy_utf8(NSString *s, char *out, size_t max) {
	if (s == nil || ![s getCString:out maxLength:max encoding:NSUTF8StringEncoding]) {
		out[0] = 0;
	}
}

int ca_mics(ca_mic *out, int max) {
	@autoreleasepool {
		AudioObjectPropertyAddress a = global(kAudioHardwarePropertyDevices);
		UInt32 size = 0;
		if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &a, 0, NULL, &size) != noErr) {
			return -1;
		}
		int n = (int)(size / sizeof(AudioObjectID));
		if (n == 0) {
			return 0;
		}
		AudioObjectID *ids = calloc((size_t)n, sizeof(AudioObjectID));
		if (ids == NULL) {
			return -1;
		}
		if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, ids) != noErr) {
			free(ids);
			return -1;
		}
		n = (int)(size / sizeof(AudioObjectID));
		AudioObjectID def = ca_default_device(1);
		int count = 0;
		for (int i = 0; i < n; i++) {
			AudioObjectPropertyAddress sa = {kAudioDevicePropertyStreams, kAudioObjectPropertyScopeInput, kAudioObjectPropertyElementMain};
			UInt32 ssize = 0;
			if (AudioObjectGetPropertyDataSize(ids[i], &sa, 0, NULL, &ssize) != noErr || ssize == 0) {
				continue;
			}
			NSString *uid = device_uid(ids[i]);
			if (uid == nil) {
				continue;
			}
			AudioObjectPropertyAddress na = global(kAudioObjectPropertyName);
			CFStringRef cfname = NULL;
			UInt32 nsize = sizeof(cfname);
			NSString *name = nil;
			if (AudioObjectGetPropertyData(ids[i], &na, 0, NULL, &nsize, &cfname) == noErr && cfname != NULL) {
				name = (__bridge_transfer NSString *)cfname;
			}
			if ([name isEqualToString:TAP_DEVICE_NAME]) {
				continue;
			}
			if (count < max) {
				ca_mic *m = &out[count];
				memset(m, 0, sizeof(*m));
				copy_utf8(uid, m->uid, sizeof(m->uid));
				copy_utf8(name, m->name, sizeof(m->name));
				m->is_default = ids[i] == def;
			}
			count++;
		}
		free(ids);
		return count;
	}
}

ca_capture *ca_mic_start(const char *uid, char *err, int errlen) {
	AudioStreamBasicDescription fmt = {0};
	fmt.mSampleRate = MIC_RATE;
	fmt.mFormatID = kAudioFormatLinearPCM;
	fmt.mFormatFlags = kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked;
	fmt.mChannelsPerFrame = 1;
	fmt.mBitsPerChannel = 32;
	fmt.mBytesPerFrame = 4;
	fmt.mFramesPerPacket = 1;
	fmt.mBytesPerPacket = 4;
	ca_capture *c = capture_new(MIC_RATE);
	if (c == NULL) {
		snprintf(err, (size_t)errlen, "out of memory");
		return NULL;
	}
	c->interleaved = 1;
	OSStatus st = AudioQueueNewInput(&fmt, mic_input, c, NULL, kCFRunLoopCommonModes, 0, &c->queue);
	if (st != noErr) {
		capture_free(c);
		fail_status(err, errlen, "open the microphone", st);
		return NULL;
	}
	if (uid != NULL && uid[0] != 0) {
		// The device chosen, by its UID, before the queue starts.
		CFStringRef dev = CFStringCreateWithCString(NULL, uid, kCFStringEncodingUTF8);
		if (dev == NULL) {
			snprintf(err, (size_t)errlen, "the microphone's name could not be read");
			ca_stop(c);
			return NULL;
		}
		st = AudioQueueSetProperty(c->queue, kAudioQueueProperty_CurrentDevice, &dev, sizeof(dev));
		CFRelease(dev);
		if (st != noErr) {
			fail_status(err, errlen, "open the microphone chosen", st);
			ca_stop(c);
			return NULL;
		}
	}
	for (int i = 0; i < MIC_BUFFERS && st == noErr; i++) {
		AudioQueueBufferRef b = NULL;
		st = AudioQueueAllocateBuffer(c->queue, MIC_FRAMES * sizeof(float), &b);
		if (st == noErr) {
			st = AudioQueueEnqueueBuffer(c->queue, b, 0, NULL);
		}
	}
	if (st == noErr) {
		st = AudioQueueStart(c->queue, NULL);
	}
	if (st != noErr) {
		fail_status(err, errlen, "start the microphone", st);
		ca_stop(c);
		return NULL;
	}
	return c;
}

double ca_rate(ca_capture *c) { return c->rate; }

int ca_read(ca_capture *c, float *out, int max) {
	uint64_t r = atomic_load_explicit(&c->r, memory_order_relaxed);
	uint64_t w = atomic_load_explicit(&c->w, memory_order_acquire);
	uint64_t n = w - r;
	if (n > (uint64_t)max) {
		n = (uint64_t)max;
	}
	for (uint64_t i = 0; i < n; i++) {
		out[i] = c->ring[(r + i) % c->cap];
	}
	atomic_store_explicit(&c->r, r + n, memory_order_release);
	return (int)n;
}

uint64_t ca_frames(ca_capture *c) { return atomic_load(&c->frames); }
uint64_t ca_dropped(ca_capture *c) { return atomic_load(&c->dropped); }
uint64_t ca_heard(ca_capture *c) { return atomic_load(&c->heard); }

void ca_stop(ca_capture *c) {
	if (c == NULL) {
		return;
	}
	if (c->queue != NULL) {
		// Synchronous: no callback runs after this returns.
		AudioQueueStop(c->queue, true);
		AudioQueueDispose(c->queue, true);
	}
	if (c->agg != kAudioObjectUnknown) {
		if (c->proc != NULL) {
			// AudioDeviceStop waits for the IO proc in flight to return.
			AudioDeviceStop(c->agg, c->proc);
			AudioDeviceDestroyIOProcID(c->agg, c->proc);
		}
		AudioHardwareDestroyAggregateDevice(c->agg);
	}
	if (@available(macOS 14.4, *)) {
		if (c->tap != kAudioObjectUnknown) {
			AudioHardwareDestroyProcessTap(c->tap);
		}
	}
	capture_free(c);
}
