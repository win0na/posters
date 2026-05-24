package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
)

func dashboardLayout(width int, left, right dashboardPane) string {
	if right.body == "" {
		return renderDashboardPane(width, left)
	}
	bodyWidth := contentWidth(width)
	if bodyWidth < dashboardWideWidth {
		return renderDashboardPane(bodyWidth, left)
	}
	leftWidth := max(28, (bodyWidth-panelGap)/2)
	rightWidth := max(28, bodyWidth-panelGap-leftWidth)
	if leftWidth+rightWidth+panelGap > bodyWidth {
		return renderDashboardPane(bodyWidth, left)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderDashboardPane(leftWidth, left), strings.Repeat(" ", panelGap), renderDashboardPane(rightWidth, right))
}

func renderDashboardPane(width int, pane dashboardPane) string {
	width = max(20, width)
	innerWidth := max(10, width-2-(panelHorizontalPad*2))
	body := wrapBody(pane.body, innerWidth)
	parts := []string{}
	if pane.title != "" {
		header := ui.panelTitle.Foreground(pane.accent).Render(iconTitle(pane.icon, pane.title))
		if pane.subtitle != "" {
			header += " " + ui.subtitle.Render("· "+pane.subtitle)
		}
		parts = append(parts, header)
		parts = append(parts, ui.divider.Render(strings.Repeat("─", min(innerWidth, max(8, lipgloss.Width(stripANSI(header)))))))
	}
	if body != "" {
		parts = append(parts, body)
	}
	content := strings.Join(parts, "\n")
	return ui.panel.Copy().Width(width).BorderForeground(pane.accent).Render(content)
}

func iconTitle(icon, title string) string {
	if icon == "" {
		return title
	}
	return icon + " " + title
}

func filterEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func chooseText(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "none"
}

func tokenStatus(token string) string {
	if strings.TrimSpace(token) == "" {
		return "absent"
	}
	return "present"
}

func chosenCount(chosen map[string]bool) int {
	count := 0
	for _, selected := range chosen {
		if selected {
			count++
		}
	}
	return count
}

func (m Model) cursorLimit() int {
	switch m.screen {
	case screenServers:
		return len(m.servers)
	case screenLibraries:
		return len(m.libs)
	case screenMode:
		return 2
	case screenMovies:
		return len(m.movies)
	case screenRunning:
		return runningCursorLimit(len(runningActivityLines(m.log, 0, contentWidth(m.width))), m.runningViewportRows())
	case screenBlacklist:
		return max(1, len(blacklistItems(m.store)))
	case screenDone:
		return reportCursorLimit(len(m.doneFullLines()), doneViewportRows(doneRows(m.height)))
	}
	return m.cursor + 2
}

func hasSavedToken(store *config.Store) bool {
	if store == nil {
		return false
	}
	state, err := store.LoadState()
	return err == nil && state.PlexToken != ""
}

func preferredServerCursor(store *config.Store, servers []plex.Server) int {
	state, err := store.LoadState()
	if err != nil {
		return 0
	}
	for i, server := range servers {
		if state.LastServerID != "" && server.ClientID == state.LastServerID {
			return i
		}
		if state.LastServerURI != "" && server.URI == state.LastServerURI {
			return i
		}
	}
	return 0
}

func preferredLibraryCursor(store *config.Store, libs []plex.Library) int {
	state, err := store.LoadState()
	if err != nil {
		return 0
	}
	for i, lib := range libs {
		if state.LastLibraryKey != "" && lib.Key == state.LastLibraryKey {
			return i
		}
	}
	return 0
}

func (m Model) statusView() string {
	state, _ := m.store.LoadState()
	metadata, _ := m.store.LoadMetadata()
	token := "absent"
	if state.PlexToken != "" {
		token = "present"
	}
	server := serverLabel(m.server)
	if server == "" {
		server = state.LastServerName
	}
	if server == "" {
		server = "none"
	}
	library := m.library.Title
	if library == "" {
		library = state.LastLibraryTitle
	}
	if library == "" {
		library = "none"
	}
	return strings.Join([]string{
		ui.frameTitle.Render("Status"),
		"",
		styleKeyValueLine("Config: " + m.store.Dir()),
		styleKeyValueLine("Plex token: " + token),
		styleKeyValueLine(fmt.Sprintf("Metadata items: %d", len(metadata.Items))),
		styleKeyValueLine(fmt.Sprintf("Blacklisted: %d", len(metadata.Blacklist))),
		styleKeyValueLine("Server: " + server),
		styleKeyValueLine("Library: " + library),
	}, "\n")
}

func (m Model) blacklistBody() string {
	items := blacklistItems(m.store)
	content := renderBlacklistItems(items, m.cursor)
	return section("Blacklist", content) + "\n\n" + controls("b      remove highlighted", "Esc    back", "Enter  back", "q      quit")
}

func blacklistItems(store *config.Store) []config.BlacklistItem {
	if store == nil {
		return nil
	}
	metadata, err := store.LoadMetadata()
	if err != nil {
		return nil
	}
	items := make([]config.BlacklistItem, 0, len(metadata.Blacklist))
	for _, item := range metadata.Blacklist {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Title != items[j].Title {
			return items[i].Title < items[j].Title
		}
		return items[i].Year < items[j].Year
	})
	return items
}

func renderBlacklistItems(items []config.BlacklistItem, cursor int) string {
	if len(items) == 0 {
		return "No blacklisted movies. Use b on the movie selection screen to add one."
	}
	lines := make([]string, len(items))
	for i, item := range items {
		prefix := "  "
		if i == cursor%max(1, len(items)) {
			prefix = "› "
		}
		lines[i] = fmt.Sprintf("%s%s (%d)", prefix, item.Title, item.Year)
	}
	return styleChoiceList(strings.Join(lines, "\n"), cursor)
}
