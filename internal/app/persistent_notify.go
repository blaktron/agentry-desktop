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
// as the title, the line as the body, a click opening the agent's page. When
// the run told the person something (a reminder, as a `tell` event), that
// message raises the run's notification instead, as it comes, and the run's
// notice raises none. None is raised while the window is in front showing
// that agent, or while the person has switched them off.

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

// noticeTask raises the notification for ev when it is a task's message to
// the person or its notice, for the agent named, and keeps which run the
// agent's events are on.
func (p *PersistentAPI) noticeTask(name string, ev session.Event, logw *supervisorLog) {
	switch ev.Kind {
	case session.KindUserTurn:
		// A run begins: a task's, named by its turn, or anything else.
		task := ""
		if ev.Task != "" && !ev.Synthetic {
			task = taskName(ev.Text)
		}
		p.mu.Lock()
		if p.tasking == nil {
			p.tasking, p.told = map[string]string{}, map[string]bool{}
		}
		p.tasking[name], p.told[name] = task, false
		p.mu.Unlock()
	case session.KindTell:
		message := strings.TrimSpace(ev.Text)
		p.mu.Lock()
		task, inTask := p.tasking[name]
		if inTask && task != "" && message != "" {
			p.told[name] = true
		}
		p.mu.Unlock()
		if !inTask || task == "" || message == "" {
			return
		}
		p.raiseTask(name, task, message, logw)
	case session.KindNotice:
		task, line, ok := taskNotice(ev.Text)
		if !ok {
			return
		}
		p.mu.Lock()
		told := p.told[name]
		delete(p.told, name)
		delete(p.tasking, name)
		p.mu.Unlock()
		// The Notifications page lists every answer, whether or not a
		// notification is raised for it.
		title, ok := p.taskTitle(name, task)
		if !ok {
			return
		}
		p.app.noticed(taskEntry(name, title), false)
		if told {
			return
		}
		if strings.TrimSpace(line) == "" {
			line = emptyAnswer
		}
		p.raiseTask(name, task, line, logw)
	}
}

// taskName is a task's name, from its turn's "Task: <name>" line; "" when
// the turn names none.
func taskName(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if n, ok := strings.CutPrefix(l, "Task: "); ok && strings.TrimSpace(n) != "" {
			return strings.TrimSpace(n)
		}
	}
	return ""
}

// taskTitle is a task's notification title, the agent's name and the
// task's; false for an agent that keeps no tasks, which has no page a click
// can open for them.
func (p *PersistentAPI) taskTitle(name, task string) (string, bool) {
	a, err := p.agent(name)
	if err != nil || !a.KeepsTime() {
		return "", false
	}
	if home, err := p.home(); err == nil {
		cfg, _ := persistent.LoadConfig(home, a.Name)
		return a.DisplayName(cfg) + ": " + task, true
	}
	return task, true
}

// raiseTask raises a task's notification with body, on its own goroutine:
// a platform may be slow to answer, and the agent's events must keep
// flowing to its page.
func (p *PersistentAPI) raiseTask(name, task, body string, logw *supervisorLog) {
	title, ok := p.taskTitle(name, task)
	if !ok || !taskNotificationsOn() || p.isViewing(name) {
		return
	}
	show := p.showNotice
	if show == nil {
		show = notify.Show
	}
	go func() {
		if err := show(notify.Notice{Title: title, Body: body, Key: name}); err != nil && logw != nil {
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

// The confirmation's words for letting the Assistant run QuickAgents
// without asking: what the agreed-run dialog stops guarding, what still
// holds, and that text reaching the Assistant can now start a run.
const (
	unaskedRunsTitle   = "Let the Assistant run QuickAgents without asking?"
	unaskedRunsMessage = "The Assistant can start a run of any QuickAgent installed on this computer without asking you first: any version, with any answers it gives the QuickAgent's questions, on any repository or file address, using the key saved in Agentry for each address's host, and with files from its share folder.\n\n" +
		"Mail, documents and web pages the Assistant reads can lead it to start a run you did not ask for.\n\n" +
		"What still holds: installing a QuickAgent and adding or removing a schedule still ask you. Each run is an ordinary run under what the QuickAgent was installed with, listed in Recent runs and writing to your output folder. Agentry Helper still asks before its runs.\n\n" +
		"You can turn this off at any time in Settings → Persistent Agents.\n\nTurn it on?"
)

// assistantRunsWithoutAsking reads the switch. A settings file that cannot
// be read is off: nothing stops asking by failing.
func assistantRunsWithoutAsking() bool {
	st, err := settings.Load()
	return err == nil && st.AssistantRunsWithoutAsking
}

// AssistantRunsWithoutAsking reports whether the Assistant starts runs of
// installed QuickAgents without the agreed-run dialog.
func (p *PersistentAPI) AssistantRunsWithoutAsking() bool {
	return assistantRunsWithoutAsking()
}

// SetAssistantRunsWithoutAsking turns that on or off, and answers how it now
// stands. The renderer is not a principal, so on is a native confirmation
// the page cannot answer, and a No leaves it off; off is immediate.
func (p *PersistentAPI) SetAssistantRunsWithoutAsking(on bool) (bool, error) {
	if on && !assistantRunsWithoutAsking() {
		if err := p.app.askConsent(unaskedRunsTitle, unaskedRunsMessage); err != nil {
			return false, err
		}
	}
	if err := settings.SetAssistantRunsWithoutAsking(on); err != nil {
		return assistantRunsWithoutAsking(), err
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
