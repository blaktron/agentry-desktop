package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// The app's questions and notices are shown as cards in a window of their
// own, which Go creates for each card and closes when it is answered. Every
// bound method is reachable from the main page's script, so a question that
// page could answer would be no question at all: the card is handed only to
// the window Go opened for it, and only that window's answer counts. Which
// window made a call is attached by the webview layer to every request; page
// script cannot set it.

// cardWindowName is the name of the window a card is shown in.
const cardWindowName = "agentry-card"

// cardLoadTimeout is how long a card window has to ask for its card before
// the question goes to the system's message box instead.
const cardLoadTimeout = 15 * time.Second

// The kinds of card, which the page uses for its icon and its tone.
const (
	cardQuestion = "question"
	cardNotice   = "notice"
)

// The tones of a card's button.
const (
	tonePrimary = "primary"
	tonePlain   = ""
)

// CardButton is one of a card's answers.
type CardButton struct {
	Label string `json:"label"`
	// Tone is "primary" for the answer drawn as the main action, or empty.
	Tone string `json:"tone"`
}

// CardView is the card a card window shows.
type CardView struct {
	// ID names this card; an answer must carry it.
	ID      string       `json:"id"`
	Kind    string       `json:"kind"`
	Title   string       `json:"title"`
	Message string       `json:"message"`
	Buttons []CardButton `json:"buttons"`
	// Cancel is the button that closing the window or Escape answers, and
	// the one that has the focus when the card opens.
	Cancel int `json:"cancel"`
}

// cardWindow is a window a card is shown in.
type cardWindow interface {
	ID() uint
	Close()
}

// errNoCardWindow is ask's answer when no card window can be opened; the
// caller asks in the system's message box instead.
var errNoCardWindow = errors.New("no card window")

// CardWindowAPI shows the app's questions and notices as cards, one at a
// time, and takes each answer from the card's own window.
type CardWindowAPI struct {
	// turn holds one card at a time: a second question waits for the
	// first to be answered.
	turn chan struct{}

	mu  sync.Mutex
	cur *pendingCard
	win cardWindow

	// open creates and shows the window for a card titled title; closed
	// runs when the person closes it. Nil is no card window.
	open func(title string, closed func()) (cardWindow, error)
	// caller is the window a bound call came from, as the webview layer
	// said; false when it named none.
	caller func(ctx context.Context) (uint, bool)
	// loadTimeout is cardLoadTimeout, shorter in a test.
	loadTimeout time.Duration
}

// newCardWindowAPI is the card windows of the running application.
func newCardWindowAPI() *CardWindowAPI {
	return &CardWindowAPI{
		turn:        make(chan struct{}, 1),
		open:        nativeOpenCard,
		caller:      nativeCaller,
		loadTimeout: cardLoadTimeout,
	}
}

// cards is the app's card windows. The native dialogs go through it, as the
// application's dialogs are the application's, not a service's.
var cards = newCardWindowAPI()

// pendingCard is the card being shown and its answer.
type pendingCard struct {
	view   CardView
	answer chan int
	loaded chan struct{}
	load   sync.Once
	// closed says the person closed the window, which then needs no Close.
	closed atomic.Bool
}

func (p *pendingCard) reply(button int) {
	select {
	case p.answer <- button:
	default:
	}
}

func (p *pendingCard) markLoaded() { p.load.Do(func() { close(p.loaded) }) }

// ask shows view as a card and returns the index of the button the person
// chose. Closing the window answers view.Cancel, and so does a cancelled
// ctx, with its error. errNoCardWindow means no card was shown and the
// question is still to be asked.
func (c *CardWindowAPI) ask(ctx context.Context, view CardView) (int, error) {
	if c == nil || c.open == nil || len(view.Buttons) == 0 {
		return view.Cancel, errNoCardWindow
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.turn <- struct{}{}:
	case <-ctx.Done():
		return view.Cancel, ctx.Err()
	}
	defer func() { <-c.turn }()

	id, err := cardID()
	if err != nil {
		return view.Cancel, errNoCardWindow
	}
	view.ID = id
	if view.Cancel < 0 || view.Cancel >= len(view.Buttons) {
		view.Cancel = len(view.Buttons) - 1
	}
	p := &pendingCard{view: view, answer: make(chan int, 1), loaded: make(chan struct{})}

	c.mu.Lock()
	c.cur = p
	c.mu.Unlock()
	w, err := c.open(view.Title, func() {
		p.closed.Store(true)
		p.reply(view.Cancel)
	})
	if err != nil || w == nil {
		c.clear()
		return view.Cancel, errNoCardWindow
	}
	c.mu.Lock()
	c.win = w
	c.mu.Unlock()
	defer func() {
		c.clear()
		if !p.closed.Load() {
			w.Close()
		}
	}()

	timeout := c.loadTimeout
	if timeout <= 0 {
		timeout = cardLoadTimeout
	}
	notLoaded := time.NewTimer(timeout)
	defer notLoaded.Stop()
	loaded := p.loaded
	for {
		select {
		case button := <-p.answer:
			return button, nil
		case <-loaded:
			notLoaded.Stop()
			loaded = nil
		case <-notLoaded.C:
			if loaded != nil {
				return view.Cancel, errNoCardWindow
			}
		case <-ctx.Done():
			return view.Cancel, ctx.Err()
		}
	}
}

func (c *CardWindowAPI) clear() {
	c.mu.Lock()
	c.cur, c.win = nil, nil
	c.mu.Unlock()
}

// fromCardWindow is the card being shown, when ctx is a call from its own
// window, and a refusal otherwise.
func (c *CardWindowAPI) fromCardWindow(ctx context.Context) (*pendingCard, error) {
	var id uint
	ok := false
	if c.caller != nil && ctx != nil {
		id, ok = c.caller(ctx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur == nil || c.win == nil {
		return nil, errors.New("no card is open")
	}
	if !ok || id != c.win.ID() {
		return nil, errors.New("a card is answered only in its own window")
	}
	return c.cur, nil
}

// Card is the card the calling window shows. Only the card's own window
// gets it.
func (c *CardWindowAPI) Card(ctx context.Context) (*CardView, error) {
	p, err := c.fromCardWindow(ctx)
	if err != nil {
		return nil, err
	}
	p.markLoaded()
	v := p.view
	v.Buttons = append([]CardButton(nil), p.view.Buttons...)
	return &v, nil
}

// AnswerCard answers the card id with the button at index button. Only the
// card's own window may answer it.
func (c *CardWindowAPI) AnswerCard(ctx context.Context, id string, button int) error {
	p, err := c.fromCardWindow(ctx)
	if err != nil {
		return err
	}
	if id != p.view.ID {
		return errors.New("that card is no longer open")
	}
	if button < 0 || button >= len(p.view.Buttons) {
		return errors.New("the card has no such answer")
	}
	p.reply(button)
	return nil
}

func cardID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// confirmCard is a Yes/No question, No by default and on cancel.
func confirmCard(title, message string) CardView {
	return CardView{Kind: cardQuestion, Title: title, Message: message, Cancel: 1,
		Buttons: []CardButton{{Label: "Yes", Tone: tonePrimary}, {Label: "No", Tone: tonePlain}}}
}

// noticeCard is a notice with one OK.
func noticeCard(title, message string) CardView {
	return CardView{Kind: cardNotice, Title: title, Message: message, Cancel: 0,
		Buttons: []CardButton{{Label: "OK", Tone: tonePrimary}}}
}
