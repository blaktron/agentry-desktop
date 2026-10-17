package draft

// Describe again writes the site's description byte for byte, so a draft started from either client
// reads back the same way in both.

import (
	"strings"
	"testing"
)

func TestDescribeAgainWritesTheSitesDescription(t *testing.T) {
	got := DescribeAgain("  Post to #general too. ", "Summarise meeting notes.\n", "name: x\n", "# X\n\nuse ```code```\n")
	want := "What the person first asked for:\nSummarise meeting notes.\n\n" +
		"What the person wants changed now:\nPost to #general too.\n\n" +
		"The current draft's agent.yaml:\n\n```yaml\nname: x\n```\n\n" +
		"The current draft's AGENT.md:\n\n````markdown\n# X\n\nuse ```code```\n````"
	if got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
	follow, first, ok := SplitDescribeAgain(got)
	if !ok || follow != "Post to #general too." || first != "Summarise meeting notes." {
		t.Fatalf("split %q %q %v", follow, first, ok)
	}
	// A chain carries the first description, not itself.
	again := DescribeAgain("And Slack.", FirstDescription(got), "name: x\n", "")
	if _, first, _ := SplitDescribeAgain(again); first != "Summarise meeting notes." {
		t.Fatalf("the chain's first description is %q", first)
	}
	if FirstDescription("  My own words. ") != "My own words." {
		t.Fatal("a description the person wrote is theirs")
	}
}

func TestDescribeAgainFitsTheRegistrysBoundAndNeverCutsTheNote(t *testing.T) {
	note := strings.Repeat("n", MaxFollowUp)
	got := DescribeAgain(note, strings.Repeat("é", 2000), strings.Repeat("y", 30000), strings.Repeat("`", 50)+strings.Repeat("m", 30000))
	if len(got) > MaxDescription {
		t.Fatalf("%d bytes: over the registry's %d", len(got), MaxDescription)
	}
	if !strings.Contains(got, againChange+note) {
		t.Fatal("the note was cut")
	}
	if !strings.HasSuffix(got, "\n```") && !strings.HasSuffix(got, "\n"+strings.Repeat("`", 51)) {
		t.Fatalf("the last fence is not closed: …%q", got[len(got)-80:])
	}
	if strings.Count(got, "�") > 0 || !strings.Contains(got, "é") {
		t.Fatal("a character was cut in half")
	}
}

func TestSplitDescribeAgainReadsTheSitesFirstFormat(t *testing.T) {
	old := "Change this QuickAgent draft as follows:\nAdd Slack.\n\nWhat it was first asked to do:\nSummarise notes.\n\nThe draft as it stands, agent.yaml:\n\n```yaml\nname: x\n```"
	follow, first, ok := SplitDescribeAgain(old)
	if !ok || follow != "Add Slack." || first != "Summarise notes." {
		t.Fatalf("split %q %q %v", follow, first, ok)
	}
	if _, _, ok := SplitDescribeAgain("What the person first asked for:\nno change label"); ok {
		t.Fatal("a description with only the first label is the person's own")
	}
}
