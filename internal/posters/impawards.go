package posters

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/win0na/posters/internal/plex"
)

func (s *Service) impCandidates(ctx context.Context, movie plex.Movie) ([]impCandidate, error) {
	for _, searchMovie := range impSearchMovies(movie) {
		if candidates := s.probeYearsConcurrent(ctx, searchMovie, false); candidates != nil {
			return candidates, nil
		}
	}
	for _, searchMovie := range impSearchMovies(movie) {
		if candidates := s.probeYearsConcurrent(ctx, searchMovie, true); candidates != nil {
			return candidates, nil
		}
	}
	return nil, fmt.Errorf("no IMP Awards poster found for %s (%d)", movie.Title, movie.Year)
}

// probeYearsConcurrent probes all candidate years concurrently.
// It cancels remaining probes on first match but waits for all goroutines
// to finish before returning, ensuring no background cache writes or
// network requests outlive the call.
func (s *Service) probeYearsConcurrent(ctx context.Context, movie plex.Movie, includeSearch bool) []impCandidate {
	years := impCandidateYears(movie.Year)
	if len(years) == 0 {
		return nil
	}

	type yearResult struct {
		candidates []impCandidate
		year       int
	}
	resultCh := make(chan yearResult, len(years))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, year := range years {
		year := year
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidates, err := s.impCandidatesForYear(ctx, movie, year, includeSearch)
			if err == nil && len(candidates) > 0 {
				select {
				case resultCh <- yearResult{candidates: candidates, year: year}:
				case <-ctx.Done():
				}
			}
		}()
	}

	// Wait for all year goroutines to finish, then close results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect all results, prefer closest year
	found := map[int][]impCandidate{}
	for res := range resultCh {
		found[res.year] = res.candidates
	}
	for _, year := range years {
		if candidates, ok := found[year]; ok {
			return candidates
		}
	}
	return nil
}

func impSearchMovies(movie plex.Movie) []plex.Movie {
	movies := []plex.Movie{movie}
	originalTitle := strings.TrimSpace(movie.OriginalTitle)
	if originalTitle != "" && normalizeTitle(originalTitle) != normalizeTitle(movie.Title) {
		original := movie
		original.Title = originalTitle
		movies = append(movies, original)
	}
	return movies
}

func (s *Service) impCandidatesForYear(ctx context.Context, movie plex.Movie, year int, includeSearch bool) ([]impCandidate, error) {
	candidates := []impCandidate{}
	seen := map[string]bool{}
	pageURLs := impProbeURLsForYear(movie, year)
	if includeSearch {
		searchURLs, err := s.impSearchURLsForYear(ctx, movie, year)
		if err == nil {
			pageURLs = append(pageURLs, searchURLs...)
		}
	}
	for i := 0; i < len(pageURLs); i++ {
		pageURL := pageURLs[i]
		if seen[pageURL] {
			continue
		}
		seen[pageURL] = true
		body, err := s.fetchText(ctx, pageURL)
		if err != nil {
			continue
		}
		candidate, ok := parseIMPCandidate(pageURL, body)
		if !ok || !titleMatches(movie.Title, candidate.Title) || candidate.Year != year {
			for _, linkedPageURL := range parseIMPPageLinksForTitle(pageURL, body, movie.Title, year) {
				if !seen[linkedPageURL] {
					pageURLs = append(pageURLs, linkedPageURL)
				}
			}
			continue
		}
		candidates = append(candidates, candidate)
		// If this candidate was found via a shortened slug (i.e. its
		// slug is not one of the full title slugs), its version variants
		// weren't probed by impVersionProbeURLsForYear. Add them now.
		if candidate.Version == 0 && !isFullTitleSlug(candidate.PageURL, movie.Title) {
			for v := 1; v <= 8; v++ {
				verURL := versionURL(pageURL, v)
				if !seen[verURL] {
					pageURLs = append(pageURLs, verURL)
				}
			}
		}
		// Discover additional version variants from the canonical page body.
		// IMP often has ver10+ (Tron has ver26, Mario has ver24) that aren't
		// covered by the standard ver1-8 probe range. Scanning the page body
		// finds only versions that actually exist on IMP.
		if candidate.Version == 0 {
			for _, verURL := range impVersionURLsFromBody(pageURL, body) {
				if !seen[verURL] {
					pageURLs = append(pageURLs, verURL)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no IMP Awards poster found for %s (%d)", movie.Title, year)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Canonical != candidates[j].Canonical {
			return candidates[i].Canonical
		}
		return candidates[i].Version < candidates[j].Version
	})
	return candidates, nil
}

func (s *Service) impSearchURLs(ctx context.Context, movie plex.Movie) ([]string, error) {
	return s.impSearchURLsForYear(ctx, movie, movie.Year)
}

func (s *Service) impSearchURLsForYear(ctx context.Context, movie plex.Movie, year int) ([]string, error) {
	queries := []string{fmt.Sprintf("%s %d", movie.Title, year), movie.Title}
	seen := map[string]bool{}
	urls := []string{}
	for _, query := range queries {
		body, err := s.fetchText(ctx, impSearchPHPURL(query))
		if err != nil {
			continue
		}
		for _, pageURL := range parseIMPSearchPHPResults(impBase+"/search.php", body) {
			if seen[pageURL] || !looksLikeIMPMoviePage(pageURL, year) {
				continue
			}
			seen[pageURL] = true
			urls = append(urls, pageURL)
		}
	}
	return urls, nil
}

func impSearchPHPURL(query string) string {
	return fmt.Sprintf("%s/search.php?search_data=%s", impBase, url.QueryEscape(query))
}
