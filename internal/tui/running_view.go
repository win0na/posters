package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
)

func (m Model) runningView(percent float64) string {
	header := m.runningHeader(percent)
	rows := m.runningViewportRows()
	activity := viewportLines(runningActivityLines(m.log, 0, contentWidth(m.width)), m.cursor, rows)
	if activity == "" && rows > 0 {
		activity = "  waiting for first update..."
	}
	if activity == "" {
		return header + "\n" + ui.footer.Render("Esc: cancel")
	}
	return header + "\n" + activity + "\n\n" + ui.footer.Render("Esc: cancel")
}

func (m Model) runningHeader(percent float64) string {
	return fmt.Sprintf("%s %s %d/%d\n%s\n%s\n\n%s\n\n%s:", ui.accent.Render(m.spinner.View()), ui.frameTitle.Render("Updating posters"), m.runningDone, m.runningTotal, ui.muted.Render(resultSummary(m.runStats, m.dryRun)), m.currentPostersLines(contentWidth(m.width)), m.bar.ViewAs(percent), ui.frameTitle.Render("Activity"))
}

func (m Model) currentPostersLines(width int) string {
	if len(m.runningCurrent) == 0 {
		return ui.worker.Render("Working:") + " " + ui.muted.Render("waiting for available worker")
	}
	lines := []string{ui.worker.Render("Working:")}
	for i, entry := range m.runningCurrent {
		line := fmt.Sprintf("  %d. %s: %s", i+1, titlePhase(entry.Phase), movieLabel(entry.Movie))
		if width > 0 && lipgloss.Width(line) > width {
			cut := visibleCut(line, max(1, width-1))
			line = strings.TrimRight(line[:cut], " ") + "…"
		}
		lines = append(lines, ui.worker.Render(line))
	}
	return strings.Join(lines, "\n")
}

func titlePhase(phase string) string {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return "Matching"
	}
	return strings.ToUpper(phase[:1]) + phase[1:]
}

func movieLabel(movie plex.Movie) string {
	if movie.Year > 0 {
		return fmt.Sprintf("%s (%d)", movie.Title, movie.Year)
	}
	return movie.Title
}

func (m Model) runningViewportRows() int {
	return runningRows(m.height, contentWidth(m.width), m.runningHeader(0))
}

func (m Model) doneView(maxRows int) string {
	footer := ui.footer.Render("Enter/q: quit")
	view := viewportLines(m.doneFullLines(), m.cursor, doneViewportRows(maxRows))
	if view == "" {
		return footer
	}
	return view + "\n\n" + footer
}

func doneViewportRows(maxRows int) int {
	return max(3, maxRows-2)
}

func (m Model) doneFullLines() []string {
	sections := []string{ui.frameTitle.Render("Done."), section("Summary:", styleSummaryBlock(resultSummaryBlock(m.runStats, m.dryRun)))}
	if m.reportPath != "" || m.reportCSVPath != "" {
		lines := []string{}
		if m.reportPath != "" {
			lines = append(lines, stylePathLine("JSON: ", m.reportPath))
		}
		if m.reportCSVPath != "" {
			lines = append(lines, stylePathLine("CSV:  ", m.reportCSVPath))
		}
		sections = append(sections, section("Reports:", strings.Join(lines, "\n")))
	}
	if results := reportItemsView(m.reportItems); results != "" {
		sections = append(sections, section("Results:", results))
	} else if recent := recentActivityView(m.log, 8); recent != "" {
		sections = append(sections, section("Recent activity:", recent))
	}
	if details := strings.Join(m.details, "\n"); details != "" {
		sections = append(sections, section("Ambiguous matches:", indentBlock(details, "  ")))
	}
	return strings.Split(strings.Join(sections, "\n\n"), "\n")
}

func reportItemsView(items []config.ReportItem) string {
	if len(items) == 0 {
		return ""
	}
	sections := make([]string, 0, len(items))
	for _, item := range items {
		sections = append(sections, formatReportItem(item))
	}
	return strings.Join(sections, "\n")
}

func formatReportItem(item config.ReportItem) string {
	status := strings.ToUpper(item.Status)
	if status == "" {
		status = "RESULT"
	}
	styled := styleReportStatus(status)
	header := fmt.Sprintf("  %-12s %s", styled, item.Title)
	if item.Year > 0 {
		header += fmt.Sprintf(" (%d)", item.Year)
	}
	lines := []string{header}
	if item.SourceURL != "" {
		lines = append(lines, styleReportKV("IMP page", item.SourceURL))
	}
	if item.ImageURL != "" {
		lines = append(lines, styleReportKV("Image", item.ImageURL))
	}
	if item.MatchReason != "" {
		lines = append(lines, styleReportKV("Match", item.MatchReason))
	}
	if item.Error != "" {
		lines = append(lines, styleReportKV("Error", item.Error))
	} else if item.Message != "" && item.SourceURL == "" && item.ImageURL == "" {
		lines = append(lines, styleReportKV("Note", item.Message))
	}
	return strings.Join(lines, "\n")
}

func recentActivityView(lines []string, limit int) string {
	recent := tail(lines, limit)
	if recent == "" {
		return ""
	}
	return indentBlock(strings.Join(styleRecentActivityLines(strings.Split(recent, "\n")), "\n"), "  ")
}

func runningActivityView(lines []string, limit, width int) string {
	return strings.Join(runningActivityLines(lines, limit, width), "\n")
}

func runningActivityLines(lines []string, limit, width int) []string {
	if len(lines) == 0 {
		return []string{"  waiting for first update..."}
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	formatted := []string{}
	for _, line := range lines {
		formatted = append(formatted, formatActivityEntry(line, width)...)
	}
	return styleRecentActivityLines(formatted)
}

func formatActivityEntry(line string, width int) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if strings.HasPrefix(line, "dry-run ") {
		return formatDryRunActivity(line, width)
	}
	if strings.HasPrefix(line, "wiki-fallback ") {
		return formatWikiFallbackActivity(line, width)
	}
	if strings.HasPrefix(line, "updated ") && strings.Contains(line, " | ") {
		return formatUpdatedActivity(line, width)
	}
	if strings.HasPrefix(line, "ambiguous ") || strings.HasPrefix(line, "skip ") || strings.HasPrefix(line, "skip-updated ") || strings.HasPrefix(line, "skip-no-imp ") {
		return formatSkipActivity(line, width)
	}
	prefix := "  • "
	marker := ui.muted.Render("•")
	if strings.HasPrefix(line, "updated ") {
		prefix = "  ✓ "
		marker = ui.good.Render("✓")
	} else if strings.HasPrefix(line, "report") {
		prefix = "  ↳ "
		marker = ui.accent2.Render("↳")
	}
	lines := wrapWithPrefix(strings.TrimPrefix(line, strings.TrimSpace(prefix)), prefix, "    ", width)
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			lines[i] = strings.Replace(l, strings.TrimSpace(prefix), marker, 1)
		}
	}
	return lines
}

func formatUpdatedActivity(line string, width int) []string {
	parts := strings.Split(line, " | ")
	main := strings.TrimPrefix(parts[0], "updated ")
	lines := wrapWithPrefix(main, "  ✓ ", "    ", width)
	for _, part := range parts[1:] {
		label := "    Info:  "
		value := part
		if v, ok := strings.CutPrefix(part, "match: "); ok {
			label, value = "    Match: ", v
		}
		lines = append(lines, wrapWithPrefix(value, label, strings.Repeat(" ", lipgloss.Width(label)), width)...)
	}
	return lines
}

func formatWikiFallbackActivity(line string, width int) []string {
	parts := strings.Split(line, " | ")
	main := strings.TrimPrefix(parts[0], "wiki-fallback ")
	lines := wrapWithPrefix(main, "  ↯ WIKI ", "    ", width)
	for _, part := range parts[1:] {
		label := "    Info:  "
		value := part
		if v, ok := strings.CutPrefix(part, "image: "); ok {
			label, value = "    Image: ", v
		} else if v, ok := strings.CutPrefix(part, "reason: "); ok {
			label, value = "    Reason: ", v
		}
		lines = append(lines, wrapWithPrefix(value, label, strings.Repeat(" ", lipgloss.Width(label)), width)...)
	}
	return lines
}

func formatSkipActivity(line string, width int) []string {
	line = strings.TrimSpace(line)
	label := "SKIP"
	main := strings.TrimPrefix(line, "skip ")
	if strings.HasPrefix(line, "ambiguous ") {
		label = "AMBIGUOUS"
		main = strings.TrimPrefix(line, "ambiguous ")
	} else if strings.HasPrefix(line, "skip-updated ") {
		label = "UPDATED"
		main = strings.TrimPrefix(line, "skip-updated ")
	} else if strings.HasPrefix(line, "skip-no-imp ") {
		label = "SKIP"
		main = strings.TrimPrefix(line, "skip-no-imp ")
	}
	title := main
	reason := ""
	if left, right, ok := splitMovieLogPayload(main); ok {
		title = left
		reason = right
	}
	lines := wrapWithPrefix(title, "  – "+label+" ", "    ", width)
	if reason != "" {
		lines = append(lines, wrapWithPrefix(reason, "    Reason: ", "            ", width)...)
	}
	return lines
}

func formatDryRunActivity(line string, width int) []string {
	parts := strings.Split(line, " | ")
	lines := []string{}
	main := strings.TrimPrefix(parts[0], "dry-run ")
	if title, source, ok := splitMovieLogPayload(main); ok {
		lines = append(lines, wrapWithPrefix(title, "  ○ DRY-RUN ", "    ", width)...)
		lines = append(lines, wrapWithPrefix(source, "    IMP:   ", "           ", width)...)
	} else {
		lines = append(lines, wrapWithPrefix(main, "  ○ DRY-RUN ", "    ", width)...)
	}
	for _, part := range parts[1:] {
		label := "    Info:  "
		value := part
		if v, ok := strings.CutPrefix(part, "image: "); ok {
			label, value = "    Image: ", v
		} else if v, ok := strings.CutPrefix(part, "reason: "); ok {
			label, value = "    Match: ", v
		}
		lines = append(lines, wrapWithPrefix(value, label, strings.Repeat(" ", lipgloss.Width(label)), width)...)
	}
	return lines
}

func splitMovieLogPayload(line string) (string, string, bool) {
	if left, right, ok := strings.Cut(line, "): "); ok {
		return left + ")", right, true
	}
	return strings.Cut(line, ": ")
}
