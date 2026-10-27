package meeting

import (
	"bufio"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// roomFixture is a recording of a meeting in a room, with its reference:
// who speaks when.
type roomFixture struct {
	name    string
	seconds int
	voices  int
}

var roomFixtures = []roomFixture{
	{"two-voices", 60, 2},
	{"three-voices", 75, 3},
	{"five-voices", 120, 5},
	{"crosstalk", 60, 4},
	{"quiet-speaker", 60, 4},
}

// rttmSpan is one stretch of one speaker's speech in a reference.
type rttmSpan struct {
	start, end float64
	speaker    string
}

func readRTTM(t *testing.T, path string) []rttmSpan {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []rttmSpan
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Fields(sc.Text())
		if len(p) < 8 || p[0] != "SPEAKER" {
			t.Fatalf("%s: a line that is not a speaker turn: %q", path, sc.Text())
		}
		start, err1 := strconv.ParseFloat(p[3], 64)
		dur, err2 := strconv.ParseFloat(p[4], 64)
		if err1 != nil || err2 != nil || dur <= 0 {
			t.Fatalf("%s: a turn with no time: %q", path, sc.Text())
		}
		out = append(out, rttmSpan{start, start + dur, p[7]})
	}
	return out
}

// The recordings are what the speaker separation is measured on: each is
// a 16 kHz mono 16-bit WAV of its stated length, and its reference names
// as many voices as its name says, every turn inside the recording.
func TestTheRoomRecordingsAreWhatTheySay(t *testing.T) {
	for _, fx := range roomFixtures {
		t.Run(fx.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "rooms", fx.name+".wav"))
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
				t.Fatal("not a WAV")
			}
			channels := binary.LittleEndian.Uint16(raw[22:24])
			rate := binary.LittleEndian.Uint32(raw[24:28])
			bits := binary.LittleEndian.Uint16(raw[34:36])
			data := binary.LittleEndian.Uint32(raw[40:44])
			if channels != 1 || rate != SampleRate || bits != 16 || string(raw[36:40]) != "data" {
				t.Fatalf("%d channels, %d Hz, %d bits", channels, rate, bits)
			}
			if got := int(data) / 2 / SampleRate; got != fx.seconds {
				t.Fatalf("%d s, want %d", got, fx.seconds)
			}
			spans := readRTTM(t, filepath.Join("testdata", "rooms", fx.name+".rttm"))
			voices := map[string]bool{}
			for _, s := range spans {
				voices[s.speaker] = true
				if s.start < 0 || s.end > float64(fx.seconds)+0.01 {
					t.Fatalf("a turn at %.3f-%.3f s, outside the recording", s.start, s.end)
				}
			}
			if len(voices) != fx.voices {
				t.Fatalf("%d voices in the reference, want %d", len(voices), fx.voices)
			}
		})
	}
}
