package app

// Background tasks, after OpenIDEA's workbench/tasks.ts: the status bar shows
// the task that works now with its progress and how many wait behind it, and
// its popup lists every running task with a Cancel. A task that ends stays for
// a moment so its full bar is seen; a cancelled one goes at once, without it.

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	taskDoneLinger = 600 * time.Millisecond
	taskBarWidth   = 64
	taskBarHeight  = 4
)

// BackgroundTask is one entry of the status bar's task surface.
type BackgroundTask struct {
	ID    string
	Title string
	// Message is the task's state line (a download's megabytes).
	Message string
	// Fraction is 0..1, or less than 0 while the task cannot say.
	Fraction  float64
	Done      bool
	StartedAt time.Time
	// Cancel stops the task. A task without one is not stopped by the user.
	Cancel func()
	// Uncancellable is why a working task offers no Cancel.
	Uncancellable string
	// Cancelling is set once Cancel ran: the task shows "Cancelling…" until
	// its producer ends it.
	Cancelling bool
	// Queued marks a task waiting for the ones ahead of it.
	Queued bool
}

// upsertTask adds or updates a task, as tasks.ts's upsertTask does: a task
// that ends lingers for a moment, and one cancelled first leaves at once.
func (m *Model) upsertTask(t BackgroundTask) {
	for i := range m.tasks {
		cur := &m.tasks[i]
		if cur.ID != t.ID {
			continue
		}
		cancelled := cur.Cancelling
		t.StartedAt = cur.StartedAt
		t.Cancelling = cancelled
		if cancelled {
			// A cancelled task keeps the message it had, as tasks.ts does, so
			// its late progress does not overwrite "Cancelling…".
			t.Message = cur.Message
		}
		m.tasks[i] = t
		if t.Done {
			m.finishTask(t.ID, cancelled)
		}
		m.invalidate()
		return
	}
	if t.Done {
		return
	}
	if t.StartedAt.IsZero() {
		t.StartedAt = time.Now()
	}
	m.tasks = append(m.tasks, t)
	m.invalidate()
}

// finishTask ends a task: a cancelled one goes now, any other after the
// linger that lets its finished bar be seen.
func (m *Model) finishTask(id string, cancelled bool) {
	if cancelled {
		m.removeTask(id)
		return
	}
	time.AfterFunc(taskDoneLinger, func() {
		m.post(func() { m.removeTask(id) })
	})
}

// removeTask drops a task from the surface.
func (m *Model) removeTask(id string) {
	kept := m.tasks[:0]
	for _, t := range m.tasks {
		if t.ID != id {
			kept = append(kept, t)
		}
	}
	m.tasks = kept
	m.invalidate()
}

// cancelTask asks task id to stop: it shows "Cancelling…" until its producer
// ends it. A task with no Cancel, or one already cancelling, ignores it.
func (m *Model) cancelTask(id string) {
	for i := range m.tasks {
		t := &m.tasks[i]
		if t.ID != id || t.Done || t.Cancelling || t.Cancel == nil {
			continue
		}
		t.Cancelling = true
		t.Message = "Cancelling…"
		cancel := t.Cancel
		m.invalidate()
		cancel()
		return
	}
}

// currentTask is the task the status bar shows and the running count behind
// it: the newest task that works, or the newest queued one, or a finished
// task still lingering.
func (m *Model) currentTask() (BackgroundTask, int) {
	var current *BackgroundTask
	behind := 0
	for i := range m.tasks {
		t := &m.tasks[i]
		if t.Done {
			continue
		}
		behind++
		if !t.Queued {
			current = t
		}
	}
	if current == nil && behind > 0 {
		current = &m.tasks[len(m.tasks)-1]
	}
	if current == nil && len(m.tasks) > 0 {
		current = &m.tasks[len(m.tasks)-1]
	}
	if current == nil {
		return BackgroundTask{}, 0
	}
	return *current, max(0, behind-1)
}

// taskStatus is the status bar's task button: the working task's title, its
// progress and the count behind it; clicking lists every running task.
func (m *Model) taskStatus(c *ui.Context, p *ui.Theme, oi oiColors) {
	cur, behind := m.currentTask()
	if cur.ID == "" {
		return
	}
	b := ui.ButtonBase(c.Key("status:tasks")).Height(oiStatusBarHeight-8).
		Padding(0, 6).Radius(4).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(oi.toolbarHover)
	}
	reason := cur.Message
	if reason == "" {
		reason = cur.Title
	}
	b.Tooltip(reason)
	b.Children(func() {
		ui.Text(c, cur.Title).FontSize(13).TextColor(oi.textMuted).SingleLine().Ellipsis("…").MaxWidth(180)
		m.taskBar(c, p, oi, cur.Fraction, taskBarWidth)
		if behind > 0 {
			ui.Text(c, fmt.Sprintf("+%d", behind)).FontSize(12).TextColor(oi.textFaint).SingleLine()
		}
	})
	open := m.tasksOpen
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		panel.Width(320).Padding(8).Background(oi.popup).
			BorderWidth(1).BorderColor(oi.borderStrong).Radius(8)
		panel.Children(func() {
			ui.Text(c, "Background Tasks").FontSize(12.5).Bold().
				TextColor(oi.text).SingleLine().Padding(0, 2, 6, 2)
			if len(m.tasks) == 0 {
				ui.Text(c, "No background tasks").FontSize(11.5).
					TextColor(oi.textMuted).SingleLine()
			}
			for i := range m.tasks {
				m.taskRow(c, p, oi, &m.tasks[i])
			}
		})
	})
	if open != m.tasksOpen {
		m.tasksOpen = open
		m.invalidate()
	}
}

// taskRow is one task in the popup: its title, percentage and Cancel, the
// progress bar, and its message.
func (m *Model) taskRow(c *ui.Context, p *ui.Theme, oi oiColors, t *BackgroundTask) {
	ui.Column(c.Key("task:"+t.ID)).Padding(4, 2).Gap(3).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, t.Title).FontSize(12).TextColor(oi.text).SingleLine().
				Ellipsis("…").Grow(1)
			if t.Fraction >= 0 {
				ui.Text(c, fmt.Sprintf("%d%%", int(t.Fraction*100+0.5))).FontSize(10.5).
					TextColor(oi.textMuted).SingleLine()
			}
			switch {
			case t.Cancelling:
				ui.Text(c, "Cancelling…").FontSize(10.5).TextColor(oi.textMuted).SingleLine()
			case t.Cancel != nil && !t.Done:
				ui.Button(c, "Cancel").OnClick(func() { m.cancelTask(t.ID) })
			case t.Uncancellable != "":
				ui.Text(c, t.Uncancellable).FontSize(10.5).TextColor(oi.textMuted).
					SingleLine().Ellipsis("…").MaxWidth(150)
			}
		})
		m.taskBar(c, p, oi, t.Fraction, 0)
		if t.Message != "" {
			ui.Text(c, t.Message).FontSize(10.5).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		}
	})
}

// taskBar draws a task's progress over its track; width 0 fills the row.
func (m *Model) taskBar(c *ui.Context, p *ui.Theme, oi oiColors, fraction float64, width float32) {
	frac := fraction
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	track := ui.Row(c).Height(taskBarHeight).Radius(taskBarHeight / 2).Background(oi.border)
	if width > 0 {
		track.Width(width)
	} else {
		track.Grow(1)
	}
	track.Children(func() {
		if frac > 0 {
			fill := ui.Row(c).Height(taskBarHeight).Radius(taskBarHeight / 2).
				Background(oi.accent)
			if width > 0 {
				fill.Width(width * float32(frac))
			} else {
				fill.Grow(float32(frac))
			}
		}
		if frac < 1 {
			rest := ui.Row(c).Height(taskBarHeight)
			if width > 0 {
				rest.Width(width * float32(1-frac))
			} else {
				rest.Grow(float32(1 - frac))
			}
		}
	})
}
