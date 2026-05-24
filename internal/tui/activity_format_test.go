package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
	posterfinder "github.com/win0na/posters/internal/posters"
)

func TestRunningActivityFormatsDryRunDetails(t *testing.T) {
	t.Parallel()

	line := "dry-run Alien (1979): http://www.impawards.com/1979/alien.html | image: http://www.impawards.com/1979/posters/alien_xxlg.jpg | reason: visual match 99.1%"
	got := stripANSI(runningActivityView([]string{line}, 6, 48))
	for _, want := range []string{"○ DRY-RUN Alien (1979)", "IMP:", "Image:", "Match:", "99.1%"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningActivityView() missing %q:\n%s", want, got)
		}
	}
	for _, row := range strings.Split(got, "\n") {
		if lipgloss.Width(row) > 48 {
			t.Fatalf("activity row too wide: width=%d row=%q\n%s", lipgloss.Width(row), row, got)
		}
	}
}

func TestRunningActivityFormatsUpdatedMatchDetails(t *testing.T) {
	t.Parallel()

	line := "updated Aliens (1986), visual match 97.4%"
	got := stripANSI(runningActivityView([]string{line}, 6, 52))
	for _, want := range []string{"✓ updated Aliens (1986), visual match 97.4%"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningActivityView() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Match:") || strings.Contains(got, "next best") {
		t.Fatalf("updated activity kept noisy detail rows:\n%s", got)
	}
	for _, row := range strings.Split(got, "\n") {
		if lipgloss.Width(row) > 52 {
			t.Fatalf("activity row too wide: width=%d row=%q\n%s", lipgloss.Width(row), row, got)
		}
	}
}

func TestUpdatedResultLineUsesVisualMatchOnly(t *testing.T) {
	t.Parallel()

	line := updatedResultLine(plex.Movie{Title: "Aliens", Year: 1986}, posterfinder.Candidate{MatchReason: "visual match 97.4%; next best 95.2%"})
	if line != "updated Aliens (1986), visual match 97.4%" {
		t.Fatalf("updatedResultLine() = %q", line)
	}
}

func TestRunningActivityKeepsColonTitlesTogether(t *testing.T) {
	t.Parallel()

	lines := []string{
		"dry-run Spider-Man: Across the Spider-Verse (2023): http://www.impawards.com/2023/spider_man_across_the_spider_verse.html | reason: visual match 91.2%",
		"skip Star Wars: Episode III - Revenge of the Sith (2005): no IMP poster available",
	}
	got := stripANSI(runningActivityView(lines, 8, 72))
	for _, want := range []string{
		"○ DRY-RUN Spider-Man: Across the Spider-Verse (2023)",
		"IMP:",
		"– SKIP Star Wars: Episode III - Revenge of the Sith (2005)",
		"Reason: no IMP poster available",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningActivityView() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "IMP:   Across the Spider-Verse") || strings.Contains(got, "Reason: Episode III") {
		t.Fatalf("colon title was split as payload:\n%s", got)
	}
}

func TestRunningActivityFormatsSkipDetails(t *testing.T) {
	t.Parallel()

	lines := []string{
		"skip-updated Alien (1979): already updated locally",
		"skip Aliens (1986): no IMP poster available",
		"ambiguous Alien 3 (1992): 2 candidates with long explanation that should not make the movie row huge",
	}
	got := stripANSI(runningActivityView(lines, 6, 48))
	for _, want := range []string{"– UPDATED Alien (1979)", "– SKIP Aliens (1986)", "– AMBIGUOUS Alien 3 (1992)", "Reason:", "2 candidates"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningActivityView() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ambiguous IMP match") {
		t.Fatalf("ambiguous reason kept redundant prefix:\n%s", got)
	}
	if strings.Contains(got, "– AMBIGUOUS Alien 3 (1992): ambiguous") || strings.Contains(got, "– SKIP Aliens (1986): no IMP") {
		t.Fatalf("skip reason stayed on title row:\n%s", got)
	}
	for _, row := range strings.Split(got, "\n") {
		if lipgloss.Width(row) > 48 {
			t.Fatalf("activity row too wide: width=%d row=%q\n%s", lipgloss.Width(row), row, got)
		}
	}
}

func TestRunningActivityFormatsWikiFallbackDetails(t *testing.T) {
	t.Parallel()

	line := "wiki-fallback Alien (1979): https://upload.wikimedia.org/poster.jpg | reason: Wikipedia fallback theatrical poster"
	styled := runningActivityView([]string{line}, 6, 48)
	if !strings.Contains(styled, ui.wiki.Render("↯ WIKI")) {
		t.Fatalf("wiki indicator did not use wiki style:\n%s", styled)
	}
	if ui.wiki.Render("↯ WIKI") == ui.skip.Render("↯ WIKI") {
		t.Fatal("wiki and skip indicators use same style")
	}
	got := stripANSI(styled)
	for _, want := range []string{"↯ WIKI Alien (1979)", "Reason:", "Wikipedia fallback theatrical poster"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningActivityView() missing %q:\n%s", want, got)
		}
	}
}

func TestRunningActivityScrollsAndKeepsFooterFixed(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenRunning
	model.width = 80
	model.height = 24
	model.runningDone = 3
	model.runningTotal = 10
	for i := 0; i < 8; i++ {
		model.log = append(model.log, fmt.Sprintf("updated Movie %d (200%d)", i, i))
	}

	before := stripANSI(model.runningView(0.3))
	if !strings.Contains(before, "more") {
		t.Fatalf("running viewport missing more marker:\n%s", before)
	}
	if strings.Count(before, "Esc: cancel") != 1 || !strings.HasSuffix(strings.TrimSpace(before), "Esc: cancel") {
		t.Fatalf("footer not fixed outside activity viewport:\n%s", before)
	}
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	afterModel := updated.(Model)
	if afterModel.cursor == 0 {
		t.Fatalf("cursor did not advance; limit=%d", model.cursorLimit())
	}
	after := stripANSI(afterModel.runningView(0.3))
	if after == before {
		t.Fatalf("scrolling down did not change activity viewport:\n%s", after)
	}
	if strings.Count(after, "Esc: cancel") != 1 || !strings.HasSuffix(strings.TrimSpace(after), "Esc: cancel") {
		t.Fatalf("footer not fixed after scroll:\n%s", after)
	}
}

func TestRunningViewportFitsTerminalWithoutCroppingBorder(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenRunning
	model.width = 80
	model.height = 18
	model.runningDone = 3
	model.runningTotal = 10
	for i := 0; i < 20; i++ {
		model.log = append(model.log, fmt.Sprintf("updated Movie %d (200%d)", i, i%10))
	}

	view := stripANSI(model.View())
	lines := strings.Split(view, "\n")
	if len(lines) > model.height {
		t.Fatalf("view height = %d, want <= %d\n%s", len(lines), model.height, view)
	}
	first := ""
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			first = strings.TrimSpace(line)
			break
		}
	}
	if !strings.HasPrefix(first, "╭") {
		t.Fatalf("top border cropped or missing; first non-empty line=%q\n%s", first, view)
	}
}

func TestRunningActivityAutoScrollsOnNewUpdate(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenRunning
	model.width = 80
	model.height = 24
	model.runningTotal = 3
	model.runningQueue = []plex.Movie{{Title: "One"}, {Title: "Two"}, {Title: "Three"}}
	for i := 0; i < 12; i++ {
		model.log = append(model.log, fmt.Sprintf("updated Movie %d (200%d)", i, i%10))
	}
	model.cursor = 0
	model, _, _ = model.startOp()

	updated, _ := model.Update(updateOneMsg{opID: model.opID, movie: plex.Movie{Title: "Latest", Year: 2024}, line: "updated Latest (2024)"})
	after := updated.(Model)
	if after.cursor == 0 {
		t.Fatalf("cursor did not auto-scroll; limit=%d", after.cursorLimit())
	}
	view := stripANSI(after.runningView(0.5))
	if !strings.Contains(view, "updated Latest (2024)") {
		t.Fatalf("latest update not visible after auto-scroll:\n%s", view)
	}
}

func TestBaseViewShrinkWrapsContentWithConfiguredPadding(t *testing.T) {
	t.Parallel()

	m := Model{screen: screenLogin, width: 100, height: 40}
	bounds, ok := detectSelectionBounds(stripANSI(m.baseView()))
	if !ok {
		t.Fatal("card bounds not detected")
	}
	if got := bounds.outerRight - bounds.outerLeft + 1; got >= 90 {
		t.Fatalf("card width = %d, want shrink-wrapped under 90", got)
	}
	if got := bounds.outerBot - bounds.outerTop + 1; got >= 20 {
		t.Fatalf("card height = %d, want shrink-wrapped under 20", got)
	}
	if bounds.outerTop == 0 || bounds.outerLeft == 0 {
		t.Fatalf("card not centered in terminal: bounds=%#v", bounds)
	}
	view := stripANSI(m.baseView())
	contentX := -1
	for _, line := range strings.Split(view, "\n") {
		if x := strings.Index(line, "Update Plex"); x >= 0 {
			contentX = x
			break
		}
	}
	if contentX < 0 {
		t.Fatalf("content not found in view:\n%s", view)
	}
	if got := contentX - bounds.outerLeft - 1; got < horizontalContentPadding {
		t.Fatalf("left padding = %d, want at least %d", got, horizontalContentPadding)
	}
	if got := bounds.top - bounds.outerTop - 1; got != verticalContentPadding {
		t.Fatalf("top padding = %d, want %d", got, verticalContentPadding)
	}
	if got := bounds.outerBot - bounds.bot - 1; got != verticalContentPadding {
		t.Fatalf("bottom padding = %d, want %d", got, verticalContentPadding)
	}
}

func TestShellLeftAlignsContentWithConfiguredPadding(t *testing.T) {
	t.Parallel()

	view := stripANSI(shellSized("Short\nSecond line", 100, 40))
	bounds, ok := detectSelectionBounds(view)
	if !ok {
		t.Fatal("card bounds not detected")
	}
	lines := strings.Split(view, "\n")
	contentY := -1
	contentX := -1
	secondX := -1
	for y, line := range lines {
		if x := strings.Index(line, "Short"); x >= 0 {
			contentY, contentX = y, x
		}
		if x := strings.Index(line, "Second line"); x >= 0 {
			secondX = x
		}
	}
	if contentY < 0 {
		t.Fatalf("content not found in view:\n%s", view)
	}
	if secondX != contentX {
		t.Fatalf("text lines not left-aligned: first x=%d second x=%d", contentX, secondX)
	}
	wantX := bounds.outerLeft + 1 + horizontalContentPadding
	if contentX != wantX {
		t.Fatalf("content x=%d, want %d", contentX, wantX)
	}
	if contentY <= bounds.top || contentY >= bounds.bot {
		t.Fatalf("content not inside vertical bounds: y=%d bounds=%#v", contentY, bounds)
	}
}

func TestRunningProgressLineDoesNotWrap(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(Model)
	model.screen = screenRunning
	model.runningDone = 1
	model.runningTotal = 2

	view := stripANSI(model.View())
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "50%") && lipgloss.Width(line) > model.width {
			t.Fatalf("progress line too wide/wrapped risk: width=%d line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestEscCancelsRunningOperation(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenRunning
	var ctx context.Context
	model, ctx, _ = model.startOp()

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if ctx.Err() != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}
	if got.screen != screenDone {
		t.Fatalf("screen = %v, want screenDone", got.screen)
	}
	if len(got.log) == 0 || got.log[len(got.log)-1] != "cancelled" {
		t.Fatalf("log = %#v, want cancellation entry", got.log)
	}
}

func TestStaleUpdateMessageIgnored(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenRunning
	model.runningTotal = 1
	var opID int
	model, _, opID = model.startOp()

	updated, _ := model.Update(updateOneMsg{opID: opID - 1, movie: plex.Movie{Title: "Alien", Year: 1979}, line: "updated"})
	got := updated.(Model)
	if got.runningDone != 0 {
		t.Fatalf("runningDone = %d, want 0", got.runningDone)
	}
	if got.screen != screenRunning {
		t.Fatalf("screen = %v, want screenRunning", got.screen)
	}
}

func TestUnauthorizedErrorClearsTokenAndReturnsLogin(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	model := New(store, fakePlex{})
	var opID int
	model, _, opID = model.startOp()

	updated, _ := model.Update(serversMsg{opID: opID, err: &plex.HTTPError{StatusCode: 401, Status: "401 Unauthorized"}})
	got := updated.(Model)
	if got.screen != screenLogin {
		t.Fatalf("screen = %v, want screenLogin", got.screen)
	}
	if !strings.Contains(got.notice, "Plex session expired") {
		t.Fatalf("notice = %q", got.notice)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState() err = %v", err)
	}
	if state.PlexToken != "" {
		t.Fatalf("PlexToken = %q, want empty", state.PlexToken)
	}
}

func TestReauthenticateKeyClearsTokenFromError(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenError
	model.err = errors.New("boom")

	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := updated.(Model)
	if got.screen != screenLogin {
		t.Fatalf("screen = %v, want screenLogin", got.screen)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState() err = %v", err)
	}
	if state.PlexToken != "" {
		t.Fatalf("PlexToken = %q, want empty", state.PlexToken)
	}
}

func TestWrapBodyWrapsTextWithoutBreakingURLs(t *testing.T) {
	t.Parallel()

	wrapped := wrapBody("Open this very long URL http://www.impawards.com/1979/posters/alien_xxlg.jpg for details", 32)
	for _, line := range strings.Split(wrapped, "\n") {
		if strings.Contains(line, "http://") {
			if line != "http://www.impawards.com/1979/posters/alien_xxlg.jpg" {
				t.Fatalf("URL line = %q, want URL kept whole", line)
			}
			continue
		}
		if len(line) > 32 {
			t.Fatalf("wrapped line too long: %q", line)
		}
	}
}

func TestLinkifyURLsKeepsVisibleText(t *testing.T) {
	t.Parallel()

	url := "http://www.impawards.com/1979/alien.html"
	got := linkifyURLs("Open " + url)
	if !strings.Contains(got, "\x1b]8;;"+url) || !strings.Contains(got, url+"\x1b]8;;\x1b\\") {
		t.Fatalf("linkifyURLs() = %q", got)
	}
	if strings.Count(got, url) != 2 {
		t.Fatalf("linkifyURLs() should keep visible URL and OSC target: %q", got)
	}
}
