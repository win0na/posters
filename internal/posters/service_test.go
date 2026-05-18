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

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func TestTitleSlugs(t *testing.T) {
	t.Parallel()

	got := titleSlugs("The Lord of the Rings: The Fellowship of the Ring")
	want := []string{"the_lord_of_the_rings_the_fellowship_of_the_ring", "lord_of_the_rings_the_fellowship_of_the_ring_the"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("titleSlugs() = %#v, want %#v", got, want)
	}
}

func TestTitleSlugsNumberWords(t *testing.T) {
	t.Parallel()

	got := titleSlugs("Despicable Me 2")
	want := []string{"despicable_me_2", "despicable_me_two"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("titleSlugs() = %#v, want %#v", got, want)
	}
}

func TestTitleSlugsRomanNumeralsAndMovieArticle(t *testing.T) {
	t.Parallel()

	got := titleSlugs("Star Wars: Episode III - Revenge of the Sith")
	if !containsString(got, "star_wars_episode_three_revenge_of_the_sith") {
		t.Fatalf("titleSlugs() missing roman numeral word variant: %#v", got)
	}

	got = titleSlugs("The Super Mario Bros. Movie")
	if !containsString(got, "the_super_mario_bros_the_movie") {
		t.Fatalf("titleSlugs() missing inserted article variant: %#v", got)
	}
}

func TestNormalizeTitleFoldsAccents(t *testing.T) {
	t.Parallel()

	if got := normalizeTitle("Pokémon: The First Movie"); got != "pokemon the first movie" {
		t.Fatalf("normalizeTitle() = %q", got)
	}
}

func TestIMPProbeURLsIncludesNumberWordSlug(t *testing.T) {
	t.Parallel()

	got := strings.Join(impProbeURLs(plex.Movie{Title: "Despicable Me 2", Year: 2013}), "\n")
	want := "http://www.impawards.com/2013/despicable_me_two.html"
	if !strings.Contains(got, want) {
		t.Fatalf("impProbeURLs() missing %s:\n%s", want, got)
	}
}

func TestIMPCandidateYearsChecksAdjacentAfterExact(t *testing.T) {
	t.Parallel()

	got := impCandidateYears(2005)
	want := []int{2005, 2004, 2006, 2003, 2007, 2002, 2008}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("impCandidateYears() = %#v, want %#v", got, want)
	}
}

func TestIMPCandidatesFallsBackToAdjacentYear(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2004/crash.html":
			_, _ = w.Write([]byte(`<html><title>Crash (2004) - Movie Poster</title><body><a href="posters/crash.jpg"><img src="posters/crash.jpg"></a></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() err = %v", err)
	}
	service := &Service{http: &http.Client{Transport: rewriteHostTransport{target: target}}, cacheDir: t.TempDir()}

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Crash", Year: 2005})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].Year != 2004 || candidates[0].PageURL != "http://www.impawards.com/2004/crash.html" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestIMPCandidatesFallsBackToOriginalTitle(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2013/original_title.html":
			_, _ = w.Write([]byte(`<html><title>Original Title (2013) - Movie Poster</title><body><a href="posters/original_title.jpg"><img src="posters/original_title.jpg"></a></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() err = %v", err)
	}
	service := &Service{http: &http.Client{Transport: rewriteHostTransport{target: target}}, cacheDir: t.TempDir()}

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Localized Title", OriginalTitle: "Original Title", Year: 2013})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].Title != "Original Title" || candidates[0].PageURL != "http://www.impawards.com/2013/original_title.html" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestIMPCandidatesTriesOriginalTitleBeforeSearchFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/htsearch":
			_, _ = w.Write([]byte(`<html><body><a href="/2022/glass_onion_fake.html">Glass Onion: Fake Sequel</a></body></html>`))
		case "/2022/glass_onion_fake.html":
			_, _ = w.Write([]byte(`<html><title>Glass Onion: Fake Sequel (2022) - Movie Poster</title><body><a href="posters/glass_onion_fake.jpg"><img src="posters/glass_onion_fake.jpg"></a></body></html>`))
		case "/2022/knives_out_two.html":
			_, _ = w.Write([]byte(`<html><title>Knives Out Two (2022) - Movie Poster</title><body><a href="posters/knives_out_two.jpg"><img src="posters/knives_out_two.jpg"></a></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() err = %v", err)
	}
	service := &Service{http: &http.Client{Transport: rewriteHostTransport{target: target}}, cacheDir: t.TempDir()}

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Glass Onion", OriginalTitle: "Knives Out Two", Year: 2022})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].Title != "Knives Out Two" || candidates[0].PageURL != "http://www.impawards.com/2022/knives_out_two.html" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestIMPCandidatesFollowsNomineePageLinks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/htsearch":
			_, _ = w.Write([]byte(`<html><body><a href="/2024/nominees_action.html">Best Action Movie Poster Nominees</a></body></html>`))
		case "/2024/nominees_action.html":
			_, _ = w.Write([]byte(`<html><body>
				<a href="/intl/uk/2024/love_lies_bleeding.html">Love Lies Bleeding</a>
			</body></html>`))
		case "/intl/uk/2024/love_lies_bleeding.html":
			_, _ = w.Write([]byte(`<html><title>Love Lies Bleeding (2024) - Movie Poster</title><body><a href="posters/love_lies_bleeding.jpg"><img src="posters/love_lies_bleeding.jpg"></a></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() err = %v", err)
	}
	service := &Service{http: &http.Client{Transport: rewriteHostTransport{target: target}}, cacheDir: t.TempDir()}

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Love Lies Bleeding", Year: 2024})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].PageURL != "http://www.impawards.com/intl/uk/2024/love_lies_bleeding.html" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestTitleMatchesAllowsSubtitleExpansion(t *testing.T) {
	t.Parallel()

	if !titleMatches("Glass Onion", "Glass Onion: A Knives Out Mystery") {
		t.Fatal("titleMatches() = false, want true for subtitle expansion")
	}
	if titleMatches("Glass", "Glass Onion") {
		t.Fatal("titleMatches() = true for one-token prefix, want false")
	}
}

func TestChooseWikipediaSearchTitlePrefersFilmPage(t *testing.T) {
	t.Parallel()

	got := chooseWikipediaSearchTitle(plex.Movie{Title: "John Wick", Year: 2014}, []string{"John Wick", "John Wick (film)", "John Wick (character)"})
	if got != "John Wick (film)" {
		t.Fatalf("chooseWikipediaSearchTitle() = %q, want film page", got)
	}
}

func TestChooseWikipediaSearchTitlePrefersSubtitleExpansionOverExact(t *testing.T) {
	t.Parallel()

	got := chooseWikipediaSearchTitle(plex.Movie{Title: "Glass Onion", Year: 2022}, []string{"Glass Onion", "Glass Onion: A Knives Out Mystery"})
	if got != "Glass Onion: A Knives Out Mystery" {
		t.Fatalf("chooseWikipediaSearchTitle() = %q, want full film title", got)
	}
}

func TestChooseWikipediaSearchTitlePrefersExactOverNonMovieExpansion(t *testing.T) {
	t.Parallel()

	got := chooseWikipediaSearchTitle(plex.Movie{Title: "Alien: Covenant", Year: 2017}, []string{"Alien: Covenant (soundtrack)", "Alien: Covenant"})
	if got != "Alien: Covenant" {
		t.Fatalf("chooseWikipediaSearchTitle() = %q, want movie page", got)
	}

	got = chooseWikipediaSearchTitle(plex.Movie{Title: "Black Dynamite", Year: 2009}, []string{"Black Dynamite (TV series)", "Black Dynamite"})
	if got != "Black Dynamite" {
		t.Fatalf("chooseWikipediaSearchTitle() = %q, want movie page", got)
	}
}

func TestChooseWikipediaSearchTitleRejectsOnlyNonMovieResults(t *testing.T) {
	t.Parallel()

	got := chooseWikipediaSearchTitle(plex.Movie{Title: "Black Dynamite", Year: 2009}, []string{"Black Dynamite (TV series)", "Black Dynamite (soundtrack)"})
	if got != "" {
		t.Fatalf("chooseWikipediaSearchTitle() = %q, want empty", got)
	}
}

func TestChooseWikipediaSearchResultPrefersExactYearHitOverSequel(t *testing.T) {
	t.Parallel()

	results := []wikipediaSearchResult{
		{Title: "The Exorcist (franchise)", Snippet: "adapted into the 1973 film of the same name"},
		{Title: "The Exorcist", Snippet: "The Exorcist is a 1973 American supernatural horror film"},
		{Title: "The Exorcist: Believer", Snippet: "legacy sequel to The Exorcist (1973). The film stars"},
	}
	got := chooseWikipediaSearchResult(plex.Movie{Title: "The Exorcist", Year: 1973}, results)
	if got != "The Exorcist" {
		t.Fatalf("chooseWikipediaSearchResult() = %q, want exact film", got)
	}
}

func TestWikipediaMovieTitleStripsFilmQualifier(t *testing.T) {
	t.Parallel()

	if got := wikipediaMovieTitle("John Wick (film)"); got != "John Wick" {
		t.Fatalf("wikipediaMovieTitle() = %q", got)
	}
	if got := wikipediaMovieTitle("Glass Onion: A Knives Out Mystery"); got != "Glass Onion: A Knives Out Mystery" {
		t.Fatalf("wikipediaMovieTitle() = %q", got)
	}
}

func TestIMPSearchMoviesSkipsDuplicateOriginalTitle(t *testing.T) {
	t.Parallel()

	movies := impSearchMovies(plex.Movie{Title: "Alien", OriginalTitle: "Alien", Year: 1979})
	if len(movies) != 1 {
		t.Fatalf("len(search movies) = %d, want 1: %#v", len(movies), movies)
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

func TestChooseCandidateWithoutWikipediaUsesCanonical(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "John Wick", Year: 2014}
	candidates := []impCandidate{
		{Title: "John Wick", Year: 2014, PageURL: "http://www.impawards.com/2014/john_wick.html", ImageURL: "http://www.impawards.com/2014/posters/john_wick.jpg", Canonical: true},
		{Title: "John Wick", Year: 2014, PageURL: "http://www.impawards.com/2014/john_wick_ver2.html", ImageURL: "http://www.impawards.com/2014/posters/john_wick_ver2.jpg", Version: 2},
	}
	chosen, reason, err := chooseCandidate(movie, candidates, wikiPoster{})
	if err != nil {
		t.Fatalf("chooseCandidate() err = %v", err)
	}
	if chosen.PageURL != "http://www.impawards.com/2014/john_wick.html" || !strings.Contains(reason, "Wikipedia poster unavailable") {
		t.Fatalf("chosen=%#v reason=%q", chosen, reason)
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

func TestChooseVisualCandidateRejectsLowConfidenceMatch(t *testing.T) {
	blue := testPosterPNG(color.RGBA{R: 20, G: 40, B: 220, A: 255}, color.RGBA{R: 240, G: 240, B: 20, A: 255})
	red := testPosterPNG(color.RGBA{R: 220, G: 30, B: 20, A: 255}, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	green := testPosterPNG(color.RGBA{R: 20, G: 190, B: 50, A: 255}, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/wiki.png":
			_, _ = w.Write(blue)
		case "/1979/posters/alien_red.png":
			_, _ = w.Write(red)
		case "/1979/posters/alien_green.png":
			_, _ = w.Write(green)
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
		{Title: "Alien", Year: 1979, PageURL: "http://www.impawards.com/1979/alien_green.html", ImageURL: "http://www.impawards.com/1979/posters/alien_green.png"},
	}
	_, _, _, err = service.chooseVisualCandidate(context.Background(), plex.Movie{Title: "Alien", Year: 1979}, candidates, wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/wiki.png"})
	var ambiguous *AmbiguousMatchError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("chooseVisualCandidate() err = %T %v, want AmbiguousMatchError", err, err)
	}
}

func TestChooseVisualCandidateAllowsCloseHighConfidenceMatch(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 20, G: 40, B: 220, A: 255}, color.RGBA{R: 240, G: 240, B: 20, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/wiki.png", "/1986/posters/aliens_main.png", "/1986/posters/aliens_ver1.png":
			_, _ = w.Write(base)
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
		{Title: "Aliens", Year: 1986, PageURL: "http://www.impawards.com/1986/aliens.html", ImageURL: "http://www.impawards.com/1986/posters/aliens_main.png"},
		{Title: "Aliens", Year: 1986, PageURL: "http://www.impawards.com/1986/aliens_ver1.html", ImageURL: "http://www.impawards.com/1986/posters/aliens_ver1.png", Version: 1},
	}
	chosen, _, reason, err := service.chooseVisualCandidate(context.Background(), plex.Movie{Title: "Aliens", Year: 1986}, candidates, wikiPoster{Poster: true, ImageURL: "https://upload.wikimedia.org/wiki.png"})
	if err != nil {
		t.Fatalf("chooseVisualCandidate() err = %v", err)
	}
	if chosen.PageURL != "http://www.impawards.com/1986/aliens_ver1.html" || !strings.Contains(reason, "%") {
		t.Fatalf("chosen=%#v reason=%q", chosen, reason)
	}
}

func TestConfidentVisualMatchAllowsClearWinnerBelowHighThreshold(t *testing.T) {
	best := matchedCandidate{Score: 0.7298}
	second := matchedCandidate{Score: 0.4949}
	if !isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() rejected clear Aliens-style winner")
	}
}

func TestConfidentVisualMatchAllowsModerateClearWinnerBelowHighThreshold(t *testing.T) {
	best := matchedCandidate{Score: 0.7601}
	second := matchedCandidate{Score: 0.6590}
	if !isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() rejected Final-Destination-style winner")
	}
}

func TestConfidentVisualMatchAllowsSameImageNameHint(t *testing.T) {
	best := matchedCandidate{Score: 0.7671, NameHint: true}
	second := matchedCandidate{Score: 0.6968}
	if !isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() rejected same-image-name candidate")
	}
}

func TestConfidentVisualMatchRejectsWeakWinnerBelowHighThreshold(t *testing.T) {
	best := matchedCandidate{Score: 0.7298}
	second := matchedCandidate{Score: 0.66}
	if isConfidentVisualMatch(best, &second) {
		t.Fatal("isConfidentVisualMatch() accepted weak margin below high threshold")
	}
}

func TestSamePosterImageNameIgnoresWikiThumbAndIMPSizeSuffix(t *testing.T) {
	if !samePosterImageName("https://upload.wikimedia.org/wikipedia/en/thumb/7/7b/Exorcist_ver2.jpg/250px-Exorcist_ver2.jpg", "http://www.impawards.com/1973/posters/exorcist_ver2_xxlg.jpg") {
		t.Fatal("samePosterImageName() did not match wiki thumb with IMP size suffix")
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

func TestImageFingerprintsIncludeTrimmedWhiteBorderVariant(t *testing.T) {
	base := testPosterPNG(color.RGBA{R: 10, G: 20, B: 200, A: 255}, color.RGBA{R: 240, G: 220, B: 20, A: 255})
	bordered := testPosterPNGWithBorder(color.RGBA{R: 10, G: 20, B: 200, A: 255}, color.RGBA{R: 240, G: 220, B: 20, A: 255}, color.White)
	baseFPs, err := imageFingerprints(base)
	if err != nil {
		t.Fatalf("imageFingerprints(base) err = %v", err)
	}
	borderedFPs, err := imageFingerprints(bordered)
	if err != nil {
		t.Fatalf("imageFingerprints(bordered) err = %v", err)
	}
	baseOnly, _ := imageFingerprint(base)
	borderedOnly, _ := imageFingerprint(bordered)
	if maxVisualSimilarity(baseFPs, borderedFPs) <= visualSimilarity(baseOnly, borderedOnly) {
		t.Fatalf("trimmed variants did not improve border match: variants=%f original=%f", maxVisualSimilarity(baseFPs, borderedFPs), visualSimilarity(baseOnly, borderedOnly))
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

func testPosterPNGWithBorder(left, right, border color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 40, 56))
	for y := 0; y < 56; y++ {
		for x := 0; x < 40; x++ {
			c := border
			if x >= 4 && x < 36 && y >= 4 && y < 52 {
				innerX, innerY := x-4, y-4
				c = left
				if innerX > innerY/2 {
					c = right
				}
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
