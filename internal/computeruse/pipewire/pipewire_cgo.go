//go:build linux && cgo

// Package pipewire reads the video of one PipeWire node, such as a window
// stream the ScreenCast portal shares, and returns its newest frame.
package pipewire

/*
#cgo pkg-config: libpipewire-0.3

#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include <spa/buffer/meta.h>
#include <spa/param/buffers.h>
#include <spa/param/video/format-utils.h>
#include <spa/pod/builder.h>
#include <pipewire/pipewire.h>

#define CAP_MAX_DIM 8192
#define CAP_MAX_BYTES ((size_t)CAP_MAX_DIM * CAP_MAX_DIM * 4)

enum { CAP_BGRX = 0, CAP_BGRA = 1, CAP_RGBX = 2, CAP_RGBA = 3 };

typedef struct cap {
	struct pw_thread_loop *loop;
	struct pw_context *context;
	struct pw_core *core;
	struct pw_stream *stream;
	struct spa_hook core_hook;
	struct spa_hook stream_hook;

	pthread_mutex_t mu;
	pthread_cond_t cond;

	// Guarded by mu.
	uint8_t *pixels;
	size_t cap_bytes;
	int width, height, format;
	uint64_t count;
	int failed;
	char err[256];

	// Negotiated format, touched only on the loop thread.
	struct spa_video_info_raw info;
	int have_format;
} cap;

static void cap_fail(cap *c, const char *msg)
{
	pthread_mutex_lock(&c->mu);
	if (!c->failed) {
		c->failed = 1;
		snprintf(c->err, sizeof c->err, "%s", msg);
	}
	pthread_cond_broadcast(&c->cond);
	pthread_mutex_unlock(&c->mu);
}

static int cap_format_code(uint32_t f)
{
	switch (f) {
	case SPA_VIDEO_FORMAT_BGRx: return CAP_BGRX;
	case SPA_VIDEO_FORMAT_BGRA: return CAP_BGRA;
	case SPA_VIDEO_FORMAT_RGBx: return CAP_RGBX;
	case SPA_VIDEO_FORMAT_RGBA: return CAP_RGBA;
	}
	return -1;
}

static void on_param_changed(void *data, uint32_t id, const struct spa_pod *param)
{
	cap *c = data;
	uint8_t buf[512];
	struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buf, sizeof buf);
	const struct spa_pod *params[2];

	if (param == NULL || id != SPA_PARAM_Format)
		return;
	uint32_t mt, mst;
	if (spa_format_parse(param, &mt, &mst) < 0 ||
	    mt != SPA_MEDIA_TYPE_video || mst != SPA_MEDIA_SUBTYPE_raw)
		return;
	if (spa_format_video_raw_parse(param, &c->info) < 0)
		return;
	if (cap_format_code(c->info.format) < 0 ||
	    c->info.size.width == 0 || c->info.size.height == 0 ||
	    c->info.size.width > CAP_MAX_DIM || c->info.size.height > CAP_MAX_DIM) {
		cap_fail(c, "the stream's video format is not supported");
		return;
	}
	c->have_format = 1;

	params[0] = spa_pod_builder_add_object(&b,
		SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
		SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int(4, 2, 16),
		SPA_PARAM_BUFFERS_blocks, SPA_POD_Int(1),
		SPA_PARAM_BUFFERS_dataType, SPA_POD_CHOICE_FLAGS_Int(
			(1 << SPA_DATA_MemPtr) | (1 << SPA_DATA_MemFd)));
	params[1] = spa_pod_builder_add_object(&b,
		SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
		SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Header),
		SPA_PARAM_META_size, SPA_POD_Int(sizeof(struct spa_meta_header)));
	pw_stream_update_params(c->stream, params, 2);
}

static void on_state_changed(void *data, enum pw_stream_state old,
			     enum pw_stream_state state, const char *error)
{
	cap *c = data;
	if (state == PW_STREAM_STATE_ERROR) {
		cap_fail(c, error && *error ? error : "the stream failed");
	} else if (state == PW_STREAM_STATE_UNCONNECTED &&
		   old != PW_STREAM_STATE_UNCONNECTED) {
		cap_fail(c, "the stream ended");
	}
}

static void on_process(void *data)
{
	cap *c = data;
	struct pw_buffer *newest = NULL, *b;

	while ((b = pw_stream_dequeue_buffer(c->stream)) != NULL) {
		if (newest)
			pw_stream_queue_buffer(c->stream, newest);
		newest = b;
	}
	if (newest == NULL)
		return;

	struct spa_buffer *sb = newest->buffer;
	struct spa_data *d = &sb->datas[0];
	struct spa_meta_header *h = spa_buffer_find_meta_data(sb, SPA_META_Header, sizeof *h);
	int code = cap_format_code(c->info.format);

	if (!c->have_format || code < 0 || sb->n_datas < 1 || d->data == NULL ||
	    d->chunk == NULL || d->chunk->size == 0 ||
	    (d->chunk->flags & SPA_CHUNK_FLAG_CORRUPTED) ||
	    (h && (h->flags & SPA_META_HEADER_FLAG_CORRUPTED)))
		goto out;

	size_t w = c->info.size.width, ht = c->info.size.height;
	size_t row = w * 4;
	size_t total = row * ht;
	if (total > CAP_MAX_BYTES)
		goto out;
	size_t off = d->chunk->offset;
	size_t stride = d->chunk->stride > 0 ? (size_t)d->chunk->stride : row;
	if (stride < row || off > d->maxsize ||
	    (ht - 1) * stride + row > d->maxsize - off)
		goto out;

	pthread_mutex_lock(&c->mu);
	if (c->cap_bytes < total) {
		uint8_t *p = realloc(c->pixels, total);
		if (p == NULL) {
			pthread_mutex_unlock(&c->mu);
			goto out;
		}
		c->pixels = p;
		c->cap_bytes = total;
	}
	const uint8_t *src = (const uint8_t *)d->data + off;
	for (size_t y = 0; y < ht; y++)
		memcpy(c->pixels + y * row, src + y * stride, row);
	c->width = (int)w;
	c->height = (int)ht;
	c->format = code;
	c->count++;
	pthread_cond_broadcast(&c->cond);
	pthread_mutex_unlock(&c->mu);
out:
	pw_stream_queue_buffer(c->stream, newest);
}

static const struct pw_stream_events stream_events = {
	PW_VERSION_STREAM_EVENTS,
	.state_changed = on_state_changed,
	.param_changed = on_param_changed,
	.process = on_process,
};

static void on_core_error(void *data, uint32_t id, int seq, int res, const char *message)
{
	cap *c = data;
	if (id == PW_ID_CORE && (res == -EPIPE || res == -ECONNRESET))
		cap_fail(c, "the PipeWire connection closed");
}

static const struct pw_core_events core_events = {
	PW_VERSION_CORE_EVENTS,
	.error = on_core_error,
};

static void cap_destroy(cap *c)
{
	if (c->loop) {
		pw_thread_loop_lock(c->loop);
		if (c->stream) {
			spa_hook_remove(&c->stream_hook);
			pw_stream_destroy(c->stream);
		}
		if (c->core) {
			spa_hook_remove(&c->core_hook);
			pw_core_disconnect(c->core);
		}
		pw_thread_loop_unlock(c->loop);
		pw_thread_loop_stop(c->loop);
		if (c->context)
			pw_context_destroy(c->context);
		pw_thread_loop_destroy(c->loop);
	}
	free(c->pixels);
	pthread_cond_destroy(&c->cond);
	pthread_mutex_destroy(&c->mu);
	free(c);
}

static void cap_init_once(void)
{
	pw_init(NULL, NULL);
}

// cap_open connects over fd (or to the default daemon when fd < 0) and starts
// the stream on node. It returns NULL and fills err on failure; it owns fd.
static cap *cap_open(int fd, uint32_t node, char *err, size_t errlen)
{
	uint8_t buf[1024];
	struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buf, sizeof buf);
	const struct spa_pod *params[1];
	struct spa_rectangle def = SPA_RECTANGLE(640, 480);
	struct spa_rectangle min = SPA_RECTANGLE(1, 1);
	struct spa_rectangle max = SPA_RECTANGLE(CAP_MAX_DIM, CAP_MAX_DIM);
	struct spa_fraction fdef = SPA_FRACTION(30, 1);
	struct spa_fraction fmin = SPA_FRACTION(0, 1);
	struct spa_fraction fmax = SPA_FRACTION(1000, 1);

	cap *c = calloc(1, sizeof *c);
	if (c == NULL) {
		snprintf(err, errlen, "out of memory");
		if (fd >= 0) close(fd);
		return NULL;
	}
	pthread_mutex_init(&c->mu, NULL);
	pthread_condattr_t ca;
	pthread_condattr_init(&ca);
	pthread_condattr_setclock(&ca, CLOCK_MONOTONIC);
	pthread_cond_init(&c->cond, &ca);
	pthread_condattr_destroy(&ca);

	c->loop = pw_thread_loop_new("agentry-capture", NULL);
	if (c->loop == NULL) {
		snprintf(err, errlen, "cannot create the PipeWire loop");
		goto fail;
	}
	c->context = pw_context_new(pw_thread_loop_get_loop(c->loop), NULL, 0);
	if (c->context == NULL) {
		snprintf(err, errlen, "cannot create the PipeWire context");
		goto fail;
	}
	if (pw_thread_loop_start(c->loop) < 0) {
		snprintf(err, errlen, "cannot start the PipeWire loop");
		goto fail;
	}

	pw_thread_loop_lock(c->loop);
	if (fd >= 0) {
		c->core = pw_context_connect_fd(c->context, fd, NULL, 0);
		fd = -1; // owned by the context from here, whatever the result
	} else {
		c->core = pw_context_connect(c->context, NULL, 0);
	}
	if (c->core == NULL) {
		snprintf(err, errlen, "cannot connect to PipeWire: %s", strerror(errno));
		pw_thread_loop_unlock(c->loop);
		goto fail;
	}
	pw_core_add_listener(c->core, &c->core_hook, &core_events, c);

	c->stream = pw_stream_new(c->core, "agentry-capture",
		pw_properties_new(PW_KEY_MEDIA_TYPE, "Video",
				  PW_KEY_MEDIA_CATEGORY, "Capture",
				  PW_KEY_MEDIA_ROLE, "Screen", NULL));
	if (c->stream == NULL) {
		snprintf(err, errlen, "cannot create the PipeWire stream");
		pw_thread_loop_unlock(c->loop);
		goto fail;
	}
	pw_stream_add_listener(c->stream, &c->stream_hook, &stream_events, c);

	params[0] = spa_pod_builder_add_object(&b,
		SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
		SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_video),
		SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
		SPA_FORMAT_VIDEO_format, SPA_POD_CHOICE_ENUM_Id(5,
			SPA_VIDEO_FORMAT_BGRx, SPA_VIDEO_FORMAT_BGRx,
			SPA_VIDEO_FORMAT_BGRA, SPA_VIDEO_FORMAT_RGBx,
			SPA_VIDEO_FORMAT_RGBA),
		SPA_FORMAT_VIDEO_size, SPA_POD_CHOICE_RANGE_Rectangle(&def, &min, &max),
		SPA_FORMAT_VIDEO_framerate, SPA_POD_CHOICE_RANGE_Fraction(&fdef, &fmin, &fmax));

	if (pw_stream_connect(c->stream, PW_DIRECTION_INPUT, node,
			      PW_STREAM_FLAG_AUTOCONNECT | PW_STREAM_FLAG_MAP_BUFFERS,
			      params, 1) < 0) {
		snprintf(err, errlen, "cannot connect to the video node");
		pw_thread_loop_unlock(c->loop);
		goto fail;
	}
	pw_thread_loop_unlock(c->loop);
	return c;

fail:
	if (fd >= 0) close(fd);
	cap_destroy(c);
	return NULL;
}

// cap_wait waits up to ms for a frame count above min. It returns 0 when one
// is there, 1 on timeout and -1 once the stream has failed.
static int cap_wait(cap *c, uint64_t min, int ms)
{
	struct timespec ts;
	int rc = 0;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	ts.tv_sec += ms / 1000;
	ts.tv_nsec += (long)(ms % 1000) * 1000000L;
	if (ts.tv_nsec >= 1000000000L) {
		ts.tv_sec++;
		ts.tv_nsec -= 1000000000L;
	}
	pthread_mutex_lock(&c->mu);
	while (c->count <= min && !c->failed && rc == 0)
		rc = pthread_cond_timedwait(&c->cond, &c->mu, &ts);
	int res = c->count > min ? 0 : (c->failed ? -1 : 1);
	pthread_mutex_unlock(&c->mu);
	return res;
}

static uint64_t cap_count(cap *c)
{
	pthread_mutex_lock(&c->mu);
	uint64_t n = c->count;
	pthread_mutex_unlock(&c->mu);
	return n;
}

// cap_snapshot returns a malloc'd copy of the newest frame, or NULL.
static uint8_t *cap_snapshot(cap *c, int *w, int *h, int *fmt, uint64_t *n)
{
	pthread_mutex_lock(&c->mu);
	uint8_t *p = NULL;
	if (c->count > 0) {
		size_t total = (size_t)c->width * c->height * 4;
		p = malloc(total);
		if (p) {
			memcpy(p, c->pixels, total);
			*w = c->width;
			*h = c->height;
			*fmt = c->format;
			*n = c->count;
		}
	}
	pthread_mutex_unlock(&c->mu);
	return p;
}

static void cap_size(cap *c, int *w, int *h)
{
	pthread_mutex_lock(&c->mu);
	*w = c->width;
	*h = c->height;
	pthread_mutex_unlock(&c->mu);
}

static void cap_error(cap *c, char *out, size_t n)
{
	pthread_mutex_lock(&c->mu);
	snprintf(out, n, "%s", c->err);
	pthread_mutex_unlock(&c->mu);
}
*/
import "C"

import (
	"context"
	"errors"
	"image"
	"sync"
	"unsafe"
)

var initOnce sync.Once

// waitSlice bounds each wait inside C so a cancelled context is noticed.
const waitSlice = 50

// Stream is a connection to one PipeWire video node.
type Stream struct {
	mu     sync.RWMutex // read-held while C code uses c, write-held to free it
	c      *C.cap
	closed bool
}

// Available reports whether this build can capture a window stream.
func Available() bool { return true }

// Open connects to PipeWire over fd (a remote from the ScreenCast portal; -1
// connects to the user's default PipeWire daemon instead, for tests) and starts
// consuming the video node. Open takes ownership of fd.
func Open(fd int, node uint32) (*Stream, error) {
	initOnce.Do(func() { C.cap_init_once() })
	var msg [256]C.char
	c := C.cap_open(C.int(fd), C.uint32_t(node), &msg[0], C.size_t(len(msg)))
	if c == nil {
		return nil, errors.New("pipewire: " + C.GoString(&msg[0]))
	}
	return &Stream{c: c}, nil
}

// Frame returns a copy of the newest frame, waiting for the first one (or a
// newer one when fresh is true) until ctx ends.
func (s *Stream) Frame(ctx context.Context, fresh bool) (*image.RGBA, error) {
	var min C.uint64_t
	if fresh {
		s.mu.RLock()
		if s.closed {
			s.mu.RUnlock()
			return nil, errClosed
		}
		min = C.cap_count(s.c)
		s.mu.RUnlock()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.mu.RLock()
		if s.closed {
			s.mu.RUnlock()
			return nil, errClosed
		}
		rc := C.cap_wait(s.c, min, waitSlice)
		if rc == 0 {
			img, err := s.snapshot()
			s.mu.RUnlock()
			return img, err
		}
		var err error
		if rc < 0 {
			var msg [256]C.char
			C.cap_error(s.c, &msg[0], C.size_t(len(msg)))
			err = errors.New("pipewire: " + C.GoString(&msg[0]))
		}
		s.mu.RUnlock()
		if err != nil {
			return nil, err
		}
	}
}

var errClosed = errors.New("pipewire: stream is closed")

// snapshot copies the newest frame into an RGBA image; the caller holds the read lock.
func (s *Stream) snapshot() (*image.RGBA, error) {
	var w, h, f C.int
	var n C.uint64_t
	p := C.cap_snapshot(s.c, &w, &h, &f, &n)
	if p == nil {
		return nil, errors.New("pipewire: no frame")
	}
	defer C.free(unsafe.Pointer(p))
	width, height := int(w), int(h)
	src := unsafe.Slice((*byte)(unsafe.Pointer(p)), width*height*4)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	swap := f == C.CAP_BGRX || f == C.CAP_BGRA
	opaque := f == C.CAP_BGRX || f == C.CAP_RGBX
	dst := img.Pix
	for i := 0; i < len(src); i += 4 {
		if swap {
			dst[i], dst[i+1], dst[i+2] = src[i+2], src[i+1], src[i]
		} else {
			dst[i], dst[i+1], dst[i+2] = src[i], src[i+1], src[i+2]
		}
		if opaque {
			dst[i+3] = 255
		} else {
			dst[i+3] = src[i+3]
		}
	}
	return img, nil
}

// Size is the newest frame's size (zero before the first).
func (s *Stream) Size() image.Point {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return image.Point{}
	}
	var w, h C.int
	C.cap_size(s.c, &w, &h)
	return image.Pt(int(w), int(h))
}

// Close stops the stream, disconnects and frees everything; a second call does nothing.
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	C.cap_destroy(s.c)
	s.c = nil
	return nil
}
