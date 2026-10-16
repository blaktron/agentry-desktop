package run

import (
	"maps"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// A secret question's answer never goes to a run In Agentry; every other
// answer goes as given, and the caller's map is left alone.
func TestHostedIntakeLeavesOutSecretAnswers(t *testing.T) {
	m := &manifest.Manifest{Intake: []manifest.IntakeQuestion{
		{ID: "topic", Prompt: "Topic"},
		{ID: "api_token", Prompt: "Token", Type: manifest.IntakeSecret},
		{ID: "tone", Prompt: "Tone", Type: manifest.IntakeChoice, Choices: []string{"dry", "warm"}},
	}}
	answers := map[string]string{"topic": "llamas", "api_token": "s3cr3t-value", "tone": "dry"}
	got := HostedIntake(m, answers)
	if want := map[string]string{"topic": "llamas", "tone": "dry"}; !maps.Equal(got, want) {
		t.Fatalf("HostedIntake = %v, want %v", got, want)
	}
	if answers["api_token"] != "s3cr3t-value" {
		t.Fatal("the caller's answers were changed")
	}
	if got := HostedIntake(nil, answers); !maps.Equal(got, answers) {
		t.Fatalf("no manifest: %v", got)
	}
	if got := HostedIntake(m, nil); got != nil {
		t.Fatalf("no answers: %v", got)
	}
}
