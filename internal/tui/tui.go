package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/popoffvg/background/internal/launcher"
	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
)

var (
	dotOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●")
	dotFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("●")
	dotRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render("●")
	dotUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("●")

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("69"))

	selectedItemStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle()

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	matchStyle = lipgloss.NewStyle().Reverse(true)

	// currentMatchStyle highlights the active match distinctly from the rest.
	currentMatchStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("214"))

	// cursorLineStyle marks the current log line; selectLineStyle marks lines in
	// the visual selection band.
	cursorLineStyle = lipgloss.NewStyle().Background(lipgloss.Color("238"))
	selectLineStyle = lipgloss.NewStyle().Background(lipgloss.Color("24"))
)

const ansiReset = "\x1b[0m"

func taskDot(category string) string {
	switch category {
	case "ok":
		return dotOK
	case "fail":
		return dotFailed
	case "running":
		return dotRunning
	default:
		return dotUnknown
	}
}

func taskTitle(t store.Task) string {
	cmd := t.Label()
	if len(cmd) > 38 {
		cmd = cmd[:35] + "..."
	}
	return cmd
}

func startTimeLabel(startedAt string) string {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "?"
	}
	t = t.Local()
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func relativeAge(startedAt string) string {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

type tickMsg time.Time
type tasksMsg []store.Task

type model struct {
	viewport   viewport.Model
	tasks      []store.Task
	cursor     int
	width      int
	height     int
	followMode bool
	tail       *logTail
	status     string
	ready      bool
	focusLeft  bool

	searchInput textinput.Model
	searching   bool       // input bar active (typing)
	searchQuery string     // active committed query
	matches     []matchPos // spans of searchQuery in current content
	matchIndex  int        // current match (0-based)

	renameInput textinput.Model
	renaming    bool // rename input bar active (typing)

	menuOpen  bool // action menu modal active
	menuIndex int  // highlighted menu item

	logCursor    int  // current line index in the log pane
	selecting    bool // visual-line select mode active
	selectAnchor int  // anchor line for visual selection
	logMissing   bool // selected task's out.log was removed

	prevState map[string]string // taskID → last seen StateCategory
	seeded    bool              // first tasksMsg seeds prevState without notifying
}

// menuItem is one action-menu entry.
type menuItem struct {
	accel string // accelerator letter
	label string
}

// menuItems lists the action-menu entries in display order.
var menuItems = []menuItem{
	{"r", "Repeat"},
	{"t", "Transform"},
	{"n", "Rename"},
	{"y", "Copy path"},
	{"k", "Kill"},
	{"d", "Delete"},
}

func newModel() model {
	vp := viewport.New(0, 0)
	ti := textinput.New()
	ti.Placeholder = "search log"
	ti.Prompt = "/"
	ri := textinput.New()
	ri.Placeholder = "task name"
	ri.Prompt = "rename: "
	return model{
		viewport:    vp,
		followMode:  true,
		focusLeft:   true,
		searchInput: ti,
		renameInput: ri,
		prevState:   map[string]string{},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tick(), loadTasks())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func loadTasks() tea.Cmd {
	return func() tea.Msg {
		tasks, _ := store.ListTasks()
		return tasksMsg(tasks)
	}
}

func (m model) selectedTask() (store.Task, bool) {
	if len(m.tasks) == 0 || m.cursor < 0 || m.cursor >= len(m.tasks) {
		return store.Task{}, false
	}
	return m.tasks[m.cursor], true
}

// markersFor returns the ANSI pre/post strings that style.Render wraps around
// content, by rendering a sentinel and splitting on it.
func markersFor(style lipgloss.Style) (string, string) {
	rendered := style.Render("\x00")
	parts := strings.SplitN(rendered, "\x00", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "\x1b[7m", "\x1b[0m"
}

// searchMarkers returns the markers for non-current matches (reverse video).
func searchMarkers() (string, string) { return markersFor(matchStyle) }

// currentMatchMarkers returns the markers for the active match (distinct color).
func currentMatchMarkers() (string, string) { return markersFor(currentMatchStyle) }

// logLines returns the current log content split into display lines.
func (m *model) logLines() []string {
	if m.tail == nil {
		return nil
	}
	return splitLines(m.tail.content)
}

// clampLogCursor keeps logCursor and selectAnchor within the current line count.
func (m *model) clampLogCursor() {
	n := len(m.logLines())
	if n == 0 {
		m.logCursor = 0
		m.selectAnchor = 0
		return
	}
	if m.logCursor > n-1 {
		m.logCursor = n - 1
	}
	if m.logCursor < 0 {
		m.logCursor = 0
	}
	if m.selectAnchor > n-1 {
		m.selectAnchor = n - 1
	}
	if m.selectAnchor < 0 {
		m.selectAnchor = 0
	}
}

// applySearch recomputes matches against the current tail content, then
// refreshes the decorated viewport view.
func (m *model) applySearch() {
	content := ""
	if m.tail != nil {
		content = m.tail.content
	}
	if m.searchQuery == "" {
		m.matches = nil
	} else {
		m.matches = findMatches(content, m.searchQuery)
		if m.matchIndex >= len(m.matches) {
			m.matchIndex = 0
		}
	}
	m.refreshLogView()
}

// refreshLogView rebuilds the viewport content: byte-level search highlighting
// first, then per-line cursor/selection backgrounds. Line decoration persists
// across embedded ANSI resets so search highlights stay visible.
func (m *model) refreshLogView() {
	content := ""
	if m.tail != nil {
		content = m.tail.content
	}
	// 1. search highlight on raw bytes
	if m.searchQuery != "" && len(m.matches) > 0 {
		pre, post := searchMarkers()
		cpre, cpost := currentMatchMarkers()
		content = highlightMatches(content, m.matches, pre, post, m.matchIndex, cpre, cpost)
	}
	// 2. per-line cursor + selection decoration
	lines := splitLines(content)
	m.clampLogCursor()
	cursorBG, _ := markersFor(cursorLineStyle)
	selBG, _ := markersFor(selectLineStyle)
	from, to := 1, 0 // disabled band
	if m.selecting {
		from, to = m.selectAnchor, m.logCursor
		if from > to {
			from, to = to, from
		}
	}
	cur := m.logCursor
	if !m.focusLeft {
		// only show the cursor line highlight when the log pane is focused
		lines = decorateLines(lines, from, to, cur, selBG, cursorBG, ansiReset)
	} else if m.selecting {
		lines = decorateLines(lines, from, to, -1, selBG, "", ansiReset)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// ensureCursorVisible scrolls the viewport so logCursor is on screen.
func (m *model) ensureCursorVisible() {
	h := m.viewport.Height
	if h <= 0 {
		return
	}
	if m.logCursor < m.viewport.YOffset {
		m.viewport.SetYOffset(m.logCursor)
	} else if m.logCursor >= m.viewport.YOffset+h {
		m.viewport.SetYOffset(m.logCursor - h + 1)
	}
}

// moveCursor changes logCursor by delta (clamped) and keeps it visible.
// Moving up disengages follow.
func (m *model) moveCursor(delta int) {
	n := len(m.logLines())
	if n == 0 {
		return
	}
	prev := m.logCursor
	m.logCursor += delta
	if m.logCursor < 0 {
		m.logCursor = 0
	}
	if m.logCursor > n-1 {
		m.logCursor = n - 1
	}
	if m.logCursor < prev {
		m.followMode = false
	}
	m.ensureCursorVisible()
	m.refreshLogView()
}

// jumpToMatch moves the cursor to the current match's line and scrolls it into
// view (disengaging follow so navigation isn't yanked to the bottom).
func (m *model) jumpToMatch() {
	if len(m.matches) == 0 || m.tail == nil {
		return
	}
	if m.matchIndex < 0 || m.matchIndex >= len(m.matches) {
		m.matchIndex = 0
	}
	line := lineOf(m.tail.content, m.matches[m.matchIndex].Start)
	m.logCursor = line
	m.followMode = false
	m.clampLogCursor()
	m.ensureCursorVisible()
	m.refreshLogView()
}

type transformDoneMsg struct {
	tmpFile string
	srcLog  string
	srcID   string
	err     error
}

// startTransform writes a template to a temp file and opens $EDITOR on it via
// tea.ExecProcess. On editor exit it emits a transformDoneMsg.
func (m *model) startTransform(task store.Task) tea.Cmd {
	tmp, err := os.CreateTemp("", "bg-transform-*.sh")
	if err != nil {
		m.status = "transform: " + err.Error()
		return nil
	}
	srcLog := task.LogPath()
	if abs, e := filepath.Abs(srcLog); e == nil {
		srcLog = abs
	}
	tmpl := transformTemplate(task.Meta.ID, task.DisplayCommand())
	_ = os.WriteFile(tmp.Name(), []byte(tmpl), 0o644)
	tmp.Close()
	argv := editorArgv(os.Getenv("EDITOR"))
	argv = append(argv, tmp.Name())
	c := exec.Command(argv[0], argv[1:]...)
	srcID := task.Meta.ID
	tmpName := tmp.Name()
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return transformDoneMsg{tmpFile: tmpName, srcLog: srcLog, srcID: srcID, err: err}
	})
}

// menuItemDisabled reports whether an action is unavailable for the task's
// current state (Kill needs running; Delete needs not-running).
func menuItemDisabled(accel string, task store.Task) bool {
	running := task.State() == "running"
	switch accel {
	case "k":
		return !running
	case "d":
		return running
	}
	return false
}

// runMenuAction executes the action identified by accel against the selected
// task. It closes the menu and returns any follow-up command (e.g. transform's
// editor exec or a task reload). For Rename it leaves the rename input active.
func (m *model) runMenuAction(accel string) tea.Cmd {
	m.menuOpen = false
	task, ok := m.selectedTask()
	if !ok {
		return nil
	}
	switch accel {
	case "r":
		newID, err := launcher.Relaunch(task.Meta)
		if err != nil {
			m.status = "repeat failed: " + err.Error()
			return nil
		}
		m.status = "repeated → " + newID
		return loadTasks()
	case "t":
		return m.startTransform(task)
	case "n":
		m.renaming = true
		m.renameInput.SetValue(task.Meta.Alias)
		m.renameInput.CursorEnd()
		return tea.Batch(m.renameInput.Focus(), textinput.Blink)
	case "y":
		path := task.LogPath()
		if abs, e := filepath.Abs(path); e == nil {
			path = abs
		}
		if _, err := copyToClipboard(path); err != nil {
			m.status = "no clipboard tool found"
		} else {
			m.status = "copied " + path
		}
		return nil
	case "k":
		if task.State() == "running" {
			if err := runner.Kill(task.Meta.Pid); err != nil {
				m.status = "kill failed: " + err.Error()
			} else {
				m.status = "sent kill signal"
			}
		} else {
			m.status = "task not running"
		}
		return nil
	case "d":
		if task.State() == "running" {
			m.status = "cannot delete running task (use Kill first)"
			return nil
		}
		if err := os.RemoveAll(task.Dir); err != nil {
			m.status = "delete failed: " + err.Error()
			return nil
		}
		m.status = "task deleted"
		return loadTasks()
	}
	return nil
}

// notifyMsg is a no-op message returned after writing notification escapes.
type notifyMsg struct{}

// detectFinishedNotifications updates the prev-state map from the current task
// list and returns a tea.Cmd that emits OSC-777 notifications for any
// running→finished transitions. The first call only seeds the map (no notify),
// so opening `bg list` over already-finished tasks doesn't spam. Returns nil if
// notifications are disabled/ungated or there's nothing to emit.
func (m *model) detectFinishedNotifications() tea.Cmd {
	if m.prevState == nil {
		m.prevState = map[string]string{}
	}

	// Seed on first load: record categories, never notify.
	if !m.seeded {
		for _, t := range m.tasks {
			m.prevState[t.Meta.ID] = t.StateCategory()
		}
		m.seeded = true
		return nil
	}

	var seqs []string
	gated := notifyAllowed()
	live := map[string]bool{}
	for _, t := range m.tasks {
		id := t.Meta.ID
		live[id] = true
		next := t.StateCategory()
		prev, seen := m.prevState[id]
		m.prevState[id] = next
		if !seen {
			continue // newly observed task: don't notify on first sight
		}
		if gated && shouldNotify(prev, next) {
			seqs = append(seqs, ghosttyNotify("bg", t.Label()+" — "+t.State()))
		}
	}
	// Drop entries for tasks that disappeared (deleted) to bound the map.
	for id := range m.prevState {
		if !live[id] {
			delete(m.prevState, id)
		}
	}

	if len(seqs) == 0 {
		return nil
	}
	joined := strings.Join(seqs, "")
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString(joined)
		return notifyMsg{}
	}
}

// syncTail ensures m.tail points at the selected task's log and the viewport
// reflects current content. Creates a fresh tail when the selection changed,
// otherwise polls for appended bytes. Auto-scrolls to bottom only when
// followMode is on and the viewport was already at the bottom.
func (m *model) syncTail() {
	task, ok := m.selectedTask()
	if !ok {
		m.tail = nil
		m.logMissing = false
		m.viewport.SetContent("")
		return
	}
	m.logMissing = !task.LogExists()
	path := task.LogPath()
	if m.tail == nil || m.tail.path != path {
		// selection changed: fresh tail. Clear any active search so highlights
		// and match navigation don't carry across to a different log. Reset the
		// line cursor and exit any visual selection.
		m.searching = false
		m.searchQuery = ""
		m.matches = nil
		m.matchIndex = 0
		m.selecting = false
		m.selectAnchor = 0
		m.tail = newLogTail(path)
		n := len(m.logLines())
		if n > 0 {
			m.logCursor = n - 1
		} else {
			m.logCursor = 0
		}
		m.refreshLogView()
		if m.followMode {
			m.viewport.GotoBottom()
		}
		return
	}
	if m.tail.poll() {
		atBottom := m.viewport.AtBottom()
		m.clampLogCursor()
		// Follow pins the cursor to the last line, unless visually selecting
		// (selection freezes the cursor so the range doesn't drift).
		if m.followMode && !m.selecting {
			if n := len(m.logLines()); n > 0 {
				m.logCursor = n - 1
			}
		}
		m.applySearch() // recompute matches + redraw cursor/selection
		if m.followMode && atBottom {
			m.viewport.GotoBottom()
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.updatePaneSizes()

	case tickMsg:
		// Clear transient status messages so they vanish after ~1s.
		m.status = ""
		cmds = append(cmds, tick(), loadTasks())
		m.syncTail()

	case tasksMsg:
		m.tasks = []store.Task(msg)
		if m.cursor >= len(m.tasks) && len(m.tasks) > 0 {
			m.cursor = len(m.tasks) - 1
		}
		if c := m.detectFinishedNotifications(); c != nil {
			cmds = append(cmds, c)
		}
		m.syncTail()

	case transformDoneMsg:
		defer os.Remove(msg.tmpFile)
		if msg.err != nil {
			m.status = "transform: editor failed"
			return m, nil
		}
		data, _ := os.ReadFile(msg.tmpFile)
		command := stripCommandBuffer(string(data))
		if command == "" {
			m.status = "transform cancelled"
			return m, nil
		}
		display := []string{"-c", command}
		newID, err := launcher.LaunchWithEnv("sh", []string{"-c", command}, display, []string{"FILE=" + msg.srcLog}, msg.srcID)
		if err != nil {
			m.status = "transform failed: " + err.Error()
			return m, nil
		}
		m.status = "transform → " + newID
		return m, loadTasks()

	case tea.KeyMsg:
		// Search input bar active: intercept keys before normal handling.
		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				m.searchQuery = ""
				m.matches = nil
				m.matchIndex = 0
				m.applySearch()
				m.status = ""
				return m, tea.Batch(cmds...)
			case "enter":
				m.searchQuery = m.searchInput.Value()
				m.searching = false
				m.searchInput.Blur()
				m.followMode = false
				// first match at/after current viewport top, else wrap to 0
				top := m.viewport.YOffset
				m.matchIndex = 0
				m.applySearch()
				if len(m.matches) == 0 {
					m.status = "no matches"
				} else {
					for i, mp := range m.matches {
						if lineOf(m.tail.content, mp.Start) >= top {
							m.matchIndex = i
							break
						}
					}
					m.applySearch()
					m.jumpToMatch()
					m.status = fmt.Sprintf("match %d/%d", m.matchIndex+1, len(m.matches))
				}
				return m, tea.Batch(cmds...)
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}

		// Rename input bar active: intercept keys before normal handling.
		if m.renaming {
			switch msg.String() {
			case "esc":
				m.renaming = false
				m.renameInput.Blur()
				m.status = ""
				return m, tea.Batch(cmds...)
			case "enter":
				m.renaming = false
				m.renameInput.Blur()
				if task, ok := m.selectedTask(); ok {
					alias := strings.TrimSpace(m.renameInput.Value())
					if err := store.SetAlias(task.Dir, alias); err != nil {
						m.status = "rename failed: " + err.Error()
					} else if alias == "" {
						m.status = "alias cleared"
					} else {
						m.status = "renamed to " + alias
					}
					cmds = append(cmds, loadTasks())
				}
				return m, tea.Batch(cmds...)
			default:
				var cmd tea.Cmd
				m.renameInput, cmd = m.renameInput.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}

		// Action menu modal: intercept keys before normal handling.
		if m.menuOpen {
			switch msg.String() {
			case "esc", "q":
				m.menuOpen = false
				return m, tea.Batch(cmds...)
			case "up":
				if m.menuIndex > 0 {
					m.menuIndex--
				}
				return m, tea.Batch(cmds...)
			case "down":
				if m.menuIndex < len(menuItems)-1 {
					m.menuIndex++
				}
				return m, tea.Batch(cmds...)
			case "enter":
				item := menuItems[m.menuIndex]
				if task, ok := m.selectedTask(); ok && menuItemDisabled(item.accel, task) {
					switch item.accel {
					case "k":
						m.status = "task not running"
					case "d":
						m.status = "cannot delete running task (use Kill first)"
					}
					m.menuOpen = false
					return m, tea.Batch(cmds...)
				}
				cmds = append(cmds, m.runMenuAction(item.accel))
				return m, tea.Batch(cmds...)
			default:
				// accelerator letter executes its item directly
				for i, it := range menuItems {
					if msg.String() == it.accel {
						if task, ok := m.selectedTask(); ok && menuItemDisabled(it.accel, task) {
							switch it.accel {
							case "k":
								m.status = "task not running"
							case "d":
								m.status = "cannot delete running task (use Kill first)"
							}
							m.menuOpen = false
							return m, tea.Batch(cmds...)
						}
						m.menuIndex = i
						cmds = append(cmds, m.runMenuAction(it.accel))
						return m, tea.Batch(cmds...)
					}
				}
				return m, tea.Batch(cmds...)
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "tab":
			m.focusLeft = !m.focusLeft

		case "a":
			if _, ok := m.selectedTask(); ok {
				m.menuOpen = true
				m.menuIndex = 0
			}

		case "enter":
			if m.focusLeft {
				if _, ok := m.selectedTask(); ok {
					m.menuOpen = true
					m.menuIndex = 0
				}
			}

		case "h":
			m.focusLeft = true

		case "l":
			m.focusLeft = false

		case "left":
			if !m.focusLeft {
				m.viewport.ScrollLeft(4)
			}

		case "right":
			if !m.focusLeft {
				m.viewport.ScrollRight(4)
			}

		case "g", "home":
			if !m.focusLeft {
				m.logCursor = 0
				m.followMode = false
				m.ensureCursorVisible()
				m.refreshLogView()
			}

		case "G", "end":
			if !m.focusLeft {
				if n := len(m.logLines()); n > 0 {
					m.logCursor = n - 1
				}
				m.followMode = true
				m.viewport.GotoBottom()
				m.refreshLogView()
			}

		case "pgup", "ctrl+b":
			if !m.focusLeft {
				m.moveCursor(-m.viewport.Height)
			}

		case "pgdown", "ctrl+f":
			if !m.focusLeft {
				m.moveCursor(m.viewport.Height)
			}

		case "ctrl+u":
			if !m.focusLeft {
				m.moveCursor(-m.viewport.Height / 2)
			}

		case "ctrl+d":
			if !m.focusLeft {
				m.moveCursor(m.viewport.Height / 2)
			}

		case "up", "k":
			if m.focusLeft {
				if m.cursor > 0 {
					m.cursor--
					m.syncTail()
				}
			} else {
				m.moveCursor(-1)
			}

		case "down", "j":
			if m.focusLeft {
				if m.cursor < len(m.tasks)-1 {
					m.cursor++
					m.syncTail()
				}
			} else {
				m.moveCursor(1)
			}

		case "V":
			if !m.focusLeft {
				if m.selecting {
					m.selecting = false
				} else {
					m.selecting = true
					m.selectAnchor = m.logCursor
				}
				m.refreshLogView()
			}

		case "K":
			if m.focusLeft {
				// Kill selected task
				if task, ok := m.selectedTask(); ok {
					if task.State() == "running" {
						if err := runner.Kill(task.Meta.Pid); err != nil {
							m.status = "kill failed: " + err.Error()
						} else {
							m.status = "sent kill signal"
						}
					} else {
						m.status = "task not running"
					}
				}
			}

		case "d":
			if m.focusLeft {
				if task, ok := m.selectedTask(); ok {
					state := task.State()
					if state == "running" {
						m.status = "cannot delete running task (use K to kill first)"
					} else {
						if err := os.RemoveAll(task.Dir); err != nil {
							m.status = "delete failed: " + err.Error()
						} else {
							m.status = "task deleted"
							cmds = append(cmds, loadTasks())
						}
					}
				}
			}

		case "y":
			// Copy log line(s): the visual selection if active, else the cursor
			// line. Works in either pane (operates on the selected task's log).
			lines := m.logLines()
			if len(lines) == 0 {
				m.status = "nothing to copy"
				break
			}
			var text, msgStr string
			if m.selecting {
				a, b := m.selectAnchor, m.logCursor
				if a > b {
					a, b = b, a
				}
				text = selectedText(lines, a, b)
				msgStr = fmt.Sprintf("copied %d lines", b-a+1)
				m.selecting = false
				m.refreshLogView()
			} else {
				text = selectedText(lines, m.logCursor, m.logCursor)
				msgStr = "copied line"
			}
			if _, err := copyToClipboard(text); err != nil {
				m.status = "no clipboard tool found"
			} else {
				m.status = msgStr
			}

		case "Y":
			// Copy the selected task's log path. Works in either pane.
			if task, ok := m.selectedTask(); ok {
				path := task.LogPath()
				if abs, e := filepath.Abs(path); e == nil {
					path = abs
				}
				if _, err := copyToClipboard(path); err != nil {
					m.status = "no clipboard tool found"
				} else {
					m.status = "copied " + path
				}
			}

		case "x":
			// Transform selected task's output into a new task. Either pane.
			if task, ok := m.selectedTask(); ok {
				return m, m.startTransform(task)
			}

		case "r":
			// Rename (alias) the selected task. Either pane.
			if task, ok := m.selectedTask(); ok {
				m.renaming = true
				m.renameInput.SetValue(task.Meta.Alias)
				m.renameInput.CursorEnd()
				cmds = append(cmds, m.renameInput.Focus(), textinput.Blink)
				return m, tea.Batch(cmds...)
			}

		case "/":
			if !m.focusLeft {
				m.searching = true
				m.searchInput.SetValue("")
				m.followMode = false
				cmds = append(cmds, m.searchInput.Focus(), textinput.Blink)
				return m, tea.Batch(cmds...)
			}

		case "n":
			if !m.focusLeft && m.searchQuery != "" && len(m.matches) > 0 {
				m.matchIndex = (m.matchIndex + 1) % len(m.matches)
				m.applySearch()
				m.jumpToMatch()
				m.status = fmt.Sprintf("match %d/%d", m.matchIndex+1, len(m.matches))
			}

		case "N":
			if !m.focusLeft && m.searchQuery != "" && len(m.matches) > 0 {
				m.matchIndex = (m.matchIndex - 1 + len(m.matches)) % len(m.matches)
				m.applySearch()
				m.jumpToMatch()
				m.status = fmt.Sprintf("match %d/%d", m.matchIndex+1, len(m.matches))
			}

		case "esc":
			if !m.focusLeft {
				if m.selecting {
					m.selecting = false
					m.refreshLogView()
				} else {
					m.searchQuery = ""
					m.matches = nil
					m.matchIndex = 0
					m.applySearch()
					m.status = ""
				}
			}

		default:
			// Arrow/movement keys are handled above; ignore other keys in the log
			// pane (no raw viewport scroll — the line cursor drives scrolling).
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp {
			m.followMode = false
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updatePaneSizes() {
	leftWidth := m.width * 35 / 100
	rightWidth := m.width - leftWidth - 4
	contentHeight := m.height - 3

	m.viewport.Width = rightWidth - 2
	m.viewport.Height = contentHeight - 2
	m.viewport.SetHorizontalStep(4)
	_ = leftWidth
}

func (m model) renderList() string {
	leftWidth := m.width*35/100 - 2
	contentHeight := m.height - 5

	if len(m.tasks) == 0 {
		return lipgloss.NewStyle().
			Width(leftWidth).
			Height(contentHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No background tasks\nRun: bg -- <cmd>")
	}

	var lines []string
	// Title
	title := lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("Background Tasks")
	lines = append(lines, title)

	maxVisible := contentHeight - 2
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.tasks) {
		end = len(m.tasks)
	}

	for i := start; i < end; i++ {
		t := m.tasks[i]
		state := t.State()
		dot := taskDot(t.StateCategory())
		title := taskTitle(t)
		age := relativeAge(t.Meta.StartedAt)
		startTime := startTimeLabel(t.Meta.StartedAt)
		desc := dimStyle.Render(fmt.Sprintf("  %s  %s  %s", state, startTime, age))

		line := fmt.Sprintf("%s %s\n%s", dot, title, desc)
		if i == m.cursor {
			line = selectedItemStyle.Width(leftWidth).Render(line)
		} else {
			line = normalItemStyle.Width(leftWidth).Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// menuBoxStyle and menu item styles for the action modal.
var (
	menuBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("69")).
			Padding(1, 2)

	menuSelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("214"))

	menuDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// renderMenu builds the centered action-menu modal overlay.
func (m model) renderMenu() string {
	task, _ := m.selectedTask()
	label := task.Label()
	if len(label) > 30 {
		label = label[:27] + "..."
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Actions — "+label))
	lines = append(lines, "")
	for i, it := range menuItems {
		text := fmt.Sprintf("[%s] %s", it.accel, it.label)
		switch {
		case menuItemDisabled(it.accel, task):
			if i == m.menuIndex {
				text = menuSelStyle.Render(text)
			} else {
				text = menuDisabledStyle.Render(text)
			}
		case i == m.menuIndex:
			text = menuSelStyle.Render(text)
		}
		lines = append(lines, text)
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("↑/↓ select · enter run · esc close"))

	box := menuBoxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.menuOpen {
		return m.renderMenu()
	}

	leftWidth := m.width * 35 / 100
	rightWidth := m.width - leftWidth - 4

	leftContent := m.renderList()
	leftPane := paneStyle.Width(leftWidth - 2).Render(leftContent)
	if m.focusLeft {
		leftPane = activePaneStyle.Width(leftWidth - 2).Render(leftContent)
	}

	rightContent := m.viewport.View()
	if m.logMissing {
		rightContent = lipgloss.NewStyle().
			Width(rightWidth-2).
			Height(m.height-5).
			Align(lipgloss.Center, lipgloss.Center).
			Render(dimStyle.Render("— output was removed —"))
	} else if m.tail == nil || m.tail.content == "" {
		rightContent = lipgloss.NewStyle().
			Width(rightWidth-2).
			Height(m.height-5).
			Align(lipgloss.Center, lipgloss.Center).
			Render("Select a task to view logs")
	}

	rightPane := paneStyle.Width(rightWidth - 2).Render(rightContent)
	if !m.focusLeft {
		rightPane = activePaneStyle.Width(rightWidth - 2).Render(rightContent)
	}

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	followIndicator := ""
	if m.followMode {
		followIndicator = " [FOLLOW]"
	}

	var statusLine string
	switch {
	case m.searching:
		statusLine = m.searchInput.View() + "    enter search · esc cancel"
	case m.renaming:
		statusLine = m.renameInput.View() + "    enter save · esc cancel · empty clears"
	case m.selecting:
		statusLine = "-- VISUAL --  ↑/↓ extend · y copy · esc cancel"
	case m.status != "":
		statusLine = m.status
	default:
		if m.focusLeft {
			statusLine = "↑/↓ select · enter/a actions · l/Tab focus log · q quit"
		} else {
			statusLine = "↑/↓ move · ←/→ h-scroll · V select · y copy line · Y copy path · / search · n/N · a actions · Tab focus · q quit"
		}
		if m.searchQuery != "" && len(m.matches) > 0 {
			statusLine += fmt.Sprintf(" [match %d/%d]", m.matchIndex+1, len(m.matches))
		}
		statusLine += followIndicator
	}

	footer := helpStyle.Render(statusLine)
	return lipgloss.JoinVertical(lipgloss.Left, main, footer)
}

// Run starts the TUI.
func Run() error {
	m := newModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
