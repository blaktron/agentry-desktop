package meeting

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestOuterAppIsTheOutermostBundle(t *testing.T) {
	for _, tc := range []struct{ path, key, name string }{
		{"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/131.0/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper", "google chrome", "Google Chrome"},
		{"/Applications/zoom.us.app/Contents/Frameworks/ZoomAudioDevice.driver/x", "zoom.us", "zoom.us"},
		{"/System/Applications/FaceTime.app/Contents/MacOS/FaceTime", "facetime", "FaceTime"},
		{"/usr/local/bin/sox", "sox", "sox"},
		{"", "", ""},
	} {
		key, name := outerApp(tc.path)
		if key != tc.key || name != tc.name {
			t.Errorf("outerApp(%q) = %q, %q; want %q, %q", tc.path, key, name, tc.key, tc.name)
		}
	}
}

const (
	chromeApp    = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	chromeHelper = "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper"
	musicApp     = "/System/Applications/Music.app/Contents/MacOS/Music"
)

func TestABrowserCallInItsHelpersIsOneCallOfTheBrowser(t *testing.T) {
	ps := []caProcess{
		{Object: 7, PID: 100, Path: chromeApp},
		{Object: 9, PID: 120, Input: true, Output: true, Path: chromeHelper},
		{Object: 11, PID: 300, Output: true, Path: musicApp},
	}
	calls := findCalls(processSnapshot(ps), "999")
	if len(calls) != 1 || calls[0].App != "Google Chrome" || calls[0].AppKey() != "google chrome" || calls[0].PID != "120" {
		t.Fatalf("calls = %+v, want Google Chrome's alone", calls)
	}
	if !BrowserApp(calls[0].AppKey()) {
		t.Fatal("a Chrome call is not a browser's")
	}
	if got := appObjects(ps, "google chrome"); len(got) != 2 || got[0] != 7 || got[1] != 9 {
		t.Fatalf("the app's objects = %v, want the browser's and its helper's", got)
	}
	if !appInput(ps, "google chrome") || appInput(ps, "music") || !appOutput(ps, "music") {
		t.Fatal("which app records and which plays was misread")
	}
}

func TestAnAppThatOnlyPlaysOrOnlyRecordsHasNoCall(t *testing.T) {
	ps := []caProcess{
		{Object: 3, PID: 300, Output: true, Path: musicApp},
		{Object: 4, PID: 400, Input: true, Path: "/Applications/Dictation.app/Contents/MacOS/Dictation"},
	}
	if calls := findCalls(processSnapshot(ps), "999"); len(calls) != 0 {
		t.Fatalf("calls = %+v, want none", calls)
	}
}

func TestThisAppsOwnRecordingIsNeverACall(t *testing.T) {
	ps := []caProcess{{Object: 5, PID: 999, Input: true, Output: true, Path: "/Applications/Agentry Desktop.app/Contents/MacOS/agentry-desktop"}}
	if calls := findCalls(processSnapshot(ps), "999"); len(calls) != 0 {
		t.Fatalf("calls = %+v, want none", calls)
	}
}

func TestACallInTheProcessListStartsAndEndsAfterTheWait(t *testing.T) {
	tr := newCallTracker("999")
	t0 := time.Unix(1000, 0)
	on := []caProcess{{Object: 9, PID: 120, Input: true, Output: true, Path: chromeHelper}}
	off := []caProcess{{Object: 9, PID: 120, Output: true, Path: chromeHelper}}
	if ev := tr.update(processSnapshot(on), t0); len(ev) != 1 || ev[0].Ended {
		t.Fatalf("the call's start = %+v", ev)
	}
	if ev := tr.update(processSnapshot(off), t0.Add(time.Second)); len(ev) != 0 {
		t.Fatalf("the call ended as soon as the mic was let go: %+v", ev)
	}
	if ev := tr.update(processSnapshot(off), t0.Add(time.Second+callEndWait)); len(ev) != 1 || !ev[0].Ended {
		t.Fatalf("the call's end = %+v", ev)
	}
}

func TestTheResamplerKeepsTimeAndLevelAcrossCalls(t *testing.T) {
	r := newResampler(48000)
	var out []int16
	phase := 0.0
	for range 100 { // one second in 10 ms pieces
		in := make([]float32, 480)
		for i := range in {
			in[i] = float32(0.5 * math.Sin(phase))
			phase += 2 * math.Pi * 440 / 48000
		}
		out = append(out, r.push(in)...)
	}
	if len(out) < SampleRate-2 || len(out) > SampleRate+2 {
		t.Fatalf("one second at 48 kHz gave %d samples, want about %d", len(out), SampleRate)
	}
	var peak int16
	for _, v := range out[SampleRate/10:] {
		peak = max(peak, v)
	}
	if peak < 14000 || peak > 17000 {
		t.Fatalf("a 440 Hz tone at half scale peaked at %d", peak)
	}
	same := newResampler(SampleRate)
	if got := same.push([]float32{0.25, -0.25}); len(got) != 2 || got[0] != toInt16(0.25) {
		t.Fatalf("16 kHz in gave %v", got)
	}
}

func TestSilenceIsWarnedOnlyWhenSomethingShouldHaveBeenHeard(t *testing.T) {
	t0 := time.Unix(1000, 0)
	var s silenceCheck
	for i := 0; i <= 60; i++ {
		if s.note(t0.Add(time.Duration(i)*time.Second), false, false) {
			t.Fatal("silence was warned while nothing played")
		}
	}
	var quiet silenceCheck
	warned := -1
	for i := 0; i <= 40 && warned < 0; i++ {
		if quiet.note(t0.Add(time.Duration(i)*time.Second), false, true) {
			warned = i
		}
	}
	if warned != int(silenceAfter/time.Second) {
		t.Fatalf("silence while the app played was warned after %d s, want %v", warned, silenceAfter)
	}
	var heard silenceCheck
	heard.note(t0, true, true)
	for i := 1; i <= 60; i++ {
		if heard.note(t0.Add(time.Duration(i)*time.Second), false, true) {
			t.Fatal("a side that was heard once was warned as silent")
		}
	}
}

func TestTheSilenceWarningNamesWhatIsSilentAndWhereToAllowIt(t *testing.T) {
	if silenceWarning(false, false) != nil {
		t.Fatal("a warning with nothing silent")
	}
	for _, tc := range []struct {
		mic, meeting bool
		settings     string
		says         []string
	}{
		{true, false, SettingsMicrophone, []string{"your microphone", "Microphone"}},
		{false, true, SettingsAudio, []string{"the meeting", "Screen & System Audio Recording"}},
		{true, true, SettingsPrivacy, []string{"your microphone and from the meeting", "Microphone", "Screen & System Audio Recording"}},
	} {
		w := silenceWarning(tc.mic, tc.meeting)
		if w == nil || w.Settings != tc.settings {
			t.Fatalf("silenceWarning(%v, %v) = %+v, want settings %q", tc.mic, tc.meeting, w, tc.settings)
		}
		for _, s := range tc.says {
			if !strings.Contains(w.Text, s) {
				t.Errorf("silenceWarning(%v, %v) does not say %q: %s", tc.mic, tc.meeting, s, w.Text)
			}
		}
	}
}
