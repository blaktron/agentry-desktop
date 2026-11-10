package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/linkservice"
)

// cardRig is a CardWindowAPI whose windows are stand-ins: the card's window
// has id cardWin, the main window mainWin, and a call's window is the one
// its context names.
type cardRig struct {
	api *CardWindowAPI

	mu     sync.Mutex
	opened chan *fakeCardWindow
	last   *fakeCardWindow
}

const (
	mainWin uint = 1
	cardWin uint = 7
)

type windowKey struct{}

func from(id uint) context.Context { return context.WithValue(context.Background(), windowKey{}, id) }

type fakeCardWindow struct {
	id     uint
	title  string
	closed func()
	mu     sync.Mutex
	shut   bool
}

func (w *fakeCardWindow) ID() uint { return w.id }
func (w *fakeCardWindow) Close() {
	w.mu.Lock()
	w.shut = true
	w.mu.Unlock()
}
func (w *fakeCardWindow) isShut() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shut
}

func newCardRig(t *testing.T) *cardRig {
	t.Helper()
	r := &cardRig{opened: make(chan *fakeCardWindow, 4)}
	r.api = &CardWindowAPI{
		turn: make(chan struct{}, 1),
		open: func(title string, closed func()) (cardWindow, error) {
			w := &fakeCardWindow{id: cardWin, title: title, closed: closed}
			r.mu.Lock()
			r.last = w
			r.mu.Unlock()
			r.opened <- w
			return w, nil
		},
		caller: func(ctx context.Context) (uint, bool) {
			id, ok := ctx.Value(windowKey{}).(uint)
			return id, ok
		},
		loadTimeout: 5 * time.Second,
	}
	return r
}

type askResult struct {
	button int
	err    error
}

func (r *cardRig) ask(ctx context.Context, v CardView) chan askResult {
	out := make(chan askResult, 1)
	go func() {
		b, err := r.api.ask(ctx, v)
		out <- askResult{b, err}
	}()
	return out
}

func (r *cardRig) window(t *testing.T) *fakeCardWindow {
	t.Helper()
	select {
	case w := <-r.opened:
		return w
	case <-time.After(5 * time.Second):
		t.Fatal("no card window opened")
		return nil
	}
}

func wait(t *testing.T, ch chan askResult) askResult {
	t.Helper()
	select {
	case a := <-ch:
		return a
	case <-time.After(5 * time.Second):
		t.Fatal("the question was not answered")
		return askResult{}
	}
}

func TestACardIsReadAndAnsweredOnlyInItsOwnWindow(t *testing.T) {
	r := newCardRig(t)
	got := r.ask(context.Background(), confirmCard("Install it?", "It runs unsigned."))
	w := r.window(t)
	if w.title != "Install it?" {
		t.Fatalf("window title %q", w.title)
	}

	if _, err := r.api.Card(from(mainWin)); err == nil {
		t.Fatal("the main window read the card")
	}
	if _, err := r.api.Card(context.Background()); err == nil {
		t.Fatal("a call that names no window read the card")
	}

	card, err := r.api.Card(from(cardWin))
	if err != nil {
		t.Fatalf("the card's window could not read it: %v", err)
	}
	if card.Title != "Install it?" || card.Message != "It runs unsigned." || len(card.Buttons) != 2 || card.Cancel != 1 {
		t.Fatalf("card %+v", card)
	}

	if err := r.api.AnswerCard(from(mainWin), card.ID, 0); err == nil {
		t.Fatal("the main window answered the card")
	}
	if err := r.api.AnswerCard(from(cardWin), "another-card", 0); err == nil {
		t.Fatal("an answer to another card counted")
	}
	if err := r.api.AnswerCard(from(cardWin), card.ID, 2); err == nil {
		t.Fatal("an answer the card does not offer counted")
	}
	select {
	case a := <-got:
		t.Fatalf("answered before the card's window answered: %+v", a)
	default:
	}

	if err := r.api.AnswerCard(from(cardWin), card.ID, 0); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if a := wait(t, got); a.err != nil || a.button != 0 {
		t.Fatalf("answer %+v", a)
	}
	if !w.isShut() {
		t.Fatal("the card window stayed open after the answer")
	}
	if _, err := r.api.Card(from(cardWin)); err == nil {
		t.Fatal("a card was still there after the answer")
	}
}

func TestClosingTheCardWindowAnswersTheCancel(t *testing.T) {
	r := newCardRig(t)
	got := r.ask(context.Background(), confirmCard("Remove?", "Three images."))
	w := r.window(t)
	w.closed()
	if a := wait(t, got); a.err != nil || a.button != 1 {
		t.Fatalf("closing answered %+v, want the cancel (1)", a)
	}
	if w.isShut() {
		t.Fatal("a window the person closed was closed again")
	}
}

func TestACancelledQuestionAnswersTheCancel(t *testing.T) {
	r := newCardRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	got := r.ask(ctx, execCard(linkservice.ExecQuestion{}))
	w := r.window(t)
	cancel()
	a := wait(t, got)
	if !errors.Is(a.err, context.Canceled) || a.button != 2 {
		t.Fatalf("cancelled answered %+v", a)
	}
	if !w.isShut() {
		t.Fatal("the card window stayed open")
	}
}

func TestACardWindowThatNeverLoadsFallsBack(t *testing.T) {
	r := newCardRig(t)
	r.api.loadTimeout = 50 * time.Millisecond
	got := r.ask(context.Background(), noticeCard("Agentry is still running", "The icon opens it again."))
	w := r.window(t)
	if a := wait(t, got); !errors.Is(a.err, errNoCardWindow) {
		t.Fatalf("a card never read answered %+v, want errNoCardWindow", a)
	}
	if !w.isShut() {
		t.Fatal("the card window stayed open")
	}
}

func TestALoadedCardWaitsPastTheLoadTimeout(t *testing.T) {
	r := newCardRig(t)
	r.api.loadTimeout = 30 * time.Millisecond
	got := r.ask(context.Background(), confirmCard("Run it?", ""))
	r.window(t)
	card, err := r.api.Card(from(cardWin))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := r.api.AnswerCard(from(cardWin), card.ID, 1); err != nil {
		t.Fatal(err)
	}
	if a := wait(t, got); a.err != nil || a.button != 1 {
		t.Fatalf("answer %+v", a)
	}
}

func TestCardsAreShownOneAtATime(t *testing.T) {
	r := newCardRig(t)
	first := r.ask(context.Background(), confirmCard("First", ""))
	r.window(t)
	second := r.ask(context.Background(), confirmCard("Second", ""))
	select {
	case w := <-r.opened:
		t.Fatalf("a second card window opened (%q) while the first was open", w.title)
	case <-time.After(100 * time.Millisecond):
	}
	c, _ := r.api.Card(from(cardWin))
	_ = r.api.AnswerCard(from(cardWin), c.ID, 1)
	wait(t, first)
	if w := r.window(t); w.title != "Second" {
		t.Fatalf("next card %q", w.title)
	}
	c, _ = r.api.Card(from(cardWin))
	_ = r.api.AnswerCard(from(cardWin), c.ID, 0)
	if a := wait(t, second); a.button != 0 {
		t.Fatalf("second answered %+v", a)
	}
}

func TestNoCardWindowLeavesTheQuestionToTheSystem(t *testing.T) {
	var none *CardWindowAPI
	if _, err := none.ask(context.Background(), confirmCard("x", "y")); !errors.Is(err, errNoCardWindow) {
		t.Fatalf("err %v", err)
	}
	r := newCardRig(t)
	r.api.open = func(string, func()) (cardWindow, error) { return nil, errNoApplication }
	if _, err := r.api.ask(context.Background(), confirmCard("x", "y")); !errors.Is(err, errNoCardWindow) {
		t.Fatalf("err %v", err)
	}
	// The turn is free again.
	r.api.open = nil
	if _, err := r.api.ask(context.Background(), confirmCard("x", "y")); !errors.Is(err, errNoCardWindow) {
		t.Fatalf("err %v", err)
	}
}

func TestACallWithNoWindowIsNotTheCardsWindow(t *testing.T) {
	if _, ok := nativeCaller(context.Background()); ok {
		t.Fatal("a context with no window named one")
	}
}

// The native questions map each card answer back to the answer they return.
func TestTheNativeQuestionsMapTheCardsAnswers(t *testing.T) {
	r := newCardRig(t)
	saved := cards
	cards = r.api
	t.Cleanup(func() { cards = saved })

	answer := func(button int) {
		r.window(t)
		c, err := r.api.Card(from(cardWin))
		if err != nil {
			t.Error(err)
			return
		}
		if err := r.api.AnswerCard(from(cardWin), c.ID, button); err != nil {
			t.Error(err)
		}
	}

	for button, want := range []bool{true, false} {
		go answer(button)
		if yes, err := nativeConfirm(context.Background(), "Q", "M"); err != nil || yes != want {
			t.Fatalf("confirm button %d: %v %v", button, yes, err)
		}
	}

	for button, want := range execAnswers {
		go answer(button)
		if a, err := nativeAskExec(context.Background(), linkservice.ExecQuestion{}); err != nil || a != want {
			t.Fatalf("exec button %d: %v %v", button, a, err)
		}
	}

	q := linkservice.ChooseQuestion{Candidates: []linkservice.Candidate{{ID: "blender", Name: "Blender"}, {ID: "gimp", Name: "GIMP"}}}
	go answer(1)
	if id, err := nativeChoose(context.Background(), q); err != nil || id != "gimp" {
		t.Fatalf("choose: %q %v", id, err)
	}
	go answer(2)
	if _, err := nativeChoose(context.Background(), q); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("None of these: %v", err)
	}

	go answer(0)
	done := make(chan struct{})
	go func() { nativeInfo(context.Background(), "T", "M"); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the notice did not return once answered")
	}
}
