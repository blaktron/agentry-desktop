package meeting

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// TranscriptFormat names the transcript's layout, for whoever reads it.
const TranscriptFormat = "agentry-meeting-transcript/1"

// echoWindow and echoSimilarity decide when a line on the microphone is the
// far side heard again through the speakers: said within echoWindow of a
// line of the far side's, with at least echoSimilarity of its words shared.
// The far side's line is kept, the microphone's dropped.
const (
	echoWindow     = 3 * time.Second
	echoSimilarity = 0.8
)

// Lines of one side said within joinGap of each other are joined into one
// turn while it stays under joinChars characters.
const (
	joinGap   = 2 * time.Second
	joinChars = 240
)

// Merge puts both sides' segments on one timeline: echoes of the far side
// on the microphone dropped, then consecutive lines of one side joined
// into turns.
func Merge(me, them []Segment) []Segment {
	var all []Segment
	for _, s := range me {
		if !echoOf(s, them) {
			s.Label = Me
			all = append(all, s)
		}
	}
	for _, s := range them {
		s.Label = Them
		all = append(all, s)
	}
	return joinTurns(all)
}

// MergeRoom is the one track of a meeting in the room as turns: its
// segments labelled Room and consecutive lines joined.
func MergeRoom(room []Segment) []Segment {
	all := make([]Segment, 0, len(room))
	for _, s := range room {
		s.Label = Room
		all = append(all, s)
	}
	return joinTurns(all)
}

// joinTurns puts segments in time order and joins consecutive lines of one
// label into turns.
func joinTurns(all []Segment) []Segment {
	sort.SliceStable(all, func(i, j int) bool { return all[i].Start < all[j].Start })
	var out []Segment
	for _, s := range all {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.Label == s.Label && s.Start-last.End <= joinGap && len(last.Text)+1+len(s.Text) <= joinChars {
				last.Text += " " + s.Text
				last.End = max(last.End, s.End)
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

func echoOf(s Segment, them []Segment) bool {
	w := words(s.Text)
	if len(w) == 0 {
		return false
	}
	for _, t := range them {
		if t.Start > s.End+echoWindow || t.End < s.Start-echoWindow {
			continue
		}
		if similarity(w, words(t.Text)) >= echoSimilarity {
			return true
		}
	}
	return false
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// similarity is the share of a's words that b also has.
func similarity(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	have := map[string]int{}
	for _, w := range b {
		have[w]++
	}
	shared := 0
	for _, w := range a {
		if have[w] > 0 {
			have[w]--
			shared++
		}
	}
	return float64(shared) / float64(len(a))
}

// Gap is a stretch of the recording with nothing on a side: a pause, or a
// stream that was missing, so absence is not read as silence.
type Gap struct {
	From   time.Duration
	To     time.Duration
	Reason string
}

// MicChangeRecord is a MicChange on the recording's timeline: At is from
// the start of the recording.
type MicChangeRecord struct {
	At   time.Duration `json:"at"`
	Lost string        `json:"lost,omitempty"`
	Now  string        `json:"now,omitempty"`
}

// micGapReason is the reason of a gap with no microphone read.
const micGapReason = "microphone disconnected"

// micGaps are the stretches with no microphone read: from a change that
// leaves none to the next that reads one, or to end.
func micGaps(changes []MicChangeRecord, end time.Duration) []Gap {
	var out []Gap
	open := false
	var from time.Duration
	for _, c := range changes {
		switch {
		case c.Now == "" && !open:
			open, from = true, c.At
		case c.Now != "" && open:
			out = append(out, Gap{From: from, To: c.At, Reason: micGapReason})
			open = false
		}
	}
	if open {
		out = append(out, Gap{From: from, To: end, Reason: micGapReason})
	}
	return out
}

// TranscriptInfo is what the transcript's frontmatter says about the
// meeting.
type TranscriptInfo struct {
	Title    string
	Started  time.Time
	Ended    time.Time
	App      string
	FarSide  FarSide
	Language string
	Profile  string
	Model    string
	Gaps     []Gap
	// SilentSides are the sides with nothing to transcribe.
	SilentSides []Label
	// FromCalendar: the title and Participants are the calendar's, for a
	// recording an armed window made; Participants are who was invited.
	FromCalendar bool
	Participants []string
	// InRoom: a meeting held in the room, recorded through the microphone
	// alone. Microphone names the microphone where the recorder said, and
	// MicMissing that the one chosen was missing so the default was
	// recorded.
	InRoom     bool
	Microphone string
	MicMissing bool
	// MicChanges are the microphone's changes during the recording, said
	// one sentence each.
	MicChanges []MicChangeRecord
	// Diarization is how telling a room's speakers apart went (a
	// Diarization constant, "" for a call), Speakers how many voices it
	// told apart, People the count the person gave (0 when it was
	// estimated), and DiarizationError why it failed or could not run.
	Diarization      string
	Speakers         int
	People           int
	DiarizationError string
}

type frontSpeaker struct {
	Label  string `yaml:"label"`
	Source string `yaml:"source"`
	Note   string `yaml:"note"`
}

type frontGap struct {
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Reason string `yaml:"reason"`
}

type frontmatter struct {
	Type            string         `yaml:"type"`
	Format          string         `yaml:"format"`
	Title           string         `yaml:"title"`
	Started         string         `yaml:"started"`
	Ended           string         `yaml:"ended"`
	DurationSeconds int64          `yaml:"duration_seconds"`
	Timezone        string         `yaml:"timezone"`
	App             string         `yaml:"app"`
	AppDetection    string         `yaml:"app_detection"`
	Participants    any            `yaml:"participants"`
	TitleSource     string         `yaml:"title_source"`
	Speakers        []frontSpeaker `yaml:"speakers"`
	Language        string         `yaml:"language"`
	LanguageProfile string         `yaml:"language_profile"`
	Model           string         `yaml:"model"`
	Transcriber     string         `yaml:"transcriber"`
	FarSide         string         `yaml:"far_side"`
	Echo            string         `yaml:"echo"`
	OutputDevice    string         `yaml:"output_device"`
	Setting         string         `yaml:"setting,omitempty"`
	Diarization     string         `yaml:"diarization,omitempty"`
	SpeakerCount    string         `yaml:"speaker_count,omitempty"`
	Diarizer        string         `yaml:"diarizer,omitempty"`
	Microphone      string         `yaml:"microphone,omitempty"`
	Silent          []string       `yaml:"silent,omitempty"`
	Gaps            []frontGap     `yaml:"gaps,omitempty"`
}

// DiarizerName names the speaker separation and its models in a transcript
// whose speakers it told apart.
const DiarizerName = "sherpa-onnx v2.0.1, segmentation pyannote 3.0, voice embedding NeMo TitaNet small"

// Transcriber names the engine in the transcript.
const Transcriber = "whisper.cpp v1.9.5, voice detection silero-v6.2.0"

// Render is the transcript as Markdown: plain YAML frontmatter, a heading,
// what the labels mean, and one line per turn, "[hh:mm:ss] Me: text".
func Render(info TranscriptInfo, lines []Segment) ([]byte, error) {
	app := info.App
	detection := "audio-stack"
	if app == "" {
		app, detection = "unknown", "none"
	}
	farNote := "everyone heard through the meeting app; not split by person"
	switch info.FarSide {
	case FarSideWholeDevice:
		farNote = "everything the computer played, the meeting included; not split by person"
	case FarSideAppDevice:
		farNote = "everything played on the speakers or headset the meeting app used, the meeting included; not split by person"
	}
	fm := frontmatter{
		Type: "meeting-transcript", Format: TranscriptFormat,
		Title:           info.Title,
		Started:         info.Started.Format(time.RFC3339),
		Ended:           info.Ended.Format(time.RFC3339),
		DurationSeconds: int64(info.Ended.Sub(info.Started).Seconds()),
		Timezone:        zoneName(info.Started),
		App:             app, AppDetection: detection,
		Participants: "unknown", TitleSource: "recording",
		Speakers: []frontSpeaker{
			{Label: string(Me), Source: "microphone", Note: "the person who owns this computer"},
			{Label: string(Them), Source: "system-audio", Note: farNote},
		},
		Language: info.Language, LanguageProfile: info.Profile, Model: info.Model,
		Transcriber: Transcriber, FarSide: string(info.FarSide),
		Echo: "dedupe-text", OutputDevice: "unknown",
	}
	if info.InRoom {
		fm.Setting = "in-room"
		fm.Speakers = []frontSpeaker{{Label: string(Room), Source: "microphone", Note: "everyone in the room; not split by person"}}
		fm.App, fm.Echo, fm.OutputDevice = "none", "none", "none"
		fm.Diarization = info.Diarization
		if info.Diarization == DiarizationDone {
			fm.Speakers = nil
			for n := 1; n <= info.Speakers; n++ {
				fm.Speakers = append(fm.Speakers, frontSpeaker{Label: string(SpeakerLabel(n)), Source: "microphone", Note: speakerNote})
			}
			fm.SpeakerCount, fm.Diarizer = "estimated", DiarizerName
			if info.People > 0 {
				fm.SpeakerCount = "given"
			}
		}
	}
	fm.Microphone = info.Microphone
	if info.MicMissing {
		fm.Microphone = "default (the one chosen was missing)"
		if info.Microphone != "" {
			fm.Microphone = info.Microphone + " (default; the one chosen was missing)"
		}
	}
	if info.FromCalendar {
		fm.TitleSource = "calendar"
		if len(info.Participants) > 0 {
			fm.Participants = info.Participants
		}
	}
	for _, s := range info.SilentSides {
		fm.Silent = append(fm.Silent, string(s))
	}
	for _, g := range info.Gaps {
		fm.Gaps = append(fm.Gaps, frontGap{From: clock(g.From), To: clock(g.To), Reason: g.Reason})
	}
	head, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(head)
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", oneLine(info.Title))
	switch {
	case info.InRoom && info.Diarization == DiarizationDone:
		b.WriteString("Transcript of a meeting in the room, produced automatically on this computer from one microphone. " + speakersSentence(info) + " Words may be wrong. Times are from the start of the recording.\n")
	case info.InRoom:
		b.WriteString("Transcript of a meeting in the room, produced automatically on this computer from one microphone. \"Room\" is everyone in the room, not split by person. Words may be wrong. Times are from the start of the recording.\n")
		if s := roomOnlySentence(info); s != "" {
			b.WriteString("\n" + s + "\n")
		}
	default:
		b.WriteString("Transcript produced automatically on this computer. Speaker labels come from the audio source, not from voice recognition: \"Them\" may be several people. Words may be wrong. Times are from the start of the recording.\n")
	}
	if info.MicMissing {
		b.WriteString("\nThe microphone chosen in the settings was not found, so the default microphone was recorded.\n")
	}
	if s := micChangeSentences(info.MicChanges); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	switch info.FarSide {
	case FarSideWholeDevice:
		b.WriteString("\nNo meeting app was found to record on its own, so \"Them\" is everything the computer played, other sounds included.\n")
	case FarSideAppDevice:
		b.WriteString("\n\"Them\" is everything played on the speakers or headset the meeting app used, so other sounds played there are in it too.\n")
	}
	b.WriteString("\n## Transcript\n\n")
	if len(lines) == 0 {
		b.WriteString("Nothing was said that could be transcribed.\n")
	}
	for _, l := range lines {
		fmt.Fprintf(&b, "[%s] %s: %s\n", clock(l.Start), l.Label, oneLine(l.Text))
	}
	return []byte(b.String()), nil
}

// speakerNote is what every Speaker n stands for, in the frontmatter.
const speakerNote = "one voice as the speaker clustering heard it, numbered by who spoke first; not an identity, and may be two people or part of one"

// speakersSentence says what the Speaker labels are, and how their number
// was had.
func speakersSentence(info TranscriptInfo) string {
	s := fmt.Sprintf("Speakers are told apart by clustering their voices: \"Speaker 1\", \"Speaker 2\" and so on are %s, numbered by who spoke first, the same number for the same voice within this transcript. They are not identities: two people may share a number, or one person appear under two.", voicesWords(info.Speakers))
	if info.People > 0 {
		return s + fmt.Sprintf(" The person recording said %d people were in the room, and the voices were split into at most that many; someone who said little may not appear.", info.People)
	}
	return s + " How many people were in the room was estimated, so one person may appear under several numbers."
}

func voicesWords(n int) string {
	if n == 1 {
		return "the one voice heard"
	}
	return fmt.Sprintf("the %d voices heard", n)
}

// roomOnlySentence says why a meeting in the room's lines are all Room,
// "" for a transcript that predates telling speakers apart.
func roomOnlySentence(info TranscriptInfo) string {
	why := oneLine(info.DiarizationError)
	switch info.Diarization {
	case DiarizationOff:
		return "Telling speakers apart is turned off in the settings, so every line is Room."
	case DiarizationUnavailable:
		return "The speakers could not be told apart (" + why + "), so every line is Room."
	case DiarizationFailed:
		return "Telling the speakers apart failed (" + why + "), so every line is Room."
	}
	return ""
}

// micChangeSentences says what happened to the microphone during the
// recording, one sentence per change, "" when nothing did.
func micChangeSentences(changes []MicChangeRecord) string {
	var out []string
	for _, c := range changes {
		at := "At [" + clock(c.At) + "] "
		switch {
		case c.Lost != "" && c.Now != "":
			out = append(out, at+oneLine(c.Lost)+" was disconnected, and "+oneLine(c.Now)+" was recorded instead.")
		case c.Lost != "":
			out = append(out, at+oneLine(c.Lost)+" was disconnected, and no other microphone could be read: nothing was recorded from a microphone until one could be (see gaps).")
		case c.Now != "":
			out = append(out, at+oneLine(c.Now)+" was recorded from then on.")
		}
	}
	return strings.Join(out, " ")
}

// zoneName is the time zone's IANA name where the system says it ("Local"
// names nothing), else its offset from UTC.
func zoneName(t time.Time) string {
	if n := t.Location().String(); n != "Local" && n != "" {
		return n
	}
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" && !strings.HasPrefix(tz, "/") {
		return tz
	}
	if p, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(p, "zoneinfo/"); ok && name != "" {
			return name
		}
	}
	return "UTC" + t.Format("-07:00")
}

func clock(d time.Duration) string {
	s := int64(d / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Slug is a file-name part from s: lower-case ASCII letters and digits
// joined by hyphens, at most 40 characters; "meeting" when nothing is left.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			if b.Len() >= 40 {
				break
			}
			continue
		}
		dash = true
	}
	if b.Len() == 0 {
		return "meeting"
	}
	return b.String()
}

// TranscriptName is the transcript's file name: the local date and start
// time first, so names sort, then the slug.
func TranscriptName(started time.Time, title string) string {
	return started.Format("2006-01-02-1504") + "-" + Slug(title) + ".md"
}

// WriteTranscript puts content into dir under name, or under name with
// "-2", "-3"… when a file of that name is there. The file is written in
// full under a temporary name in the same folder and then linked into
// place, so a reader never sees half a transcript and an existing file is
// never written over. It returns the path written.
func WriteTranscript(dir, name string, content []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return writeVia(dir, dir, name, content, 0o600)
}

// CopyIntoShare puts a transcript into dir, a folder in the Assistant's
// share, as WriteTranscript does, but never writes there: the copy is
// written whole in the first of stages (folders outside the share) on the
// share's filesystem, then linked into place, so a walk of the share never
// sees half a file and nothing in it is ever written over or changed.
func CopyIntoShare(dir string, stages []string, name string, content []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var errs []error
	for _, stage := range stages {
		if err := os.MkdirAll(stage, 0o700); err != nil {
			errs = append(errs, err)
			continue
		}
		// Readable by the agent's tools, as a file the person adds is.
		out, err := writeVia(stage, dir, name, content, 0o644)
		if err == nil {
			return out, nil
		}
		errs = append(errs, err)
	}
	return "", fmt.Errorf("the transcript could not be put in the share folder: %w", errors.Join(errs...))
}

// writeVia writes content under a temporary name in stage, with perm,
// then links it into dir under name, or name with a number, never over a
// file.
func writeVia(stage, dir, name string, content []byte, perm os.FileMode) (string, error) {
	tmp, err := tempFile(stage)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := writeSynced(tmp, content); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(name, ".md")
	for i := 1; i <= 100; i++ {
		final := filepath.Join(dir, name)
		if i > 1 {
			final = filepath.Join(dir, fmt.Sprintf("%s-%d.md", base, i))
		}
		err := linkNoClobber(tmp, final)
		if err == nil {
			return final, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free name for %s in %s", name, dir)
}

// linkNoClobber gives tmp the name final unless final exists. A hard link
// does that in one step; a filesystem with none (FAT, some network
// shares) gets a rename after a check, with the small race that leaves.
func linkNoClobber(tmp, final string) error {
	err := os.Link(tmp, final)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	if _, serr := os.Lstat(final); serr == nil {
		return fs.ErrExist
	}
	return os.Rename(tmp, final)
}

func tempFile(dir string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return filepath.Join(dir, ".transcript-"+hex.EncodeToString(b)+".tmp"), nil
}

func writeSynced(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a temporary name of our own in the output folder
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
