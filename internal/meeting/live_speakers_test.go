package meeting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveSpeakers transcribes each recording of testdata/rooms with the
// real transcriber and tells its speakers apart with the real speaker
// separation, with the count of voices given and estimated, and scores the
// words against the reference: the share of words said by one person whose
// voice number is that person's, under the best one-to-one pairing. It
// runs only when AGENTRY_MEETING_LIVE is set, with:
//
//	AGENTRY_MEETING_SIDECAR         the whisper-cli executable
//	AGENTRY_MEETING_MODELS          a folder holding ggml-small.en-q5_1.bin and
//	                                ggml-silero-v6.2.0.bin, as the catalogue pins them
//	AGENTRY_MEETING_DIARIZER        the sherpa-onnx-offline-speaker-diarization program
//	AGENTRY_MEETING_DIARIZE_MODELS  a folder holding the catalogue's two
//	                                diarization models, by their file names
//
// It fails when a run fails, when a given count is exceeded, or when a
// recording's words agree less than half the time with the count given.
func TestLiveSpeakers(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	cat, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	models := &Models{Dir: os.Getenv("AGENTRY_MEETING_MODELS")}
	model, err := models.Verify(cat.Models["small.en-q5_1"])
	if err != nil {
		t.Fatal(err)
	}
	vad, err := models.Verify(cat.VAD)
	if err != nil {
		t.Fatal(err)
	}
	dm := &Models{Dir: os.Getenv("AGENTRY_MEETING_DIARIZE_MODELS")}
	seg, err := dm.Verify(cat.Diarization.Segmentation)
	if err != nil {
		t.Fatal(err)
	}
	emb, err := dm.Verify(cat.Diarization.Embedding)
	if err != nil {
		t.Fatal(err)
	}
	eng := Sidecar{Path: os.Getenv("AGENTRY_MEETING_SIDECAR")}
	dia := DiarizeSidecar{Path: os.Getenv("AGENTRY_MEETING_DIARIZER")}
	ctx := context.Background()
	for _, fx := range roomFixtures {
		t.Run(fx.name, func(t *testing.T) {
			// The transcriber writes beside the audio: a copy in a folder
			// of the test's own.
			raw, err := os.ReadFile(filepath.Join("testdata", "rooms", fx.name+".wav"))
			if err != nil {
				t.Fatal(err)
			}
			audio := filepath.Join(t.TempDir(), fx.name+".wav")
			if err := os.WriteFile(audio, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			ref := readRTTM(t, filepath.Join("testdata", "rooms", fx.name+".rttm"))
			res, err := eng.Transcribe(ctx, TranscribeRequest{Audio: audio, Model: model, VADModel: vad, Language: "en"})
			if err != nil {
				t.Fatal(err)
			}
			words := wordsOf(res)
			for _, given := range []int{fx.voices, 0} {
				spans, err := dia.Diarize(ctx, DiarizeRequest{Audio: audio, Segmentation: seg, Embedding: emb, Speakers: given})
				if err != nil {
					t.Fatal(err)
				}
				lines, voices, ok := SpeakerLines(words, spans)
				if !ok {
					t.Fatal("no word was put to a voice")
				}
				agree, scored := agreement(words, spans, ref)
				how := "estimated"
				if given > 0 {
					how = "given"
				}
				t.Logf("%s, count %s: %d voices of %d, %d lines, %.0f%% of %d single-speaker words agree", fx.name, how, voices, fx.voices, len(lines), 100*agree, scored)
				for _, l := range lines[:min(3, len(lines))] {
					t.Logf("  [%s] %s: %s", clock(l.Start), l.Label, truncate(l.Text, 70))
				}
				if given > 0 && voices > given {
					t.Errorf("%d voices told apart, more than the %d given", voices, given)
				}
				if given > 0 && agree < 0.5 {
					t.Errorf("only %.0f%% of words agree with the reference", 100*agree)
				}
			}
		})
	}
}

// agreement scores the words against the reference: of the words said
// while exactly one person spoke, the share whose voice (by speakerOf) is
// paired with that person, under the best one-to-one pairing.
func agreement(words []Word, spans []SpeakerSpan, ref []rttmSpan) (share float64, scored int) {
	ids := speakerOf(words, spans)
	count := map[[2]string]int{}
	for i, w := range words {
		mid := (w.Start + w.End).Seconds() / 2
		var who []string
		for _, r := range ref {
			if r.start <= mid && mid < r.end && !containsString(who, r.speaker) {
				who = append(who, r.speaker)
			}
		}
		if len(who) != 1 || ids[i] < 0 {
			continue
		}
		p := [2]string{fmt.Sprint(ids[i]), who[0]}
		count[p]++
		scored++
	}
	// Greedy pairing, the most shared first: enough for a handful of voices.
	used := map[string]bool{}
	agree := 0
	for len(count) > 0 {
		var best [2]string
		n := -1
		for p, c := range count {
			if c > n || (c == n && p[0]+p[1] < best[0]+best[1]) {
				best, n = p, c
			}
		}
		if !used["h"+best[0]] && !used["r"+best[1]] {
			used["h"+best[0]], used["r"+best[1]] = true, true
			agree += n
		}
		delete(count, best)
	}
	if scored == 0 {
		return 0, 0
	}
	return float64(agree) / float64(scored), scored
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}
