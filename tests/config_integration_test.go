package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/win0na/posters/internal/config"
)

func TestConfigPosterUpdated(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}

	updated, err := store.PosterUpdated("1")
	if err != nil {
		t.Fatalf("PosterUpdated() err = %v", err)
	}
	if updated {
		t.Fatal("PosterUpdated() = true before mark")
	}

	err = store.MarkPosterUpdated(config.PosterItem{RatingKey: "1", Title: "Alien", Year: 1979, SourceURL: "http://www.impawards.com/1979/alien.html"})
	if err != nil {
		t.Fatalf("MarkPosterUpdated() err = %v", err)
	}

	updated, err = store.PosterUpdated("1")
	if err != nil {
		t.Fatalf("PosterUpdated() err = %v", err)
	}
	if !updated {
		t.Fatal("PosterUpdated() = false after mark")
	}
}

func TestConfigBlacklistMovie(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}

	blacklisted, err := store.MovieBlacklisted("1")
	if err != nil {
		t.Fatalf("MovieBlacklisted() err = %v", err)
	}
	if blacklisted {
		t.Fatal("MovieBlacklisted() = true before mark")
	}

	if err := store.BlacklistMovie(config.BlacklistItem{RatingKey: "1", Title: "Alien", Year: 1979}); err != nil {
		t.Fatalf("BlacklistMovie() err = %v", err)
	}
	blacklisted, err = store.MovieBlacklisted("1")
	if err != nil {
		t.Fatalf("MovieBlacklisted() err = %v", err)
	}
	if !blacklisted {
		t.Fatal("MovieBlacklisted() = false after mark")
	}

	metadata, err := store.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata() err = %v", err)
	}
	if metadata.Blacklist["1"].Title != "Alien" {
		t.Fatalf("blacklist = %#v", metadata.Blacklist)
	}

	if err := store.UnblacklistMovie("1"); err != nil {
		t.Fatalf("UnblacklistMovie() err = %v", err)
	}
	blacklisted, err = store.MovieBlacklisted("1")
	if err != nil {
		t.Fatalf("MovieBlacklisted() err = %v", err)
	}
	if blacklisted {
		t.Fatal("MovieBlacklisted() = true after unblacklist")
	}
}

func TestConfigClearPlexTokenPreservesClientID(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	if err := store.ClearPlexToken(); err != nil {
		t.Fatalf("ClearPlexToken() err = %v", err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState() err = %v", err)
	}
	if state.ClientID != "client-id" {
		t.Fatalf("ClientID = %q, want preserved", state.ClientID)
	}
	if state.PlexToken != "" {
		t.Fatalf("PlexToken = %q, want empty", state.PlexToken)
	}
}

func TestConfigSaveLastSelection(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "token"}); err != nil {
		t.Fatalf("SaveState() err = %v", err)
	}
	if err := store.SaveLastSelection("server-id", "NAS", "http://nas:32400", "7", "Movies"); err != nil {
		t.Fatalf("SaveLastSelection() err = %v", err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState() err = %v", err)
	}
	if state.LastServerID != "server-id" || state.LastServerName != "NAS" || state.LastLibraryKey != "7" || state.LastLibraryTitle != "Movies" {
		t.Fatalf("state = %#v", state)
	}
}

func TestConfigSaveRunReport(t *testing.T) {
	t.Parallel()

	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	jsonPath, csvPath, err := store.SaveRunReport(config.RunReport{
		Stats: config.ReportStats{Updated: 1},
		Items: []config.ReportItem{{RatingKey: "1", Title: "Alien", Year: 1979, Status: "updated", SourceURL: "http://www.impawards.com/1979/alien.html"}},
	})
	if err != nil {
		t.Fatalf("SaveRunReport() err = %v", err)
	}
	if filepath.Dir(jsonPath) != filepath.Join(store.Dir(), "reports") || filepath.Dir(csvPath) != filepath.Join(store.Dir(), "reports") {
		t.Fatalf("paths = %q %q", jsonPath, csvPath)
	}
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read json err = %v", err)
	}
	if !strings.Contains(string(jsonData), `"updated": 1`) || !strings.Contains(string(jsonData), `"source_url"`) {
		t.Fatalf("json report = %s", string(jsonData))
	}
	csvData, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read csv err = %v", err)
	}
	if !strings.Contains(string(csvData), "rating_key,title,year,status") || !strings.Contains(string(csvData), "Alien") {
		t.Fatalf("csv report = %s", string(csvData))
	}
}
