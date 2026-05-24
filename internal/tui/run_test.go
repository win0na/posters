package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
	posterfinder "github.com/win0na/posters/internal/posters"
)

func TestStartRunLaunchesConcurrentUpdates(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenMovies
	model.mode = modeAll
	for i := 0; i < posterUpdateConcurrency+2; i++ {
		model.movies = append(model.movies, plex.Movie{RatingKey: fmt.Sprint(i), Title: fmt.Sprintf("Movie %d", i), Year: 2024})
	}

	updated, cmd := model.startRun()
	after := updated.(Model)
	if cmd == nil {
		t.Fatal("startRun() cmd = nil, want batched update commands")
	}
	if after.runningActive != posterUpdateConcurrency || after.runningNext != posterUpdateConcurrency {
		t.Fatalf("active=%d next=%d, want %d/%d", after.runningActive, after.runningNext, posterUpdateConcurrency, posterUpdateConcurrency)
	}
}

func TestUpdateMessageKeepsConcurrentUpdatesFull(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenRunning
	model.runningTotal = posterUpdateConcurrency + 2
	model.runningActive = posterUpdateConcurrency
	model.runningNext = posterUpdateConcurrency
	for i := 0; i < model.runningTotal; i++ {
		model.runningQueue = append(model.runningQueue, plex.Movie{RatingKey: fmt.Sprint(i), Title: fmt.Sprintf("Movie %d", i), Year: 2024})
	}
	model, _, _ = model.startOp()

	updated, cmd := model.Update(updateOneMsg{opID: model.opID, movie: plex.Movie{RatingKey: "0", Title: "Movie 0", Year: 2024}, line: "updated Movie 0 (2024)"})
	after := updated.(Model)
	if cmd == nil {
		t.Fatal("Update(updateOneMsg) cmd = nil, want replacement update command")
	}
	if after.runningDone != 1 || after.runningActive != posterUpdateConcurrency || after.runningNext != posterUpdateConcurrency+1 {
		t.Fatalf("done=%d active=%d next=%d", after.runningDone, after.runningActive, after.runningNext)
	}
	for _, entry := range after.runningCurrent {
		if entry.Movie.RatingKey == "0" {
			t.Fatalf("completed movie still marked current: %#v", after.runningCurrent)
		}
	}
}

func TestRunningHeaderShowsCurrentPosters(t *testing.T) {
	t.Parallel()

	model := Model{
		width:        80,
		runningDone:  1,
		runningTotal: 4,
		runningCurrent: []runningMovie{
			{Movie: plex.Movie{Title: "Alien", Year: 1979}, Phase: "Matching"},
			{Movie: plex.Movie{Title: "Love Lies Bleeding", Year: 2024}, Phase: "uploading"},
		},
	}
	styled := model.runningHeader(0.25)
	if !strings.Contains(styled, ui.worker.Render("Working:")) {
		t.Fatalf("runningHeader() missing worker style:\n%s", styled)
	}
	if ui.worker.Render("Working:") == ui.accent.Render("Working:") {
		t.Fatal("worker style matches accent style, want distinct color")
	}
	if ui.worker.Render("Working:") != ui.warn.Render("Working:") {
		t.Fatal("worker style should use yellow/warn color")
	}
	got := stripANSI(styled)
	for _, want := range []string{"Working:", "Alien (1979)", "Love Lies Bleeding (2024)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningHeader() missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "1. Matching: Alien (1979)") || !strings.Contains(got, "2. Uploading: Love Lies Bleeding (2024)") {
		t.Fatalf("runningHeader() did not render one row per worker:\n%s", got)
	}
	if strings.Contains(got, "worker 1:") || strings.Contains(got, "worker 2:") {
		t.Fatalf("runningHeader() kept redundant worker labels:\n%s", got)
	}
}

func TestForceKeyTogglesInMode(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenMode
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	got := updated.(Model)
	if !got.force {
		t.Fatal("force = false, want true")
	}
}

func TestDryRunKeyTogglesInMode(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenMode
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	got := updated.(Model)
	if !got.dryRun {
		t.Fatal("dryRun = false, want true")
	}
}

func TestWikipediaFallbackKeyTogglesInMode(t *testing.T) {
	t.Parallel()

	model := Model{screen: screenMode}
	updated, _ := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	got := updated.(Model)
	if !got.wikiFallback {
		t.Fatal("wikiFallback = false, want true")
	}
}

func TestUpdateMovieDryRunSkipsUploadAndMetadata(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	client := &spyPlex{}
	finder := &fakeResolver{fakeFinder: fakeFinder{candidate: posterfinder.Candidate{ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", SourceURL: "http://www.impawards.com/1979/alien.html", MatchReason: "single canonical IMP candidate", Bytes: []byte("jpg")}}}
	movie := plex.Movie{RatingKey: "1", Title: "Alien", Year: 1979}
	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, false, true, false, nil)().(updateOneMsg)
	if msg.err != nil {
		t.Fatalf("updateMovie() err = %v", msg.err)
	}
	if !strings.Contains(msg.line, "dry-run Alien (1979): http://www.impawards.com/1979/alien.html") || !strings.Contains(msg.line, "reason: single canonical IMP candidate") {
		t.Fatalf("line = %q", msg.line)
	}
	if client.uploads != 0 {
		t.Fatalf("uploads = %d, want 0", client.uploads)
	}
	if finder.includeBytes {
		t.Fatal("dry-run resolver requested upload bytes")
	}
	updated, err := store.PosterUpdated("1")
	if err != nil {
		t.Fatalf("PosterUpdated() err = %v", err)
	}
	if updated {
		t.Fatal("PosterUpdated() = true, want false")
	}
}

func TestUpdateMovieNormalUploadsAndMarksMetadata(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	client := &spyPlex{}
	finder := &fakeResolver{fakeFinder: fakeFinder{candidate: posterfinder.Candidate{ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", SourceURL: "http://www.impawards.com/1979/alien.html", Bytes: []byte("jpg")}}}
	movie := plex.Movie{RatingKey: "1", Title: "Alien", Year: 1979}
	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, false, false, false, nil)().(updateOneMsg)
	if msg.err != nil {
		t.Fatalf("updateMovie() err = %v", msg.err)
	}
	if !finder.includeBytes {
		t.Fatal("normal resolver did not request upload bytes")
	}
	if client.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", client.uploads)
	}
	updated, err := store.PosterUpdated("1")
	if err != nil {
		t.Fatalf("PosterUpdated() err = %v", err)
	}
	if !updated {
		t.Fatal("PosterUpdated() = false, want true")
	}
}

func TestUpdateMovieForceRefreshBypassesPosterCache(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	client := &spyPlex{}
	finder := &fakeResolver{fakeFinder: fakeFinder{candidate: posterfinder.Candidate{ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", SourceURL: "http://www.impawards.com/1979/alien.html", Bytes: []byte("jpg")}}}
	movie := plex.Movie{RatingKey: "1", Title: "Alien", Year: 1979}

	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, true, false, false, nil)().(updateOneMsg)
	if msg.err != nil {
		t.Fatalf("updateMovie() err = %v", msg.err)
	}
	if !finder.forceRefresh {
		t.Fatal("force refresh was not passed to poster resolver")
	}
}

func TestUpdateMovieFinderError(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	wantErr := errors.New("no match")
	msg := updateMovie(context.Background(), 7, store, &spyPlex{}, fakeFinder{err: wantErr}, plex.Server{}, plex.Movie{Title: "Alien", Year: 1979}, false, true, false, nil)().(updateOneMsg)
	if !errors.Is(msg.err, wantErr) {
		t.Fatalf("err = %v, want %v", msg.err, wantErr)
	}
}

func TestUpdateMovieWikipediaFallbackUploadsAndMarksMetadata(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	client := &spyPlex{}
	finder := fakeFinder{
		err: errors.New("no IMP Awards poster found for Alien (1979)"),
		wikiCandidate: posterfinder.Candidate{
			ImageURL:    "https://upload.wikimedia.org/wikipedia/en/a/alien.jpg",
			SourceURL:   "https://upload.wikimedia.org/wikipedia/en/a/alien.jpg",
			MatchReason: "Wikipedia fallback theatrical poster",
			Bytes:       []byte("wiki"),
		},
	}
	movie := plex.Movie{RatingKey: "1", Title: "Alien", Year: 1979}
	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, false, false, true, nil)().(updateOneMsg)
	if msg.err != nil {
		t.Fatalf("updateMovie() err = %v", msg.err)
	}
	if !strings.Contains(msg.line, "wiki-fallback Alien (1979):") || !strings.Contains(msg.line, "Wikipedia fallback theatrical poster") {
		t.Fatalf("line = %q", msg.line)
	}
	if client.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", client.uploads)
	}
	updated, err := store.PosterUpdated("1")
	if err != nil {
		t.Fatalf("PosterUpdated() err = %v", err)
	}
	if !updated {
		t.Fatal("PosterUpdated() = false, want true")
	}
}

func TestRecordUpdateResultCounts(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.recordUpdateResult(updateOneMsg{line: "updated Alien (1979)"})
	model.recordUpdateResult(updateOneMsg{line: "dry-run Aliens (1986): http://www.impawards.com/1986/aliens.html"})
	model.recordUpdateResult(updateOneMsg{line: "wiki-fallback Prometheus (2012): https://upload.wikimedia.org/poster.jpg"})
	model.recordUpdateResult(updateOneMsg{movie: plex.Movie{Title: "Prometheus", Year: 2012}, err: errors.New("no IMP Awards poster found for Prometheus (2012)")})
	model.recordUpdateResult(updateOneMsg{movie: plex.Movie{Title: "Alien 3", Year: 1992}, err: errors.New("upload failed")})

	if model.runStats.Updated != 1 || model.runStats.DryRun != 1 || model.runStats.WikiFallback != 1 || model.runStats.Skipped != 1 || model.runStats.Failed != 1 {
		t.Fatalf("stats = %#v, want updated/dry-run/wiki/skipped/failed counts", model.runStats)
	}
	if len(model.reportItems) != 5 || model.reportItems[0].Status != "updated" || model.reportItems[1].Status != "dry-run" || model.reportItems[2].Status != "wiki-fallback" || model.reportItems[3].Status != "skipped" || model.reportItems[4].Status != "failed" {
		t.Fatalf("reportItems = %#v", model.reportItems)
	}
	got := resultSummary(model.runStats, false)
	if !strings.Contains(got, "updated: 1") || strings.Contains(got, "dry-run:") || !strings.Contains(got, "wiki: 1") || !strings.Contains(got, "skipped: 1") || !strings.Contains(got, "failed: 1") {
		t.Fatalf("resultSummary() = %q", got)
	}
	dryRun := resultSummary(model.runStats, true)
	if !strings.Contains(dryRun, "dry-run: 1") || strings.Contains(dryRun, "updated:") {
		t.Fatalf("resultSummary(dryRun) = %q", dryRun)
	}
}

func TestFinishRunWritesReport(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.runStats = runStats{Updated: 1}
	model.reportItems = []config.ReportItem{{RatingKey: "1", Title: "Alien", Status: "updated"}}
	model = model.finishRun(false)
	if model.reportPath == "" || !strings.Contains(strings.Join(model.log, "\n"), "report:") {
		t.Fatalf("reportPath=%q log=%#v", model.reportPath, model.log)
	}
}

func TestRecordUpdateResultAmbiguousDetails(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	ambiguous := &posterfinder.AmbiguousMatchError{
		Movie: plex.Movie{Title: "Alien", Year: 1979},
		Candidates: []posterfinder.CandidateSummary{
			{PageURL: "http://www.impawards.com/1979/alien.html", Version: 1, Canonical: true, VisualScore: 0.78, HasVisualScore: true},
			{PageURL: "http://www.impawards.com/1979/alien_ver2.html", Version: 2, VisualScore: 0.74, HasVisualScore: true},
		},
	}
	model.recordUpdateResult(updateOneMsg{movie: plex.Movie{Title: "Alien", Year: 1979}, err: ambiguous})

	if model.runStats.Ambiguous != 1 || model.runStats.Failed != 0 {
		t.Fatalf("stats = %#v, want one ambiguous", model.runStats)
	}
	details := strings.Join(model.details, "\n")
	if !strings.Contains(details, "Alien (1979):") || !strings.Contains(details, "alien_ver2.html") || !strings.Contains(details, "visual match 74.0%") {
		t.Fatalf("details = %q", details)
	}
	if model.reportItems[0].Error != "" || strings.Contains(model.reportItems[0].Message, "impawards.com") {
		t.Fatalf("reportItems = %#v, want links only in ambiguous details", model.reportItems)
	}
	if !strings.Contains(model.reportItems[0].Message, "best visual match 78.0%") {
		t.Fatalf("message = %q, want best visual match", model.reportItems[0].Message)
	}
}
