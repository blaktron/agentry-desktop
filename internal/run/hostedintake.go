package run

import (
	"github.com/blaktron/agentry/pkg/manifest"
)

// HostedIntake is the intake a run, a session or a schedule In Agentry
// sends: the answers as given, less any answer to one of m's secret
// questions. The registry keeps a hosted run's intake with the run, shows
// it on the run's page and replays it on a rerun and on every fire of a
// schedule, so it refuses a secret answer outright; a hosted run takes a
// secret question's value from the account's secrets instead, under the
// name the CLI gives it, where the release declares that secret. On this
// computer a secret answer is injected like a declared key and is never
// written to the intake file, so a local run sends every answer.
func HostedIntake(m *manifest.Manifest, answers map[string]string) map[string]string {
	if m == nil || answers == nil {
		return answers
	}
	out := make(map[string]string, len(answers))
	for id, v := range answers {
		out[id] = v
	}
	for _, q := range m.Intake {
		if q.Type == manifest.IntakeSecret {
			delete(out, q.ID)
		}
	}
	return out
}
