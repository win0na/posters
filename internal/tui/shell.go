package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/win0na/posters/internal/plex"
)

func shell(body string, width int) string {
	return shellSized(body, width, 0)
}

func shellSized(body string, width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("posters")
	wrapWidth := contentWidth(width)
	body = wrapBody(body, wrapWidth)
	content := title + "\n\n" + linkifyURLs(body)
	contentPlain := "posters\n\n" + body
	boxWidth := maxVisibleLineWidth(contentPlain) + (lipglossHorizontalPadding * 2)
	maxBoxWidth := max(1, cardWidth(width)-2)
	boxWidth = min(maxBoxWidth, max(1, boxWidth))
	style := lipgloss.NewStyle().Padding(verticalContentPadding, lipglossHorizontalPadding).Width(boxWidth).AlignVertical(lipgloss.Center).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63"))
	return style.Render(content)
}

func cardWidth(width int) int {
	if width <= 0 {
		return 80
	}
	return max(40, width-2)
}

func cardHeight(height int) int {
	return 0
}

func contentWidth(width int) int {
	return max(20, cardWidth(width)-2-(horizontalContentPadding*2))
}

func progressBarWidth(width int) int {
	return max(10, min(60, contentWidth(width)-8))
}

func maxVisibleLineWidth(text string) int {
	maxWidth := 0
	for _, line := range strings.Split(text, "\n") {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	return maxWidth
}

func renderedLineCount(view string) int {
	return len(strings.Split(stripANSI(view), "\n"))
}

func centerView(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return view
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, view)
}

func wrapBody(body string, width int) string {
	lines := strings.Split(body, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return []string{line}
	}
	indent := leadingWhitespace(line)
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := ""
	continuation := indent
	if continuation != "" && lipgloss.Width(continuation) >= width-4 {
		continuation = ""
	}
	for _, word := range words {
		if current == "" {
			current = indent + word
			continue
		}
		candidate := current + " " + word
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = continuation + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func linkifyURLs(text string) string {
	return urlRE.ReplaceAllStringFunc(text, func(raw string) string {
		return "\x1b]8;;" + raw + "\x1b\\" + raw + "\x1b]8;;\x1b\\"
	})
}

func renderChoices[T any](items []T, cursor int, label func(T) string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		prefix := "  "
		if i == cursor%max(1, len(items)) {
			prefix = "› "
		}
		lines[i] = prefix + label(item)
	}
	if len(lines) == 0 {
		return "No choices."
	}
	return strings.Join(lines, "\n")
}

func renderLines(items []string, cursor int) string {
	return renderChoices(items, cursor, func(s string) string { return s })
}

func renderMovies(movies []plex.Movie, cursor int, chosen map[string]bool, blacklist map[string]bool, maxRows int) string {
	if len(movies) == 0 {
		return "No choices."
	}
	if maxRows <= 0 || maxRows > len(movies) {
		maxRows = len(movies)
	}
	selected := min(max(0, cursor), len(movies)-1)
	if len(movies) <= maxRows {
		return renderMovieRows(movies, 0, len(movies), selected, chosen, blacklist)
	}
	if maxRows < 3 {
		row := renderMovieRows(movies, selected, selected+1, selected, chosen, blacklist)
		if maxRows == 1 {
			return row
		}
		marker := ""
		if selected < len(movies)-1 {
			marker = fmt.Sprintf("… %d more", len(movies)-selected-1)
		} else if selected > 0 {
			marker = fmt.Sprintf("… %d earlier", selected)
		}
		return row + "\n" + marker
	}
	contentRows := maxRows - 2
	start := selected - contentRows/2
	start = min(max(0, start), max(0, len(movies)-contentRows))
	end := min(len(movies), start+contentRows)
	topMarker := ""
	if start > 0 {
		topMarker = fmt.Sprintf("… %d earlier", start)
	}
	bottomMarker := ""
	if end < len(movies) {
		bottomMarker = fmt.Sprintf("… %d more", len(movies)-end)
	}
	lines := []string{topMarker}
	lines = append(lines, strings.Split(renderMovieRows(movies, start, end, selected, chosen, blacklist), "\n")...)
	lines = append(lines, bottomMarker)
	return strings.Join(lines, "\n")
}

func renderMovieRows(movies []plex.Movie, start, end, selected int, chosen map[string]bool, blacklist map[string]bool) string {
	start = max(0, start)
	end = min(len(movies), end)
	lines := []string{}
	for i := start; i < end; i++ {
		movie := movies[i]
		mark := "[ ]"
		if blacklist[movie.RatingKey] {
			mark = "[!]"
		} else if chosen[movie.RatingKey] {
			mark = "[x]"
		}
		prefix := "  "
		if i == selected {
			prefix = "› "
		}
		lines = append(lines, fmt.Sprintf("%s%s %s (%d)", prefix, mark, movie.Title, movie.Year))
	}
	return strings.Join(lines, "\n")
}

func movieListRows(height int) int {
	if height <= 0 {
		return 12
	}
	return max(1, height-13)
}

func tail(lines []string, n int) string {
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
