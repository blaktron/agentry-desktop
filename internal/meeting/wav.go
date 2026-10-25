package meeting

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"
)

// wavHeaderSize is a canonical PCM WAV header's length.
const wavHeaderSize = 44

// lateSamples is how far behind its arrival time a track may fall before
// silence is put in to bring it level: a stream that stopped for longer (a
// pause, a device that went quiet, a stream that started late) keeps its
// place on the timeline. Smaller gaps are the audio stack's own jitter.
const lateSamples = SampleRate / 4

// wavTrack is a Track written to a 16-bit mono WAV file. Its sample position
// follows the wall clock from the recording's start: the first samples, and
// any after a gap, are preceded by enough silence to land when they arrived.
type wavTrack struct {
	mu      sync.Mutex
	f       *os.File
	w       *bufio.Writer
	start   time.Time
	now     func() time.Time
	samples int64
	err     error
	ended   bool
	// loud counts the 100 ms windows whose level was over silenceLevel.
	loud   int
	window []int16
}

func newWAVTrack(path string, start time.Time, now func() time.Time) (*wavTrack, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a file in the recording's own folder
	if err != nil {
		return nil, err
	}
	t := &wavTrack{f: f, w: bufio.NewWriterSize(f, 64<<10), start: start, now: now}
	if _, err := t.w.Write(make([]byte, wavHeaderSize)); err != nil {
		_ = f.Close()
		return nil, err
	}
	return t, nil
}

// Write appends samples that arrived just now, after silence if the track
// had fallen behind the clock.
func (t *wavTrack) Write(samples []int16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended || t.err != nil {
		return
	}
	due := int64(t.now().Sub(t.start).Seconds()*SampleRate) - int64(len(samples))
	if gap := due - t.samples; gap > lateSamples {
		t.writeSilence(gap)
	}
	t.writeSamples(samples)
}

func (t *wavTrack) writeSilence(n int64) {
	zero := make([]int16, 4096)
	for n > 0 && t.err == nil {
		k := min(n, int64(len(zero)))
		t.writeSamples(zero[:k])
		n -= k
	}
}

func (t *wavTrack) writeSamples(s []int16) {
	if t.err = binary.Write(t.w, binary.LittleEndian, s); t.err != nil {
		return
	}
	t.samples += int64(len(s))
	t.measure(s)
}

// measure keeps the count of loud 100 ms windows, so a silent track can be
// told apart without reading it back.
func (t *wavTrack) measure(s []int16) {
	const win = SampleRate / 10
	for len(s) > 0 {
		k := min(len(s), win-len(t.window))
		t.window = append(t.window, s[:k]...)
		s = s[k:]
		if len(t.window) == win {
			if rms(t.window) > silenceLevel {
				t.loud++
			}
			t.window = t.window[:0]
		}
	}
}

// End finishes the file: the header gets the data's length.
func (t *wavTrack) End() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return
	}
	t.ended = true
	if t.err == nil {
		t.err = t.w.Flush()
	}
	if t.err == nil {
		_, t.err = t.f.WriteAt(wavHeader(t.samples), 0)
	}
	if err := t.f.Close(); t.err == nil {
		t.err = err
	}
}

// Silent says whether the track held no window louder than silenceLevel.
func (t *wavTrack) Silent() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.loud < minLoudWindows
}

// Duration is the track's length on the timeline.
func (t *wavTrack) Duration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Duration(t.samples) * time.Second / SampleRate
}

// Err is the first write error, if any.
func (t *wavTrack) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

func wavHeader(samples int64) []byte {
	data := uint32(samples * 2)
	h := make([]byte, wavHeaderSize)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+data)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // mono
	binary.LittleEndian.PutUint32(h[24:], SampleRate)
	binary.LittleEndian.PutUint32(h[28:], SampleRate*2)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], data)
	return h
}

// The level below which a 100 ms window counts as silence, as a fraction
// of full scale (about -60 dBFS): a room's noise on a laptop microphone sits
// well under it, speech well over. A track with fewer than minLoudWindows
// loud windows (half a second of sound) is silent: Whisper turns silence
// into words that were never said, so a silent track is not transcribed.
const (
	silenceLevel   = 0.001
	minLoudWindows = 5
)

func rms(s []int16) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		f := float64(v) / 32768
		sum += f * f
	}
	return math.Sqrt(sum / float64(len(s)))
}

// SilentWAV reads a 16-bit mono WAV and says whether it is silent by the
// same measure a track keeps as it records.
func SilentWAV(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // a track this package wrote
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(wavHeaderSize, io.SeekStart); err != nil {
		return false, err
	}
	r := bufio.NewReaderSize(f, 64<<10)
	win := make([]int16, SampleRate/10)
	loud := 0
	for {
		err := binary.Read(r, binary.LittleEndian, win)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return loud < minLoudWindows, nil
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}
		if rms(win) > silenceLevel {
			loud++
			if loud >= minLoudWindows {
				return false, nil
			}
		}
	}
}
