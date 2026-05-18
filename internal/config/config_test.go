package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPosterUpdated(t *testing.T) {
	t.Parallel()

	store, err := OpenDir(t.TempDir())
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

	err = store.MarkPosterUpdated(PosterItem{RatingKey: "1", Title: "Alien", Year: 1979, SourceURL: "http://www.impawards.com/1979/alien.html"})
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

func TestClearPlexTokenPreservesClientID(t *testing.T) {
	t.Parallel()

	store, err := OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(State{ClientID: "client-id", PlexToken: "token"}); err != nil {
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

func TestSaveLastSelection(t *testing.T) {
	t.Parallel()

	store, err := OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	if err := store.SaveState(State{ClientID: "client-id", PlexToken: "token"}); err != nil {
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

func TestSaveRunReport(t *testing.T) {
	t.Parallel()

	store, err := OpenDir(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDir() err = %v", err)
	}
	jsonPath, csvPath, err := store.SaveRunReport(RunReport{
		Stats: ReportStats{Updated: 1},
		Items: []ReportItem{{RatingKey: "1", Title: "Alien", Year: 1979, Status: "updated", SourceURL: "http://www.impawards.com/1979/alien.html"}},
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
