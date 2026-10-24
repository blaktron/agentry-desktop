package notify

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanDrawsEachLineAsOneLineOfText(t *testing.T) {
	n := Clean(Notice{
		Title: "Morning\nsummary\t\x1b[31m red \u202e",
		Body:  "  first\r\nsecond  ",
		Key:   "assistant",
	})
	if n.Title != "Morning summary [31m red" {
		t.Errorf("title %q", n.Title)
	}
	if n.Body != "first second" {
		t.Errorf("body %q", n.Body)
	}
	if n.Key != "assistant" {
		t.Errorf("the key changed: %q", n.Key)
	}
}

func TestCleanCutsALongTitleAndBody(t *testing.T) {
	n := Clean(Notice{Title: strings.Repeat("é", maxTitle+10), Body: strings.Repeat("word ", 200)})
	if got := utf8.RuneCountInString(n.Title); got != maxTitle {
		t.Errorf("title is %d characters, want %d", got, maxTitle)
	}
	if got := utf8.RuneCountInString(n.Body); got > maxBody || !strings.HasSuffix(n.Body, "…") {
		t.Errorf("body is %d characters and ends %q", got, n.Body[len(n.Body)-5:])
	}
	if n := Clean(Notice{Title: "short"}); n.Title != "short" {
		t.Errorf("a short title changed: %q", n.Title)
	}
}

func TestCleanMendsInvalidText(t *testing.T) {
	if n := Clean(Notice{Body: "ok \xff\xfe done"}); !utf8.ValidString(n.Body) || !strings.HasPrefix(n.Body, "ok ") {
		t.Errorf("body %q", n.Body)
	}
}

// A title, a body or a key can carry nothing but text into a toast: no
// element and no attribute of its own.
func TestToastXMLEscapesEveryValue(t *testing.T) {
	n := Notice{
		Title: `Task <image src="https://example.com/x.png"/>`,
		Body:  `& <action activationType="protocol" arguments="https://example.com"/>`,
		Key:   `assistant" activationType="protocol`,
	}
	doc := toastXML(n)
	var toast struct {
		XMLName        xml.Name `xml:"toast"`
		ActivationType string   `xml:"activationType,attr"`
		Launch         string   `xml:"launch,attr"`
		Texts          []string `xml:"visual>binding>text"`
		Images         []string `xml:"visual>binding>image"`
		Actions        []string `xml:"actions>action"`
	}
	if err := xml.Unmarshal([]byte(doc), &toast); err != nil {
		t.Fatalf("not XML: %v\n%s", err, doc)
	}
	if toast.ActivationType != "foreground" || toast.Launch != n.Key {
		t.Errorf("attributes changed: %q %q", toast.ActivationType, toast.Launch)
	}
	if len(toast.Texts) != 2 || toast.Texts[0] != n.Title || toast.Texts[1] != n.Body {
		t.Errorf("texts %q", toast.Texts)
	}
	if len(toast.Images) != 0 || len(toast.Actions) != 0 || strings.Count(doc, "<text>") != 2 {
		t.Errorf("a value added an element:\n%s", doc)
	}
}

func TestAClickReachesTheHandlerWithItsKey(t *testing.T) {
	got := make(chan string, 1)
	SetOnClick(func(key string) { got <- key })
	t.Cleanup(func() { SetOnClick(nil) })
	clicked("")
	clicked("assistant")
	if k := <-got; k != "assistant" {
		t.Errorf("key %q", k)
	}
}
