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

type fakePlex struct{}

func (fakePlex) StartPIN(context.Context) (plex.Pin, string, error) { return plex.Pin{}, "", nil }
func (fakePlex) PollPIN(context.Context, int) (string, error)       { return "", nil }
func (fakePlex) ListServers(context.Context) ([]plex.Server, error) { return nil, nil }
func (fakePlex) ListLibraries(context.Context, plex.Server) ([]plex.Library, error) {
	return nil, nil
}

func TestFormatUpdateErrorAmbiguous(t *testing.T) {
	err := &posterfinder.AmbiguousMatchError{
		Movie: plex.Movie{Title: "Alien", Year: 1979},
		Candidates: []posterfinder.CandidateSummary{
			{PageURL: "http://www.impawards.com/1979/alien_ver2.html"},
			{PageURL: "http://www.impawards.com/1979/alien_ver3.html"},
		},
	}
	got := formatUpdateError(plex.Movie{Title: "Alien", Year: 1979}, err)
	if !strings.Contains(got, "ambiguous Alien (1979): ambiguous IMP match: 2 candidates") {
		t.Fatalf("formatUpdateError() = %q", got)
	}
}

func TestFormatUpdateErrorNoIMP(t *testing.T) {
	got := formatUpdateError(plex.Movie{Title: "Alien", Year: 1979}, errors.New("no IMP Awards poster found for Alien (1979)"))
	if got != "skip Alien (1979): no IMP poster available" {
		t.Fatalf("formatUpdateError() = %q", got)
	}
}

func (fakePlex) ListMovies(context.Context, plex.Server, plex.Library) ([]plex.Movie, error) {
	return nil, nil
}
func (fakePlex) UploadPoster(context.Context, plex.Server, plex.Movie, string, []byte, string) error {
	return nil
}

type spyPlex struct {
	fakePlex
	uploads int
}

func (p *spyPlex) UploadPoster(context.Context, plex.Server, plex.Movie, string, []byte, string) error {
	p.uploads++
	return nil
}

type fakeFinder struct {
	candidate     posterfinder.Candidate
	wikiCandidate posterfinder.Candidate
	err           error
}

func (f fakeFinder) FindTheatricalPoster(context.Context, plex.Movie) (posterfinder.Candidate, error) {
	if f.err != nil {
		return posterfinder.Candidate{}, f.err
	}
	return f.candidate, nil
}

func (f fakeFinder) FindWikipediaPoster(context.Context, plex.Movie) (posterfinder.Candidate, error) {
	if f.err != nil && f.wikiCandidate.Bytes == nil {
		return posterfinder.Candidate{}, f.err
	}
	return f.wikiCandidate, nil
}

func TestServersMsgShowsServerPicker(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	var opID int
	model, _, opID = model.startOp()
	updated, _ := model.Update(serversMsg{opID: opID, servers: []plex.Server{{Name: "NAS", URI: "http://nas:32400"}}})
	got := updated.(Model)
	if got.screen != screenServers {
		t.Fatalf("screen = %v, want screenServers", got.screen)
	}
	if got.servers[0].Name != "NAS" {
		t.Fatalf("servers = %#v", got.servers)
	}
}

func TestNewUsesSavedToken(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	model := New(store, fakePlex{})
	if model.screen != screenAuthWait {
		t.Fatalf("screen = %v, want screenAuthWait", model.screen)
	}
	if model.ctx == nil || model.cancel == nil || model.opID == 0 {
		t.Fatalf("saved-token model missing active operation context")
	}
	if !hasSavedToken(store) {
		t.Fatal("hasSavedToken() = false")
	}
}

func TestNewWithoutSavedTokenShowsLogin(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	if model.screen != screenLogin {
		t.Fatalf("screen = %v, want screenLogin", model.screen)
	}
	if hasSavedToken(store) {
		t.Fatal("hasSavedToken() = true")
	}
}

func TestServerLabel(t *testing.T) {
	t.Parallel()

	got := serverLabel(plex.Server{Name: "NAS", URI: "http://nas:32400"})
	if got != "NAS" {
		t.Fatalf("serverLabel() = %q", got)
	}
	got = serverLabel(plex.Server{Name: "NAS"})
	if got != "NAS" {
		t.Fatalf("serverLabel() without URI = %q", got)
	}
}

func TestRenderMoviesUsesViewportAroundCursor(t *testing.T) {
	movies := make([]plex.Movie, 20)
	for i := range movies {
		movies[i] = plex.Movie{RatingKey: string(rune('a' + i)), Title: "Movie " + string(rune('A'+i)), Year: 2000 + i}
	}
	got := renderMovies(movies, 10, map[string]bool{}, 5)
	if strings.Contains(got, "Movie A") {
		t.Fatalf("viewport included first movie: %q", got)
	}
	if !strings.Contains(got, "› [ ] Movie K (2010)") {
		t.Fatalf("viewport missing selected movie: %q", got)
	}
	if !strings.Contains(got, "… 9 earlier") || !strings.Contains(got, "… 8 more") {
		t.Fatalf("viewport missing overflow markers: %q", got)
	}
}

func TestRenderMoviesKeepsConstantHeightWhileScrolling(t *testing.T) {
	movies := make([]plex.Movie, 20)
	for i := range movies {
		movies[i] = plex.Movie{RatingKey: string(rune('a' + i)), Title: "Movie", Year: 2000 + i}
	}
	heights := map[int]bool{}
	for _, cursor := range []int{0, 1, 5, 10, 18, 19} {
		got := renderMovies(movies, cursor, map[string]bool{}, 6)
		heights[len(strings.Split(got, "\n"))] = true
	}
	if len(heights) != 1 || !heights[6] {
		t.Fatalf("viewport heights = %#v, want exactly 6", heights)
	}
}

func TestMoviePickerBodyFitsSmallTerminal(t *testing.T) {
	movies := make([]plex.Movie, 30)
	for i := range movies {
		movies[i] = plex.Movie{RatingKey: string(rune('a' + i)), Title: "Movie", Year: 2000 + i}
	}
	m := Model{screen: screenMovies, movies: movies, cursor: 20, chosen: map[string]bool{}, width: 80, height: 16}
	view := m.baseView()
	lines := strings.Split(stripANSI(view), "\n")
	if len(lines) > 16 {
		t.Fatalf("view height = %d, want <= 16", len(lines))
	}
	if !strings.Contains(stripANSI(view), "› [ ] Movie (2020)") {
		t.Fatalf("view missing cursor row: %q", view)
	}
}

func TestMoviePickerKeepsTopBorderOnTinyTerminal(t *testing.T) {
	movies := make([]plex.Movie, 30)
	for i := range movies {
		movies[i] = plex.Movie{RatingKey: string(rune('a' + i)), Title: "Movie", Year: 2000 + i}
	}
	for _, height := range []int{14, 15, 16, 17, 18} {
		m := Model{screen: screenMovies, movies: movies, cursor: 20, chosen: map[string]bool{}, width: 80, height: height}
		view := stripANSI(m.baseView())
		lines := strings.Split(view, "\n")
		if len(lines) > height {
			t.Fatalf("height %d: view height = %d, want <= %d\n%s", height, len(lines), height, view)
		}
		first := ""
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				first = line
				break
			}
		}
		if !strings.Contains(first, "╭") {
			t.Fatalf("height %d: top border cropped; first visible line = %q\n%s", height, first, view)
		}
		if !strings.Contains(view, "› [ ] Movie (2020)") {
			t.Fatalf("height %d: view missing cursor row: %q", height, view)
		}
	}
}

func TestDownKeyClampsMovieCursor(t *testing.T) {
	m := Model{screen: screenMovies, movies: []plex.Movie{{RatingKey: "1"}, {RatingKey: "2"}}, cursor: 1}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if got.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", got.cursor)
	}
}

func TestPreferredCursorsUseSavedSelection(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveLastSelection("server-2", "NAS", "http://nas:32400", "7", "Movies"); err != nil {
		t.Fatalf("SaveLastSelection() err = %v", err)
	}
	servers := []plex.Server{{ClientID: "server-1", URI: "http://one:32400"}, {ClientID: "server-2", URI: "http://nas:32400"}}
	libs := []plex.Library{{Key: "1", Title: "Other"}, {Key: "7", Title: "Movies"}}
	if got := preferredServerCursor(store, servers); got != 1 {
		t.Fatalf("preferredServerCursor() = %d, want 1", got)
	}
	if got := preferredLibraryCursor(store, libs); got != 1 {
		t.Fatalf("preferredLibraryCursor() = %d, want 1", got)
	}
}

func TestStatusViewIncludesConfigAndMetadata(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token", LastServerName: "NAS", LastLibraryTitle: "Movies"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	if err := store.MarkPosterUpdated(config.PosterItem{RatingKey: "1", Title: "Alien"}); err != nil {
		t.Fatalf("MarkPosterUpdated() err = %v", err)
	}
	model := NewWithOptions(store, fakePlex{}, Options{Force: true, DryRun: true})
	model.screen = screenStatus
	view := stripANSI(model.View())
	for _, want := range []string{"Status", "Plex token: present", "Metadata items: 1", "Server: NAS", "Library: Movies"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Force refresh:", "Dry run:"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("View() contains %q:\n%s", unwanted, view)
		}
	}
}

func TestPendingMoviesSkipsUpdated(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.MarkPosterUpdated(config.PosterItem{RatingKey: "1", Title: "Alien", Year: 1979}); err != nil {
		t.Fatalf("MarkPosterUpdated() err = %v", err)
	}

	model := New(store, fakePlex{})
	pending, skipped, err := model.pendingMovies([]plex.Movie{
		{RatingKey: "1", Title: "Alien", Year: 1979},
		{RatingKey: "2", Title: "Aliens", Year: 1986},
	})
	if err != nil {
		t.Fatalf("pendingMovies() err = %v", err)
	}
	if len(pending) != 1 || pending[0].RatingKey != "2" {
		t.Fatalf("pending = %#v, want only rating key 2", pending)
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want one skip", skipped)
	}
}

func TestPendingMoviesForceIncludesUpdated(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.MarkPosterUpdated(config.PosterItem{RatingKey: "1", Title: "Alien", Year: 1979}); err != nil {
		t.Fatalf("MarkPosterUpdated() err = %v", err)
	}

	model := New(store, fakePlex{})
	model.force = true
	pending, skipped, err := model.pendingMovies([]plex.Movie{{RatingKey: "1", Title: "Alien", Year: 1979}})
	if err != nil {
		t.Fatalf("pendingMovies() err = %v", err)
	}
	if len(pending) != 1 || pending[0].RatingKey != "1" {
		t.Fatalf("pending = %#v, want updated movie included", pending)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want none", skipped)
	}
}

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
	for _, movie := range after.runningCurrent {
		if movie.RatingKey == "0" {
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
		runningCurrent: []plex.Movie{
			{Title: "Alien", Year: 1979},
			{Title: "Love Lies Bleeding", Year: 2024},
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
	if !strings.Contains(got, "1: Alien (1979)") || !strings.Contains(got, "2: Love Lies Bleeding (2024)") {
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
	finder := fakeFinder{candidate: posterfinder.Candidate{ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", SourceURL: "http://www.impawards.com/1979/alien.html", MatchReason: "single canonical IMP candidate", Bytes: []byte("jpg")}}
	movie := plex.Movie{RatingKey: "1", Title: "Alien", Year: 1979}
	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, true, false)().(updateOneMsg)
	if msg.err != nil {
		t.Fatalf("updateMovie() err = %v", msg.err)
	}
	if !strings.Contains(msg.line, "dry-run Alien (1979): http://www.impawards.com/1979/alien.html") || !strings.Contains(msg.line, "reason: single canonical IMP candidate") {
		t.Fatalf("line = %q", msg.line)
	}
	if client.uploads != 0 {
		t.Fatalf("uploads = %d, want 0", client.uploads)
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
	finder := fakeFinder{candidate: posterfinder.Candidate{ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", SourceURL: "http://www.impawards.com/1979/alien.html", Bytes: []byte("jpg")}}
	movie := plex.Movie{RatingKey: "1", Title: "Alien", Year: 1979}
	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, false, false)().(updateOneMsg)
	if msg.err != nil {
		t.Fatalf("updateMovie() err = %v", msg.err)
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

func TestUpdateMovieFinderError(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	wantErr := errors.New("no match")
	msg := updateMovie(context.Background(), 7, store, &spyPlex{}, fakeFinder{err: wantErr}, plex.Server{}, plex.Movie{Title: "Alien", Year: 1979}, true, false)().(updateOneMsg)
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
	msg := updateMovie(context.Background(), 7, store, client, finder, plex.Server{}, movie, false, true)().(updateOneMsg)
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
			{PageURL: "http://www.impawards.com/1979/alien.html", Version: 1, Canonical: true},
			{PageURL: "http://www.impawards.com/1979/alien_ver2.html", Version: 2},
		},
	}
	model.recordUpdateResult(updateOneMsg{movie: plex.Movie{Title: "Alien", Year: 1979}, err: ambiguous})

	if model.runStats.Ambiguous != 1 || model.runStats.Failed != 0 {
		t.Fatalf("stats = %#v, want one ambiguous", model.runStats)
	}
	details := strings.Join(model.details, "\n")
	if !strings.Contains(details, "Alien (1979):") || !strings.Contains(details, "alien_ver2.html") {
		t.Fatalf("details = %q", details)
	}
}

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
		"ambiguous Alien 3 (1992): ambiguous IMP match: 2 candidates with long explanation that should not make the movie row huge",
	}
	got := stripANSI(runningActivityView(lines, 6, 48))
	for _, want := range []string{"– UPDATED Alien (1979)", "– SKIP Aliens (1986)", "– AMBIGUOUS Alien 3 (1992)", "Reason:", "ambiguous IMP match"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runningActivityView() missing %q:\n%s", want, got)
		}
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
