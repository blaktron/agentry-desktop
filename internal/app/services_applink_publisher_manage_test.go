package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/runnerimage/applink"
)

// TestQuickAgentManageAsksInThePublisherSectionsWords: each owner action
// asks in the Publisher section's words, naming the agent; a No reaches
// nothing, a Yes takes the one action and answers its done sentence, and
// each call asks again.
func TestQuickAgentManageAsksInThePublisherSectionsWords(t *testing.T) {
	for _, c := range []struct {
		action string
		act    publisherAct
	}{{applink.ManageMakePrivate, actPrivate}, {applink.ManageRetire, actRetire}, {applink.ManageRestore, actRestore}} {
		rig, ops, _ := newPublisherRig(t)
		rig.m.asks = func() string { return "Ada" }
		ops.retired, ops.retiredAs = true, "private"
		rig.answer = false
		args := map[string]any{"ref": "alice/demo", "action": c.action}
		if got := rig.call(t, applink.QuickAgentManage, args); !got.IsError || !strings.Contains(got.Text, "said no") {
			t.Errorf("%s, a No: %+v", c.action, got)
		}
		words := publisherActs[c.act]
		if len(ops.owners) != 0 || len(rig.asked) != 1 || rig.asked[0] != "Ada asks: "+words.title("alice/demo")+"\n"+words.question("alice/demo") {
			t.Errorf("%s: asked %q, acted %v", c.action, rig.asked, ops.owners)
		}
		rig.answer = true
		got := rig.call(t, applink.QuickAgentManage, args)
		if got.IsError || got.Text != words.done("alice/demo") {
			t.Errorf("%s, a Yes: %+v", c.action, got)
		}
		rig.call(t, applink.QuickAgentManage, args)
		if len(rig.asked) != 3 || len(ops.owners) != 2 || ops.owners[0] != "alice/demo:"+string(c.act) {
			t.Errorf("%s: asked %d, acted %v", c.action, len(rig.asked), ops.owners)
		}
	}
}

// TestQuickAgentManageRestoresOnlyWhatWasPrivate: restore is refused, with
// nothing asked or sent, for a QuickAgent that was public when it was
// retired, one the registry does not say, and one not retired.
func TestQuickAgentManageRestoresOnlyWhatWasPrivate(t *testing.T) {
	rig, ops, _ := newPublisherRig(t)
	for _, c := range []struct {
		retired bool
		as      string
		want    string
	}{{true, "public", "Publisher section"}, {true, "", "does not say"}, {false, "", "not retired"}} {
		ops.retired, ops.retiredAs = c.retired, c.as
		if got := rig.call(t, applink.QuickAgentManage, map[string]any{"ref": "alice/demo", "action": "restore"}); !got.IsError || !strings.Contains(got.Text, c.want) {
			t.Errorf("retired %v as %q: %+v", c.retired, c.as, got)
		}
	}
	if len(rig.asked) != 0 || len(ops.owners) != 0 {
		t.Errorf("asked %v, acted %v", rig.asked, ops.owners)
	}
}

// TestQuickAgentManageYanksAndWithdrawsVersions: yank takes a published
// version and its reason, withdraw a version held for review; each asks,
// quoting the reason, and a version in the other state is refused before
// anything is asked.
func TestQuickAgentManageYanksAndWithdrawsVersions(t *testing.T) {
	rig, ops, _ := newPublisherRig(t)
	rig.m.asks = func() string { return "Ada" }
	rig.answer = false
	yank := map[string]any{"ref": "alice/demo", "action": "yank", "version": "1.0.0", "reason": "It leaks the page title."}
	if got := rig.call(t, applink.QuickAgentManage, yank); !got.IsError || len(ops.versionActs) != 0 {
		t.Fatalf("a No: %+v, acted %v", got, ops.versionActs)
	}
	if !strings.HasPrefix(rig.asked[0], "Ada asks: Yank alice/demo@1.0.0?\n") || !strings.Contains(rig.asked[0], `"It leaks the page title."`) {
		t.Errorf("the question: %q", rig.asked[0])
	}
	rig.answer = true
	if got := rig.call(t, applink.QuickAgentManage, yank); got.IsError || !strings.Contains(got.Text, "is yanked") {
		t.Errorf("a yank: %+v", got)
	}
	withdraw := map[string]any{"ref": "alice/demo", "action": "withdraw", "version": "1.1.0"}
	if got := rig.call(t, applink.QuickAgentManage, withdraw); got.IsError || !strings.Contains(got.Text, "is withdrawn") {
		t.Errorf("a withdrawal: %+v", got)
	}
	if strings.Join(ops.versionActs, ",") != "alice/demo@1.0.0:yank:It leaks the page title.,alice/demo@1.1.0:withdraw:" {
		t.Errorf("acted %v", ops.versionActs)
	}
	asked, acted := len(rig.asked), len(ops.versionActs)
	for _, a := range []map[string]any{
		{"ref": "alice/demo", "action": "yank", "version": "1.1.0", "reason": "x"},
		{"ref": "alice/demo", "action": "withdraw", "version": "1.0.0"},
		{"ref": "alice/demo", "action": "yank", "version": "1.0.0"},
		{"ref": "alice/demo", "action": "yank", "version": "9.9.9", "reason": "x"},
		{"ref": "alice/demo", "action": "withdraw"},
		{"ref": "alice/demo", "action": "retire", "version": "1.0.0"},
		{"ref": "alice/demo@1.0.0", "action": "yank", "reason": "x"},
		{"ref": "alice/demo", "action": "delete"},
		{"ref": "alice/demo", "action": "make_public"},
		{"ref": "watchers/demo", "action": "retire"},
		{"ref": "bob/tool", "action": "retire"},
		{"ref": "alice/demo", "action": "yank", "version": "1.0.0", "reason": strings.Repeat("x", 501)},
	} {
		if got := rig.call(t, applink.QuickAgentManage, a); !got.IsError {
			t.Errorf("%v: %+v", a, got)
		}
	}
	if len(rig.asked) != asked || len(ops.versionActs) != acted || len(ops.owners) != 0 {
		t.Errorf("a refused action asked or acted: %v, %v, %v", rig.asked[asked:], ops.versionActs[acted:], ops.owners)
	}
}

// TestQuickAgentManagePassesARefusalOn: the registry's refusal, in the
// Publisher section's words, reaches the agent as the answer.
func TestQuickAgentManagePassesARefusalOn(t *testing.T) {
	rig, ops, _ := newPublisherRig(t)
	ops.actErr = errors.New(publisherRefusal{status: 403}.sentence(actRetire, "alice", "demo"))
	if got := rig.call(t, applink.QuickAgentManage, map[string]any{"ref": "alice/demo", "action": "retire"}); !got.IsError || !strings.Contains(got.Text, "Nothing changed: only alice itself") {
		t.Errorf("a refusal: %+v", got)
	}
}
