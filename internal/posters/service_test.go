package posters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/win0na/posters/internal/plex"
)

func TestTitleSlugs(t *testing.T) {
	t.Parallel()

	got := titleSlugs("The Lord of the Rings: The Fellowship of the Ring")
	want := []string{"the_lord_of_the_rings_the_fellowship_of_the_ring", "lord_of_the_rings_the_fellowship_of_the_ring_the"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("titleSlugs() = %#v, want %#v", got, want)
	}
}

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
		<a href="/1979/alien_gallery.html">gallery</a>
		<a href="http://example.com/1979/alien.html">external</a>
		</body></html>`

	got := parseIMPSearchResults("http://www.impawards.com/cgi-bin/htsearch", body)
	want := []string{"http://www.impawards.com/1979/alien.html", "http://www.impawards.com/1979/alien_ver2.html"}
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

func TestIMPSearchURL(t *testing.T) {
	t.Parallel()

	got := impSearchURL("The Wild Robot 2024", 2)
	want := "http://www.impawards.com/cgi-bin/htsearch?words=The+Wild+Robot+2024;page=2"
	if got != want {
		t.Fatalf("impSearchURL() = %q, want %q", got, want)
	}
}

func TestChooseCandidateAmbiguous(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_ver2.html", ImageURL: "http://www.impawards.com/1979/posters/alien_ver2.jpg", Version: 2},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_ver3.html", ImageURL: "http://www.impawards.com/1979/posters/alien_ver3.jpg", Version: 3},
	}
	_, _, err := chooseCandidate(movie, candidates, wikiPoster{Poster: true})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("chooseCandidate() err = %v, want ambiguous", err)
	}
	var ambiguous *AmbiguousMatchError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("chooseCandidate() err = %T, want AmbiguousMatchError", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous.Candidates = %#v", ambiguous.Candidates)
	}
	if ambiguous.Summary() != "ambiguous IMP match: 2 candidates" {
		t.Fatalf("ambiguous.Summary() = %q", ambiguous.Summary())
	}
}

func TestChooseCandidateCanonical(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien.html", ImageURL: "http://www.impawards.com/1979/posters/alien.jpg", Canonical: true},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_ver2.html", ImageURL: "http://www.impawards.com/1979/posters/alien_ver2.jpg", Version: 2},
	}
	chosen, reason, err := chooseCandidate(movie, candidates, wikiPoster{Poster: true})
	if err != nil {
		t.Fatalf("chooseCandidate() err = %v", err)
	}
	if chosen.PageURL != "http://www.impawards.com/1979/alien.html" {
		t.Fatalf("chosen.PageURL = %q", chosen.PageURL)
	}
	if !strings.Contains(reason, "canonical") {
		t.Fatalf("reason = %q, want canonical", reason)
	}
}

func TestChooseCandidateWikipediaSignal(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_teaser.html", ImageURL: "http://www.impawards.com/1979/posters/alien_teaser.jpg"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_regular_domestic.html", ImageURL: "http://www.impawards.com/1979/posters/alien_regular_domestic.jpg"},
	}
	wiki := wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/alien_regular_domestic_poster.jpg", Caption: "Theatrical regular domestic release poster"}
	chosen, reason, err := chooseCandidate(movie, candidates, wiki)
	if err != nil {
		t.Fatalf("chooseCandidate() err = %v", err)
	}
	if chosen.ImageURL != "http://www.impawards.com/1979/posters/alien_regular_domestic.jpg" {
		t.Fatalf("chosen.ImageURL = %q", chosen.ImageURL)
	}
	if !strings.Contains(reason, "token match") {
		t.Fatalf("reason = %q, want token match", reason)
	}
}

func TestChooseCandidateLowConfidenceStaysAmbiguous(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Alien", Year: 1979}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_red.html", ImageURL: "http://www.impawards.com/1979/posters/alien_red.jpg"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_blue.html", ImageURL: "http://www.impawards.com/1979/posters/alien_blue.jpg"},
	}
	wiki := wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/alien_poster.jpg", Caption: "Theatrical release poster"}
	_, _, err := chooseCandidate(movie, candidates, wiki)
	var ambiguous *AmbiguousMatchError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("chooseCandidate() err = %T, want AmbiguousMatchError", err)
	}
}

func TestChooseVisualCandidatePicksImageMatch(t *testing.T) {
	blue := testPosterPNG(color.RGBA{R: 20, G: 40, B: 220, A: 255}, color.RGBA{R: 240, G: 240, B: 20, A: 255})
	red := testPosterPNG(color.RGBA{R: 220, G: 30, B: 20, A: 255}, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/wiki.png", "/1979/posters/alien_blue.png":
			_, _ = w.Write(blue)
		case "/1979/posters/alien_red.png":
			_, _ = w.Write(red)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client := server.Client()
	client.Transport = rewriteHostTransport{target: serverURL, base: http.DefaultTransport}
	service := &Service{http: client, cacheDir: t.TempDir()}
	candidates := []impCandidate{
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_red.html", ImageURL: "http://www.impawards.com/1979/posters/alien_red.png"},
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_blue.html", ImageURL: "http://www.impawards.com/1979/posters/alien_blue.png"},
	}
	chosen, data, reason, err := service.chooseVisualCandidate(context.Background(), plex.Movie{Title: "Alien", Year: 1979}, candidates, wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/wiki.png"})
	if err != nil {
		t.Fatalf("chooseVisualCandidate() err = %v", err)
	}
	if chosen.ImageURL != "http://www.impawards.com/1979/posters/alien_blue.png" {
		t.Fatalf("chosen.ImageURL = %q", chosen.ImageURL)
	}
	if !bytes.Equal(data, blue) {
		t.Fatal("returned bytes are not chosen IMP image")
	}
	if !strings.Contains(reason, "visual match ") || !strings.Contains(reason, "%") {
		t.Fatalf("reason = %q", reason)
	}
}

func TestVisualScorePercent(t *testing.T) {
	if got := visualMatchReason(0.9876); got != "visual match 98.8%" {
		t.Fatalf("visualMatchReason() = %q", got)
	}
	if got := fmt.Sprintf("%s; next best %s", visualMatchReason(0.9876), visualScorePercent(0.731)); got != "visual match 98.8%; next best 73.1%" {
		t.Fatalf("next best reason = %q", got)
	}
}

type rewriteHostTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func TestVisualSimilarityRanksCloserPosterHigher(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 10, G: 20, B: 200, A: 255}, color.RGBA{R: 240, G: 220, B: 20, A: 255})
	near := testPosterPNG(color.RGBA{R: 12, G: 22, B: 205, A: 255}, color.RGBA{R: 235, G: 215, B: 25, A: 255})
	far := testPosterPNG(color.RGBA{R: 200, G: 20, B: 20, A: 255}, color.RGBA{R: 20, G: 220, B: 20, A: 255})
	baseFP, err := imageFingerprint(base)
	if err != nil {
		t.Fatalf("imageFingerprint(base) err = %v", err)
	}
	nearFP, err := imageFingerprint(near)
	if err != nil {
		t.Fatalf("imageFingerprint(near) err = %v", err)
	}
	farFP, err := imageFingerprint(far)
	if err != nil {
		t.Fatalf("imageFingerprint(far) err = %v", err)
	}
	if visualSimilarity(baseFP, nearFP) <= visualSimilarity(baseFP, farFP) {
		t.Fatalf("near score <= far score: near=%f far=%f", visualSimilarity(baseFP, nearFP), visualSimilarity(baseFP, farFP))
	}
}

func testPosterPNG(left, right color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 32; x++ {
			c := left
			if x > y/2 {
				c = right
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestDescriptiveTokensIgnoreTitleAndPosterNoise(t *testing.T) {
	t.Parallel()

	tokens := descriptiveTokens("Alien regular theatrical release poster 1979 jpg", plex.Movie{Title: "Alien", Year: 1979})
	if !tokens["regular"] {
		t.Fatalf("tokens = %#v, want regular", tokens)
	}
	for _, ignored := range []string{"alien", "theatrical", "release", "poster", "1979", "jpg"} {
		if tokens[ignored] {
			t.Fatalf("tokens[%q] = true, want ignored; tokens = %#v", ignored, tokens)
		}
	}
}
