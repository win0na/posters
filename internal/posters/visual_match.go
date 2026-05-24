package posters

import (
	"context"
	"fmt"

	"github.com/win0na/posters/internal/plex"
)

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
	matches := s.compareVisualCandidates(ctx, orderedVisualCandidates(movie, candidates, wiki), wiki, wikiFPs)
	best, second, ok := bestVisualMatch(matches)
	if !ok {
		return impCandidate{}, nil, "", fmt.Errorf("no IMP candidate image could be visually compared")
	}
	if !isConfidentVisualMatch(best, second) {
		return impCandidate{}, nil, "", &AmbiguousMatchError{Movie: movie, Candidates: summarizeMatchedCandidates(matches)}
	}
	reason := visualMatchReason(best.Score)
	if second != nil {
		reason = fmt.Sprintf("%s; next best %s", visualMatchReason(best.Score), visualScorePercent(second.Score))
	}
	return best.Candidate, best.Bytes, reason, nil
}
