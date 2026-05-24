package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func wrapWithPrefix(text, firstPrefix, nextPrefix string, width int) []string {
	available := max(10, width-lipgloss.Width(firstPrefix))
	wrapped := wrapLineHard(text, available)
	if len(wrapped) == 0 {
		return []string{firstPrefix}
	}
	out := make([]string, len(wrapped))
	for i, line := range wrapped {
		prefix := firstPrefix
		if i > 0 {
			prefix = nextPrefix
		}
		out[i] = prefix + line
	}
	return out
}

func wrapLineHard(text string, width int) []string {
	soft := wrapLine(text, width)
	out := []string{}
	for _, line := range soft {
		for lipgloss.Width(line) > width {
			cut := visibleCut(line, width)
			out = append(out, strings.TrimRight(line[:cut], " "))
			line = strings.TrimLeft(line[cut:], " ")
		}
		out = append(out, line)
	}
	return out
}

func visibleCut(text string, width int) int {
	if width <= 0 {
		return 0
	}
	x := 0
	for i, r := range text {
		w := lipgloss.Width(string(r))
		if w == 0 {
			w = 1
		}
		if x+w > width {
			return i
		}
		x += w
	}
	return len(text)
}

func viewportLines(lines []string, offset, maxRows int) string {
	if len(lines) == 0 {
		return ""
	}
	if maxRows <= 0 {
		return ""
	}
	if len(lines) <= maxRows {
		return strings.Join(lines, "\n")
	}
	if maxRows == 1 {
		offset = clamp(offset, 0, len(lines)-1)
		return lines[offset]
	}
	if maxRows == 2 {
		offset = clamp(offset, 0, len(lines)-1)
		marker := ""
		if offset+1 < len(lines) {
			marker = fmt.Sprintf("… %d more (↑/↓ scroll)", len(lines)-offset-1)
		} else if offset > 0 {
			marker = fmt.Sprintf("… %d earlier", offset)
		}
		return strings.Join([]string{lines[offset], marker}, "\n")
	}
	contentRows := maxRows - 2
	offset = clamp(offset, 0, max(0, len(lines)-contentRows))
	end := min(len(lines), offset+contentRows)
	top := ""
	if offset > 0 {
		top = fmt.Sprintf("… %d earlier", offset)
	}
	bottom := ""
	if end < len(lines) {
		bottom = fmt.Sprintf("… %d more (↑/↓ scroll)", len(lines)-end)
	}
	visible := []string{top}
	visible = append(visible, lines[offset:end]...)
	visible = append(visible, bottom)
	return strings.Join(visible, "\n")
}

func reportCursorLimit(lineCount, maxRows int) int {
	if lineCount <= 0 || maxRows <= 0 || lineCount <= maxRows {
		return 1
	}
	if maxRows <= 2 {
		return lineCount
	}
	contentRows := maxRows - 2
	return max(1, lineCount-contentRows+1)
}

func runningCursorLimit(lineCount, maxRows int) int {
	return reportCursorLimit(lineCount, maxRows)
}

func runningRows(height, width int, header string) int {
	if height <= 0 {
		return 8
	}
	headerRows := len(strings.Split(wrapBody(header, width), "\n"))
	return max(0, height-8-headerRows-2)
}

func doneRows(height int) int {
	if height <= 0 {
		return 18
	}
	return max(6, height-2-(verticalContentPadding*2)-3)
}

func indentBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func forceLine(force bool) string {
	if force {
		return "Force refresh: on (f toggles)"
	}
	return "Force refresh: off (f toggles)"
}

func dryRunLine(dryRun bool) string {
	if dryRun {
		return "Dry run: on (d toggles)"
	}
	return "Dry run: off (d toggles)"
}

func wikiFallbackLine(enabled bool) string {
	if enabled {
		return "Wikipedia fallback: on (w toggles)"
	}
	return "Wikipedia fallback: off (w toggles)"
}
