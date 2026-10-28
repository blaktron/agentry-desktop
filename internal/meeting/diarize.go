package meeting

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Telling speakers apart: a meeting in the room is one microphone, so the
// voices on it are told apart after transcription by clustering them, and
// each word takes the voice heard at its time. The rest of the feature
// never names the engine that does it.

// SpeakerSpan is a stretch of the audio where one voice speaks, by the
// diarizer's own number for it. Spans of different voices may overlap.
type SpeakerSpan struct {
	Start, End time.Duration
	Speaker    int
}

// DiarizeRequest is one track whose voices to tell apart.
type DiarizeRequest struct {
	Audio        string // a 16 kHz mono WAV
	Segmentation string // the speech segmentation model file
	Embedding    string // the speaker embedding model file
	// Speakers is how many people were in the room, 0 to estimate it.
	Speakers int
	// Progress, when set, is told how far along it is, 0 to 100.
	Progress func(percent int)
}

// Diarizer tells the voices of one track apart.
type Diarizer interface {
	Diarize(ctx context.Context, req DiarizeRequest) ([]SpeakerSpan, error)
}

// How telling the speakers of a meeting in the room apart went, as the
// meeting and its transcript say.
const (
	// DiarizationDone: each line is a Speaker n.
	DiarizationDone = "done"
	// DiarizationOff: the person turned it off; the lines are Room.
	DiarizationOff = "off"
	// DiarizationUnavailable: this copy carries no speaker separation, or
	// its models could not be had; the lines are Room.
	DiarizationUnavailable = "unavailable"
	// DiarizationFailed: it ran and failed, or timed out; the lines are
	// Room.
	DiarizationFailed = "failed"
)

// DefaultDiarizeTimeout is how long a diarization may run beyond the
// audio's own length: it takes about a seventh of it on a desktop CPU.
const DefaultDiarizeTimeout = 15 * time.Minute

// The count of people in the room a person may give.
const (
	MinPeople = 2
	MaxPeople = 20
)

// CheckPeople refuses a count of people in the room out of range; 0 leaves
// it to be estimated.
func CheckPeople(n int) error {
	if n != 0 && (n < MinPeople || n > MaxPeople) {
		return fmt.Errorf("the number of people in the room is %d to %d, or left empty", MinPeople, MaxPeople)
	}
	return nil
}

// clusterThreshold is how far apart two voices must sound to be counted
// as two when the count is estimated: measured on recordings of meetings
// in a room with two to five voices, it gave the fewest errors.
const clusterThreshold = 0.9

// DiarizeSidecar is sherpa-onnx's offline speaker diarization program, run
// once per meeting in the room. It answers one line per stretch of speech,
// "start -- end speaker_NN", on its standard output, and its progress on
// its standard error.
type DiarizeSidecar struct {
	// Path is the program.
	Path string
	// Threads is how many CPU threads it uses; 0 picks from the CPU.
	Threads int
	// Run, when set, replaces starting the process: a test's stand-in. It
	// writes the program's output to stdout and its progress to stderr.
	Run func(ctx context.Context, path string, args []string, stdout, stderr io.Writer) error
}

var (
	spanLine         = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?) -- ([0-9]+(?:\.[0-9]+)?) speaker_([0-9]+)\s*$`)
	diarizeProgress  = regexp.MustCompile(`progress ([0-9]+(?:\.[0-9]+)?)%`)
	errNoDiarization = errors.New("the speaker separation is not installed with this copy of Agentry")
)

// Diarize runs the program on one track.
func (d DiarizeSidecar) Diarize(ctx context.Context, req DiarizeRequest) ([]SpeakerSpan, error) {
	if d.Path == "" {
		return nil, errNoDiarization
	}
	threads := d.Threads
	if threads <= 0 {
		threads = defaultThreads()
	}
	args := []string{
		"--segmentation.pyannote-model=" + req.Segmentation,
		"--embedding.model=" + req.Embedding,
		fmt.Sprintf("--segmentation.num-threads=%d", threads),
		fmt.Sprintf("--embedding.num-threads=%d", threads),
	}
	if req.Speakers > 0 {
		args = append(args, fmt.Sprintf("--clustering.num-clusters=%d", req.Speakers))
	} else {
		args = append(args, fmt.Sprintf("--clustering.cluster-threshold=%g", clusterThreshold))
	}
	args = append(args, req.Audio)

	var stdout bytes.Buffer
	pr, pw := io.Pipe()
	var tail []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 4096), 1<<20)
		// The program rewrites its progress on one line with carriage
		// returns.
		sc.Split(splitLinesOrReturns)
		for sc.Scan() {
			line := sc.Text()
			if m := diarizeProgress.FindStringSubmatch(line); m != nil && req.Progress != nil {
				if p, err := strconv.ParseFloat(m[1], 64); err == nil {
					req.Progress(min(max(int(p), 0), 100))
				}
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			tail = append(tail, line)
			if len(tail) > 6 {
				tail = tail[1:]
			}
		}
		_, _ = io.Copy(io.Discard, pr)
	}()
	run := d.Run
	if run == nil {
		run = startDiarize
	}
	err := run(ctx, d.Path, args, &stdout, pw)
	_ = pw.Close()
	<-done
	if err != nil {
		return nil, fmt.Errorf("the speaker separation failed: %w (%s)", err, strings.Join(tail, " | "))
	}
	return parseSpans(stdout.Bytes())
}

// parseSpans reads the program's answer. Lines that are not a span (the
// program also prints its configuration and a "Started" line) are skipped.
func parseSpans(out []byte) ([]SpeakerSpan, error) {
	var spans []SpeakerSpan
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		m := spanLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		start, err1 := strconv.ParseFloat(m[1], 64)
		end, err2 := strconv.ParseFloat(m[2], 64)
		n, err3 := strconv.Atoi(m[3])
		if err1 != nil || err2 != nil || err3 != nil || end < start {
			return nil, fmt.Errorf("the speaker separation answered a line it should not: %q", sc.Text())
		}
		spans = append(spans, SpeakerSpan{
			Start:   time.Duration(start * float64(time.Second)),
			End:     time.Duration(end * float64(time.Second)),
			Speaker: n,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("the speaker separation's answer could not be read: %w", err)
	}
	if len(spans) == 0 {
		return nil, errors.New("the speaker separation heard no speech")
	}
	return spans, nil
}

func splitLinesOrReturns(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func startDiarize(ctx context.Context, path string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // the program this copy of Agentry carries, with arguments built here
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = filepath.Dir(path)
	hideConsole(cmd)
	return cmd.Run()
}

// SpeakerLabel is the label of the nth voice, numbered from 1 by who spoke
// first.
func SpeakerLabel(n int) Label { return Label("Speaker " + strconv.Itoa(n)) }

// speakerOf is the voice each word was said in: the one whose spans
// overlap the word most; a word no span overlaps takes the voice of the
// word before it (the first such words, of the word after them). -1 when
// no word overlaps any span.
func speakerOf(words []Word, spans []SpeakerSpan) []int {
	out := make([]int, len(words))
	for i, w := range words {
		out[i] = -1
		// A voice's overlap is summed over its spans.
		heard := map[int]time.Duration{}
		for _, s := range spans {
			if o := min(w.End, s.End) - max(w.Start, s.Start); o > 0 {
				heard[s.Speaker] += o
			}
		}
		best := time.Duration(0)
		for v, o := range heard {
			if o > best || (o == best && v < out[i]) {
				best, out[i] = o, v
			}
		}
	}
	prev := -1
	for i := range out {
		if out[i] < 0 {
			out[i] = prev
		} else {
			prev = out[i]
		}
	}
	next := -1
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 0 {
			out[i] = next
		} else {
			next = out[i]
		}
	}
	return out
}

// SpeakerLines are the room's lines with the voice that said them: each
// word takes its voice (speakerOf), the voices are numbered by who spoke
// first, and consecutive words of one voice make a line, broken at the end
// of a sentence as the room's lines are, then joined into turns. It also
// says how many voices it numbered. ok is false when no word could be put
// to a voice.
func SpeakerLines(words []Word, spans []SpeakerSpan) (lines []Segment, voices int, ok bool) {
	ids := speakerOf(words, spans)
	if len(ids) == 0 || ids[0] < 0 {
		return nil, 0, false
	}
	var raw []int // each line's voice, by the diarizer's number
	var line []word
	cur := -1
	flush := func() {
		if seg, ok := lineOf(line); ok {
			lines = append(lines, seg)
			raw = append(raw, cur)
		}
		line = line[:0]
	}
	for i, w := range words {
		if ids[i] != cur && len(line) > 0 {
			flush()
		}
		cur = ids[i]
		line = append(line, word{w.Text, w.Start, w.End})
		if endsSentence(w.Text) || len(line) >= lineWords {
			flush()
		}
	}
	flush()
	// Numbered by who spoke first, among the lines kept: a voice whose
	// words were all marks of sound that is not speech is not counted.
	number := map[int]int{}
	for i := range lines {
		if _, seen := number[raw[i]]; !seen {
			number[raw[i]] = len(number) + 1
		}
		lines[i].Label = SpeakerLabel(number[raw[i]])
	}
	return joinTurns(lines), len(number), len(lines) > 0
}

// wordsOf are a track's words, or, from an engine that reports none, each
// segment taken as one word.
func wordsOf(res Transcription) []Word {
	if len(res.Words) > 0 {
		return res.Words
	}
	out := make([]Word, 0, len(res.Segments))
	for _, s := range res.Segments {
		out = append(out, Word{Text: " " + s.Text, Start: s.Start, End: s.End})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
