// Core Audio for Meeting notes on macOS 14.4 and later: the processes that
// use audio, a process tap of a meeting app's processes (or of everything
// but this app), and an input device, the default or one chosen. Each
// capture fills a ring of mono
// float samples at its own rate from Core Audio's thread; Go drains it.
#ifndef AGENTRY_COREAUDIO_DARWIN_H
#define AGENTRY_COREAUDIO_DARWIN_H

#include <stdint.h>

// ca_supported is 1 on macOS 14.4 and later, where process taps work.
int ca_supported(void);

// ca_proc is one process Core Audio lists.
typedef struct {
	uint32_t object; // its audio process object
	int32_t pid;
	int input;  // it records from an input device now
	int output; // it plays to an output device now
	char path[4096]; // its executable's path (PROC_PIDPATHINFO_MAXSIZE), "" when unknown
} ca_proc;

// ca_processes fills out with up to max processes and returns how many
// there are (which may be more than max), or -1.
int ca_processes(ca_proc *out, int max);

// ca_default_device is the default input (input != 0) or output device.
uint32_t ca_default_device(int input);

// ca_self_object is this process's audio process object, 0 when it has none.
uint32_t ca_self_object(void);

// The microphone permission, as AVCaptureDevice reports it.
enum { CA_AUTH_UNDETERMINED = 0, CA_AUTH_RESTRICTED = 1, CA_AUTH_DENIED = 2, CA_AUTH_AUTHORIZED = 3 };
int ca_mic_authorization(void);
// ca_mic_ask has macOS ask the person for the microphone, if it has not
// asked yet, and returns at once: the answer comes later.
void ca_mic_ask(void);

typedef struct ca_capture ca_capture;

// ca_tap_start taps the processes objects names (n of them), or with
// global != 0 every process but those it names, mixed down to one channel.
// On failure it returns NULL and writes why into err.
ca_capture *ca_tap_start(const uint32_t *objects, int n, int global, char *err, int errlen);

// ca_mic is one input device: its UID, its name, and whether it is the
// default input.
typedef struct {
	char uid[512];
	char name[512];
	int is_default;
} ca_mic;

// ca_mics fills out with up to max input devices (this app's own tap
// devices left out) and returns how many there are, or -1.
int ca_mics(ca_mic *out, int max);

// ca_mic_start records the input device whose UID is uid, or the default
// input when uid is NULL or empty. On failure it returns NULL and writes why
// into err.
ca_capture *ca_mic_start(const char *uid, char *err, int errlen);

// ca_rate is the capture's sample rate.
double ca_rate(ca_capture *c);

// ca_read moves up to max samples out of the ring and returns how many.
int ca_read(ca_capture *c, float *out, int max);

// ca_frames is how many frames Core Audio has delivered, ca_dropped how
// many were lost to a full ring, and ca_heard how many were not exactly
// zero: a tap macOS has not allowed delivers only zeros.
uint64_t ca_frames(ca_capture *c);
uint64_t ca_dropped(ca_capture *c);
uint64_t ca_heard(ca_capture *c);

// ca_stop stops the capture and frees it.
void ca_stop(ca_capture *c);

#endif
