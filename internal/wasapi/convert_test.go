package wasapi

import (
	"encoding/binary"
	"math"
	"testing"
)

func floatStereo(samples []float32) []byte {
	b := make([]byte, 8*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint32(b[8*i:], math.Float32bits(s))
		binary.LittleEndian.PutUint32(b[8*i+4:], math.Float32bits(s))
	}
	return b
}

// The format a stream is first asked for passes through as it is.
func TestTheDirectFormatPassesThrough(t *testing.T) {
	c := newConverter(direct)
	got := c.convert([]byte{0x01, 0x00, 0xff, 0x7f, 0x00, 0x80, 0x07})
	want := []int16{1, 32767, -32768}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// 48 kHz float stereo becomes a third as many 16 kHz samples, however the
// engine cuts its packets, and a tone keeps its level.
func TestTheEnginesFormatIsResampledAcrossPackets(t *testing.T) {
	f := Format{Channels: 2, Rate: 48000, Bits: 32, Float: true, BlockAlign: 8}
	if !f.supported() {
		t.Fatal("48 kHz float stereo is not supported")
	}
	c := newConverter(f)
	const n = 48000
	in := make([]float32, n)
	for i := range in {
		in[i] = float32(0.5 * math.Sin(2*math.Pi*440*float64(i)/48000))
	}
	var out []int16
	for start := 0; start < n; {
		end := min(start+(start%997)+1, n) // packets of uneven size
		out = append(out, c.convert(floatStereo(in[start:end]))...)
		start = end
	}
	if len(out) < 15999 || len(out) > 16001 {
		t.Fatalf("%d samples from a second at 48 kHz, want 16000", len(out))
	}
	var peak int16
	for _, s := range out[100:] {
		peak = max(peak, s)
	}
	if peak < 15000 || peak > 17000 {
		t.Fatalf("a 0.5 tone peaks at %d, want about 16384", peak)
	}
}

// What is above 8 kHz is kept from folding into speech: a 15 kHz tone at
// 48 kHz comes out far quieter than it went in.
func TestHighSoundsAreFilteredBeforeTheRateDrops(t *testing.T) {
	f := Format{Channels: 2, Rate: 48000, Bits: 32, Float: true, BlockAlign: 8}
	c := newConverter(f)
	in := make([]float32, 48000)
	for i := range in {
		in[i] = float32(0.5 * math.Sin(2*math.Pi*15000*float64(i)/48000))
	}
	var peak int16
	for _, s := range c.convert(floatStereo(in))[100:] {
		peak = max(peak, s, -s)
	}
	if peak > 6000 {
		t.Fatalf("a 15 kHz tone at 0.5 peaks at %d after conversion", peak)
	}
}

func TestIntegerFormatsAreRead(t *testing.T) {
	for _, bits := range []int{16, 24, 32} {
		f := Format{Channels: 1, Rate: Rate, Bits: bits, BlockAlign: bits / 8}
		b := make([]byte, bits/8)
		// Half of full scale, in each width.
		switch bits {
		case 16:
			binary.LittleEndian.PutUint16(b, 0x4000)
		case 24:
			b[2] = 0x40
		case 32:
			binary.LittleEndian.PutUint32(b, 0x40000000)
		}
		c := newConverter(f)
		got := c.convert(append(b, b...))
		if len(got) == 0 || got[len(got)-1] < 16000 || got[len(got)-1] > 16800 {
			t.Errorf("%d bits: half scale became %v", bits, got)
		}
	}
	if (Format{Channels: 2, Rate: 44100, Bits: 8, BlockAlign: 2}).supported() {
		t.Error("8-bit samples are not read")
	}
}
