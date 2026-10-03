package run

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

type wakingDocker struct {
	*dockerx.Fake
	failures int
	attempts []time.Time
}

func (d *wakingDocker) Available(context.Context) error {
	d.attempts = append(d.attempts, time.Now())
	if len(d.attempts) <= d.failures {
		return errors.New("engine is waking")
	}
	return nil
}

func TestDockerDetectionRetriesTwiceFiveSecondsApart(t *testing.T) {
	for _, surface := range []string{"setup", "prerequisites"} {
		t.Run(surface, func(t *testing.T) {
			isolateRunEnv(t)
			d := &wakingDocker{Fake: readyImagesFake(), failures: 2}
			withFakeDocker(t, d)
			dockerRetryInterval = 5 * time.Second
			old := keychainPresent
			keychainPresent = func(string) (bool, error) { return false, nil }
			t.Cleanup(func() { keychainPresent = old })
			synctest.Test(t, func(t *testing.T) {
				var ok bool
				if surface == "setup" {
					st, err := GetStatus(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					ok = st.DockerOK
				} else {
					ok = DetectPrereqs(t.Context()).DockerOK
				}
				if !ok {
					t.Fatal("an engine ready after two retries is unavailable")
				}
				if len(d.attempts) != 3 {
					t.Fatalf("attempts = %d, want 3", len(d.attempts))
				}
				for i := 1; i < 3; i++ {
					if delay := d.attempts[i].Sub(d.attempts[i-1]); delay != 5*time.Second {
						t.Fatalf("retry interval = %s", delay)
					}
				}
			})
		})
	}
}

func TestDockerDetectionStopsAfterTwoRetriesAndCanBeCheckedAgain(t *testing.T) {
	d := &wakingDocker{Fake: readyImagesFake(), failures: 3}
	withFakeDocker(t, d)
	dockerRetryInterval = 5 * time.Second
	synctest.Test(t, func(t *testing.T) {
		p := DetectPrereqs(t.Context())
		if p.DockerOK || p.DockerErr != "engine is waking" {
			t.Fatalf("failure = %+v", p)
		}
		if len(d.attempts) != 3 {
			t.Fatalf("attempts = %d, want 3", len(d.attempts))
		}
		if !DetectPrereqs(t.Context()).DockerOK {
			t.Fatal("explicit recheck must recover")
		}
	})
}

func TestDockerDetectionCancellationDuringRetry(t *testing.T) {
	d := &wakingDocker{Fake: readyImagesFake(), failures: 3}
	withFakeDocker(t, d)
	dockerRetryInterval = 5 * time.Second
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)
		p := DetectPrereqs(ctx)
		if p.DockerOK || len(d.attempts) != 1 {
			t.Fatalf("cancelled check = %+v, attempts %d", p, len(d.attempts))
		}
	})
}

type unresponsiveDocker struct{ *dockerx.Fake }

func (d *unresponsiveDocker) Available(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestDockerDetectionBoundsUnresponsiveProbes(t *testing.T) {
	withFakeDocker(t, &unresponsiveDocker{Fake: readyImagesFake()})
	dockerRetryInterval = 5 * time.Second
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		p := DetectPrereqs(t.Context())
		if p.DockerOK || !strings.Contains(p.DockerErr, "deadline exceeded") {
			t.Fatalf("unresponsive engine = %+v", p)
		}
		if elapsed := time.Since(started); elapsed != 55*time.Second {
			t.Fatalf("elapsed = %s, want three 15s probes and two 5s waits", elapsed)
		}
	})
}
