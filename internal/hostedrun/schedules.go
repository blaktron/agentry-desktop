package hostedrun

// Schedules In Agentry (plan quickagent-scheduling.md §6, §7, M10;
// agentry-desktop#745): the client for the enterprise schedules extension
// (agentry-enterprise schemas/x-schedules.yaml, M7). A schedule runs a
// QuickAgent In Agentry once later or on a repeat; every occurrence fires
// once, through the runs extension's own create, as its owner. The routes
// take the account's token: run:read to read, run:write to change, the
// scopes a run In Agentry already asks for, so the app mints no new one.
//
// No token reads the account's secrets or connections (DEV-165), and the
// extension has no dry run: a create is the check (D8). It runs the run's
// own create, stopped before anything is held, with the schedule's
// credential, and stores nothing unless every input a fire needs is in
// place; its refusal names what is missing (secret-missing,
// secret-not-kept, connection-missing, …) for the app to offer the fix.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The extension's own refusals, beside the run create's that it passes on.
const (
	// SlugSecretNotKept: a declared secret is set for one run only, and a
	// schedule resolves kept secrets alone. Missing names them.
	SlugSecretNotKept = "secret-not-kept"
	// SlugSecretMissing: a declared secret is not set at all. Missing
	// names them.
	SlugSecretMissing = "secret-missing"
	// SlugScheduleTier: the payer's plan does not schedule In Agentry
	// (Standard).
	SlugScheduleTier = "schedule-tier"
	// SlugScheduleLimit: the account keeps its most schedules.
	SlugScheduleLimit = "schedule-limit"
)

// Occurrence states (x-schedules.yaml ScheduleOccurrence.state).
const (
	OccurrenceUpcoming = "upcoming"
	OccurrencePending  = "pending"
	OccurrenceFiring   = "firing"
	OccurrenceRan      = "ran"
	OccurrenceSkipped  = "skipped"
	OccurrenceFailed   = "failed"
)

// MaxOccurrenceRange is the widest range one occurrences read takes.
const MaxOccurrenceRange = 92 * 24 * time.Hour

// Recurrence is when a schedule is due, as the extension keeps it (the
// CLI's pkg/schedule.Recurrence on the wire): a wall-clock time in an IANA
// zone. Start is read only, so it is never sent.
type Recurrence struct {
	Kind     string   `json:"kind"`
	Date     string   `json:"date,omitempty"`
	Time     string   `json:"time"`
	Days     []string `json:"days,omitempty"`
	MonthDay int      `json:"monthDay,omitempty"`
	Zone     string   `json:"zone"`
	Start    string   `json:"start,omitempty"`
	End      *End     `json:"end,omitempty"`
}

// End is how a repeat ends: never, on a day, or after a number of runs.
type End struct {
	Kind  string `json:"kind"`
	On    string `json:"on,omitempty"`
	After int    `json:"after,omitempty"`
}

// ScheduleRequest is a create's body, or an edit's: every field of an
// edit is optional, and only what is set is sent.
type ScheduleRequest struct {
	Name      string            `json:"name,omitempty"`
	Ref       string            `json:"ref,omitempty"`
	Task      string            `json:"task,omitempty"`
	Intake    map[string]string `json:"intake,omitempty"`
	Knowledge []string          `json:"knowledge,omitempty"`
	// Voice is a pointer so an edit that does not touch it sends nothing.
	Voice  *bool       `json:"voice,omitempty"`
	Inputs []string    `json:"inputs,omitempty"`
	When   *Recurrence `json:"when,omitempty"`
	// AskOptionalSecrets makes the create's or the edit's check refuse an
	// optional secret the account does not keep, as a required one
	// (agentry-enterprise#517); the schedule does not keep it.
	AskOptionalSecrets bool `json:"ask_optional_secrets,omitempty"`
}

// ScheduleInput is one of a schedule's files, copied into storage it owns.
type ScheduleInput struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// ScheduleFire is one occurrence a schedule has met, as GET of one
// schedule lists it.
type ScheduleFire struct {
	ScheduleID   string `json:"scheduleId"`
	OccurrenceAt string `json:"occurrenceAt"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Detail       string `json:"detail,omitempty"`
	RunID        string `json:"runId,omitempty"`
	Attempts     int    `json:"attempts"`
	DoneAt       string `json:"doneAt,omitempty"`
}

// Schedule is one of the account's schedules.
type Schedule struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Ref       string          `json:"ref"`
	Digest    string          `json:"digest"`
	Task      string          `json:"task"`
	Knowledge []string        `json:"knowledge"`
	Voice     bool            `json:"voice"`
	Inputs    []ScheduleInput `json:"inputs"`
	When      Recurrence      `json:"when"`
	// Repeats is when, in words, its zone named.
	Repeats string `json:"repeats"`
	// State is active, paused or ended; PausedReason says who paused it:
	// owner, refusals, entitlement or account.
	State        string         `json:"state"`
	PausedReason string         `json:"pausedReason,omitempty"`
	NextAt       string         `json:"nextAt,omitempty"`
	Runs         int            `json:"runs"`
	Refusals     int            `json:"refusals"`
	CreatedAt    string         `json:"createdAt"`
	UpdatedAt    string         `json:"updatedAt"`
	Fires        []ScheduleFire `json:"fires,omitempty"`
}

// Occurrence is one occurrence of a schedule in a calendar's range: a
// coming one (upcoming), or a past one as its fire recorded it.
type Occurrence struct {
	ScheduleID string `json:"scheduleId"`
	Name       string `json:"name"`
	At         string `json:"at"`
	Zone       string `json:"zone"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	RunID      string `json:"runId,omitempty"`
}

// schedulesPath builds an absolute URL under the schedules extension's
// mount, each segment escaped as path does for runs.
func (c *Client) schedulesPath(segments ...string) string {
	u := c.base + "/api/v1/x/schedules"
	for _, s := range segments {
		u += "/" + url.PathEscape(s)
	}
	return u
}

// jsonBody marshals v for a request.
func jsonBody(v any) (*strings.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(string(b)), nil
}

// Schedules lists the account's schedules, newest first, and how many
// that have not ended an account may keep.
func (c *Client) Schedules(ctx context.Context) ([]Schedule, int, error) {
	req, err := c.request(ctx, http.MethodGet, c.schedulesPath()+"/", nil, "")
	if err != nil {
		return nil, 0, err
	}
	var out struct {
		Items []Schedule `json:"items"`
		Limit int        `json:"limit"`
	}
	if err := c.do("list the account's schedules", req, http.StatusOK, &out); err != nil {
		return nil, 0, err
	}
	if out.Items == nil {
		out.Items = []Schedule{}
	}
	return out.Items, out.Limit, nil
}

// CreateSchedule makes a schedule, checked as its fires would be; a gap is
// a *Refusal naming it.
func (c *Client) CreateSchedule(ctx context.Context, sr ScheduleRequest) (*Schedule, error) {
	body, err := jsonBody(sr)
	if err != nil {
		return nil, err
	}
	// POST /api/v1/x/schedules/ — the trailing slash is the route.
	req, err := c.request(ctx, http.MethodPost, c.schedulesPath()+"/", body, "application/json")
	if err != nil {
		return nil, err
	}
	var s Schedule
	if err := c.do("schedule the run", req, http.StatusCreated, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSchedule reads one schedule with its latest fires.
func (c *Client) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	req, err := c.request(ctx, http.MethodGet, c.schedulesPath(id), nil, "")
	if err != nil {
		return nil, err
	}
	var s Schedule
	if err := c.do("read the schedule", req, http.StatusOK, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateSchedule edits a schedule; a new When starts its repeat over from
// now.
func (c *Client) UpdateSchedule(ctx context.Context, id string, sr ScheduleRequest) (*Schedule, error) {
	body, err := jsonBody(sr)
	if err != nil {
		return nil, err
	}
	req, err := c.request(ctx, http.MethodPatch, c.schedulesPath(id), body, "application/json")
	if err != nil {
		return nil, err
	}
	var s Schedule
	if err := c.do("change the schedule", req, http.StatusOK, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSchedule deletes a schedule with its fires and files; the runs it
// started stay.
func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	req, err := c.request(ctx, http.MethodDelete, c.schedulesPath(id), nil, "")
	if err != nil {
		return err
	}
	return c.do("delete the schedule", req, http.StatusNoContent, nil)
}

// scheduleAct posts pause or resume.
func (c *Client) scheduleAct(ctx context.Context, id, verb, op string) (*Schedule, error) {
	req, err := c.request(ctx, http.MethodPost, c.schedulesPath(id, verb), nil, "")
	if err != nil {
		return nil, err
	}
	var s Schedule
	if err := c.do(op, req, http.StatusOK, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// PauseSchedule pauses a schedule: nothing fires until it is resumed.
func (c *Client) PauseSchedule(ctx context.Context, id string) (*Schedule, error) {
	return c.scheduleAct(ctx, id, "pause", "pause the schedule")
}

// ResumeSchedule resumes a paused schedule from its next occurrence after
// now, checked again as a create is.
func (c *Client) ResumeSchedule(ctx context.Context, id string) (*Schedule, error) {
	return c.scheduleAct(ctx, id, "resume", "resume the schedule")
}

// RunScheduleNow starts an ordinary run of the schedule's setup now, which
// is no occurrence, and answers the run's id.
func (c *Client) RunScheduleNow(ctx context.Context, id string) (string, error) {
	req, err := c.request(ctx, http.MethodPost, c.schedulesPath(id, "run"), nil, "")
	if err != nil {
		return "", err
	}
	var out struct {
		RunID string `json:"runId"`
	}
	if err := c.do("run the schedule now", req, http.StatusCreated, &out); err != nil {
		return "", err
	}
	return out.RunID, nil
}

// Occurrences reads every occurrence of the account's schedules in
// [from, to), at most MaxOccurrenceRange apart, by time.
func (c *Client) Occurrences(ctx context.Context, from, to time.Time) ([]Occurrence, error) {
	q := url.Values{}
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	req, err := c.request(ctx, http.MethodGet, c.schedulesPath("occurrences")+"?"+q.Encode(), nil, "")
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []Occurrence `json:"items"`
	}
	if err := c.do("read the schedules' calendar", req, http.StatusOK, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		out.Items = []Occurrence{}
	}
	return out.Items, nil
}
