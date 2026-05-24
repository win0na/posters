package posters

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/win0na/posters/internal/plex"
)

const (
	impBase                = "http://www.impawards.com"
	wikipediaAPI           = "https://en.wikipedia.org/w/api.php"
	maxPosterSize          = 25 << 20
	minVisualMatchScore    = 0.82
	clearVisualMatchScore  = 0.70
	clearVisualMatchMargin = 0.10
	negativeCacheTTL       = 14 * 24 * time.Hour
	visualFetchConcurrency = 4
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
	PageURL        string
	ImageURL       string
	Version        int
	Canonical      bool
	VisualScore    float64
	HasVisualScore bool
}

type AmbiguousMatchError struct {
	Movie      plex.Movie
	Candidates []CandidateSummary
}

func (e *AmbiguousMatchError) Error() string {
	return fmt.Sprintf("%s for %s (%d): %s", errAmbiguous, e.Movie.Title, e.Movie.Year, e.Summary())
}

func (e *AmbiguousMatchError) Summary() string {
	if best, ok := e.bestVisualScore(); ok {
		return fmt.Sprintf("%d candidates; best visual match %s", len(e.Candidates), visualScorePercent(best))
	}
	return fmt.Sprintf("%d candidates", len(e.Candidates))
}

func (e *AmbiguousMatchError) bestVisualScore() (float64, bool) {
	var best float64
	found := false
	for _, candidate := range e.Candidates {
		if candidate.HasVisualScore && (!found || candidate.VisualScore > best) {
			best = candidate.VisualScore
			found = true
		}
	}
	return best, found
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

type forceRefreshContextKey struct{}

func WithForceRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceRefreshContextKey{}, true)
}

func ForceRefresh(ctx context.Context) bool {
	return forceRefresh(ctx)
}

func forceRefresh(ctx context.Context) bool {
	forced, _ := ctx.Value(forceRefreshContextKey{}).(bool)
	return forced
}

type Service struct {
	http     *http.Client
	cacheDir string

	mu       sync.Mutex
	limiters map[string]*hostLimiter
	group    singleflight.Group
}

type hostLimiter struct {
	sem chan struct{}
}

func newHostLimiter(max int) *hostLimiter {
	return &hostLimiter{sem: make(chan struct{}, max)}
}

const (
	impMaxConcurrent          = 4
	wikipediaAPIMaxConcurrent = 2
	wikiImageMaxConcurrent    = 4
	defaultMaxConcurrent      = 4
)

func hostMaxConcurrent(host string) int {
	switch host {
	case "en.wikipedia.org":
		return wikipediaAPIMaxConcurrent
	case "upload.wikimedia.org", "commons.wikimedia.org":
		return wikiImageMaxConcurrent
	case "www.impawards.com", "impawards.com":
		return impMaxConcurrent
	default:
		return defaultMaxConcurrent
	}
}

func NewService() *Service {
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Service{
		http:     &http.Client{Timeout: 30 * time.Second, Transport: transport},
		cacheDir: defaultCacheDir(),
	}
}

func (s *Service) FindTheatricalPoster(ctx context.Context, movie plex.Movie) (Candidate, error) {
	return s.ResolveTheatricalPoster(ctx, movie, true)
}

func (s *Service) ResolveTheatricalPoster(ctx context.Context, movie plex.Movie, includeBytes bool) (Candidate, error) {
	type wikiResult struct {
		poster wikiPoster
		err    error
	}
	type impResult struct {
		candidates []impCandidate
		err        error
	}
	wikiCh := make(chan wikiResult, 1)
	impCh := make(chan impResult, 1)
	go func() {
		wiki, err := s.wikipediaPoster(ctx, movie)
		wikiCh <- wikiResult{poster: wiki, err: err}
	}()
	go func() {
		candidates, err := s.impCandidates(ctx, movie)
		impCh <- impResult{candidates: candidates, err: err}
	}()
	wikiRes := <-wikiCh
	impRes := <-impCh
	wiki, wikiErr := wikiRes.poster, wikiRes.err
	candidates, err := impRes.candidates, impRes.err
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
		chosen, visualData, reason, err := s.chooseVisualCandidate(ctx, movie, candidates, wiki)
		if err == nil {
			var data []byte
			if includeBytes {
				if visualIMPImageURL(chosen.ImageURL) == chosen.ImageURL {
					data = visualData
				} else {
					data, err = s.downloadIMPImage(ctx, chosen.ImageURL)
					if err != nil {
						return Candidate{}, err
					}
				}
			}
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
	var data []byte
	if includeBytes {
		data, err = s.downloadIMPImage(ctx, chosen.ImageURL)
		if err != nil {
			return Candidate{}, err
		}
	}
	return Candidate{Movie: movie, ImageURL: chosen.ImageURL, SourceURL: chosen.PageURL, MatchReason: reason, Bytes: data}, nil
}
