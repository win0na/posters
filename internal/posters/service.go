package posters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/win0na/posters/internal/plex"
)

const (
	impBase                = "http://www.impawards.com"
	wikipediaAPI           = "https://en.wikipedia.org/w/api.php"
	maxPosterSize          = 25 << 20
	minVisualMatchScore    = 0.82
	clearVisualMatchScore  = 0.70
	clearVisualMatchMargin = 0.10
)

var (
	errAmbiguous = errors.New("ambiguous poster match")

	impHeadingRE  = regexp.MustCompile(`(?is)<title>\s*(.*?)\s*\((\d{4})\).*?</title>|<h[1-6][^>]*>\s*(.*?)\s*\((\d{4})\).*?</h[1-6]>`)
	impHRE        = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	impImageRE    = regexp.MustCompile(`(?is)(?:href|src)=["']([^"']*posters/[^"']+\.(?:jpg|jpeg|png))["']`)
	impSizePageRE = regexp.MustCompile(`(?is)href=["']([^"']*_(?:xlg|xxlg)\.html)["']`)
	impLinkRE     = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+\.html)["'][^>]*>(.*?)</a>`)
	wikiImgRE     = regexp.MustCompile(`(?is)<table[^>]+class="[^"]*infobox[^"]*".*?<img[^>]+(?:src|data-src)="([^"]+)"[^>]*(?:alt="([^"]*)")?`)
	wikiCapRE     = regexp.MustCompile(`(?is)<table[^>]+class="[^"]*infobox[^"]*".*?</table>`)
	tagRE         = regexp.MustCompile(`(?is)<[^>]+>`)
)

type Candidate struct {
	Movie       plex.Movie
	ImageURL    string
	SourceURL   string
	MatchReason string
	Bytes       []byte
}

type CandidateSummary struct {
	PageURL   string
	ImageURL  string
	Version   int
	Canonical bool
}

type AmbiguousMatchError struct {
	Movie      plex.Movie
	Candidates []CandidateSummary
}

func (e *AmbiguousMatchError) Error() string {
	parts := make([]string, 0, min(3, len(e.Candidates)))
	for i, candidate := range e.Candidates {
		if i >= 3 {
			break
		}
		parts = append(parts, candidate.PageURL)
	}
	if len(e.Candidates) > 3 {
		parts = append(parts, fmt.Sprintf("+%d more", len(e.Candidates)-3))
	}
	return fmt.Sprintf("%s for %s (%d): %d IMP candidates [%s]", errAmbiguous, e.Movie.Title, e.Movie.Year, len(e.Candidates), strings.Join(parts, ", "))
}

func (e *AmbiguousMatchError) Summary() string {
	return fmt.Sprintf("ambiguous IMP match: %d candidates", len(e.Candidates))
}

func (e *AmbiguousMatchError) Is(target error) bool {
	return target == errAmbiguous
}

type impCandidate struct {
	Title     string
	Year      int
	PageURL   string
	ImageURL  string
	Version   int
	Canonical bool
}

type matchedCandidate struct {
	Candidate impCandidate
	Bytes     []byte
	Score     float64
	Reason    string
	NameHint  bool
	Err       error
}

type wikiPoster struct {
	PageTitle string
	ImageURL  string
	Alt       string
	Caption   string
	Poster    bool
}

type wikipediaSearchResult struct {
	Title   string
	Snippet string
}

type Service struct {
	http      *http.Client
	mu        sync.Mutex
	lastFetch time.Time
	cacheDir  string
}

func NewService() *Service {
	return &Service{http: &http.Client{Timeout: 30 * time.Second}, cacheDir: defaultCacheDir()}
}

func (s *Service) FindTheatricalPoster(ctx context.Context, movie plex.Movie) (Candidate, error) {
	wiki, wikiErr := s.wikipediaPoster(ctx, movie)
	candidates, err := s.impCandidates(ctx, movie)
	if err != nil && wiki.PageTitle != "" {
		wikiMovie := movie
		wikiMovie.Title = wikipediaMovieTitle(wiki.PageTitle)
		if normalizeTitle(wikiMovie.Title) != normalizeTitle(movie.Title) {
			if wikiCandidates, wikiCandidateErr := s.impCandidates(ctx, wikiMovie); wikiCandidateErr == nil {
				candidates = wikiCandidates
				err = nil
			}
		}
	}
	if err != nil {
		return Candidate{}, err
	}

	if wikiErr == nil && wiki.Poster {
		chosen, data, reason, err := s.chooseVisualCandidate(ctx, movie, candidates, wiki)
		if err == nil {
			return Candidate{Movie: movie, ImageURL: chosen.ImageURL, SourceURL: chosen.PageURL, MatchReason: reason, Bytes: data}, nil
		}
		return Candidate{}, err
	}

	chosen, reason, err := chooseStructuredCandidate(movie, candidates, wiki)
	if err != nil {
		if wikiErr != nil {
			return Candidate{}, fmt.Errorf("%w; wikipedia poster check: %v", err, wikiErr)
		}
		return Candidate{}, err
	}
	data, err := s.downloadIMPImage(ctx, chosen.ImageURL)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Movie: movie, ImageURL: chosen.ImageURL, SourceURL: chosen.PageURL, MatchReason: reason, Bytes: data}, nil
}

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
	for _, query := range queries {
		values := url.Values{
			"action": {"query"}, "list": {"search"}, "format": {"json"}, "srlimit": {"5"}, "srsearch": {query},
		}
		body, err := s.fetchText(ctx, wikipediaAPI+"?"+values.Encode())
		if err != nil {
			return "", err
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
			return "", err
		}
		results := make([]wikipediaSearchResult, 0, len(response.Query.Search))
		for _, result := range response.Query.Search {
			results = append(results, wikipediaSearchResult{Title: result.Title, Snippet: result.Snippet})
		}
		if title := chooseWikipediaSearchResult(movie, results); title != "" {
			return title, nil
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

func chooseWikipediaSearchResult(movie plex.Movie, results []wikipediaSearchResult) string {
	titles := make([]string, 0, len(results))
	for _, result := range results {
		titles = append(titles, result.Title)
	}
	if len(titles) == 0 {
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
	if bestTitle != "" {
		return bestTitle
	}
	return ""
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

func (s *Service) impCandidates(ctx context.Context, movie plex.Movie) ([]impCandidate, error) {
	for _, searchMovie := range impSearchMovies(movie) {
		for _, year := range impCandidateYears(searchMovie.Year) {
			candidates, err := s.impCandidatesForYear(ctx, searchMovie, year, false)
			if err == nil && len(candidates) > 0 {
				return candidates, nil
			}
		}
	}
	for _, searchMovie := range impSearchMovies(movie) {
		for _, year := range impCandidateYears(searchMovie.Year) {
			candidates, err := s.impCandidatesForYear(ctx, searchMovie, year, true)
			if err == nil && len(candidates) > 0 {
				return candidates, nil
			}
		}
	}
	return nil, fmt.Errorf("no IMP Awards poster found for %s (%d)", movie.Title, movie.Year)
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
	queries := []string{
		fmt.Sprintf("%s %d", movie.Title, year),
		movie.Title,
	}
	seen := map[string]bool{}
	urls := []string{}
	for _, query := range queries {
		body, err := s.fetchText(ctx, impSearchURL(query, 1))
		if err != nil {
			continue
		}
		for _, pageURL := range parseIMPSearchResults(impBase+"/cgi-bin/htsearch", body) {
			if seen[pageURL] {
				continue
			}
			if !looksLikeIMPMoviePage(pageURL, year) {
				continue
			}
			seen[pageURL] = true
			urls = append(urls, pageURL)
		}
	}
	return urls, nil
}

func impSearchURL(query string, page int) string {
	return fmt.Sprintf("%s/cgi-bin/htsearch?words=%s;page=%d", impBase, url.QueryEscape(query), page)
}

func (s *Service) fetchText(ctx context.Context, rawURL string) (string, error) {
	if data, ok := s.readCache("text", rawURL); ok {
		return string(data), nil
	}
	if err := s.throttle(ctx); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "posters/0.1 (+https://github.com/win0na/posters)")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("not found: %s", rawURL)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch %s: %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	s.writeCache("text", rawURL, data)
	return string(data), nil
}

func (s *Service) downloadIMPImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Host != "www.impawards.com" && u.Host != "impawards.com" {
		return nil, fmt.Errorf("refusing non-IMP image source: %s", rawURL)
	}
	return s.downloadImage(ctx, rawURL)
}

func (s *Service) downloadWikipediaImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported wikipedia image URL: %s", rawURL)
	}
	return s.downloadImage(ctx, rawURL)
}

func (s *Service) downloadImage(ctx context.Context, rawURL string) ([]byte, error) {
	if data, ok := s.readCache("images", rawURL); ok {
		return data, nil
	}
	if err := s.throttle(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "posters/0.1 (+https://github.com/win0na/posters)")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download poster: %s", resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("poster response is not image content: %s", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPosterSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPosterSize {
		return nil, fmt.Errorf("poster image too large")
	}
	s.writeCache("images", rawURL, data)
	return data, nil
}

func defaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "posters")
}

func (s *Service) cachePath(kind, rawURL string) string {
	if s.cacheDir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(rawURL))
	return filepath.Join(s.cacheDir, kind, hex.EncodeToString(sum[:]))
}

func (s *Service) readCache(kind, rawURL string) ([]byte, bool) {
	path := s.cachePath(kind, rawURL)
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

func (s *Service) writeCache(kind, rawURL string, data []byte) {
	path := s.cachePath(kind, rawURL)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func (s *Service) throttle(ctx context.Context) error {
	const delay = 250 * time.Millisecond
	s.mu.Lock()
	defer s.mu.Unlock()
	wait := time.Until(s.lastFetch.Add(delay))
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	s.lastFetch = time.Now()
	return nil
}

func (s *Service) chooseVisualCandidate(ctx context.Context, movie plex.Movie, candidates []impCandidate, wiki wikiPoster) (impCandidate, []byte, string, error) {
	if len(candidates) == 0 {
		return impCandidate{}, nil, "", fmt.Errorf("no poster candidates")
	}
	if !wiki.Poster {
		return impCandidate{}, nil, "", fmt.Errorf("wikipedia did not confirm theatrical poster")
	}
	wikiData, err := s.downloadWikipediaImage(ctx, wiki.ImageURL)
	if err != nil {
		return impCandidate{}, nil, "", fmt.Errorf("download wikipedia poster for visual match: %w", err)
	}
	wikiFPs, err := imageFingerprints(wikiData)
	if err != nil {
		return impCandidate{}, nil, "", fmt.Errorf("decode wikipedia poster for visual match: %w", err)
	}
	matches := make([]matchedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		data, err := s.downloadIMPImage(ctx, candidate.ImageURL)
		match := matchedCandidate{Candidate: candidate, Bytes: data, Err: err}
		if err == nil {
			impFPs, fpErr := imageFingerprints(data)
			if fpErr != nil {
				match.Err = fpErr
			} else {
				match.Score = maxVisualSimilarity(wikiFPs, impFPs)
				match.Reason = visualMatchReason(match.Score)
				match.NameHint = samePosterImageName(wiki.ImageURL, candidate.ImageURL)
			}
		}
		matches = append(matches, match)
	}
	best, second, ok := bestVisualMatch(matches)
	if !ok {
		return impCandidate{}, nil, "", fmt.Errorf("no IMP candidate image could be visually compared")
	}
	if !isConfidentVisualMatch(best, second) {
		return impCandidate{}, nil, "", &AmbiguousMatchError{Movie: movie, Candidates: summarizeCandidates(candidates)}
	}
	reason := visualMatchReason(best.Score)
	if second != nil {
		reason = fmt.Sprintf("%s; next best %s", visualMatchReason(best.Score), visualScorePercent(second.Score))
	}
	return best.Candidate, best.Bytes, reason, nil
}

func isConfidentVisualMatch(best matchedCandidate, second *matchedCandidate) bool {
	if best.Score >= minVisualMatchScore {
		return true
	}
	if best.NameHint && best.Score >= clearVisualMatchScore {
		return true
	}
	if second == nil {
		return best.Score >= minVisualMatchScore
	}
	return best.Score >= clearVisualMatchScore && best.Score-second.Score >= clearVisualMatchMargin
}

func visualMatchReason(score float64) string {
	return "visual match " + visualScorePercent(score)
}

func visualScorePercent(score float64) string {
	return fmt.Sprintf("%.1f%%", score*100)
}

func bestVisualMatch(matches []matchedCandidate) (matchedCandidate, *matchedCandidate, bool) {
	valid := []matchedCandidate{}
	for _, match := range matches {
		if match.Err == nil && len(match.Bytes) > 0 {
			valid = append(valid, match)
		}
	}
	if len(valid) == 0 {
		return matchedCandidate{}, nil, false
	}
	sort.SliceStable(valid, func(i, j int) bool {
		if scoreDelta := valid[i].Score - valid[j].Score; scoreDelta > 0.002 || scoreDelta < -0.002 {
			return valid[i].Score > valid[j].Score
		}
		return candidatePreferenceRank(valid[i].Candidate) > candidatePreferenceRank(valid[j].Candidate)
	})
	if len(valid) == 1 {
		return valid[0], nil, true
	}
	second := valid[1]
	return valid[0], &second, true
}

func candidatePreferenceRank(candidate impCandidate) int {
	if candidate.Version == 1 {
		return 3
	}
	if !candidate.Canonical && candidate.Version > 0 {
		return 2
	}
	if candidate.Canonical {
		return 1
	}
	return 0
}

func samePosterImageName(a, b string) bool {
	left, right := posterImageStem(a), posterImageStem(b)
	return left != "" && left == right
}

func posterImageStem(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name, err := url.PathUnescape(path.Base(parsed.Path))
	if err != nil {
		return ""
	}
	stem := strings.TrimSuffix(name, path.Ext(name))
	if idx := strings.Index(strings.ToLower(stem), "px-"); idx > 0 {
		stem = stem[idx+3:]
	}
	stem = strings.TrimSuffix(stem, "_xxlg")
	stem = strings.TrimSuffix(stem, "_xlg")
	return normalizeTitle(stem)
}

func chooseCandidate(movie plex.Movie, candidates []impCandidate, wiki wikiPoster) (impCandidate, string, error) {
	return chooseStructuredCandidate(movie, candidates, wiki)
}

func chooseStructuredCandidate(movie plex.Movie, candidates []impCandidate, wiki wikiPoster) (impCandidate, string, error) {
	if len(candidates) == 0 {
		return impCandidate{}, "", fmt.Errorf("no poster candidates")
	}
	if len(candidates) == 1 {
		return candidates[0], structuredReason("only IMP candidate", wiki), nil
	}
	if wiki.Poster {
		if chosen, score, ok := chooseByWikipediaSignal(movie, candidates, wiki); ok {
			return chosen, fmt.Sprintf("Wikipedia/IMP descriptive token match score %d", score), nil
		}
	}
	canonical := []impCandidate{}
	for _, candidate := range candidates {
		if candidate.Canonical || candidate.Version == 1 {
			canonical = append(canonical, candidate)
		}
	}
	if len(canonical) == 1 {
		return canonical[0], structuredReason("single canonical IMP candidate", wiki), nil
	}
	return impCandidate{}, "", &AmbiguousMatchError{Movie: movie, Candidates: summarizeCandidates(candidates)}
}

func structuredReason(reason string, wiki wikiPoster) string {
	if wiki.Poster {
		return reason + "; Wikipedia confirmed poster"
	}
	return reason + "; Wikipedia poster unavailable"
}

func chooseByWikipediaSignal(movie plex.Movie, candidates []impCandidate, wiki wikiPoster) (impCandidate, int, bool) {
	wikiTokens := descriptiveTokens(wiki.ImageURL+" "+wiki.Alt+" "+wiki.Caption, movie)
	if len(wikiTokens) == 0 {
		return impCandidate{}, 0, false
	}
	bestIndex, bestScore, secondScore := -1, 0, 0
	for i, candidate := range candidates {
		score := tokenOverlapScore(wikiTokens, descriptiveTokens(candidate.PageURL+" "+candidate.ImageURL, movie))
		if score > bestScore {
			secondScore = bestScore
			bestScore = score
			bestIndex = i
			continue
		}
		if score > secondScore {
			secondScore = score
		}
	}
	if bestIndex == -1 || bestScore < 2 || bestScore-secondScore < 2 {
		return impCandidate{}, 0, false
	}
	return candidates[bestIndex], bestScore, true
}

func tokenOverlapScore(a, b map[string]bool) int {
	score := 0
	for token := range a {
		if b[token] {
			score++
		}
	}
	return score
}

func descriptiveTokens(text string, movie plex.Movie) map[string]bool {
	ignored := map[string]bool{
		"a": true, "an": true, "and": true, "by": true, "cover": true, "film": true, "image": true,
		"jpg": true, "jpeg": true, "lg": true, "movie": true, "of": true, "one": true, "png": true,
		"poster": true, "release": true, "sheet": true, "the": true, "theatrical": true, "thumb": true,
		"ver": true, "xlg": true, "xxlg": true,
		strconv.Itoa(movie.Year): true,
	}
	for _, token := range strings.Fields(normalizeTitle(movie.Title)) {
		ignored[token] = true
	}
	tokens := map[string]bool{}
	for _, token := range strings.Fields(normalizeTitle(splitVersionMarkers(text))) {
		if len(token) < 3 || ignored[token] {
			continue
		}
		tokens[token] = true
	}
	return tokens
}

func splitVersionMarkers(text string) string {
	replacer := strings.NewReplacer("_ver", " ver ", "_xlg", " xlg", "_xxlg", " xxlg")
	return replacer.Replace(text)
}

func summarizeCandidates(candidates []impCandidate) []CandidateSummary {
	summary := make([]CandidateSummary, 0, len(candidates))
	for _, candidate := range candidates {
		summary = append(summary, CandidateSummary{PageURL: candidate.PageURL, ImageURL: candidate.ImageURL, Version: candidate.Version, Canonical: candidate.Canonical})
	}
	return summary
}

func parseIMPCandidate(pageURL, body string) (impCandidate, bool) {
	title, year, ok := parseIMPHeading(body)
	if !ok {
		return impCandidate{}, false
	}
	imageURL := bestIMPImage(pageURL, body)
	if imageURL == "" {
		return impCandidate{}, false
	}
	version := versionFromURL(pageURL)
	return impCandidate{Title: title, Year: year, PageURL: pageURL, ImageURL: imageURL, Version: version, Canonical: !strings.Contains(path.Base(pageURL), "_ver")}, true
}

func parseIMPSearchResults(baseURL, body string) []string {
	matches := impLinkRE.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	urls := []string{}
	for _, match := range matches {
		raw := html.UnescapeString(match[1])
		pageURL := absoluteURL(baseURL, raw)
		if seen[pageURL] || !isIMPURL(pageURL) {
			continue
		}
		if !strings.HasSuffix(pageURL, ".html") || strings.Contains(pageURL, "_gallery") || strings.Contains(pageURL, "/news/") {
			continue
		}
		seen[pageURL] = true
		urls = append(urls, pageURL)
	}
	return urls
}

func isIMPURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Host == "www.impawards.com" || u.Host == "impawards.com"
}

func parseIMPPageLinksForTitle(baseURL, body, title string, year int) []string {
	seen := map[string]bool{}
	urls := []string{}
	for _, match := range impLinkRE.FindAllStringSubmatch(body, -1) {
		if len(match) < 3 {
			continue
		}
		pageURL := absoluteURL(baseURL, html.UnescapeString(match[1]))
		if seen[pageURL] || !looksLikeIMPMoviePage(pageURL, year) {
			continue
		}
		linkText := cleanText(match[2])
		if !titleMatches(title, linkText) {
			continue
		}
		seen[pageURL] = true
		urls = append(urls, pageURL)
	}
	return urls
}

func looksLikeIMPMoviePage(rawURL string, year int) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if !isIMPURL(rawURL) {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return false
	}
	yearText := strconv.Itoa(year)
	yearIndex := -1
	for i, part := range parts[:len(parts)-1] {
		if part == yearText {
			yearIndex = i
			break
		}
	}
	if yearIndex == -1 {
		return false
	}
	file := parts[len(parts)-1]
	return strings.HasSuffix(file, ".html") && !strings.Contains(file, "_gallery")
}

func parseIMPHeading(body string) (string, int, bool) {
	matches := impHeadingRE.FindStringSubmatch(body)
	if len(matches) > 0 {
		title, yearText := matches[1], matches[2]
		if title == "" {
			title, yearText = matches[3], matches[4]
		}
		title = cleanText(title)
		year, err := strconv.Atoi(yearText)
		if err == nil {
			return title, year, true
		}
	}
	for _, match := range impHRE.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		text := cleanText(match[1])
		parts := regexp.MustCompile(`^(.*?)\s*\(\s*(\d{4})\s*\)`).FindStringSubmatch(text)
		if len(parts) != 3 {
			continue
		}
		year, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		return strings.TrimSpace(parts[1]), year, true
	}
	return "", 0, false
}

func bestIMPImage(pageURL, body string) string {
	best := ""
	for _, match := range impImageRE.FindAllStringSubmatch(body, -1) {
		candidate := absoluteURL(pageURL, html.UnescapeString(match[1]))
		if best == "" || imageRank(candidate) > imageRank(best) {
			best = candidate
		}
	}
	for _, match := range impSizePageRE.FindAllStringSubmatch(body, -1) {
		candidate := imageURLFromIMPSizePage(pageURL, html.UnescapeString(match[1]))
		if candidate == "" {
			continue
		}
		if best == "" || imageRank(candidate) > imageRank(best) {
			best = candidate
		}
	}
	if upgraded := upgradeIMPImageFromSizeLinks(pageURL, best, body); upgraded != "" && imageRank(upgraded) > imageRank(best) {
		best = upgraded
	}
	return best
}

func upgradeIMPImageFromSizeLinks(pageURL, imageURL, body string) string {
	if imageURL == "" || strings.Contains(imageURL, "_xlg") || strings.Contains(imageURL, "_xxlg") {
		return ""
	}
	u, err := url.Parse(imageURL)
	if err != nil {
		return ""
	}
	base := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	pageDir := path.Dir(mustURLPath(pageURL))
	for _, suffix := range []string{"_xxlg", "_xlg"} {
		pageName := base + suffix + ".html"
		if !strings.Contains(body, pageName) {
			continue
		}
		upgraded := *u
		upgraded.Path = path.Join(pageDir, "posters", base+suffix+".jpg")
		upgraded.RawQuery = ""
		upgraded.Fragment = ""
		return upgraded.String()
	}
	return ""
}

func mustURLPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Path
}

func imageURLFromIMPSizePage(pageURL, raw string) string {
	sizePage := absoluteURL(pageURL, raw)
	u, err := url.Parse(sizePage)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if !strings.HasSuffix(base, ".html") {
		return ""
	}
	imageBase := strings.TrimSuffix(base, ".html") + ".jpg"
	u.Path = path.Join(path.Dir(u.Path), "posters", imageBase)
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func parseWikipediaPoster(title, body string) wikiPoster {
	poster := wikiPoster{PageTitle: title}
	matches := wikiImgRE.FindStringSubmatch(body)
	if len(matches) >= 2 {
		poster.ImageURL = normalizeWikiImageURL(matches[1])
	}
	if len(matches) >= 3 {
		poster.Alt = cleanText(matches[2])
	}
	infobox := wikiCapRE.FindString(body)
	poster.Caption = cleanText(infobox)
	signal := strings.ToLower(poster.ImageURL + " " + poster.Alt + " " + poster.Caption)
	poster.Poster = strings.Contains(signal, "poster") || strings.Contains(signal, "one-sheet") || strings.Contains(signal, "one sheet") || strings.Contains(signal, "theatrical")
	return poster
}

func impProbeURLs(movie plex.Movie) []string {
	return impProbeURLsForYear(movie, movie.Year)
}

func impProbeURLsForYear(movie plex.Movie, year int) []string {
	urls := []string{}
	for _, slug := range titleSlugs(movie.Title) {
		base := fmt.Sprintf("%s/%d/%s", impBase, year, slug)
		urls = append(urls, base+".html")
		for version := 1; version <= 8; version++ {
			urls = append(urls, fmt.Sprintf("%s_ver%d.html", base, version))
		}
	}
	return urls
}

func impCandidateYears(year int) []int {
	if year <= 0 {
		return nil
	}
	return []int{year, year - 1, year + 1, year - 2, year + 2, year - 3, year + 3}
}

func titleSlugs(title string) []string {
	normal := normalizeTitle(title)
	parts := strings.Fields(normal)
	if len(parts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	slugs := []string{}
	for _, variant := range titleSlugPartVariants(parts) {
		for _, slugParts := range articleSlugPartVariants(variant) {
			slug := strings.Join(slugParts, "_")
			if !seen[slug] {
				seen[slug] = true
				slugs = append(slugs, slug)
			}
		}
	}
	return slugs
}

func titleSlugPartVariants(parts []string) [][]string {
	variants := [][]string{append([]string(nil), parts...)}
	numberWords := map[string]string{
		"0":   "zero",
		"1":   "one",
		"2":   "two",
		"3":   "three",
		"4":   "four",
		"5":   "five",
		"6":   "six",
		"7":   "seven",
		"8":   "eight",
		"9":   "nine",
		"i":   "one",
		"ii":  "two",
		"iii": "three",
		"iv":  "four",
		"v":   "five",
		"vi":  "six",
	}
	replaced := append([]string(nil), parts...)
	changed := false
	for i, part := range replaced {
		if word, ok := numberWords[part]; ok {
			replaced[i] = word
			changed = true
		}
	}
	if changed {
		variants = append(variants, replaced)
	}
	if len(parts) >= 2 && parts[len(parts)-1] == "movie" && parts[len(parts)-2] != "the" {
		withThe := append([]string(nil), parts[:len(parts)-1]...)
		withThe = append(withThe, "the", "movie")
		variants = append(variants, withThe)
	}
	return variants
}

func titleMatches(movieTitle, candidateTitle string) bool {
	movie := normalizeTitle(movieTitle)
	candidate := normalizeTitle(candidateTitle)
	if movie == "" || candidate == "" {
		return false
	}
	if movie == candidate {
		return true
	}
	movieTokens := strings.Fields(movie)
	candidateTokens := strings.Fields(candidate)
	if len(movieTokens) >= 2 && len(candidateTokens) > len(movieTokens) {
		matchesPrefix := true
		for i, token := range movieTokens {
			if candidateTokens[i] != token {
				matchesPrefix = false
				break
			}
		}
		if matchesPrefix {
			return true
		}
	}
	return false
}

func articleSlugPartVariants(parts []string) [][]string {
	variants := [][]string{append([]string(nil), parts...)}
	if len(parts) > 1 && (parts[0] == "the" || parts[0] == "a" || parts[0] == "an") {
		moved := append([]string(nil), parts[1:]...)
		moved = append(moved, parts[0])
		variants = append(variants, moved)
	}
	return variants
}

func normalizeTitle(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	for _, r := range title {
		r = foldTitleRune(r)
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == ':' || r == '&' || r == '/' || r == '.':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func foldTitleRune(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ä', 'ã', 'å', 'ā':
		return 'a'
	case 'ç':
		return 'c'
	case 'é', 'è', 'ê', 'ë', 'ē':
		return 'e'
	case 'í', 'ì', 'î', 'ï', 'ī':
		return 'i'
	case 'ñ':
		return 'n'
	case 'ó', 'ò', 'ô', 'ö', 'õ', 'ō':
		return 'o'
	case 'ú', 'ù', 'û', 'ü', 'ū':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	}
	return r
}

func cleanText(text string) string {
	text = tagRE.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.Join(strings.Fields(text), " ")
}

func absoluteURL(baseURL, raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func normalizeWikiImageURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func wikipediaOriginalImageURL(raw string) string {
	u, err := url.Parse(normalizeWikiImageURL(raw))
	if err != nil {
		return normalizeWikiImageURL(raw)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	thumb := -1
	for i, part := range parts {
		if part == "thumb" {
			thumb = i
			break
		}
	}
	if thumb == -1 || len(parts) < thumb+5 {
		return u.String()
	}
	parts = append(parts[:thumb], parts[thumb+1:len(parts)-1]...)
	u.Path = "/" + strings.Join(parts, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func versionFromURL(rawURL string) int {
	base := path.Base(rawURL)
	start := strings.Index(base, "_ver")
	if start == -1 {
		return 0
	}
	start += len("_ver")
	end := start
	for end < len(base) && base[end] >= '0' && base[end] <= '9' {
		end++
	}
	version, _ := strconv.Atoi(base[start:end])
	return version
}

func imageRank(rawURL string) int {
	score := 0
	if strings.Contains(rawURL, "/posters/") {
		score += 10
	}
	if strings.Contains(rawURL, "_xxlg") {
		score += 4
	}
	if strings.Contains(rawURL, "_xlg") {
		score += 3
	}
	return score
}
