package wasapi

import (
	"encoding/binary"
	"math"
)

// Rate is the rate every stream is delivered at: 16 kHz mono, the
// transcriber's own.
const Rate = 16000

// Format is the shape of an endpoint's samples: integer PCM of 16, 24 or 32
// bits, or 32-bit float, interleaved.
type Format struct {
	Channels   int
	Rate       int
	Bits       int
	Float      bool
	BlockAlign int
}

// direct is the format a stream is first asked for, which the engine
// converts to; the converter then only reads it.
var direct = Format{Channels: 1, Rate: Rate, Bits: 16, BlockAlign: 2}

// supported says whether the converter reads f.
func (f Format) supported() bool {
	if f.Channels < 1 || f.Rate < 1 || f.BlockAlign != f.Channels*f.Bits/8 {
		return false
	}
	if f.Float {
		return f.Bits == 32
	}
	return f.Bits == 16 || f.Bits == 24 || f.Bits == 32
}

// converter turns an endpoint's samples into 16 kHz mono 16-bit ones: the
// channels averaged, then resampled by linear interpolation after a moving
// average as wide as the rate's ratio, which keeps what is above 8 kHz from
// folding into speech. It keeps its place between calls.
type converter struct {
	f    Format
	step float64 // input samples per output sample
	pos  float64 // the next output sample's place, from prev
	prev float64 // the last input sample of the previous call
	win  []float64
	sum  float64
	wi   int
}

func newConverter(f Format) *converter {
	c := &converter{f: f, step: float64(f.Rate) / Rate, pos: 1}
	if w := int(math.Round(c.step)); w > 1 {
		c.win = make([]float64, w)
	}
	return c
}

// convert takes whole frames; a trailing part frame is dropped.
func (c *converter) convert(b []byte) []int16 {
	f := c.f
	frames := len(b) / f.BlockAlign
	if f == direct {
		out := make([]int16, frames)
		for i := range out {
			out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
		}
		return out
	}
	mono := make([]float64, frames)
	bytes := f.Bits / 8
	for i := range mono {
		var sum float64
		for ch := range f.Channels {
			sum += sample(b[i*f.BlockAlign+ch*bytes:], f)
		}
		mono[i] = c.smooth(sum / float64(f.Channels))
	}
	var out []int16
	// pos counts from prev (0) through mono (1 to frames).
	for ; c.pos <= float64(frames); c.pos += c.step {
		i := int(c.pos)
		frac := c.pos - float64(i)
		a := c.prev
		if i > 0 {
			a = mono[i-1]
		}
		v := a
		if frac > 0 && i < frames {
			v = a + (mono[i]-a)*frac
		}
		out = append(out, clamp16(v))
	}
	c.pos -= float64(frames)
	if frames > 0 {
		c.prev = mono[frames-1]
	}
	return out
}

// smooth is the moving average, or the sample itself when the rate is not
// lowered.
func (c *converter) smooth(v float64) float64 {
	if c.win == nil {
		return v
	}
	c.sum += v - c.win[c.wi]
	c.win[c.wi] = v
	c.wi = (c.wi + 1) % len(c.win)
	return c.sum / float64(len(c.win))
}

// sample reads one sample as a float in [-1, 1].
func sample(b []byte, f Format) float64 {
	switch {
	case f.Float:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case f.Bits == 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case f.Bits == 24:
		v := int32(b[0])<<8 | int32(b[1])<<16 | int32(b[2])<<24
		return float64(v) / 2147483648
	default:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	}
}

func clamp16(v float64) int16 {
	v *= 32767
	switch {
	case v > 32767:
		return 32767
	case v < -32768:
		return -32768
	}
	return int16(math.Round(v))
}
