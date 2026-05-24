package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !strings.Contains(got, "ambiguous Alien (1979): 2 candidates") {
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

type fakeResolver struct {
	fakeFinder
	includeBytes bool
	forceRefresh bool
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

func (f *fakeResolver) ResolveTheatricalPoster(ctx context.Context, movie plex.Movie, includeBytes bool) (posterfinder.Candidate, error) {
	f.includeBytes = includeBytes
	f.forceRefresh = posterfinder.ForceRefresh(ctx)
	return f.fakeFinder.FindTheatricalPoster(ctx, movie)
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
	if model.screen != screenLogin {
		t.Fatalf("screen = %v, want screenLogin", model.screen)
	}
	if model.ctx != nil || model.cancel != nil || model.opID != 0 {
		t.Fatalf("saved-token model started operation before user confirmed")
	}
	if !hasSavedToken(store) {
		t.Fatal("hasSavedToken() = false")
	}
}

func TestSavedLoginEnterContinuesToServerLoading(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	model := New(store, fakePlex{})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.screen != screenAuthWait {
		t.Fatalf("screen = %v, want screenAuthWait", got.screen)
	}
	if got.ctx == nil || got.cancel == nil || got.opID == 0 {
		t.Fatalf("saved-token continue did not start operation")
	}
	if cmd == nil {
		t.Fatal("saved-token continue returned nil command")
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
	got := renderMovies(movies, 10, map[string]bool{}, nil, 5)
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
		got := renderMovies(movies, cursor, map[string]bool{}, nil, 6)
		heights[len(strings.Split(got, "\n"))] = true
	}
	if len(heights) != 1 || !heights[6] {
		t.Fatalf("viewport heights = %#v, want exactly 6", heights)
	}
}

func TestRenderMoviesMarksBlacklisted(t *testing.T) {
	t.Parallel()

	got := renderMovies([]plex.Movie{{RatingKey: "1", Title: "Alien", Year: 1979}}, 0, map[string]bool{"1": true}, map[string]bool{"1": true}, 5)
	if !strings.Contains(got, "[!] Alien (1979)") {
		t.Fatalf("renderMovies() = %q", got)
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
	pending, skipped, blacklisted, err := model.pendingMovies([]plex.Movie{
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
	if blacklisted != 0 {
		t.Fatalf("blacklisted = %d, want none", blacklisted)
	}
}

func TestPendingMoviesSkipsBlacklisted(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.BlacklistMovie(config.BlacklistItem{RatingKey: "1", Title: "Alien", Year: 1979}); err != nil {
		t.Fatalf("BlacklistMovie() err = %v", err)
	}

	model := New(store, fakePlex{})
	model.force = true
	pending, updated, blacklisted, err := model.pendingMovies([]plex.Movie{
		{RatingKey: "1", Title: "Alien", Year: 1979},
		{RatingKey: "2", Title: "Aliens", Year: 1986},
	})
	if err != nil {
		t.Fatalf("pendingMovies() err = %v", err)
	}
	if len(pending) != 1 || pending[0].RatingKey != "2" {
		t.Fatalf("pending = %#v, want only rating key 2", pending)
	}
	if updated != 0 || blacklisted != 1 {
		t.Fatalf("updated=%d blacklisted=%d, want 0/1", updated, blacklisted)
	}
}

func TestMovieBlacklistTogglePersistsAndClearsSelection(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenMovies
	model.movies = []plex.Movie{{RatingKey: "1", Title: "Alien", Year: 1979}}
	model.chosen["1"] = true

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	got := updated.(Model)
	blacklisted, err := store.MovieBlacklisted("1")
	if err != nil {
		t.Fatalf("MovieBlacklisted() err = %v", err)
	}
	if !blacklisted {
		t.Fatal("movie not blacklisted")
	}
	if got.chosen["1"] {
		t.Fatal("blacklisted movie remained selected")
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	blacklisted, err = store.MovieBlacklisted("1")
	if err != nil {
		t.Fatalf("MovieBlacklisted() err = %v", err)
	}
	if blacklisted {
		t.Fatal("movie still blacklisted after second toggle")
	}
}

func TestBlacklistScreenRemovesHighlightedItem(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.BlacklistMovie(config.BlacklistItem{RatingKey: "1", Title: "Alien", Year: 1979}); err != nil {
		t.Fatalf("BlacklistMovie() err = %v", err)
	}
	model := New(store, fakePlex{})
	model.screen = screenBlacklist

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	got := updated.(Model)
	blacklisted, err := store.MovieBlacklisted("1")
	if err != nil {
		t.Fatalf("MovieBlacklisted() err = %v", err)
	}
	if blacklisted {
		t.Fatal("blacklist screen did not remove highlighted item")
	}
	if !strings.Contains(got.notice, "Removed Alien") {
		t.Fatalf("notice = %q", got.notice)
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
	pending, skipped, blacklisted, err := model.pendingMovies([]plex.Movie{{RatingKey: "1", Title: "Alien", Year: 1979}})
	if err != nil {
		t.Fatalf("pendingMovies() err = %v", err)
	}
	if len(pending) != 1 || pending[0].RatingKey != "1" {
		t.Fatalf("pending = %#v, want updated movie included", pending)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want none", skipped)
	}
	if blacklisted != 0 {
		t.Fatalf("blacklisted = %d, want none", blacklisted)
	}
}
