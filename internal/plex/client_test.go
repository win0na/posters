package plex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/win0na/posters/internal/config"
)

func TestUploadPosterMultipart(t *testing.T) {
	store := testStore(t)
	var gotPath, gotToken, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Plex-Token")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "poster-data") {
			t.Fatalf("multipart body missing poster data: %q", string(body))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(store)
	err := client.UploadPoster(context.Background(), Server{URI: server.URL}, Movie{RatingKey: "123"}, "poster.jpg", []byte("poster-data"), "")
	if err != nil {
		t.Fatalf("UploadPoster() error = %v", err)
	}
	if gotPath != "/library/metadata/123/posters" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "state-token" {
		t.Fatalf("X-Plex-Token = %q", gotToken)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
}

func TestUploadPosterFallsBackToSourceURL(t *testing.T) {
	store := testStore(t)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.String())
		if len(requests) == 3 {
			if got := r.URL.Query().Get("url"); got != "http://www.impawards.com/1979/posters/alien.jpg" {
				t.Fatalf("url query = %q", got)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
	}))
	defer server.Close()

	client := NewClient(store)
	err := client.UploadPoster(context.Background(), Server{URI: server.URL}, Movie{RatingKey: "456"}, "poster.jpg", []byte("poster-data"), "http://www.impawards.com/1979/posters/alien.jpg")
	if err != nil {
		t.Fatalf("UploadPoster() error = %v", err)
	}
	want := []string{
		"POST /library/metadata/456/posters",
		"POST /library/metadata/456/poster",
		"POST /library/metadata/456/posters?url=http%3A%2F%2Fwww.impawards.com%2F1979%2Fposters%2Falien.jpg",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(requests, "\n"))
	}
}

func TestUploadPosterFailureGivesSmokeTestGuidance(t *testing.T) {
	store := testStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
	}))
	defer server.Close()

	client := NewClient(store)
	err := client.UploadPoster(context.Background(), Server{URI: server.URL}, Movie{RatingKey: "789"}, "poster.jpg", []byte("poster-data"), "http://www.impawards.com/1979/posters/alien.jpg")
	if err == nil {
		t.Fatal("UploadPoster() err = nil")
	}
	text := err.Error()
	for _, want := range []string{"PMS artwork endpoint may differ", "smoke test", "upload poster multipart /library/metadata/789/posters", "415 Unsupported Media Type"} {
		if !strings.Contains(text, want) {
			t.Fatalf("err missing %q: %v", want, err)
		}
	}
}

func TestListMoviesPaginates(t *testing.T) {
	store := testStore(t)
	starts := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		starts = append(starts, r.URL.Query().Get("X-Plex-Container-Start"))
		start, _ := strconv.Atoi(r.URL.Query().Get("X-Plex-Container-Start"))
		movies := []map[string]any{}
		for i := start; i < start+moviePageSize && i < 205; i++ {
			movies = append(movies, map[string]any{"ratingKey": strconv.Itoa(i), "title": "Movie " + strconv.Itoa(i), "originalTitle": "Original " + strconv.Itoa(i), "year": 2000 + i%20, "guid": "guid-" + strconv.Itoa(i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"totalSize": 205, "Metadata": movies}})
	}))
	defer server.Close()

	client := NewClient(store)
	movies, err := client.ListMovies(context.Background(), Server{URI: server.URL}, Library{Key: "1"})
	if err != nil {
		t.Fatalf("ListMovies() error = %v", err)
	}
	if len(movies) != 205 {
		t.Fatalf("len(movies) = %d", len(movies))
	}
	wantStarts := []string{"0", "100", "200"}
	if strings.Join(starts, ",") != strings.Join(wantStarts, ",") {
		t.Fatalf("starts = %#v", starts)
	}
	if movies[204].RatingKey != "204" {
		t.Fatalf("last movie = %#v", movies[204])
	}
	if movies[204].OriginalTitle != "Original 204" {
		t.Fatalf("last movie original title = %q", movies[204].OriginalTitle)
	}
}

func TestUnauthorizedHTTPError(t *testing.T) {
	store := testStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(store)
	_, err := client.ListMovies(context.Background(), Server{URI: server.URL}, Library{Key: "1"})
	if err == nil {
		t.Fatal("ListMovies() err = nil")
	}
	if !errors.Is(err, ErrUnauthorized) || !IsUnauthorized(err) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func testStore(t *testing.T) *config.Store {
	t.Helper()
	store, err := config.OpenDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(config.State{ClientID: "client-id", PlexToken: "state-token"}); err != nil {
		t.Fatal(err)
	}
	return store
}
