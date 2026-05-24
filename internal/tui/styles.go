package tui

import "strings"

func styleBindingLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if left, sep, right, ok := splitLabelLine(line); ok {
		return ui.headerLabel.Render(left) + sep + ui.footer.Render(right)
	}
	if left, sep, right, ok := splitColonLine(line); ok {
		return ui.headerLabel.Render(left+sep) + ui.footer.Render(right)
	}
	return ui.footer.Render(line)
}

func styleChoiceList(rendered string, cursor int) string {
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "› ") {
			lines[i] = ui.selected.Render(line)
			continue
		}
		if i == cursor && strings.TrimSpace(line) != "" {
			lines[i] = ui.selected.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func styleMovieList(rendered string, cursor int, chosen map[string]bool) string {
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "…") {
			continue
		}
		if strings.HasPrefix(line, "› ") {
			lines[i] = styleMovieRow(line, chosen)
			continue
		}
		lines[i] = styleMovieRow(line, chosen)
	}
	return strings.Join(lines, "\n")
}

func styleMovieRow(line string, chosen map[string]bool) string {
	indent := leadingWhitespace(line)
	core := strings.TrimLeft(line, " ")
	if core == "" {
		return line
	}
	selected := strings.HasPrefix(core, "› ")
	if selected {
		core = strings.TrimPrefix(core, "› ")
	}
	markStyle := ui.footer
	if strings.Contains(core, "[x]") {
		markStyle = ui.good
	}
	core = strings.Replace(core, "[x]", markStyle.Render("[x]"), 1)
	core = strings.Replace(core, "[ ]", ui.dim.Render("[ ]"), 1)
	core = strings.Replace(core, "[!]", ui.skip.Render("[!]"), 1)
	if selected {
		core = ui.selected.Render(ui.accent.Render("›") + " " + core)
	}
	return indent + core
}

func styleToggleLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	label, sep, rest, ok := splitColonLine(line)
	if !ok {
		return styleBindingLine(line)
	}
	state, gap, hint, ok := splitLabelLine(rest)
	if !ok {
		return ui.headerLabel.Render(label+sep) + ui.panelValue.Render(rest)
	}
	stateStyle := ui.muted
	if state == "on" {
		stateStyle = ui.good
	} else if state == "off" {
		stateStyle = ui.bad
	}
	return ui.headerLabel.Render(label+sep) + stateStyle.Render(state) + gap + ui.footer.Render(hint)
}

func styleSummaryBlock(block string) string {
	if block == "" {
		return ""
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = styleKeyValueLine(line)
	}
	return strings.Join(lines, "\n")
}

func stylePathLine(prefix, value string) string {
	return ui.headerLabel.Render(prefix) + ui.code.Render(value)
}

func styleReportStatus(status string) string {
	style := ui.accent
	switch status {
	case "UPDATED":
		style = ui.good
	case "DRY-RUN":
		style = ui.warn
	case "WIKI-FALLBACK":
		status = "WIKI"
		style = ui.wiki
	case "NO-IMP":
		status = "SKIPPED"
		style = ui.skip
	case "SKIPPED":
		style = ui.skip
	case "AMBIGUOUS":
		style = ui.accent2
	case "FAILED":
		style = ui.bad
	case "RESULT":
		style = ui.accent
	}
	return style.Render(status)
}

func styleReportKV(label, value string) string {
	return "    " + ui.headerLabel.Render(label+":") + " " + ui.panelValue.Render(value)
}

func styleKeyValueLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if label, sep, value, ok := splitColonLine(line); ok {
		return ui.headerLabel.Render(label+sep) + ui.panelValue.Render(value)
	}
	if label, sep, value, ok := splitLabelLine(line); ok {
		return ui.headerLabel.Render(label) + sep + ui.panelValue.Render(value)
	}
	return ui.panelValue.Render(line)
}

func styleRecentActivityLines(lines []string) []string {
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		styled = append(styled, styleActivityLine(line))
	}
	return styled
}

func styleActivityLine(line string) string {
	indent := leadingWhitespace(line)
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, "updated ") {
		return indent + ui.good.Render("✓") + " " + ui.panelValue.Render(line)
	}
	if strings.HasPrefix(line, "wiki-fallback ") {
		return indent + ui.wiki.Render("↯ WIKI") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "wiki-fallback "))
	}
	if strings.HasPrefix(line, "ambiguous ") {
		return indent + ui.accent2.Render("– AMBIGUOUS") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "ambiguous "))
	}
	if strings.HasPrefix(line, "↯ WIKI ") {
		return indent + ui.wiki.Render("↯ WIKI") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "↯ WIKI "))
	}
	if strings.HasPrefix(line, "skip ") {
		return indent + ui.skip.Render("–") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "skip "))
	}
	if strings.HasPrefix(line, "skip-updated ") {
		return indent + ui.skip.Render("– UPDATED") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "skip-updated "))
	}
	if strings.HasPrefix(line, "skip-no-imp ") {
		return indent + ui.skip.Render("– SKIP") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "skip-no-imp "))
	}
	if strings.HasPrefix(line, "– AMBIGUOUS ") {
		return indent + ui.accent2.Render("– AMBIGUOUS") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "– AMBIGUOUS "))
	}
	if strings.HasPrefix(line, "– SKIP ") {
		return indent + ui.skip.Render("– SKIP") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "– SKIP "))
	}
	if strings.HasPrefix(line, "– UPDATED ") {
		return indent + ui.skip.Render("– UPDATED") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "– UPDATED "))
	}
	if strings.HasPrefix(line, "– NO IMP ") {
		return indent + ui.skip.Render("– SKIP") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "– NO IMP "))
	}
	if strings.HasPrefix(line, "report") {
		return indent + ui.accent2.Render("↳") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "report"))
	}
	if strings.HasPrefix(line, "○ DRY-RUN ") {
		return indent + ui.warn.Render("○ DRY-RUN") + " " + ui.panelValue.Render(strings.TrimPrefix(line, "○ DRY-RUN "))
	}
	if strings.HasPrefix(line, "IMP:") {
		if _, value, ok := strings.Cut(line, ": "); ok {
			return indent + ui.headerLabel.Render("IMP:") + " " + ui.code.Render(value)
		}
	}
	if strings.HasPrefix(line, "Image:") {
		if _, value, ok := strings.Cut(line, ": "); ok {
			return indent + ui.headerLabel.Render("Image:") + " " + ui.code.Render(value)
		}
	}
	if strings.HasPrefix(line, "Match:") {
		if _, value, ok := strings.Cut(line, ": "); ok {
			return indent + ui.headerLabel.Render("Match:") + " " + ui.panelValue.Render(value)
		}
	}
	if strings.HasPrefix(line, "Reason:") {
		if _, value, ok := strings.Cut(line, ": "); ok {
			return indent + ui.headerLabel.Render("Reason:") + " " + ui.panelValue.Render(value)
		}
	}
	if strings.HasPrefix(line, "Error:") {
		if _, value, ok := strings.Cut(line, ": "); ok {
			return indent + ui.headerLabel.Render("Error:") + " " + ui.bad.Render(value)
		}
	}
	if strings.HasPrefix(line, "Note:") {
		if _, value, ok := strings.Cut(line, ": "); ok {
			return indent + ui.headerLabel.Render("Note:") + " " + ui.footer.Render(value)
		}
	}
	return indent + ui.muted.Render(line)
}

func splitColonLine(line string) (string, string, string, bool) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", "", false
	}
	end := idx + 1
	for end < len(line) && line[end] == ' ' {
		end++
	}
	return line[:idx], line[idx:end], line[end:], true
}

func splitLabelLine(line string) (string, string, string, bool) {
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return "", "", "", false
	}
	end := idx
	for end < len(line) && line[end] == ' ' {
		end++
	}
	if end == idx {
		return "", "", "", false
	}
	return line[:idx], line[idx:end], line[end:], true
}
