package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/win0na/posters/internal/plex"
)

func startPIN(ctx context.Context, opID int, client Plex) tea.Cmd {
	return func() tea.Msg {
		pin, url, err := client.StartPIN(ctx)
		return pinStartedMsg{opID: opID, pin: pin, url: url, err: err}
	}
}

func pollPIN(ctx context.Context, opID int, client Plex, pinID int) tea.Cmd {
	return func() tea.Msg {
		token, err := client.PollPIN(ctx, pinID)
		return authPollMsg{opID: opID, token: token, err: err}
	}
}

func waitAndPollPIN(ctx context.Context, opID int, client Plex, pinID int) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		token, err := client.PollPIN(ctx, pinID)
		return authPollMsg{opID: opID, token: token, err: err}
	})
}

func loadServers(ctx context.Context, opID int, client Plex) tea.Cmd {
	return func() tea.Msg {
		servers, err := client.ListServers(ctx)
		return serversMsg{opID: opID, servers: servers, err: err}
	}
}

func loadLibraries(ctx context.Context, opID int, client Plex, server plex.Server) tea.Cmd {
	return func() tea.Msg {
		libs, err := client.ListLibraries(ctx, server)
		if err != nil {
			return librariesMsg{opID: opID, err: err}
		}
		return librariesMsg{opID: opID, libs: libs, err: err}
	}
}

func serverLabel(server plex.Server) string {
	return server.Name
}

func optionLines(force bool, dryRun bool, wikiFallback bool) string {
	return styleToggleLine(forceLine(force)) + "\n" + styleToggleLine(dryRunLine(dryRun)) + "\n" + styleToggleLine(wikiFallbackLine(wikiFallback))
}

func resultSummary(stats runStats, dryRun bool) string {
	processed := fmt.Sprintf("updated: %d", stats.Updated)
	if dryRun {
		processed = fmt.Sprintf("dry-run: %d", stats.DryRun)
	}
	parts := []string{
		processed,
		fmt.Sprintf("wiki: %d", stats.WikiFallback),
		fmt.Sprintf("skipped: %d", stats.Skipped),
		fmt.Sprintf("ambiguous: %d", stats.Ambiguous),
		fmt.Sprintf("failed: %d", stats.Failed),
	}
	if stats.Cancelled {
		parts = append(parts, "cancelled")
	}
	return strings.Join(parts, " · ")
}

func resultSummaryBlock(stats runStats) string {
	lines := []string{
		fmt.Sprintf("Updated:   %d", stats.Updated),
		fmt.Sprintf("Dry runs:  %d", stats.DryRun),
		fmt.Sprintf("Wiki:      %d", stats.WikiFallback),
		fmt.Sprintf("Skipped:   %d", stats.Skipped),
		fmt.Sprintf("Ambiguous: %d", stats.Ambiguous),
		fmt.Sprintf("Failed:    %d", stats.Failed),
	}
	if stats.Cancelled {
		lines = append(lines, "Status:    cancelled")
	}
	return strings.Join(lines, "\n")
}

func section(title, body string) string {
	title = ui.frameTitle.Render(title)
	if strings.TrimSpace(body) == "" {
		return title
	}
	return title + "\n" + indentBlock(body, "  ")
}

func controls(lines ...string) string {
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		styled = append(styled, styleBindingLine(line))
	}
	return section("Controls", strings.Join(styled, "\n"))
}
