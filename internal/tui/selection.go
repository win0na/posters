package tui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type point struct {
	x int
	y int
}

type selectionState struct {
	active bool
	start  point
	end    point
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Shift || msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
		return m, nil
	}
	view := m.baseView()
	pos := point{x: msg.X, y: msg.Y}
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		var ok bool
		pos, ok = clampSelectionPoint(view, pos, true)
		if !ok {
			return m, nil
		}
		m.selection = selectionState{active: true, start: pos, end: pos}
		return m, nil
	case tea.MouseActionMotion:
		if !m.selection.active {
			return m, nil
		}
		pos, _ = clampSelectionPoint(view, pos, false)
		m.selection.end = pos
		return m, nil
	case tea.MouseActionRelease:
		if !m.selection.active {
			return m, nil
		}
		pos, _ = clampSelectionPoint(view, pos, false)
		m.selection.end = pos
		text := selectionTextFromRendered(view, m.selection.start, m.selection.end)
		m.selection = selectionState{}
		if text == "" {
			return m, nil
		}
		return m, copySelectionCmd(text)
	}
	return m, nil
}

func selectionTextFromRendered(rendered string, start, end point) string {
	plain := stripANSI(rendered)
	lines := strings.Split(plain, "\n")
	start, end = clampSelectionToContent(plain, start, end)
	start, end = orderedPoints(start, end)
	if start.y < 0 {
		start.y = 0
	}
	if end.y >= len(lines) {
		end.y = len(lines) - 1
	}
	if len(lines) == 0 || start.y > end.y || end.y < 0 {
		return ""
	}
	selected := make([]string, 0, end.y-start.y+1)
	for y := start.y; y <= end.y; y++ {
		line := lines[y]
		from, to := 0, lipgloss.Width(line)
		if y == start.y {
			from = start.x
		}
		if y == end.y {
			to = end.x + 1
		}
		part := visibleSlice(line, from, to)
		part = cleanSelectionLine(part)
		if part == "" {
			continue
		}
		selected = append(selected, part)
	}
	return strings.Join(selected, "\n")
}

func highlightRenderedSelection(rendered string, start, end point) string {
	return highlightPlainSelection(stripANSI(rendered), start, end)
}

func highlightPlainSelection(plain string, start, end point) string {
	lines := strings.Split(plain, "\n")
	bounds, hasBounds := detectSelectionBounds(plain)
	start, end = clampSelectionToContent(plain, start, end)
	start, end = orderedPoints(start, end)
	for y := range lines {
		if y < start.y || y > end.y {
			continue
		}
		from, to := 0, lipgloss.Width(lines[y])
		if hasBounds {
			from, to = bounds.left, bounds.right+1
		}
		if y == start.y {
			from = start.x
		}
		if y == end.y {
			to = end.x + 1
		}
		if hasBounds {
			from = clamp(from, bounds.left, bounds.right+1)
			to = clamp(to, bounds.left, bounds.right+1)
		}
		lines[y] = highlightLine(lines[y], from, to)
	}
	return strings.Join(lines, "\n")
}

func highlightLine(line string, from, to int) string {
	if from > to {
		from, to = to, from
	}
	if to <= 0 || from >= lipgloss.Width(line) {
		return line
	}
	if from < 0 {
		from = 0
	}
	if to > lipgloss.Width(line) {
		to = lipgloss.Width(line)
	}
	before := visibleSlice(line, 0, from)
	mid := visibleSlice(line, from, to)
	after := visibleSlice(line, to, lipgloss.Width(line))
	if mid == "" {
		return line
	}
	return before + "\x1b[7m" + mid + "\x1b[27m" + after
}

type selectionBounds struct {
	outerLeft  int
	outerRight int
	outerTop   int
	outerBot   int
	left       int
	right      int
	top        int
	bot        int
}

func clampSelectionPoint(rendered string, p point, requireOuterHit bool) (point, bool) {
	plain := stripANSI(rendered)
	bounds, ok := detectSelectionBounds(plain)
	if !ok {
		return p, true
	}
	if requireOuterHit && (p.x < bounds.outerLeft || p.x > bounds.outerRight || p.y < bounds.outerTop || p.y > bounds.outerBot) {
		return p, false
	}
	return point{x: clamp(p.x, bounds.left, bounds.right), y: clamp(p.y, bounds.top, bounds.bot)}, true
}

func clampSelectionToContent(plain string, start, end point) (point, point) {
	bounds, ok := detectSelectionBounds(plain)
	if !ok {
		return start, end
	}
	return point{x: clamp(start.x, bounds.left, bounds.right), y: clamp(start.y, bounds.top, bounds.bot)}, point{x: clamp(end.x, bounds.left, bounds.right), y: clamp(end.y, bounds.top, bounds.bot)}
}

func detectSelectionBounds(plain string) (selectionBounds, bool) {
	lines := strings.Split(plain, "\n")
	bounds := selectionBounds{outerTop: -1, outerBot: -1}
	for y, line := range lines {
		left := cellIndex(line, '╭', false)
		right := cellIndex(line, '╮', true)
		if left >= 0 && right > left {
			bounds.outerTop = y
			bounds.outerLeft = left
			bounds.outerRight = right
			break
		}
	}
	for y := len(lines) - 1; y >= 0; y-- {
		left := cellIndex(lines[y], '╰', false)
		right := cellIndex(lines[y], '╯', true)
		if left >= 0 && right > left {
			bounds.outerBot = y
			if bounds.outerTop < 0 {
				bounds.outerLeft = left
				bounds.outerRight = right
			}
			break
		}
	}
	if bounds.outerTop < 0 || bounds.outerBot <= bounds.outerTop || bounds.outerRight <= bounds.outerLeft+1 {
		return selectionBounds{}, false
	}
	bounds.left = bounds.outerLeft + 1 + lipglossHorizontalPadding
	bounds.right = bounds.outerRight - 1 - lipglossHorizontalPadding
	bounds.top = bounds.outerTop + 1 + verticalContentPadding
	bounds.bot = bounds.outerBot - 1 - verticalContentPadding
	return bounds, true
}

func cellIndex(line string, target rune, last bool) int {
	index := -1
	x := 0
	for _, r := range line {
		if r == target {
			index = x
			if !last {
				return index
			}
		}
		w := lipgloss.Width(string(r))
		if w == 0 {
			w = 1
		}
		x += w
	}
	return index
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\x1b' {
			r, size := utf8.DecodeRuneInString(s[i:])
			out.WriteRune(r)
			i += size
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case ']':
			i = skipOSC(s, i+2)
		case '[':
			i = skipCSI(s, i+2)
		case 'P':
			i = skipST(s, i+2)
		default:
			i += 2
		}
	}
	return out.String()
}

func skipOSC(s string, i int) int {
	for i < len(s) {
		if s[i] == '\a' {
			return i + 1
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return len(s)
}

func skipST(s string, i int) int {
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return len(s)
}

func skipCSI(s string, i int) int {
	for i < len(s) {
		if s[i] >= 0x40 && s[i] <= 0x7e {
			return i + 1
		}
		i++
	}
	return len(s)
}

func visibleSlice(line string, from, to int) string {
	if from < 0 {
		from = 0
	}
	if to < from {
		return ""
	}
	var out strings.Builder
	x := 0
	for _, r := range line {
		w := lipgloss.Width(string(r))
		if w == 0 {
			w = 1
		}
		next := x + w
		if next > from && x < to {
			out.WriteRune(r)
		}
		x = next
		if x >= to {
			break
		}
	}
	return out.String()
}

func orderedPoints(a, b point) (point, point) {
	if a.y > b.y || (a.y == b.y && a.x > b.x) {
		return b, a
	}
	return a, b
}

func cleanSelectionLine(line string) string {
	line = strings.TrimRight(line, " \t")
	line = strings.TrimLeft(line, " \t")
	line = strings.TrimPrefix(line, "│")
	line = strings.TrimSuffix(line, "│")
	line = strings.Trim(line, " \t")
	if line == "" || strings.Trim(line, "─╭╮╰╯") == "" {
		return ""
	}
	return line
}

func copySelectionCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return selectionCopiedMsg{err: copySelection(text)}
	}
}

func copySelection(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := copyWithCommand(text); err == nil {
		return nil
	}
	return copyWithOSC52(text)
}

func copyWithCommand(text string) error {
	commands := clipboardCommands()
	var errs []error
	for _, cmd := range commands {
		if _, err := exec.LookPath(cmd.name); err != nil {
			continue
		}
		command := exec.Command(cmd.name, cmd.args...)
		command.Stdin = strings.NewReader(cmd.input(text))
		var stderr bytes.Buffer
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w %s", cmd.name, err, strings.TrimSpace(stderr.String())))
			continue
		}
		return nil
	}
	if len(errs) == 0 {
		return errors.New("no clipboard command found")
	}
	return errors.Join(errs...)
}

type clipboardCommand struct {
	name  string
	args  []string
	input func(string) string
}

func clipboardCommands() []clipboardCommand {
	plain := func(s string) string { return s }
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	commands := []clipboardCommand{}
	if runtime.GOOS == "darwin" {
		commands = append(commands, clipboardCommand{name: "pbcopy", input: plain})
	}
	if runtime.GOOS == "windows" {
		commands = append(commands, clipboardCommand{name: "clip", input: crlf})
	}
	commands = append(commands,
		clipboardCommand{name: "clip.exe", input: crlf},
		clipboardCommand{name: "wl-copy", input: plain},
		clipboardCommand{name: "xclip", args: []string{"-selection", "clipboard"}, input: plain},
		clipboardCommand{name: "xsel", args: []string{"--clipboard", "--input"}, input: plain},
	)
	return commands
}

func copyWithOSC52(text string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	seq := "\x1b]52;c;" + encoded + "\x1b\\"
	if os.Getenv("TMUX") != "" {
		seq = "\x1bPtmux;\x1b" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	_, err := os.Stdout.WriteString(seq)
	return err
}
