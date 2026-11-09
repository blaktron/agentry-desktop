package app

// quickagent_manage: one owner action on one of the person's QuickAgents,
// asked each time in the Publisher section's own words, naming the agent
// that asks. The actions are make private, retire, restore, yank and
// withdraw. Making a QuickAgent public, deleting one and restoring one that
// was public when it was retired (which makes it public again) stay with
// the person, in the Publisher section.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-cli/runnerimage/applink"

	"github.com/blaktron/agentry-desktop/internal/registry"
)

// maxYankReason bounds a yank's or a withdrawal's reason.
const maxYankReason = 500

func (m *mainLink) quickAgentManage(ref, action, version, reason string) (string, error) {
	ns, name, v, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	if v != "" {
		return "", errors.New("name the QuickAgent as namespace/name, and the version, for yank and withdraw, in version")
	}
	if !slices.Contains(applink.ManageActions, action) {
		return "", fmt.Errorf("the action is one of %s, not %s", strings.Join(applink.ManageActions, ", "), quoted(action))
	}
	versioned := action == applink.ManageYank || action == applink.ManageWithdraw
	switch {
	case versioned && version == "":
		return "", fmt.Errorf("%s takes the version, in version", action)
	case versioned && !versionPattern.MatchString(version):
		return "", fmt.Errorf("%s is not a version", quoted(version))
	case !versioned && version != "":
		return "", fmt.Errorf("%s acts on the whole QuickAgent and takes no version", action)
	case action == applink.ManageYank && reason == "":
		return "", errors.New("a yank takes a reason, in a sentence: the people who installed the version see it")
	case len(reason) > maxYankReason:
		return "", fmt.Errorf("the reason is at most %d bytes", maxYankReason)
	}
	reg := m.ops.registry()
	acct, err := m.publisher.account(reg)
	if err != nil {
		return "", err
	}
	if acct == nil {
		return "", errSignedOut
	}
	if !slices.Contains(acct.publishes(), ns) {
		return "", fmt.Errorf("%s is neither the person's namespace nor one of the organizations they publish for (%s)", quoted(ns), strings.Join(acct.publishes(), ", "))
	}
	full := ns + "/" + name
	if versioned {
		return m.manageVersion(reg, ns, name, version, reason, action == applink.ManageWithdraw)
	}
	act := map[string]publisherAct{applink.ManageMakePrivate: actPrivate, applink.ManageRetire: actRetire, applink.ManageRestore: actRestore}[action]
	if act == actRestore {
		retired, was, err := m.publisher.retirement(reg, ns, name)
		switch {
		case err != nil:
			return "", err
		case !retired:
			return "", fmt.Errorf("%s is not retired, so there is nothing to restore", full)
		case was == "public":
			return "", fmt.Errorf("%s was public when it was retired, so restoring it makes it public again, which is the person's to do: they restore it in Agentry's Publisher section", full)
		case was != "private":
			return "", fmt.Errorf("the registry does not say whether %s was public when it was retired, and restoring one that was makes it public again: the person restores it in Agentry's Publisher section", full)
		}
	}
	words := publisherActs[act]
	if err := m.app.askConsent(m.who()+" asks: "+words.title(full), words.question(full)); err != nil {
		return "", err
	}
	return m.publisher.ownerAction(reg, ns, name, act)
}

// manageVersion yanks or withdraws one version, asked first.
func (m *mainLink) manageVersion(reg, ns, name, version, reason string, withdraw bool) (string, error) {
	ref := ns + "/" + name + "@" + version
	vs, err := m.publisher.versions(reg, ns, name)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(vs, func(v apiclient.AgentVersion) bool { return v.Version == version })
	if i < 0 {
		return "", fmt.Errorf("%s/%s has no version %s: quickagent_versions lists them", ns, name, version)
	}
	state := versionState(vs[i].Status)
	var title, question, done string
	if withdraw {
		if vs[i].Status != apiclient.VersionStatusNeedsReview {
			return "", fmt.Errorf("%s is %s, and only a version held for review is withdrawn: a published one is yanked", ref, state)
		}
		title = m.who() + " asks: Withdraw " + ref + "?"
		question = ref + " is taken back from review. It is never installable, and its version number stays used."
		done = ref + " is withdrawn. It is never installable, and its version number stays used."
	} else {
		if vs[i].Status != apiclient.VersionStatusPublished {
			return "", fmt.Errorf("%s is %s, and only a published version is yanked: one held for review is withdrawn", ref, state)
		}
		title = m.who() + " asks: Yank " + ref + "?"
		question = ref + " is no longer offered to new installs or updates. Anyone who installed it keeps it, and is told why."
		done = ref + " is yanked: it is no longer offered to new installs or updates."
	}
	if reason != "" {
		question += "\n\nThe reason given: " + quoted(reason)
	}
	if err := m.app.askConsent(title, question); err != nil {
		return "", err
	}
	if err := m.publisher.versionAction(reg, ns, name, version, reason, withdraw); err != nil {
		return "", err
	}
	return done, nil
}

func (o appPublisherOps) ownerAction(reg, ns, name string, act publisherAct) (string, error) {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return "", err
	}
	done, err := o.pub.perform(o.pub.ctx(), client, regURL, token, ns, name, act, "")
	if err != nil {
		return "", err
	}
	return done.Sentence, nil
}

func (o appPublisherOps) retirement(reg, ns, name string) (bool, string, error) {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return false, "", err
	}
	res, err := client.GetAgentWithResponse(o.pub.ctx(), ns, name)
	if err != nil {
		return false, "", withoutToken(fmt.Errorf("reach %s: %w", originOf(regURL), err), token)
	}
	if res.JSON200 == nil {
		return false, "", withoutToken(registry.Problem("read "+ns+"/"+name, res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body), token)
	}
	if res.JSON200.RetiredAt == nil {
		return false, "", nil
	}
	// A registry that says what Restore gives back sends it beside the
	// record; one that does not leaves it unknown.
	var extra struct {
		RetiredVisibility string `json:"retiredVisibility"`
	}
	_ = json.Unmarshal(res.Body, &extra)
	switch extra.RetiredVisibility {
	case "public", "private":
		return true, extra.RetiredVisibility, nil
	}
	return true, "", nil
}

func (o appPublisherOps) versionAction(reg, ns, name, version, reason string, withdraw bool) error {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return err
	}
	ctx := o.pub.ctx()
	var status int
	var problem *apiclient.Problem
	var body []byte
	if withdraw {
		b := apiclient.WithdrawVersionJSONRequestBody{}
		if reason != "" {
			b.Reason = &reason
		}
		res, err := client.WithdrawVersionWithResponse(ctx, ns, name, version, b)
		if err != nil {
			return withoutToken(fmt.Errorf("reach %s: %w", originOf(regURL), err), token)
		}
		status, problem, body = res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body
	} else {
		res, err := client.YankVersionWithResponse(ctx, ns, name, version, apiclient.YankVersionJSONRequestBody{Reason: reason})
		if err != nil {
			return withoutToken(fmt.Errorf("reach %s: %w", originOf(regURL), err), token)
		}
		status, problem, body = res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body
	}
	if status == http.StatusNoContent || status == http.StatusOK {
		return nil
	}
	_, detail := registry.ProblemSlug(problem, body)
	what := "yank"
	if withdraw {
		what = "withdraw"
	}
	switch {
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%s the person is signed out of Agentry", publisherNothingChanged)
	case status == http.StatusForbidden:
		return fmt.Errorf("%s the person's sign-in may not %s versions of %s/%s", publisherNothingChanged, what, ns, name)
	case status == http.StatusNotFound:
		return fmt.Errorf("%s %s/%s@%s is not there any more", publisherNothingChanged, ns, name, version)
	case detail != "":
		return fmt.Errorf("%s the registry says: %s", publisherNothingChanged, quoted(detail))
	}
	return fmt.Errorf("%s the registry refused it (%d)", publisherNothingChanged, status)
}
