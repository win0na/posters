package posters

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchTextUsesCache(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("cached body"))
	}))
	defer server.Close()

	service := &Service{http: server.Client(), cacheDir: t.TempDir()}
	first, err := service.fetchText(context.Background(), server.URL+"/page")
	if err != nil {
		t.Fatalf("fetchText first err = %v", err)
	}
	second, err := service.fetchText(context.Background(), server.URL+"/page")
	if err != nil {
		t.Fatalf("fetchText second err = %v", err)
	}
	if first != "cached body" || second != "cached body" || requests != 1 {
		t.Fatalf("first=%q second=%q requests=%d", first, second, requests)
	}
}

func TestFetchTextForceRefreshBypassesCache(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(fmt.Sprintf("body %d", requests)))
	}))
	defer server.Close()

	service := &Service{http: server.Client(), cacheDir: t.TempDir()}
	first, err := service.fetchText(context.Background(), server.URL+"/page")
	if err != nil {
		t.Fatalf("fetchText first err = %v", err)
	}
	forced, err := service.fetchText(WithForceRefresh(context.Background()), server.URL+"/page")
	if err != nil {
		t.Fatalf("fetchText forced err = %v", err)
	}
	if first != "body 1" || forced != "body 2" || requests != 2 {
		t.Fatalf("first=%q forced=%q requests=%d", first, forced, requests)
	}
}

func TestFetchTextCachesNotFound(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer server.Close()

	service := &Service{http: server.Client(), cacheDir: t.TempDir()}
	if _, err := service.fetchText(context.Background(), server.URL+"/missing"); err == nil {
		t.Fatal("fetchText first err = nil, want not found")
	}
	if _, err := service.fetchText(context.Background(), server.URL+"/missing"); err == nil {
		t.Fatal("fetchText second err = nil, want cached not found")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestFetchTextForceRefreshBypassesNegativeCache(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("fresh body"))
	}))
	defer server.Close()

	service := &Service{http: server.Client(), cacheDir: t.TempDir()}
	if _, err := service.fetchText(context.Background(), server.URL+"/missing"); err == nil {
		t.Fatal("fetchText first err = nil, want not found")
	}
	forced, err := service.fetchText(WithForceRefresh(context.Background()), server.URL+"/missing")
	if err != nil {
		t.Fatalf("fetchText forced err = %v", err)
	}
	if forced != "fresh body" || requests != 2 {
		t.Fatalf("forced=%q requests=%d", forced, requests)
	}
}

func TestDownloadImageForceRefreshBypassesCache(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		if requests == 1 {
			_, _ = w.Write(testPosterPNG(color.Black, color.White))
			return
		}
		_, _ = w.Write(testPosterPNG(color.RGBA{R: 255, A: 255}, color.Black))
	}))
	defer server.Close()

	service := &Service{http: server.Client(), cacheDir: t.TempDir()}
	first, err := service.downloadImage(context.Background(), server.URL+"/poster.png")
	if err != nil {
		t.Fatalf("downloadImage first err = %v", err)
	}
	forced, err := service.downloadImage(WithForceRefresh(context.Background()), server.URL+"/poster.png")
	if err != nil {
		t.Fatalf("downloadImage forced err = %v", err)
	}
	if bytes.Equal(first, forced) || requests != 2 {
		t.Fatalf("force refresh did not bypass image cache: equal=%v requests=%d", bytes.Equal(first, forced), requests)
	}
}

func TestVisualIMPImageURLPrefersSmallerImage(t *testing.T) {
	t.Parallel()

	got := visualIMPImageURL("http://www.impawards.com/1979/posters/alien_xxlg.jpg")
	want := "http://www.impawards.com/1979/posters/alien.jpg"
	if got != want {
		t.Fatalf("visualIMPImageURL() = %q, want %q", got, want)
	}
}

func TestParseIMPCandidate(t *testing.T) {
	t.Parallel()

	body := `
		<html>
		<title>Alien (1979) - Movie Poster</title>
		<body>
		<a href="posters/alien_ver2_xxlg.jpg"><img src="thumbs/alien_ver2.jpg"></a>
		<img src="posters/alien_ver2.jpg">
		</body>
		</html>`

	candidate, ok := parseIMPCandidate("http://www.impawards.com/1979/alien_ver2.html", body)
	if !ok {
		t.Fatal("parseIMPCandidate() ok = false")
	}
	if candidate.Title != "Alien" || candidate.Year != 1979 || candidate.Version != 2 {
		t.Fatalf("candidate metadata = %#v", candidate)
	}
	if candidate.ImageURL != "http://www.impawards.com/1979/posters/alien_ver2_xxlg.jpg" {
		t.Fatalf("candidate.ImageURL = %q", candidate.ImageURL)
	}
}

func TestParseIMPCandidateIMPHeadingWithLinkedYear(t *testing.T) {
	body := `
		<html>
		<title>Alien Movie Poster (#1 of 5) - IMP Awards</title>
		<body>
		<h4>Alien (<a href="alpha1.html">1979</a>)</h4>
		other sizes: <a href="alien_xlg.html">982x1500</a> / <a href="alien_xxlg.html">1760x2688</a>
		<a href="posters/alien.jpg"><img src="posters/alien.jpg"></a>
		</body>
		</html>`
	candidate, ok := parseIMPCandidate("http://www.impawards.com/1979/alien.html", body)
	if !ok {
		t.Fatal("parseIMPCandidate() ok = false")
	}
	if candidate.Title != "Alien" || candidate.Year != 1979 {
		t.Fatalf("candidate metadata = %#v", candidate)
	}
	if candidate.ImageURL != "http://www.impawards.com/1979/posters/alien_xxlg.jpg" {
		t.Fatalf("candidate.ImageURL = %q", candidate.ImageURL)
	}
}

func TestImageURLFromIMPSizePage(t *testing.T) {
	got := imageURLFromIMPSizePage("http://www.impawards.com/1979/alien.html", "alien_ver2_xxlg.html")
	want := "http://www.impawards.com/1979/posters/alien_ver2_xxlg.jpg"
	if got != want {
		t.Fatalf("imageURLFromIMPSizePage() = %q, want %q", got, want)
	}
}

func TestWikipediaOriginalImageURL(t *testing.T) {
	got := wikipediaOriginalImageURL("https://upload.wikimedia.org/wikipedia/en/thumb/a/a1/Crash_%282004_film%29_poster.jpg/220px-Crash_%282004_film%29_poster.jpg")
	want := "https://upload.wikimedia.org/wikipedia/en/a/a1/Crash_%282004_film%29_poster.jpg"
	if got != want {
		t.Fatalf("wikipediaOriginalImageURL() = %q, want %q", got, want)
	}
}

func TestBestIMPImageUpgradesAlienBasePosterToXXLG(t *testing.T) {
	body := `
		<html>
		<body>
		other sizes: <a href="alien_xlg.html">982x1500</a> / <a href="alien_xxlg.html">1760x2688</a>
		<a href="posters/alien.jpg"><img src="posters/alien.jpg"></a>
		</body>
		</html>`

	got := bestIMPImage("http://www.impawards.com/1979/alien.html", body)
	want := "http://www.impawards.com/1979/posters/alien_xxlg.jpg"
	if got != want {
		t.Fatalf("bestIMPImage() = %q, want %q", got, want)
	}
}

func TestParseWikipediaPoster(t *testing.T) {
	t.Parallel()

	body := `
		<table class="infobox vevent">
		<tr><td><span><img src="//upload.wikimedia.org/poster.jpg" alt="Theatrical release poster"></span></td></tr>
		<tr><td>Theatrical release poster</td></tr>
		</table>`

	poster := parseWikipediaPoster("Alien (film)", body)
	if poster.ImageURL != "https://upload.wikimedia.org/poster.jpg" {
		t.Fatalf("poster.ImageURL = %q", poster.ImageURL)
	}
	if !poster.Poster {
		t.Fatal("poster.Poster = false")
	}
}

func TestParseIMPSearchResults(t *testing.T) {
	t.Parallel()

	body := `
		<html><body>
		<a href="/1979/alien.html">Alien Movie Poster</a>
		<a href="http://www.impawards.com/1979/alien_ver2.html">Alien advance</a>
		<a href="https://impawards.com/1979/alien_ver3.html">Alien advance</a>
		<a href="/1979/alien_gallery.html">gallery</a>
		<a href="http://example.com/1979/alien.html">external</a>
		</body></html>`

	got := parseIMPSearchResults("http://www.impawards.com/cgi-bin/htsearch", body)
	want := []string{"http://www.impawards.com/1979/alien.html", "http://www.impawards.com/1979/alien_ver2.html", "https://impawards.com/1979/alien_ver3.html"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parseIMPSearchResults() = %#v, want %#v", got, want)
	}
}

func TestLooksLikeIMPMoviePage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
		year int
		want bool
	}{
		{name: "movie", url: "http://www.impawards.com/1979/alien.html", year: 1979, want: true},
		{name: "version", url: "http://www.impawards.com/1979/alien_ver2.html", year: 1979, want: true},
		{name: "intl", url: "https://impawards.com/intl/japan/2016/shin_gojira.html", year: 2016, want: true},
		{name: "wrong year", url: "http://www.impawards.com/1980/alien.html", year: 1979, want: false},
		{name: "gallery", url: "http://www.impawards.com/1979/alien_gallery.html", year: 1979, want: false},
		{name: "external", url: "http://example.com/1979/alien.html", year: 1979, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeIMPMoviePage(tc.url, tc.year); got != tc.want {
				t.Fatalf("looksLikeIMPMoviePage(%q, %d) = %v, want %v", tc.url, tc.year, got, tc.want)
			}
		})
	}
}
