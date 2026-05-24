package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
)

func (m Model) View() string {
	view := m.baseView()
	if m.selection.active {
		return highlightRenderedSelection(view, m.selection.start, m.selection.end)
	}
	return view
}

func (m Model) baseView() string {
	return centerView(shellSized(m.body(), m.width, m.height), m.width, m.height)
}

func (m Model) body() string {
	body := ""
	switch m.screen {
	case screenLogin:
		body = section("Goal", "Update Plex movie posters to original theatrical posters.")
		if m.notice != "" {
			body += "\n\n" + section("Notice", m.notice)
		}
		body += "\n\n" + controls(loginControl(m.store), "s      status", "r      clear saved login", "q      quit")
	case screenAuthWait:
		if m.authURL != "" {
			body = section("Plex login", m.spinner.View()+" Waiting for browser approval") + "\n\n" + section("Open", m.authURL) + "\n\n" + section("Code", m.pin.Code) + "\n\n" + controls("Enter  poll now", "Esc    cancel")
		} else {
			body = section("Loading", m.spinner.View()+" Contacting Plex...") + "\n\n" + controls("Esc    cancel")
		}
	case screenServers:
		body = section("Choose Plex server", styleChoiceList(renderChoices(m.servers, m.cursor, func(s plex.Server) string { return serverLabel(s) }), m.cursor)) + "\n\n" + controls("s      status")
	case screenLibraries:
		body = section("Choose movie library", styleChoiceList(renderChoices(m.libs, m.cursor, func(l plex.Library) string { return l.Title }), m.cursor)) + "\n\n" + controls("Esc    servers", "s      status")
	case screenMode:
		body = section("Update mode", styleChoiceList(renderLines([]string{"All posters (default)", "Specific posters"}, m.cursor), m.cursor)) + "\n\n" + section("Options", optionLines(m.force, m.dryRun, m.wikiFallback)) + "\n\n" + controls("b      blacklist", "s      status")
	case screenMovies:
		body = m.movieBody()
	case screenStatus:
		body = m.statusView() + "\n\n" + controls("Enter  back", "s      back")
	case screenBlacklist:
		body = m.blacklistBody()
	case screenRunning:
		percent := float64(m.runningDone) / float64(max(1, m.runningTotal))
		body = m.runningView(percent)
	case screenDone:
		body = m.doneView(doneRows(m.height))
	case screenError:
		body = section("Error", fmt.Sprintf("%v", m.err)) + "\n\n" + controls("r      clear saved login and reauthenticate", "Enter  quit", "q      quit")
	}
	return body
}

func loginControl(store *config.Store) string {
	if hasSavedToken(store) {
		return "Enter  continue with saved login"
	}
	return "Enter  login to Plex"
}

func (m Model) movieBody() string {
	if m.height <= 0 {
		return m.movieBodyForRows(movieListRows(m.height))
	}
	for rows := movieListRows(m.height); rows >= 1; rows-- {
		body := m.movieBodyForRows(rows)
		if renderedLineCount(shellSized(body, m.width, m.height)) <= m.height {
			return body
		}
	}
	return m.movieBodyForRows(1)
}

func (m Model) movieBodyForRows(rows int) string {
	movies := styleMovieList(renderMovies(m.movies, m.cursor, m.chosen, m.blacklistedRatingKeys(), rows), m.cursor, m.chosen)
	if m.height > 0 && m.height <= 14 {
		return ui.frameTitle.Render("Select movies") + "\n" + movies + "\n\n" + ui.footer.Render("space toggle · Enter start · Esc back")
	}
	if m.height > 0 && m.height <= 18 {
		return ui.frameTitle.Render("Select movies") + "\n" + movies + "\n\n" + ui.footer.Render("space toggle · Enter start · Esc back") + "\n" + optionLines(m.force, m.dryRun, m.wikiFallback)
	}
	return section("Select movies", movies) + "\n\n" + controls("space  toggle", "b      blacklist", "Enter  start", "Esc    back") + "\n\n" + section("Options", optionLines(m.force, m.dryRun, m.wikiFallback))
}

type dashboardPane struct {
	title    string
	subtitle string
	icon     string
	accent   lipgloss.Color
	body     string
}

func (m Model) loginBody() string {
	left := dashboardPane{
		title:    "Login",
		subtitle: "Plex auth · local config only",
		icon:     "◆",
		accent:   accentMagenta,
		body: strings.Join(filterEmptyStrings([]string{
			"Update Plex movie posters to original theatrical posters.",
			m.notice,
		}), "\n\n"),
	}
	right := dashboardPane{
		title:    "Flow",
		subtitle: "No API keys · no env vars",
		icon:     "↗",
		accent:   accentCyan,
		body: strings.Join([]string{
			"1. Log in with Plex.",
			"2. Pick server and library.",
			"3. Choose all posters or specific movies.",
			"4. Review progress and report.",
			"",
			"Enter: login",
			"s: status",
			"r: clear saved login",
			"q / ctrl+c: quit",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) authBody() string {
	body := ""
	if m.authURL != "" {
		body = strings.Join([]string{
			m.spinner.View() + " Waiting for Plex login",
			"",
			"Open:",
			m.authURL,
			"",
			"Code: " + m.pin.Code,
		}, "\n")
	} else {
		body = m.spinner.View() + " Loading..."
	}
	left := dashboardPane{
		title:    "Authenticate",
		subtitle: "Watch the code and return here",
		icon:     "●",
		accent:   accentAmber,
		body:     body,
	}
	right := dashboardPane{
		title:    "Steps",
		subtitle: "Fast path",
		icon:     "→",
		accent:   accentCyan,
		body: strings.Join([]string{
			"1. Open the link.",
			"2. Enter the code.",
			"3. Wait for the token.",
			"",
			"Enter: poll now",
			"Esc: cancel",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) choiceBody(title, subtitle, help string, choices string) string {
	left := dashboardPane{
		title:    title,
		subtitle: subtitle,
		icon:     "▸",
		accent:   accentCyan,
		body:     choices,
	}
	right := dashboardPane{
		title:    "Help",
		subtitle: "Keep it fast",
		icon:     "⌁",
		accent:   accentMagenta,
		body: strings.Join(filterEmptyStrings([]string{
			help,
			"↑/↓ or j/k move",
			"Enter selects",
			"q / ctrl+c quits",
		}), "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) modeBody() string {
	left := dashboardPane{
		title:    "Update mode",
		subtitle: "All posters is the default",
		icon:     "◆",
		accent:   accentMagenta,
		body:     renderLines([]string{"All posters (default)", "Specific posters"}, m.cursor),
	}
	right := dashboardPane{
		title:    "Toggles",
		subtitle: "Apply to either mode",
		icon:     "◌",
		accent:   accentCyan,
		body: strings.Join([]string{
			optionLines(m.force, m.dryRun, m.wikiFallback),
			"",
			"f: force refresh",
			"d: dry run",
			"w: wiki fallback",
			"Enter starts",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) moviesBody() string {
	left := dashboardPane{
		title:    "Select movies",
		subtitle: "Space toggles · Enter starts",
		icon:     "☑",
		accent:   accentCyan,
		body:     renderMovies(m.movies, m.cursor, m.chosen, m.blacklistedRatingKeys(), movieListRows(m.height)),
	}
	right := dashboardPane{
		title:    "Run settings",
		subtitle: "Selection summary",
		icon:     "↳",
		accent:   accentAmber,
		body: strings.Join([]string{
			fmt.Sprintf("Selected: %d / %d", chosenCount(m.chosen), len(m.movies)),
			optionLines(m.force, m.dryRun, m.wikiFallback),
			"",
			"Space: toggle row",
			"b: blacklist row",
			"Enter: update now",
			"Esc: back",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) statusBody() string {
	state, _ := m.store.LoadState()
	metadata, _ := m.store.LoadMetadata()
	left := dashboardPane{
		title:    "Status",
		subtitle: "Local store is source of truth",
		icon:     "◐",
		accent:   accentMagenta,
		body: strings.Join([]string{
			"Config: " + m.store.Dir(),
			"Plex token: " + tokenStatus(state.PlexToken),
			fmt.Sprintf("Metadata items: %d", len(metadata.Items)),
			fmt.Sprintf("Blacklisted: %d", len(metadata.Blacklist)),
			"Server: " + chooseText(serverLabel(m.server), state.LastServerName),
			"Library: " + chooseText(m.library.Title, state.LastLibraryTitle),
			forceLine(m.force),
			dryRunLine(m.dryRun),
		}, "\n"),
	}
	right := dashboardPane{
		title:    "Controls",
		subtitle: "Quick status toggles",
		icon:     "⌘",
		accent:   accentCyan,
		body: strings.Join([]string{
			forceLine(m.force),
			dryRunLine(m.dryRun),
			"",
			"b: blacklist",
			"s: back",
			"q / ctrl+c: quit",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) runningBody(percent float64) string {
	header := m.runningHeader(percent)
	activity := viewportLines(runningActivityLines(m.log, 0, contentWidth(m.width)), m.cursor, m.runningViewportRows())
	if activity == "" {
		activity = "  waiting for first update..."
	}
	bodyWidth := contentWidth(m.width)
	var body string
	if bodyWidth < dashboardWideWidth {
		body = strings.Join(filterEmptyStrings([]string{
			header,
			activity,
		}), "\n\n")
	} else {
		left := dashboardPane{
			title:    "Progress",
			subtitle: resultSummary(m.runStats, m.dryRun),
			icon:     "▣",
			accent:   accentGreen,
			body:     header,
		}
		right := dashboardPane{
			title:    "Activity",
			subtitle: "Newest updates land at the bottom",
			icon:     "↟",
			accent:   accentCyan,
			body:     activity,
		}
		body = dashboardLayout(m.width, left, right)
	}
	footer := ui.footer.Render("Esc: cancel")
	if body == "" {
		return footer
	}
	return body + "\n\n" + footer
}

func (m Model) doneBody(maxRows int) string {
	footer := ui.footer.Render("Enter/q: quit")
	contentRows := max(3, maxRows-2)
	view := viewportLines(m.doneFullLines(), m.cursor, contentRows)
	if view == "" {
		return footer
	}
	return view + "\n\n" + footer
}

func (m Model) errorBody() string {
	left := dashboardPane{
		title:    "Error",
		subtitle: "Saved login can be cleared",
		icon:     "!",
		accent:   accentRed,
		body:     fmt.Sprintf("%v", m.err),
	}
	right := dashboardPane{
		title:    "Recovery",
		subtitle: "Quick reset",
		icon:     "↺",
		accent:   accentAmber,
		body: strings.Join([]string{
			"r: clear saved login",
			"Enter: retry",
			"q / ctrl+c: quit",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}
