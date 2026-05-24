package posters

import (
	"context"
	"fmt"
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
	if !containsString(got, "star_wars_episode_three") {
		t.Fatalf("titleSlugs() missing Star Wars episode short variant: %#v", got)
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

func TestTitleMatchesStarWarsEpisodeNumbers(t *testing.T) {
	t.Parallel()

	if !titleMatches("Star Wars: Episode I - The Phantom Menace", "Star Wars Episode 1: The Phantom Menace") {
		t.Fatal("titleMatches() rejected episode roman/numeric variant")
	}
	if !titleMatches("Star Wars: Episode III - Revenge of the Sith", "Star Wars Episode 3: Revenge of the Sith") {
		t.Fatal("titleMatches() rejected episode roman/numeric variant")
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

func TestIMPCandidatesFindsSouthlandTalesPlusOneYear(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2007/southland_tales.html":
			_, _ = w.Write([]byte(`<html>
				<title>Southland Tales Movie Poster (#1 of 4) - IMP Awards</title>
				<body>
					<h3 class="hidden-xs">Southland Tales (<a href="alpha1.html">2007</a>)</h3>
					<img src="posters/southland_tales.jpg" alt="Southland Tales Movie Poster">
				</body>
			</html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Southland Tales", Year: 2006})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].Year != 2007 || candidates[0].PageURL != "http://www.impawards.com/2007/southland_tales.html" {
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

func TestIMPCandidatesTriesExactVersionBeforeSearchFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/htsearch":
			_, _ = w.Write([]byte(`<html><body><a href="/1986/aliens_fake.html">Aliens Fake</a></body></html>`))
		case "/1986/aliens_fake.html":
			_, _ = w.Write([]byte(`<html><title>Aliens Fake (1986) - Movie Poster</title><body><a href="posters/aliens_fake.jpg"><img src="posters/aliens_fake.jpg"></a></body></html>`))
		case "/1986/aliens_ver1.html":
			_, _ = w.Write([]byte(`<html><title>Aliens (1986) - Movie Poster</title><body><a href="posters/aliens_ver1.jpg"><img src="posters/aliens_ver1.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Aliens", Year: 1986})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].PageURL != "http://www.impawards.com/1986/aliens_ver1.html" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestVisualCandidatePriorityPrefersVersionOne(t *testing.T) {
	t.Parallel()

	movie := plex.Movie{Title: "Aliens", Year: 1986}
	wiki := wikiPoster{ImageURL: "https://upload.wikimedia.org/theatrical.jpg"}
	canonical := impCandidate{PageURL: "http://www.impawards.com/1986/aliens.html", ImageURL: "http://www.impawards.com/1986/posters/aliens.jpg", Year: 1986, Canonical: true}
	ver1 := impCandidate{PageURL: "http://www.impawards.com/1986/aliens_ver1.html", ImageURL: "http://www.impawards.com/1986/posters/aliens_ver1.jpg", Year: 1986, Version: 1}

	if visualCandidatePriority(movie, ver1, wiki) <= visualCandidatePriority(movie, canonical, wiki) {
		t.Fatalf("ver1 priority = %d, canonical priority = %d", visualCandidatePriority(movie, ver1, wiki), visualCandidatePriority(movie, canonical, wiki))
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
