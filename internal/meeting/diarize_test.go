package meeting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func sec(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func w(text string, from, to float64) Word { return Word{Text: text, Start: sec(from), End: sec(to)} }

func TestEachWordTakesTheVoiceThatOverlapsItMost(t *testing.T) {
	words := []Word{
		w(" Hello", 0.5, 0.9),  // inside voice 7
		w(" there.", 0.9, 1.6), // 0.1 s of 7, 0.5 s of 3: voice 3
		w(" Yes", 2.0, 2.3),    // in no span: the voice before it
		w(" quite.", 4.0, 4.4), // voice 7 again
	}
	spans := []SpeakerSpan{{sec(0), sec(1.0), 7}, {sec(1.0), sec(1.8), 3}, {sec(3.9), sec(5), 7}}
	if got := speakerOf(words, spans); !slices.Equal(got, []int{7, 3, 3, 7}) {
		t.Fatalf("voices %v", got)
	}
	// Words before any span take the first voice after them.
	early := []Word{w(" Um", 0.1, 0.2), w(" so.", 1.2, 1.5)}
	if got := speakerOf(early, []SpeakerSpan{{sec(1), sec(2), 4}}); !slices.Equal(got, []int{4, 4}) {
		t.Fatalf("voices %v", got)
	}
	if got := speakerOf(early, nil); !slices.Equal(got, []int{-1, -1}) {
		t.Fatalf("with no spans: %v", got)
	}
}

func TestVoicesAreNumberedByWhoSpokeFirstAndWordsJoinIntoLines(t *testing.T) {
	words := []Word{
		w(" Shall", 0.0, 0.3), w(" we", 0.3, 0.5), w(" start?", 0.5, 0.9),
		w(" Yes,", 1.2, 1.5), w(" let's.", 1.5, 1.9),
		w(" Good.", 5.0, 5.3),
		w(" [BLANK_AUDIO]", 6.0, 6.5),
	}
	// The diarizer's own numbers are not the order of speech.
	spans := []SpeakerSpan{{sec(0), sec(1), 5}, {sec(1.1), sec(2), 2}, {sec(4.8), sec(5.5), 5}, {sec(6), sec(7), 9}}
	lines, voices, ok := SpeakerLines(words, spans)
	if !ok || voices != 2 {
		t.Fatalf("ok %v, %d voices", ok, voices)
	}
	var got []string
	for _, l := range lines {
		got = append(got, fmt.Sprintf("%s: %s", l.Label, l.Text))
	}
	want := []string{"Speaker 1: Shall we start?", "Speaker 2: Yes, let's.", "Speaker 1: Good."}
	if !slices.Equal(got, want) {
		t.Fatalf("lines\n%q\nwant\n%q", got, want)
	}
	if _, _, ok := SpeakerLines(words, nil); ok {
		t.Fatal("lines with no voice at all")
	}
}

func TestAVoiceChangeMidSentenceStartsANewLine(t *testing.T) {
	words := []Word{w(" I", 0, 0.2), w(" think", 0.2, 0.5), w(" right", 0.6, 0.9), w(" exactly.", 0.9, 1.3)}
	spans := []SpeakerSpan{{sec(0), sec(0.55), 0}, {sec(0.55), sec(1.4), 1}}
	lines, voices, _ := SpeakerLines(words, spans)
	if voices != 2 || len(lines) != 2 || lines[0].Text != "I think" || lines[1].Text != "right exactly." || lines[1].Label != "Speaker 2" {
		t.Fatalf("%d voices, lines %+v", voices, lines)
	}
}

// stubDiarize is the program as a test sees it: what it was asked, and
// what it answers.
func stubDiarize(out string, progress []string, err error, got *[]string) func(context.Context, string, []string, io.Writer, io.Writer) error {
	return func(_ context.Context, _ string, args []string, stdout, stderr io.Writer) error {
		*got = args
		for _, p := range progress {
			_, _ = io.WriteString(stderr, p)
		}
		_, _ = io.WriteString(stdout, out)
		return err
	}
}

func TestTheDiarizerIsGivenTheCountOrEstimatesIt(t *testing.T) {
	out := "OfflineSpeakerDiarizationConfig(...)\nStarted\n0.470 -- 8.384 speaker_00\n12.603 -- 25.664 speaker_01\n"
	var args []string
	var seen []int
	d := DiarizeSidecar{Path: "/x/sherpa-onnx-offline-speaker-diarization", Threads: 2,
		Run: stubDiarize(out, []string{"progress 10.00%\r", "progress 55.50%\r", "progress 100.00%\n"}, nil, &args)}
	spans, err := d.Diarize(context.Background(), DiarizeRequest{Audio: "a.wav", Segmentation: "s.onnx", Embedding: "e.onnx", Speakers: 3,
		Progress: func(p int) { seen = append(seen, p) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].Speaker != 0 || spans[1].Start != sec(12.603) || spans[1].Speaker != 1 {
		t.Fatalf("spans %+v", spans)
	}
	if !slices.Contains(args, "--clustering.num-clusters=3") || slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "threshold") }) ||
		args[len(args)-1] != "a.wav" || !slices.Contains(args, "--embedding.model=e.onnx") || !slices.Contains(args, "--segmentation.num-threads=2") {
		t.Fatalf("args %q", args)
	}
	if !slices.Equal(seen, []int{10, 55, 100}) {
		t.Fatalf("progress %v", seen)
	}
	if _, err := d.Diarize(context.Background(), DiarizeRequest{Audio: "a.wav"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "--clustering.cluster-threshold=0.9") || slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "num-clusters") }) {
		t.Fatalf("with no count: args %q", args)
	}
}

func TestADiarizerThatFailsOrHearsNothingIsAnError(t *testing.T) {
	var args []string
	for name, d := range map[string]DiarizeSidecar{
		"exit":     {Path: "/x/p", Run: stubDiarize("", nil, errors.New("exit status 255"), &args)},
		"nothing":  {Path: "/x/p", Run: stubDiarize("Started\n", nil, nil, &args)},
		"bad line": {Path: "/x/p", Run: stubDiarize("9.0 -- 2.0 speaker_00\n", nil, nil, &args)},
		"no path":  {},
	} {
		if _, err := d.Diarize(context.Background(), DiarizeRequest{Audio: "a.wav"}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestTheSpeakerSeparationIsCarriedBesideTheTranscriber(t *testing.T) {
	for _, c := range []struct{ dir, goos, want string }{
		{"/usr/bin", "linux", "/usr/lib/agentry-desktop/diarize/sherpa-onnx-offline-speaker-diarization"},
		{"/home/me/agentry", "linux", "/home/me/agentry/diarize/sherpa-onnx-offline-speaker-diarization"},
		{"/Applications/Agentry Desktop.app/Contents/MacOS", "darwin", "/Applications/Agentry Desktop.app/Contents/Resources/diarize/sherpa-onnx-offline-speaker-diarization"},
		{"/tmp/loose", "darwin", ""},
	} {
		if got := diarizerBeside(c.dir, c.goos); got != c.want {
			t.Errorf("diarizerBeside(%q, %s) = %q, want %q", c.dir, c.goos, got, c.want)
		}
	}
}

func TestAVoiceHeardInTwoSpansOverlapsAWordByTheirSum(t *testing.T) {
	// 0.3 s + 0.3 s of voice 1 outweigh 0.5 s of voice 2.
	words := []Word{w(" word", 0, 1.1)}
	spans := []SpeakerSpan{{sec(0), sec(0.3), 1}, {sec(0.3), sec(0.8), 2}, {sec(0.8), sec(1.1), 1}}
	if got := speakerOf(words, spans); !slices.Equal(got, []int{1}) {
		t.Fatalf("voices %v", got)
	}
}
