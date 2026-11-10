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
	at     cardRect
	closed func()
	mu     sync.Mutex
	shut   bool
	placed []cardRect
}

func (w *fakeCardWindow) place(r cardRect) {
	w.mu.Lock()
	w.placed = append(w.placed, r)
	w.mu.Unlock()
}

func (w *fakeCardWindow) lastPlaced() (cardRect, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.placed) == 0 {
		return cardRect{}, false
	}
	return w.placed[len(w.placed)-1], true
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
		open: func(title string, at cardRect, closed func()) (cardWindow, error) {
			w := &fakeCardWindow{id: cardWin, title: title, at: at, closed: closed}
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
		anchor:      func() cardAnchor { return twoScreens(cardRect{X: 2200, Y: 200, Width: 1200, Height: 800}, true) },
		loadTimeout: 5 * time.Second,
	}
	return r
}

// twoScreens is a primary screen 1920x1080 with a 25-high menu bar, and a
// second 2560x1440 one to its right, with the main window at main.
func twoScreens(main cardRect, shown bool) cardAnchor {
	return cardAnchor{Main: main, Shown: shown, Screens: []cardScreen{
		{Bounds: cardRect{0, 0, 1920, 1080}, Work: cardRect{0, 25, 1920, 1055}, Primary: true},
		{Bounds: cardRect{1920, 0, 2560, 1440}, Work: cardRect{1920, 0, 2560, 1400}},
	}}
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
	r.api.open = func(string, cardRect, func()) (cardWindow, error) { return nil, errNoApplication }
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

func TestACardIsCentredOverTheMainWindowOnItsScreen(t *testing.T) {
	main := cardRect{X: 2200, Y: 200, Width: 1200, Height: 800}
	for _, c := range []struct {
		name   string
		anchor cardAnchor
		w, h   int
		want   cardRect
	}{
		{"over the window on the second screen", twoScreens(main, true), 520, 260,
			cardRect{X: 2540, Y: 470, Width: 520, Height: 260}},
		{"over the window on the primary screen", twoScreens(cardRect{X: 100, Y: 100, Width: 800, Height: 600}, true), 520, 260,
			cardRect{X: 240, Y: 270, Width: 520, Height: 260}},
		{"hidden: centred on the window's screen", twoScreens(main, false), 520, 260,
			cardRect{X: 2940, Y: 570, Width: 520, Height: 260}},
		{"window near an edge: kept inside its screen", twoScreens(cardRect{X: 4300, Y: 1300, Width: 300, Height: 200}, true), 520, 260,
			cardRect{X: 3960, Y: 1140, Width: 520, Height: 260}},
		{"window above the menu bar: below it", twoScreens(cardRect{X: 0, Y: 0, Width: 600, Height: 100}, true), 520, 260,
			cardRect{X: 40, Y: 25, Width: 520, Height: 260}},
		{"taller than the screen: as tall as its work area", twoScreens(cardRect{X: 0, Y: 25, Width: 1920, Height: 1055}, true), 520, 2000,
			cardRect{X: 700, Y: 25, Width: 520, Height: 1055}},
		{"window off every screen: the primary screen", twoScreens(cardRect{X: -5000, Y: -5000, Width: 800, Height: 600}, false), 520, 260,
			cardRect{X: 700, Y: 422, Width: 520, Height: 260}},
		{"no main window: the primary screen", cardAnchor{Screens: twoScreens(main, true).Screens}, 520, 260,
			cardRect{X: 700, Y: 422, Width: 520, Height: 260}},
		{"no screens: left to the system", cardAnchor{Main: main, Shown: true}, 520, 260, cardRect{}},
	} {
		if got := c.anchor.bounds(c.w, c.h); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestACardOpensOverTheMainWindowAndStaysThereAsItGrows(t *testing.T) {
	r := newCardRig(t)
	got := r.ask(context.Background(), confirmCard("Q", "M"))
	w := r.window(t)
	if want := (cardRect{X: 2540, Y: 470, Width: cardWidth, Height: cardHeight}); w.at != want {
		t.Fatalf("opened at %+v, want %+v", w.at, want)
	}

	if err := r.api.FitCard(from(mainWin), 400); err == nil {
		t.Fatal("the main window sized the card")
	}
	if _, placed := w.lastPlaced(); placed {
		t.Fatal("a refused fit moved the card")
	}

	if err := r.api.FitCard(from(cardWin), 400); err != nil {
		t.Fatal(err)
	}
	if at, _ := w.lastPlaced(); at != (cardRect{X: 2540, Y: 400, Width: cardWidth, Height: 400}) {
		t.Fatalf("fitted to %+v", at)
	}
	if err := r.api.FitCard(from(cardWin), 5000); err != nil {
		t.Fatal(err)
	}
	if at, _ := w.lastPlaced(); at.Height != cardMaxHeight || at.Y != 280 {
		t.Fatalf("a long card fitted to %+v, want %d tall over the window", at, cardMaxHeight)
	}

	c, err := r.api.Card(from(cardWin))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.api.AnswerCard(from(cardWin), c.ID, 0); err != nil {
		t.Fatal(err)
	}
	wait(t, got)
	if err := r.api.FitCard(from(cardWin), 300); err == nil {
		t.Fatal("an answered card's window sized itself")
	}
}
