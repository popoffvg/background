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
)

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
	cmd := t.DisplayCommand()
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
}

func newModel() model {
	vp := viewport.New(0, 0)
	ti := textinput.New()
	ti.Placeholder = "search log"
	ti.Prompt = "/"
	return model{
		viewport:    vp,
		followMode:  true,
		focusLeft:   true,
		searchInput: ti,
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

// applySearch recomputes matches against the current tail content and updates
// the viewport with highlighted content (or plain content if no query).
func (m *model) applySearch() {
	content := ""
	if m.tail != nil {
		content = m.tail.content
	}
	if m.searchQuery == "" {
		m.matches = nil
		m.viewport.SetContent(content)
		return
	}
	m.matches = findMatches(content, m.searchQuery)
	if m.matchIndex >= len(m.matches) {
		m.matchIndex = 0
	}
	pre, post := searchMarkers()
	cpre, cpost := currentMatchMarkers()
	m.viewport.SetContent(highlightMatches(content, m.matches, pre, post, m.matchIndex, cpre, cpost))
}

// jumpToMatch scrolls the viewport so the current match's line is visible.
func (m *model) jumpToMatch() {
	if len(m.matches) == 0 || m.tail == nil {
		return
	}
	if m.matchIndex < 0 || m.matchIndex >= len(m.matches) {
		m.matchIndex = 0
	}
	line := lineOf(m.tail.content, m.matches[m.matchIndex].Start)
	m.viewport.SetYOffset(line)
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

// syncTail ensures m.tail points at the selected task's log and the viewport
// reflects current content. Creates a fresh tail when the selection changed,
// otherwise polls for appended bytes. Auto-scrolls to bottom only when
// followMode is on and the viewport was already at the bottom.
func (m *model) syncTail() {
	task, ok := m.selectedTask()
	if !ok {
		m.tail = nil
		m.viewport.SetContent("")
		return
	}
	path := task.LogPath()
	if m.tail == nil || m.tail.path != path {
		// selection changed: fresh tail. Clear any active search so highlights
		// and match navigation don't carry across to a different log.
		m.searching = false
		m.searchQuery = ""
		m.matches = nil
		m.matchIndex = 0
		m.tail = newLogTail(path)
		m.viewport.SetContent(m.tail.content)
		if m.followMode {
			m.viewport.GotoBottom()
		}
		return
	}
	if m.tail.poll() {
		atBottom := m.viewport.AtBottom()
		if m.searchQuery != "" {
			// recompute matches/highlight against the appended content
			m.applySearch()
		} else {
			m.viewport.SetContent(m.tail.content)
		}
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

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "tab":
			m.focusLeft = !m.focusLeft

		case "h", "left":
			m.focusLeft = true

		case "l", "right":
			m.focusLeft = false

		case "g", "home":
			if !m.focusLeft {
				m.viewport.GotoTop()
				m.followMode = false
			}

		case "G", "end":
			if !m.focusLeft {
				m.viewport.GotoBottom()
				m.followMode = true
			}

		case "pgup", "ctrl+b":
			if !m.focusLeft {
				m.viewport.ScrollUp(m.viewport.Height)
				m.followMode = false
			}

		case "pgdown", "ctrl+f":
			if !m.focusLeft {
				m.viewport.ScrollDown(m.viewport.Height)
			}

		case "ctrl+u":
			if !m.focusLeft {
				m.viewport.HalfViewUp()
				m.followMode = false
			}

		case "ctrl+d":
			if !m.focusLeft {
				m.viewport.HalfViewDown()
			}

		case "up", "k":
			if m.focusLeft {
				if m.cursor > 0 {
					m.cursor--
					m.syncTail()
				}
			} else {
				m.viewport.ScrollUp(1)
				m.followMode = false
			}

		case "down", "j":
			if m.focusLeft {
				if m.cursor < len(m.tasks)-1 {
					m.cursor++
					m.syncTail()
				}
			} else {
				m.viewport.ScrollDown(1)
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
			// Copy selected task's log path. Works in either pane.
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
				m.searchQuery = ""
				m.matches = nil
				m.matchIndex = 0
				m.applySearch()
				m.status = ""
			}

		default:
			if !m.focusLeft {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				cmds = append(cmds, cmd)
			}
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

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	leftWidth := m.width * 35 / 100
	rightWidth := m.width - leftWidth - 4

	leftContent := m.renderList()
	leftPane := paneStyle.Width(leftWidth - 2).Render(leftContent)
	if m.focusLeft {
		leftPane = activePaneStyle.Width(leftWidth - 2).Render(leftContent)
	}

	rightContent := m.viewport.View()
	if m.tail == nil || m.tail.content == "" {
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
		statusLine = m.searchInput.View()
	case m.status != "":
		statusLine = m.status
	default:
		if m.focusLeft {
			statusLine = "↑/↓ select · K kill · d delete · y copy-path · x transform · l/→/Tab focus log · q quit"
		} else {
			statusLine = "↑/↓ scroll · / search · n/N next/prev · y copy-path · x transform · g/G top/bottom · h/←/Tab focus list · q quit"
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
