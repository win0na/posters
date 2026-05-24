package posters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/win0na/posters/internal/plex"
)

func (s *Service) FindWikipediaPoster(ctx context.Context, movie plex.Movie) (Candidate, error) {
	wiki, err := s.wikipediaPoster(ctx, movie)
	if err != nil {
		return Candidate{}, err
	}
	if !wiki.Poster {
		return Candidate{}, fmt.Errorf("wikipedia main image is not a poster for %s (%d)", movie.Title, movie.Year)
	}
	imageURL := wikipediaOriginalImageURL(wiki.ImageURL)
	data, err := s.downloadWikipediaImage(ctx, imageURL)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Movie: movie, ImageURL: imageURL, SourceURL: imageURL, MatchReason: "Wikipedia fallback theatrical poster", Bytes: data}, nil
}

func (s *Service) wikipediaPoster(ctx context.Context, movie plex.Movie) (wikiPoster, error) {
	title, err := s.wikipediaPageTitle(ctx, movie)
	if err != nil {
		return wikiPoster{}, err
	}
	pageURL := "https://en.wikipedia.org/api/rest_v1/page/html/" + url.PathEscape(title)
	body, err := s.fetchText(ctx, pageURL)
	if err != nil {
		return wikiPoster{}, err
	}
	poster := parseWikipediaPoster(title, body)
	if poster.ImageURL == "" {
		return wikiPoster{}, fmt.Errorf("no wikipedia infobox poster found for %s", title)
	}
	return poster, nil
}

func wikipediaMovieTitle(pageTitle string) string {
	title := strings.TrimSpace(pageTitle)
	for _, suffix := range []string{" (film)", " (movie)"} {
		if strings.HasSuffix(strings.ToLower(title), suffix) {
			return strings.TrimSpace(title[:len(title)-len(suffix)])
		}
	}
	return title
}

func (s *Service) wikipediaPageTitle(ctx context.Context, movie plex.Movie) (string, error) {
	queries := []string{
		fmt.Sprintf("%s %d film", movie.Title, movie.Year),
		fmt.Sprintf("%s film", movie.Title),
		movie.Title,
	}
	// Run queries sequentially in order of specificity so the most
	// targeted query ("Title Year film") always has first chance to
	// find a result. Concurrent goroutines create a race where a
	// less-specific query (e.g. "Hellraiser film") can return a
	// wrong-period film page (e.g. "Hellraiser (2022 film)") before
	// the year-qualified query resolves.
	for _, query := range queries {
		values := url.Values{"action": {"query"}, "list": {"search"}, "format": {"json"}, "srlimit": {"5"}, "srsearch": {query}}
		body, err := s.fetchText(ctx, wikipediaAPI+"?"+values.Encode())
		if err != nil {
			continue
		}
		var response struct {
			Query struct {
				Search []struct {
					Title   string `json:"title"`
					Snippet string `json:"snippet"`
				} `json:"search"`
			} `json:"query"`
		}
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			continue
		}
		results := make([]wikipediaSearchResult, 0, len(response.Query.Search))
		for _, result := range response.Query.Search {
			results = append(results, wikipediaSearchResult{Title: result.Title, Snippet: result.Snippet})
		}
		if title := chooseWikipediaSearchResult(movie, results); title != "" {
			return title, nil
		}
	}
	// Fallback: try direct REST API lookups for common Wikipedia titles.
	// Catches cases where search fails (e.g., exact title is not a movie article
	// but a disambiguation page, or the search result pool lacks the movie page).
	for _, candidate := range []string{
		movie.Title + " (film)",
		fmt.Sprintf("%s (%d film)", movie.Title, movie.Year),
	} {
		pageURL := "https://en.wikipedia.org/api/rest_v1/page/html/" + url.PathEscape(candidate)
		body, err := s.fetchText(ctx, pageURL)
		if err != nil {
			continue
		}
		poster := parseWikipediaPoster(candidate, body)
		if poster.ImageURL != "" {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no wikipedia page found for %s (%d)", movie.Title, movie.Year)
}

func chooseWikipediaSearchTitle(movie plex.Movie, titles []string) string {
	results := make([]wikipediaSearchResult, 0, len(titles))
	for _, title := range titles {
		results = append(results, wikipediaSearchResult{Title: title})
	}
	return chooseWikipediaSearchResult(movie, results)
}

const minWikipediaSearchScore = 300

func chooseWikipediaSearchResult(movie plex.Movie, results []wikipediaSearchResult) string {
	if len(results) == 0 {
		return ""
	}
	movieTitle := normalizeTitle(movie.Title)
	filmTitle := normalizeTitle(movie.Title + " film")
	year := strconv.Itoa(movie.Year)
	bestTitle, bestScore := "", 0
	for i, result := range results {
		title := result.Title
		if isNonMovieWikipediaTitle(title) {
			continue
		}
		normal := normalizeTitle(title)
		text := normalizeTitle(title + " " + result.Snippet)
		score := -i
		if strings.Contains(text, year) {
			score += 200
		}
		if normal == filmTitle {
			score += 1000
		} else if strings.HasPrefix(normal, movieTitle+" ") && strings.Contains(normal, "film") {
			score += 900
		} else if normal != movieTitle && !strings.Contains(title, "(") && titleMatches(movie.Title, title) {
			if result.Snippet == "" {
				score += 180
			} else {
				score += 120
			}
		} else if normal == movieTitle {
			score += 150
		} else if normal != movieTitle && titleMatches(movie.Title, title) {
			score += 50
		}
		if score > bestScore {
			bestTitle, bestScore = title, score
		}
	}
	if bestScore < minWikipediaSearchScore {
		return ""
	}
	return bestTitle
}

func isNonMovieWikipediaTitle(title string) bool {
	normal := normalizeTitle(title)
	for _, marker := range []string{" tv series", " television series", " soundtrack", " album", " video game"} {
		if strings.Contains(normal, marker) {
			return true
		}
	}
	return false
}
