package meeting

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Sidecar is the whisper.cpp command-line transcriber, run once per track.
// Every run has voice detection on, so silence is skipped rather than
// turned into words, and keeps no text context between windows (-mc 0), so
// a phrase said twice is transcribed twice rather than skipped as a repeat.
// It reports each word as a segment of its own (-ml 1 -sow): with voice
// detection on, only segments are timed on the recording's own timeline,
// so a line's time is its first word's.
// A run that fails is tried once more on the CPU: a graphics backend that
// loads and then fails takes the run down with it.
type Sidecar struct {
	// Path is the whisper-cli executable.
	Path string
	// Threads is how many CPU threads a run uses; 0 picks from the CPU.
	Threads int
	// Run, when set, replaces starting the process: a test's stand-in.
	Run func(ctx context.Context, path string, args []string, stderr io.Writer) error
}

// Threads is the default thread count: half the logical CPUs, from 1 to 8.
// A call goes on while a transcription runs, so it does not take them all.
func defaultThreads() int {
	return min(max(runtime.NumCPU()/2, 1), 8)
}

// Transcribe runs the sidecar on one track.
func (s Sidecar) Transcribe(ctx context.Context, req TranscribeRequest) (Transcription, error) {
	if s.Path == "" {
		return Transcription{}, errors.New("the transcriber is not installed with this copy of Agentry")
	}
	// The result is written beside the audio, in the recording's own
	// folder, never in a shared temporary folder.
	out, err := os.MkdirTemp(filepath.Dir(req.Audio), ".transcribe-")
	if err != nil {
		return Transcription{}, err
	}
	defer func() { _ = os.RemoveAll(out) }()
	threads := s.Threads
	if threads <= 0 {
		threads = defaultThreads()
	}
	lang := req.Language
	if lang == "" {
		lang = "auto"
	}
	args := []string{
		"-m", req.Model, "-f", req.Audio, "-l", lang,
		"--vad", "-vm", req.VADModel,
		"-mc", "0",
		"-ml", "1", "-sow",
		"-oj", "-of", filepath.Join(out, "out"),
		"-pp", "-t", strconv.Itoa(threads),
	}
	took, err := s.run(ctx, args, req.Progress)
	if err != nil && ctx.Err() == nil {
		took, err = s.run(ctx, append(args, "-ng"), req.Progress)
	}
	if err != nil {
		return Transcription{}, err
	}
	raw, err := os.ReadFile(filepath.Join(out, "out.json")) //nolint:gosec // the sidecar's output in our own temp folder
	if err != nil {
		return Transcription{}, fmt.Errorf("the transcriber wrote no result: %w", err)
	}
	t, err := parseWhisperJSON(raw)
	if err != nil {
		return Transcription{}, err
	}
	t.Took = took
	return t, nil
}

var (
	progressLine = regexp.MustCompile(`progress =\s*(\d+)%`)
	timingLine   = regexp.MustCompile(`whisper_print_timings:\s+(load|total) time =\s*([0-9.]+) ms`)
)

// run starts the sidecar once and reads its progress and timings from its
// standard error. It returns the time the run took, its model load left
// out, by the sidecar's own timings when it printed them.
func (s Sidecar) run(ctx context.Context, args []string, progress func(int)) (time.Duration, error) {
	pr, pw := io.Pipe()
	var load, total float64
	var tail []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 4096), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			if m := progressLine.FindStringSubmatch(line); m != nil && progress != nil {
				if p, err := strconv.Atoi(m[1]); err == nil {
					progress(p)
				}
			}
			if m := timingLine.FindStringSubmatch(line); m != nil {
				v, _ := strconv.ParseFloat(m[2], 64)
				if m[1] == "load" {
					load = v
				} else {
					total = v
				}
			}
			tail = append(tail, line)
			if len(tail) > 8 {
				tail = tail[1:]
			}
		}
		_, _ = io.Copy(io.Discard, pr)
	}()
	began := time.Now()
	run := s.Run
	if run == nil {
		run = startSidecar
	}
	err := run(ctx, s.Path, args, pw)
	_ = pw.Close()
	<-done
	if err != nil {
		return 0, fmt.Errorf("the transcriber failed: %w (%s)", err, strings.Join(tail, " | "))
	}
	if total > 0 {
		return time.Duration((total - load) * float64(time.Millisecond)), nil
	}
	return time.Since(began), nil
}

func startSidecar(ctx context.Context, path string, args []string, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // the sidecar this copy of Agentry carries, with arguments built here
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	cmd.Dir = filepath.Dir(path)
	hideConsole(cmd)
	return cmd.Run()
}

// whisperJSON is the part of whisper-cli's --output-json the transcript
// needs: with -ml 1, one segment per word.
type whisperJSON struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

// A line ends at the end of a sentence or at lineWords words. A word's
// end is taken as at most maxWord after its start: the last word before a
// silence is reported as lasting through it.
//
// The first word after a silence is often timed at the end of the speech
// before it, a second or more early, while the word after it is on time.
// So a line whose second word comes more than leadJump after its first
// starts wordLead before its second word: a pause that long after a
// line's first word is rarer than a first word timed early.
const (
	lineWords = 60
	maxWord   = 1500 * time.Millisecond
	leadJump  = 700 * time.Millisecond
	wordLead  = 300 * time.Millisecond
)

type word struct {
	text     string
	from, to time.Duration
}

// parseWhisperJSON turns the sidecar's words into lines of a sentence
// each, timed by when they were said.
func parseWhisperJSON(raw []byte) (Transcription, error) {
	var w whisperJSON
	if err := json.Unmarshal(raw, &w); err != nil {
		return Transcription{}, fmt.Errorf("the transcriber's result could not be read: %w", err)
	}
	t := Transcription{Language: w.Result.Language}
	var line []word
	flush := func() {
		if seg, ok := lineOf(line); ok {
			t.Segments = append(t.Segments, seg)
		}
		line = line[:0]
	}
	for _, seg := range w.Transcription {
		if strings.TrimSpace(seg.Text) == "" {
			continue
		}
		from := time.Duration(seg.Offsets.From) * time.Millisecond
		to := max(min(time.Duration(seg.Offsets.To)*time.Millisecond, from+maxWord), from)
		line = append(line, word{seg.Text, from, to})
		if endsSentence(seg.Text) || len(line) >= lineWords {
			flush()
		}
	}
	flush()
	return t, nil
}

// lineOf is one line from its words.
func lineOf(ws []word) (Segment, bool) {
	var b strings.Builder
	for _, w := range ws {
		b.WriteString(w.text)
	}
	text := cleanText(b.String())
	if text == "" {
		return Segment{}, false
	}
	start := ws[0].from
	if len(ws) > 1 && ws[1].from-ws[0].from > leadJump {
		start = ws[1].from - wordLead
	}
	end := start
	for _, w := range ws {
		end = max(end, w.to)
	}
	return Segment{Start: start, End: end, Text: text}, true
}

func endsSentence(tok string) bool {
	tok = strings.TrimSpace(tok)
	return strings.HasSuffix(tok, ".") || strings.HasSuffix(tok, "?") || strings.HasSuffix(tok, "!")
}

// Whisper marks sound that is not speech in brackets ("[BLANK_AUDIO]",
// "[ Silence ]"), and a line that is nothing but a parenthesis ("(music)")
// is such a mark too.
var (
	bracketed = regexp.MustCompile(`\[[^\]]*\]`)
	nonSpeech = regexp.MustCompile(`^\([^)]*\)$`)
)

// cleanText is a line as the transcript keeps it: one line, valid UTF-8,
// without the marks Whisper writes for sound that is not speech.
func cleanText(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = bracketed.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	if nonSpeech.MatchString(s) {
		return ""
	}
	return s
}
