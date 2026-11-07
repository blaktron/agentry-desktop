// The macOS window system as the computer-use backend reads and drives it.
// Every position is in points of the global display space (origin at the
// main display's top left), as Quartz gives a window's bounds.
#include <stdint.h>

typedef struct {
	uint32_t id;
	int32_t pid;
	double x, y, w, h;
	int32_t layer;
	double alpha;
	char owner[256];
	char title[512];
	char bundle[256];
} qz_win;

// qz_windows lists the windows on screen, front to back, into out (at most
// max); it returns how many there are, which may be more than max.
int qz_windows(qz_win *out, int max);
// qz_frontmost_pid is the process of the frontmost application.
int32_t qz_frontmost_pid(void);
// qz_scale is the backing scale of the display the rectangle is on.
double qz_scale(double x, double y, double w, double h);
// qz_main_display is the main display's bounds in points.
void qz_main_display(double *x, double *y, double *w, double *h);

// qz_capture_window captures one window at pw by ph pixels; qz_capture_display
// the main display. Each returns 0 and an RGBA buffer the caller frees with
// qz_free, or -1 and a message in err.
int qz_capture_window(uint32_t wid, int pw, int ph, uint8_t **out, int *ow, int *oh, char *err, int errn);
int qz_capture_display(int pw, int ph, uint8_t **out, int *ow, int *oh, char *err, int errn);
void qz_free(void *p);

// qz_activate brings the application to the front and raises its window at
// the bounds given; 0 on success.
int qz_activate(int32_t pid, double x, double y, double w, double h);

int qz_cursor(double *x, double *y);
int qz_move(double x, double y, uint64_t flags);
int qz_button(int button, int down, double x, double y, uint64_t flags, int clicks);
int qz_scroll(int32_t dy, int32_t dx, double x, double y, uint64_t flags);
int qz_key(uint16_t code, int down, uint64_t flags);
int qz_unicode(uint32_t cp, int down, uint64_t flags);

// The privacy permissions computer use needs: Screen Recording to capture,
// Accessibility to post input and raise a window.
int qz_screen_granted(void);
void qz_request_screen(void);
int qz_ax_granted(void);
void qz_request_ax(void);
// qz_capture_kit says whether ScreenCaptureKit's screenshot API (macOS 14)
// loaded; 0 means the older window image call is used.
int qz_capture_kit(void);

