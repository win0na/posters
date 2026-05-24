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
	want := []string{"the_lord_of_the_rings_the_fellowship_of_the_ring", "lord_of_the_rings_the_fellowship_of_the_ring_the", "lord_of_the_rings_the_fellowship_of_the_ring"}
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

func TestVersionURLAppendsVerSuffix(t *testing.T) {
	t.Parallel()

	got := versionURL("http://www.impawards.com/2024/furiosa.html", 1)
	if got != "http://www.impawards.com/2024/furiosa_ver1.html" {
		t.Fatalf("versionURL() = %q, want furiosa_ver1", got)
	}
	got = versionURL("http://www.impawards.com/2024/furiosa.html", 3)
	if got != "http://www.impawards.com/2024/furiosa_ver3.html" {
		t.Fatalf("versionURL() = %q, want furiosa_ver3", got)
	}
}

func TestIsFullTitleSlug(t *testing.T) {
	t.Parallel()

	// Full slug match
	if !isFullTitleSlug("http://www.impawards.com/2024/furiosa_a_mad_max_saga.html", "Furiosa: A Mad Max Saga") {
		t.Fatal("isFullTitleSlug() = false for full title slug, want true")
	}
	// Shortened slug should NOT match
	if isFullTitleSlug("http://www.impawards.com/2024/furiosa.html", "Furiosa: A Mad Max Saga") {
		t.Fatal("isFullTitleSlug() = true for shortened slug, want false")
	}
	// Version suffix (_ver1) means it's not a title slug
	if isFullTitleSlug("http://www.impawards.com/1986/aliens_ver1.html", "Aliens") {
		t.Fatal("isFullTitleSlug() = true for aliens_ver1 (has _ver suffix), want false")
	}
}

func TestIMPShortenedSlugAlsoProbesVersionVariants(t *testing.T) {
	t.Parallel()

	// Full slug 404s, shortened slug finds page, then auto-probes version variants
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2024/furiosa.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster</title><body><a href="posters/furiosa_xxlg.jpg"><img src="posters/furiosa.jpg"></a></body></html>`))
		case "/2024/furiosa_ver1.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster (#1)</title><body><a href="posters/furiosa_ver1_xxlg.jpg"><img src="posters/furiosa_ver1.jpg"></a></body></html>`))
		case "/2024/furiosa_ver2.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster (#2)</title><body><a href="posters/furiosa_ver2_xxlg.jpg"><img src="posters/furiosa_ver2.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Furiosa: A Mad Max Saga", Year: 2024})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) < 2 {
		t.Fatalf("expected >=2 candidates (canonical + version variants), got %d: %#v", len(candidates), candidates)
	}
	// First candidate from shortened slug should be canonical
	if candidates[0].Version != 0 || !candidates[0].Canonical {
		t.Fatalf("first candidate not canonical: %#v", candidates[0])
	}
	// Version variants should be included
	hasVer1 := false
	hasVer2 := false
	for _, c := range candidates {
		if c.Version == 1 {
			hasVer1 = true
		}
		if c.Version == 2 {
			hasVer2 = true
		}
	}
	if !hasVer1 || !hasVer2 {
		t.Fatalf("missing version variants: candidates=%#v", candidates)
	}
}

func TestIMPCandidatesFollowsNomineePageLinks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.String(), "search.php"):
			// search.php returns a nominees listing page (not a direct movie page)
			_, _ = w.Write([]byte(`<html><body><a href="/2024/nominees_action.html"><h3>Best Action Movie Poster Nominees</a> (2024)</h3></body></html>`))
		case r.URL.Path == "/2024/nominees_action.html":
			_, _ = w.Write([]byte(`<html><body>
				<a href="/intl/uk/2024/love_lies_bleeding.html">Love Lies Bleeding</a>
			</body></html>`))
		case r.URL.Path == "/intl/uk/2024/love_lies_bleeding.html":
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

	// forward: movie (short) prefix-matches candidate (long)
	if !titleMatches("Glass Onion", "Glass Onion: A Knives Out Mystery") {
		t.Fatal("titleMatches() = false, want true for subtitle expansion")
	}
	if titleMatches("Glass", "Glass Onion") {
		t.Fatal("titleMatches() = true for one-token prefix, want false")
	}

	// reverse: candidate (short) prefix-matches movie (long)
	// e.g. IMP heading "Furiosa" vs Plex "Furiosa: A Mad Max Saga"
	if !titleMatches("Furiosa: A Mad Max Saga", "Furiosa") {
		t.Fatal("titleMatches() = false for shorter heading, want true")
	}
	// non-matching single-word should still fail
	if titleMatches("Furiosa: A Mad Max Saga", "Mad") {
		t.Fatal("titleMatches() = true for non-matching single-token candidate, want false")
	}
	// multi-word candidate shorter than movie, correct prefix
	if !titleMatches("The Matrix Reloaded", "The Matrix") {
		t.Fatal("titleMatches() = false for shorter candidate, want true")
	}
	// same-length different tokens should still fail
	if titleMatches("The Matrix Revolutions", "The Matrix Reloaded") {
		t.Fatal("titleMatches() = true for same-length different sequel, want false")
	}
	// shorter candidate that isn't a prefix should fail
	if titleMatches("Batman Begins", "Begins") {
		t.Fatal("titleMatches() = true for non-prefix single token, want false")
	}
}

func TestChooseWikipediaSearchTitlePrefersFilmPage(t *testing.T) {
	t.Parallel()

	got := chooseWikipediaSearchTitle(plex.Movie{Title: "John Wick", Year: 2014}, []string{"John Wick", "John Wick (film)", "John Wick (character)"})
	if got != "John Wick (film)" {
		t.Fatalf("chooseWikipediaSearchTitle() = %q, want film page", got)
	}
}

func TestChooseWikipediaSearchResultPrefersSubtitleExpansionOverExact(t *testing.T) {
	t.Parallel()

	// Real Wikipedia search for "Glass Onion 2022 film" returns full film title first
	results := []wikipediaSearchResult{
		{Title: "Glass Onion: A Knives Out Mystery", Snippet: "Glass Onion: A Knives Out Mystery is a 2022 American mystery film directed by Rian Johnson"},
		{Title: "Glass Onion", Snippet: "Glass Onion (disambiguation)"},
	}
	got := chooseWikipediaSearchResult(plex.Movie{Title: "Glass Onion", Year: 2022}, results)
	if got != "Glass Onion: A Knives Out Mystery" {
		t.Fatalf("chooseWikipediaSearchResult() = %q, want full film title", got)
	}
}

func TestChooseWikipediaSearchResultPrefersExactOverNonMovieExpansion(t *testing.T) {
	t.Parallel()

	// Real Wikipedia search returns snippets with year info
	got := chooseWikipediaSearchResult(plex.Movie{Title: "Alien: Covenant", Year: 2017}, []wikipediaSearchResult{
		{Title: "Alien: Covenant (soundtrack)", Snippet: "soundtrack album"},
		{Title: "Alien: Covenant", Snippet: "Alien: Covenant is a 2017 American science fiction film"},
	})
	if got != "Alien: Covenant" {
		t.Fatalf("chooseWikipediaSearchResult() = %q, want movie page", got)
	}

	got = chooseWikipediaSearchResult(plex.Movie{Title: "Black Dynamite", Year: 2009}, []wikipediaSearchResult{
		{Title: "Black Dynamite (TV series)", Snippet: "television series"},
		{Title: "Black Dynamite", Snippet: "Black Dynamite is a 2009 American blaxploitation comedy film"},
	})
	if got != "Black Dynamite" {
		t.Fatalf("chooseWikipediaSearchResult() = %q, want movie page", got)
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

func TestIMPShortenedProbeURLsFindsShorterIMPPages(t *testing.T) {
	t.Parallel()

	// Simulate IMP having only furiosa.html, not full title slug
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2024/furiosa.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster</title><body><a href="posters/furiosa_xxlg.jpg"><img src="posters/furiosa.jpg"></a></body></html>`))
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

	// Probe without search - should find via shortened slug
	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Furiosa: A Mad Max Saga", Year: 2024})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].Title != "Furiosa" {
		t.Fatalf("candidates = %#v, want 1 candidate with Furiosa title", candidates)
	}
}

func TestIMPShortenedProbeURLsDeduplicatesAcrossSlugVariants(t *testing.T) {
	t.Parallel()

	urls := impShortenedCanonicalProbeURLsForYear(plex.Movie{Title: "The Super Mario Bros. Movie", Year: 2023}, 2023)
	seen := map[string]bool{}
	for _, u := range urls {
		if seen[u] {
			t.Fatalf("duplicate URL: %s", u)
		}
		seen[u] = true
	}
	if len(urls) > 15 || len(urls) < 5 {
		t.Fatalf("expected 5-15 unique shortened URLs, got %d: %v", len(urls), urls)
	}
	if !containsString(urls, "http://www.impawards.com/2023/the_super_mario_bros.html") {
		t.Fatalf("missing expected shortened URL 'the_super_mario_bros': %v", urls)
	}
}

func TestIMPShortenedProbeURLsHandlesShortTitle(t *testing.T) {
	t.Parallel()

	urls := impShortenedCanonicalProbeURLsForYear(plex.Movie{Title: "Alien", Year: 1979}, 1979)
	if len(urls) != 0 {
		t.Fatalf("expected 0 shortened URLs for 'Alien', got %d: %v", len(urls), urls)
	}
}

func TestIMPProbeURLsIncludesShortenedForSubtitleTitles(t *testing.T) {
	t.Parallel()

	urls := impProbeURLs(plex.Movie{Title: "Furiosa: A Mad Max Saga", Year: 2024})
	shortenedCount := 0
	for _, u := range urls {
		if strings.Contains(u, "/2024/furiosa.html") && !strings.Contains(u, "_ver") {
			shortenedCount++
		}
	}
	if shortenedCount == 0 {
		t.Fatal("impProbeURLs() missing shortened furiosa.html probe for Furiosa: A Mad Max Saga")
	}
}

func TestIMPProbeURLsIncludesShortenedForSpiderVerse(t *testing.T) {
	t.Parallel()

	urls := impProbeURLs(plex.Movie{Title: "Spider-Man: Across the Spider-Verse", Year: 2023})
	found := false
	for _, u := range urls {
		if strings.Contains(u, "/2023/spider_man") && !strings.Contains(u, "_ver") {
			// Should find shortened probes like spider_man.html
			if strings.Count(u, "_") == 1 && strings.Contains(u, "spider_man.html") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("impProbeURLs() missing shortened spider_man probe for Spider-Man: Across the Spider-Verse")
	}
}

func TestIMPSearchPHPURLFormat(t *testing.T) {
	t.Parallel()

	got := impSearchPHPURL("Shin Godzilla")
	if got != "http://www.impawards.com/search.php?search_data=Shin+Godzilla" {
		t.Fatalf("impSearchPHPURL() = %q, want search.php with query", got)
	}
	got = impSearchPHPURL("Shin Godzilla 2016")
	if got != "http://www.impawards.com/search.php?search_data=Shin+Godzilla+2016" {
		t.Fatalf("impSearchPHPURL() = %q, want search.php with query+year", got)
	}
}

func TestParseIMPSearchPHPResultsExtractsMovieLinks(t *testing.T) {
	t.Parallel()

	body := `<html>
		<div class = row align = center><div class = col-sm-12>
			<a href = /intl/japan/2016/shin_gojira.html><h3>Shin Godzilla</a> (2016)</h3>
			<a href = /intl/japan/tv/gojira_shingyura_pointo.html><h3>Godzilla Singular Point</a> (tv)</h3>
		</div></div>
	</html>`
	urls := parseIMPSearchPHPResults("http://www.impawards.com/search.php", body)
	if len(urls) != 1 {
		t.Fatalf("parseIMPSearchPHPResults() = %d results, want 1 (only movie pages): %v", len(urls), urls)
	}
	if urls[0] != "http://www.impawards.com/intl/japan/2016/shin_gojira.html" {
		t.Fatalf("parseIMPSearchPHPResults()[0] = %q, want /intl/japan/2016/shin_gojira.html", urls[0])
	}
}

func TestParseIMPSearchPHPResultsHandlesFuriosaFormat(t *testing.T) {
	t.Parallel()

	body := `<html>
		<div class = row align = center><div class = col-sm-12>
			<a href = /2024/furiosa.html><h3>Furiosa: A Mad Max Saga</a> (2024)</h3>
		</div></div>
	</html>`
	urls := parseIMPSearchPHPResults("http://www.impawards.com/search.php", body)
	if len(urls) != 1 {
		t.Fatalf("parseIMPSearchPHPResults() = %d results, want 1: %v", len(urls), urls)
	}
	if urls[0] != "http://www.impawards.com/2024/furiosa.html" {
		t.Fatalf("parseIMPSearchPHPResults()[0] = %q, want /2024/furiosa.html", urls[0])
	}
}

func TestParseIMPSearchPHPResultsNoResults(t *testing.T) {
	t.Parallel()

	body := `<html><body><p>We're sorry, but the search you performed did not return any results.</p></body></html>`
	urls := parseIMPSearchPHPResults("http://www.impawards.com/search.php", body)
	if len(urls) != 0 {
		t.Fatalf("parseIMPSearchPHPResults() = %d results, want 0 for no-results page: %v", len(urls), urls)
	}
}

func TestIMPSearchPHPFindsIntlPath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.String(), "search.php"):
			// Return search.php-style results
			_, _ = w.Write([]byte(`<html><body>
				<a href = /intl/uk/2024/love_lies_bleeding.html><h3>Love Lies Bleeding</a> (2024)</h3>
			</body></html>`))
		case r.URL.Path == "/intl/uk/2024/love_lies_bleeding.html":
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
		t.Fatalf("candidates = %#v, want 1 candidate at /intl/uk/2024/", candidates)
	}
}

func TestIMPSearchPHPFindsShinGodzillaViaIntl(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.String(), "search_data=Shin+Godzilla"):
			_, _ = w.Write([]byte(`<html><body>
				<a href = /intl/japan/2016/shin_gojira.html><h3>Shin Godzilla</a> (2016)</h3>
			</body></html>`))
		case r.URL.Path == "/intl/japan/2016/shin_gojira.html":
			_, _ = w.Write([]byte(`<html><title>Shin Godzilla (2016) - Movie Poster</title><body><a href="posters/shin_gojira_xxlg.jpg"><img src="posters/shin_gojira.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Shin Godzilla", Year: 2016})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 || candidates[0].PageURL != "http://www.impawards.com/intl/japan/2016/shin_gojira.html" {
		t.Fatalf("candidates = %#v, want 1 candidate at /intl/japan/2016/shin_gojira.html", candidates)
	}
}

func TestIMPSearchPHPFindsSuperMarioBros(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.String(), "search.php"):
			_, _ = w.Write([]byte(`<html><body>
				<a href = /2023/super_mario_bros_the_movie.html><h3>The Super Mario Bros. Movie</a> (2023)</h3>
			</body></html>`))
		case r.URL.Path == "/2023/super_mario_bros_the_movie.html":
			_, _ = w.Write([]byte(`<html><title>The Super Mario Bros. Movie (2023) - Movie Poster</title><body><a href="posters/mario.jpg"><img src="posters/mario.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "The Super Mario Bros. Movie", Year: 2023})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want 1 candidate", candidates)
	}
}

func TestIMPSearchPHPFindsSpiderVerse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.String(), "search.php"):
			_, _ = w.Write([]byte(`<html><body>
				<a href = /2023/spiderman_across_the_spiderverse.html><h3>Spider-Man: Across the Spider-Verse</a> (2023)</h3>
			</body></html>`))
		case r.URL.Path == "/2023/spiderman_across_the_spiderverse.html":
			_, _ = w.Write([]byte(`<html><title>Spider-Man: Across the Spider-Verse (2023) - Movie Poster</title><body><a href="posters/spider_ver.jpg"><img src="posters/spider_ver.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Spider-Man: Across the Spider-Verse", Year: 2023})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want 1 candidate", candidates)
	}
}

func TestIMPProbeURLsShortenedSpiderVerseFindsPage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2023/spider_man_across_the_spider_verse.html":
			_, _ = w.Write([]byte(`<html><title>Spider-Man: Across the Spider-Verse (2023) - Movie Poster</title><body><a href="posters/spider_ver.jpg"><img src="posters/spider_ver.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Spider-Man: Across the Spider-Verse", Year: 2023})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	if len(candidates) < 1 {
		t.Fatal("expected at least 1 candidate for Spider-Man: Across the Spider-Verse")
	}
}

func TestIMPVersionURLsFromBodyExtractsVersionLinks(t *testing.T) {
	t.Parallel()

	body := `<html><body>
		<a href = tron_legacy_ver2.html>Tron Legacy ver2</a>
		<a href = tron_legacy_ver10.html>Tron Legacy ver10</a>
		<a href = tron_legacy_ver11.html>Tron Legacy ver11</a>
		<a href = /2010/tron_legacy_ver26.html>Tron Legacy ver26</a>
	</body></html>`
	pageURL := "http://www.impawards.com/2010/tron_legacy.html"

	urls := impVersionURLsFromBody(pageURL, body)
	if len(urls) != 4 {
		t.Fatalf("expected 4 version URLs, got %d: %v", len(urls), urls)
	}
	expected := []string{
		"http://www.impawards.com/2010/tron_legacy_ver2.html",
		"http://www.impawards.com/2010/tron_legacy_ver10.html",
		"http://www.impawards.com/2010/tron_legacy_ver11.html",
		"http://www.impawards.com/2010/tron_legacy_ver26.html",
	}
	for _, exp := range expected {
		found := false
		for _, u := range urls {
			if u == exp {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing expected URL: %s\n got: %v", exp, urls)
		}
	}
}

func TestIMPVersionURLsFromBodyDeduplicates(t *testing.T) {
	t.Parallel()

	body := `<html><body>
		<a href = tron_legacy_ver10.html>Tron Legacy ver10</a>
		<a href = /2010/tron_legacy_ver10.html>Tron Legacy ver10 again</a>
	</body></html>`
	pageURL := "http://www.impawards.com/2010/tron_legacy.html"

	urls := impVersionURLsFromBody(pageURL, body)
	if len(urls) != 1 {
		t.Fatalf("expected 1 unique version URL, got %d: %v", len(urls), urls)
	}
}

func TestIMPVersionURLsFromBodySkipsNonVersionReferences(t *testing.T) {
	t.Parallel()

	body := `<html><body>
		<a href = tron_legacy.html>Canonical link</a>
		<a href = posters/tron_legacy_ver10.jpg>Image reference</a>
		<a href = /2010/tron_legacy.html>Absolute canonical</a>
	</body></html>`
	pageURL := "http://www.impawards.com/2010/tron_legacy.html"

	urls := impVersionURLsFromBody(pageURL, body)
	if len(urls) != 0 {
		t.Fatalf("expected 0 version URLs (no _verN.html patterns), got %d: %v", len(urls), urls)
	}
}

func TestIMPShortenedSlugDiscoversHigherVersionsFromBody(t *testing.T) {
	t.Parallel()

	// Full slug 404s, shortened slug finds canonical page.
	// Canonical page body links to ver3 and ver11.
	// System should discover and probe them both.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2024/furiosa.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster</title><body>
				<a href = furiosa_ver1.html>Furiosa ver1</a>
				<a href = furiosa_ver3.html>Furiosa ver3</a>
				<a href = furiosa_ver11.html>Furiosa ver11</a>
				<a href = "posters/furiosa_xxlg.jpg"><img src="posters/furiosa.jpg"></a>
			</body></html>`))
		case "/2024/furiosa_ver1.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster (#1)</title><body><a href="posters/furiosa_ver1.jpg"><img src="posters/furiosa_ver1.jpg"></a></body></html>`))
		case "/2024/furiosa_ver3.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster (#3)</title><body><a href="posters/furiosa_ver3.jpg"><img src="posters/furiosa_ver3.jpg"></a></body></html>`))
		case "/2024/furiosa_ver11.html":
			_, _ = w.Write([]byte(`<html><title>Furiosa (2024) - Movie Poster (#11)</title><body><a href="posters/furiosa_ver11.jpg"><img src="posters/furiosa_ver11.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Furiosa: A Mad Max Saga", Year: 2024})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	hasVer1 := false
	hasVer3 := false
	hasVer11 := false
	for _, c := range candidates {
		if c.Version == 1 {
			hasVer1 = true
		}
		if c.Version == 3 {
			hasVer3 = true
		}
		if c.Version == 11 {
			hasVer11 = true
		}
	}
	if !hasVer1 {
		t.Fatal("missing ver1 candidate (should be found via shortened slug probe)")
	}
	if !hasVer3 {
		t.Fatal("missing ver3 candidate (should be discovered from body)")
	}
	if !hasVer11 {
		t.Fatal("missing ver11 candidate (should be discovered from body)")
	}
}

func TestIMPCanonicalFullSlugProbesHighVersionsFromBody(t *testing.T) {
	t.Parallel()

	// Full slug matches directly, canonical page body links to ver10+.
	// System should discover those from the canonical page body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2010/tron_legacy.html":
			_, _ = w.Write([]byte(`<html><title>Tron Legacy (2010) - Movie Poster</title><body>
				<a href = tron_legacy_ver1.html>ver1</a>
				<a href = tron_legacy_ver2.html>ver2</a>
				<a href = tron_legacy_ver10.html>ver10</a>
				<a href = tron_legacy_ver11.html>ver11</a>
				<a href = tron_legacy_ver26.html>ver26</a>
				<a href = "posters/tron_legacy_xxlg.jpg"><img src="posters/tron_legacy.jpg"></a>
			</body></html>`))
		case "/2010/tron_legacy_ver1.html":
			_, _ = w.Write([]byte(`<html><title>Tron Legacy (2010) - Movie Poster (#1)</title><body><a href="posters/tron_ver1.jpg"><img src="posters/tron_ver1.jpg"></a></body></html>`))
		case "/2010/tron_legacy_ver2.html":
			_, _ = w.Write([]byte(`<html><title>Tron Legacy (2010) - Movie Poster (#2)</title><body><a href="posters/tron_ver2.jpg"><img src="posters/tron_ver2.jpg"></a></body></html>`))
		case "/2010/tron_legacy_ver10.html":
			_, _ = w.Write([]byte(`<html><title>Tron Legacy (2010) - Movie Poster (#10)</title><body><a href="posters/tron_ver10.jpg"><img src="posters/tron_ver10.jpg"></a></body></html>`))
		case "/2010/tron_legacy_ver11.html":
			_, _ = w.Write([]byte(`<html><title>Tron Legacy (2010) - Movie Poster (#11)</title><body><a href="posters/tron_ver11.jpg"><img src="posters/tron_ver11.jpg"></a></body></html>`))
		case "/2010/tron_legacy_ver26.html":
			_, _ = w.Write([]byte(`<html><title>Tron Legacy (2010) - Movie Poster (#26)</title><body><a href="posters/tron_ver26.jpg"><img src="posters/tron_ver26.jpg"></a></body></html>`))
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

	candidates, err := service.impCandidates(context.Background(), plex.Movie{Title: "Tron: Legacy", Year: 2010})
	if err != nil {
		t.Fatalf("impCandidates() err = %v", err)
	}
	hasVer1 := false
	hasVer2 := false
	hasVer10 := false
	hasVer11 := false
	hasVer26 := false
	for _, c := range candidates {
		switch c.Version {
		case 1:
			hasVer1 = true
		case 2:
			hasVer2 = true
		case 10:
			hasVer10 = true
		case 11:
			hasVer11 = true
		case 26:
			hasVer26 = true
		}
	}
	if !hasVer1 || !hasVer2 {
		t.Fatal("missing standard version candidates (1-8)")
	}
	if !hasVer10 || !hasVer11 || !hasVer26 {
		t.Fatalf("missing high version candidates (10, 11, 26): candidates=%#v", candidates)
	}
}


