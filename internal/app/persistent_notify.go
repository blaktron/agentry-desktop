package app

import (
	"strconv"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// A task's turn ends with the runner's notice on the agent's event stream,
// `Task "<name>" ran: <the answer's first line>`, and the app raises a desktop
// notification from it for an agent it runs: the agent's name and the task's
// as the title, the line as the body, a click opening the agent's page. None
// is raised while the window is in front showing that agent, or while the
// person has switched them off.

// emptyAnswer is the body of a notification for a task whose answer had no
// text.
const emptyAnswer = "It ran, and its answer had no text."

// taskNotice reads a task's notice: its name and the first line of its
// answer. Any other notice is not one.
func taskNotice(text string) (task, line string, ok bool) {
	rest, ok := strings.CutPrefix(text, "Task ")
	if !ok {
		return "", "", false
	}
	quoted, err := strconv.QuotedPrefix(rest)
	if err != nil {
		return "", "", false
	}
	task, err = strconv.Unquote(quoted)
	if err != nil || task == "" {
		return "", "", false
	}
	line, ok = strings.CutPrefix(rest[len(quoted):], " ran: ")
	if !ok {
		return "", "", false
	}
	return task, line, true
}

// noticeTask raises the notification for ev when it is a task's notice of
// the agent named, on its own goroutine: a platform may be slow to answer,
// and the agent's events must keep flowing to its page.
func (p *PersistentAPI) noticeTask(name string, ev session.Event, logw *supervisorLog) {
	if ev.Kind != session.KindNotice {
		return
	}
	task, line, ok := taskNotice(ev.Text)
	if !ok || !taskNotificationsOn() || p.isViewing(name) {
		return
	}
	// Only an agent that keeps tasks has a page a click can open for them.
	a, err := p.agent(name)
	if err != nil || !a.Builtin.KeepsTime() {
		return
	}
	title := task
	if home, err := p.home(); err == nil {
		cfg, _ := persistent.LoadConfig(home, a.Name)
		title = a.DisplayName(cfg) + ": " + task
	}
	if strings.TrimSpace(line) == "" {
		line = emptyAnswer
	}
	show := p.showNotice
	if show == nil {
		show = notify.Show
	}
	go func() {
		if err := show(notify.Notice{Title: title, Body: line, Key: name}); err != nil && logw != nil {
			logw.note("desktop notification for task " + strconv.Quote(task) + " not raised: " + err.Error())
		}
	}()
}

// taskNotificationsOn is Settings → Persistent Agents' "Notify me when a
// task runs"; a settings file that cannot be read leaves it on.
func taskNotificationsOn() bool {
	st, err := settings.Load()
	return err != nil || !st.TaskNotificationsOff
}

// isViewing reports whether the window is in front showing the agent named:
// the page's report, unless the window is minimised, which a page cannot
// always tell.
func (p *PersistentAPI) isViewing(name string) bool {
	p.mu.Lock()
	viewing, minimised := p.viewing, p.minimised
	p.mu.Unlock()
	if viewing == "" || viewing != name {
		return false
	}
	if minimised == nil {
		minimised = func() bool { return p.app != nil && p.app.ctx != nil && nativeWindowMinimised() }
	}
	return !minimised()
}

// SetViewing is the page's report of what the person is looking at: the
// persistent agent whose page is shown while the window has the focus, or ""
// for anything else. A task of that agent raises no notification then: its
// answer is already in front of them.
func (p *PersistentAPI) SetViewing(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.viewing = name
}

// TaskNotifications reports whether a task's answer raises a desktop
// notification.
func (p *PersistentAPI) TaskNotifications() bool {
	return taskNotificationsOn()
}

// SetTaskNotifications switches the desktop notifications of tasks on or
// off, and answers how they now stand.
func (p *PersistentAPI) SetTaskNotifications(on bool) (bool, error) {
	st, err := settings.Load()
	if err != nil {
		return taskNotificationsOn(), err
	}
	st.TaskNotificationsOff = !on
	if err := settings.Save(st); err != nil {
		return taskNotificationsOn(), err
	}
	return on, nil
}

// PageView is the view a persistent agent's page is drawn in: one of
// settings.AgentViews.
func (p *PersistentAPI) PageView() (string, error) { return settings.AgentViewNow() }

// SetPageView keeps the person's choice of view for every persistent
// agent's page in desktop.yaml, and answers the view now kept.
func (p *PersistentAPI) SetPageView(view string) (string, error) {
	if err := settings.SetAgentView(view); err != nil {
		now, _ := settings.AgentViewNow()
		return now, err
	}
	return settings.AgentViewNow()
}
