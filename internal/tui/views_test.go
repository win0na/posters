package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/win0na/posters/internal/config"
)

func TestDoneViewIncludesSummaryAndDetails(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenDone
	model.height = 40
	model.runStats = runStats{Updated: 1, Skipped: 2, Ambiguous: 1}
	model.log = []string{"updated Alien (1979)", "skip Aliens (1986): already updated"}
	model.details = []string{"Alien 3 (1992):", "  - http://www.impawards.com/1992/alien_three.html"}

	view := stripANSI(model.View())
	for _, want := range []string{"Updated:   1", "Skipped:   2", "Ambiguous: 1", "Ambiguous matches:", "alien_three.html"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestDoneViewShowsLegibleReportSections(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenDone
	model.height = 80
	model.runStats = runStats{DryRun: 1}
	model.reportPath = "/tmp/posters/report.json"
	model.reportCSVPath = "/tmp/posters/report.csv"
	model.reportItems = []config.ReportItem{{Title: "Alien", Year: 1979, Status: "dry-run", SourceURL: "http://www.impawards.com/1979/alien.html", ImageURL: "http://www.impawards.com/1979/posters/alien_xxlg.jpg", MatchReason: "visual match score 0.991"}}

	view := stripANSI(model.View())
	for _, want := range []string{"Summary:", "Reports:", "Results:", "DRY-RUN Alien (1979)", "IMP page:", "Image:", "Match:"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestLoginViewUsesReadableSections(t *testing.T) {
	t.Parallel()

	view := stripANSI(Model{screen: screenLogin, width: 100, height: 32}.View())
	for _, want := range []string{"Goal", "Controls", "Enter  login to Plex", "r      clear saved login"} {
		if !strings.Contains(view, want) {
			t.Fatalf("login view missing %q:\n%s", want, view)
		}
	}
}

func TestLoginViewOffersSavedLoginClearBeforeLibrarySelection(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	view := stripANSI(New(store, fakePlex{}).View())
	for _, want := range []string{"Enter  continue with saved login", "r      clear saved login"} {
		if !strings.Contains(view, want) {
			t.Fatalf("login view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Saved login") {
		t.Fatalf("login view kept saved-login description:\n%s", view)
	}
}

func TestStyledViewsEmitANSI(t *testing.T) {
	t.Parallel()

	login := Model{screen: screenLogin, width: 100, height: 32}.View()
	if !strings.Contains(login, "\x1b[") {
		t.Fatal("login view missing ANSI styling")
	}

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	styled := New(store, fakePlex{})
	styled.screen = screenDone
	styled.reportItems = []config.ReportItem{{Title: "Alien", Year: 1979, Status: "updated"}}
	done := styled.doneView(doneRows(18))
	if !strings.Contains(done, "\x1b[") {
		t.Fatal("done view missing ANSI styling")
	}
}

func TestDoneViewScrollsLongReport(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenDone
	for i := 0; i < 12; i++ {
		model.reportItems = append(model.reportItems, config.ReportItem{Title: "Movie", Year: 2000 + i, Status: "dry-run", SourceURL: "http://www.impawards.com/1979/alien.html"})
	}

	first := model.doneView(8)
	if !strings.Contains(first, "more") || strings.Contains(first, "Movie (2011)") {
		t.Fatalf("first viewport not clipped as expected:\n%s", first)
	}
	model.cursor = 999
	second := model.doneView(8)
	if !strings.Contains(second, "earlier") || !strings.Contains(second, "Movie (2011)") {
		t.Fatalf("scrolled viewport missing tail:\n%s", second)
	}
}

func TestDoneReportCanScrollWhenBottomMarkerShowsMore(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenDone
	model.height = 18
	for i := 0; i < 5; i++ {
		model.reportItems = append(model.reportItems, config.ReportItem{Title: "Movie", Year: 2000 + i, Status: "dry-run", SourceURL: "http://example.com/poster"})
	}
	before := model.doneView(doneRows(model.height))
	if !strings.Contains(before, "more") {
		t.Fatalf("initial viewport missing more marker:\n%s", before)
	}
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	afterModel := updated.(Model)
	if afterModel.cursor == 0 {
		t.Fatalf("cursor did not advance despite more marker; limit=%d", model.cursorLimit())
	}
	after := afterModel.doneView(doneRows(afterModel.height))
	if after == before {
		t.Fatalf("scrolling down did not change report viewport:\n%s", after)
	}
}

func TestDoneFooterStaysOutsideScrollViewport(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenDone
	model.height = 18
	for i := 0; i < 8; i++ {
		model.reportItems = append(model.reportItems, config.ReportItem{Title: "Movie", Year: 2000 + i, Status: "dry-run", SourceURL: "http://example.com/poster"})
	}
	view := model.doneView(doneRows(model.height))
	if strings.Count(view, "Enter/q: quit") != 1 {
		t.Fatalf("footer count wrong:\n%s", view)
	}
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
	scrolled := updated.(Model).doneView(doneRows(model.height))
	plain := stripANSI(scrolled)
	if strings.Count(plain, "Enter/q: quit") != 1 {
		t.Fatalf("footer count wrong after scroll:\n%s", scrolled)
	}
	if !strings.HasSuffix(strings.TrimSpace(plain), "Enter/q: quit") {
		t.Fatalf("footer not fixed at bottom:\n%s", scrolled)
	}
}

func TestDoneReportBottomMarkerMatchesScrollLimit(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenDone
	model.height = 18
	for i := 0; i < 8; i++ {
		model.reportItems = append(model.reportItems, config.ReportItem{Title: "Movie", Year: 2000 + i, Status: "dry-run", SourceURL: "http://example.com/poster"})
	}
	for i := 0; i < model.cursorLimit()+5; i++ {
		updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	view := model.doneView(doneRows(model.height))
	if strings.Contains(view, "more") {
		t.Fatalf("bottom viewport still advertises hidden lines:\n%s", view)
	}
	if !strings.Contains(view, "Enter/q: quit") {
		t.Fatalf("footer missing:\n%s", view)
	}
}
