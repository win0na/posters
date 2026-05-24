package posters

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/win0na/posters/internal/plex"
)

func orderedVisualCandidates(movie plex.Movie, candidates []impCandidate, wiki wikiPoster) []impCandidate {
	ordered := append([]impCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return visualCandidatePriority(movie, ordered[i], wiki) > visualCandidatePriority(movie, ordered[j], wiki)
	})
	return ordered
}

func visualCandidatePriority(movie plex.Movie, candidate impCandidate, wiki wikiPoster) int {
	score := 0
	if samePosterImageName(wiki.ImageURL, candidate.ImageURL) {
		score += 1000
	}
	score += int(tokenOverlapScore(descriptiveTokens(wiki.ImageURL+" "+wiki.Alt+" "+wiki.Caption, movie), descriptiveTokens(candidate.PageURL+" "+candidate.ImageURL, movie)) * 100)
	score += candidatePreferenceRank(candidate) * 100
	if candidate.Year == movie.Year {
		score += 25
	}
	return score
}

func (s *Service) compareVisualCandidates(ctx context.Context, candidates []impCandidate, wiki wikiPoster, wikiFPs []visualFingerprint) []matchedCandidate {
	matches := make([]matchedCandidate, len(candidates))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(visualFetchConcurrency, len(candidates))
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				matches[i] = s.compareVisualCandidate(ctx, candidates[i], wiki, wikiFPs)
			}
		}()
	}
	for i := range candidates {
		select {
		case <-ctx.Done():
			matches[i] = matchedCandidate{Candidate: candidates[i], Err: ctx.Err()}
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	return matches
}

func (s *Service) compareVisualCandidate(ctx context.Context, candidate impCandidate, wiki wikiPoster, wikiFPs []visualFingerprint) matchedCandidate {
	visualURL := visualIMPImageURL(candidate.ImageURL)
	data, err := s.downloadIMPImage(ctx, visualURL)
	if err != nil && visualURL != candidate.ImageURL {
		data, err = s.downloadIMPImage(ctx, candidate.ImageURL)
	}
	match := matchedCandidate{Candidate: candidate, Bytes: data, Err: err}
	if err != nil {
		return match
	}
	impFPs, fpErr := imageFingerprints(data)
	if fpErr != nil {
		match.Err = fpErr
		return match
	}
	match.Score = maxVisualSimilarity(wikiFPs, impFPs)
	if visualURL != candidate.ImageURL && match.Score < clearVisualMatchScore {
		fullData, fullErr := s.downloadIMPImage(ctx, candidate.ImageURL)
		if fullErr == nil {
			fullFPs, fullFPErr := imageFingerprints(fullData)
			if fullFPErr == nil {
				fullScore := maxVisualSimilarity(wikiFPs, fullFPs)
				if fullScore > match.Score {
					match.Bytes = fullData
					match.Score = fullScore
				}
			}
		}
	}
	match.Reason = visualMatchReason(match.Score)
	match.NameHint = samePosterImageName(wiki.ImageURL, candidate.ImageURL)
	return match
}

func visualIMPImageURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	ext := path.Ext(u.Path)
	base := strings.TrimSuffix(u.Path, ext)
	for _, suffix := range []string{"_xxlg", "_xlg"} {
		if strings.HasSuffix(base, suffix) {
			u.Path = strings.TrimSuffix(base, suffix) + ext
			u.RawQuery = ""
			u.Fragment = ""
			return u.String()
		}
	}
	return rawURL
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

func summarizeMatchedCandidates(matches []matchedCandidate) []CandidateSummary {
	sorted := append([]matchedCandidate(nil), matches...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if left.Err == nil && right.Err != nil {
			return true
		}
		if left.Err != nil && right.Err == nil {
			return false
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		return candidatePreferenceRank(left.Candidate) > candidatePreferenceRank(right.Candidate)
	})
	summary := make([]CandidateSummary, 0, len(sorted))
	for _, match := range sorted {
		candidate := CandidateSummary{PageURL: match.Candidate.PageURL, ImageURL: match.Candidate.ImageURL, Version: match.Candidate.Version, Canonical: match.Candidate.Canonical}
		if match.Err == nil && len(match.Bytes) > 0 {
			candidate.VisualScore = match.Score
			candidate.HasVisualScore = true
		}
		summary = append(summary, candidate)
	}
	return summary
}
