package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
	posterfinder "github.com/win0na/posters/internal/posters"
)

func loadMovies(ctx context.Context, opID int, client Plex, server plex.Server, library plex.Library) tea.Cmd {
	return func() tea.Msg {
		movies, err := client.ListMovies(ctx, server, library)
		return moviesMsg{opID: opID, movies: movies, err: err}
	}
}

func formatUpdateError(movie plex.Movie, err error) string {
	var ambiguous *posterfinder.AmbiguousMatchError
	if errors.As(err, &ambiguous) {
		return fmt.Sprintf("ambiguous %s (%d): %s", movie.Title, movie.Year, ambiguous.Summary())
	}
	if isNoIMPPosterError(err) {
		return fmt.Sprintf("skip %s (%d): no IMP poster available", movie.Title, movie.Year)
	}
	return fmt.Sprintf("skip %s (%d): %v", movie.Title, movie.Year, err)
}

func isNoIMPPosterError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no IMP Awards poster found") || strings.Contains(msg, "no IMP candidate image could be visually compared") || strings.Contains(msg, "no poster candidates")
}

func formatAmbiguousDetails(movie plex.Movie, err *posterfinder.AmbiguousMatchError) []string {
	lines := []string{fmt.Sprintf("%s (%d):", movie.Title, movie.Year)}
	for i, candidate := range err.Candidates {
		if i >= 4 {
			lines = append(lines, fmt.Sprintf("  +%d more", len(err.Candidates)-i))
			break
		}
		bits := []string{candidate.PageURL}
		if candidate.Version > 0 {
			bits = append(bits, fmt.Sprintf("ver%d", candidate.Version))
		}
		if candidate.Canonical {
			bits = append(bits, "canonical")
		}
		if candidate.HasVisualScore {
			bits = append(bits, fmt.Sprintf("visual match %.1f%%", candidate.VisualScore*100))
		}
		lines = append(lines, "  - "+strings.Join(bits, " · "))
	}
	return lines
}

func waitUpdatePhase(ctx context.Context, opID int, ch <-chan updatePhaseMsg) tea.Cmd {
	return func() tea.Msg {
		if ctx == nil || ch == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case msg := <-ch:
			msg.opID = opID
			return msg
		}
	}
}

func sendUpdatePhase(ctx context.Context, ch chan<- updatePhaseMsg, opID int, movie plex.Movie, phase string) {
	if ch == nil {
		return
	}
	select {
	case ch <- updatePhaseMsg{opID: opID, movie: movie, phase: phase}:
	case <-ctx.Done():
	}
}

func updateMovie(ctx context.Context, opID int, store *config.Store, client Plex, finder PosterFinder, server plex.Server, movie plex.Movie, force bool, dryRun bool, wikiFallback bool, progressCh chan<- updatePhaseMsg) tea.Cmd {
	return func() tea.Msg {
		if force {
			ctx = posterfinder.WithForceRefresh(ctx)
		}
		sendUpdatePhase(ctx, progressCh, opID, movie, "Matching")
		candidate, err := findTheatricalPoster(ctx, finder, movie, !dryRun)
		if err != nil {
			var ambiguous *posterfinder.AmbiguousMatchError
			if wikiFallback && (errors.As(err, &ambiguous) || isNoIMPPosterError(err)) {
				sendUpdatePhase(ctx, progressCh, opID, movie, "Fallback")
				fallback, fallbackErr := finder.FindWikipediaPoster(ctx, movie)
				if fallbackErr != nil {
					return updateOneMsg{opID: opID, movie: movie, err: fallbackErr}
				}
				if dryRun {
					sendUpdatePhase(ctx, progressCh, opID, movie, "Reporting")
					return updateOneMsg{opID: opID, movie: movie, line: wikiFallbackResultLine(movie, fallback), sourceURL: fallback.SourceURL, imageURL: fallback.ImageURL, matchReason: fallback.MatchReason}
				}
				sendUpdatePhase(ctx, progressCh, opID, movie, "Uploading")
				if err := client.UploadPoster(ctx, server, movie, "poster.jpg", fallback.Bytes, fallback.ImageURL); err != nil {
					return updateOneMsg{opID: opID, movie: movie, err: err}
				}
				sendUpdatePhase(ctx, progressCh, opID, movie, "Saving")
				if err := store.MarkPosterUpdated(config.PosterItem{RatingKey: movie.RatingKey, Title: movie.Title, Year: movie.Year, SourceURL: fallback.SourceURL}); err != nil {
					return updateOneMsg{opID: opID, movie: movie, err: err}
				}
				return updateOneMsg{opID: opID, movie: movie, line: wikiFallbackResultLine(movie, fallback), sourceURL: fallback.SourceURL, imageURL: fallback.ImageURL, matchReason: fallback.MatchReason}
			}
			return updateOneMsg{opID: opID, movie: movie, err: err}
		}
		if dryRun {
			sendUpdatePhase(ctx, progressCh, opID, movie, "Reporting")
			return updateOneMsg{opID: opID, movie: movie, line: dryRunResultLine(movie, candidate), sourceURL: candidate.SourceURL, imageURL: candidate.ImageURL, matchReason: candidate.MatchReason}
		}
		sendUpdatePhase(ctx, progressCh, opID, movie, "Uploading")
		if err := client.UploadPoster(ctx, server, movie, "poster.jpg", candidate.Bytes, candidate.ImageURL); err != nil {
			return updateOneMsg{opID: opID, movie: movie, err: err}
		}
		sendUpdatePhase(ctx, progressCh, opID, movie, "Saving")
		if err := store.MarkPosterUpdated(config.PosterItem{RatingKey: movie.RatingKey, Title: movie.Title, Year: movie.Year, SourceURL: candidate.SourceURL}); err != nil {
			return updateOneMsg{opID: opID, movie: movie, err: err}
		}
		return updateOneMsg{opID: opID, movie: movie, line: updatedResultLine(movie, candidate), sourceURL: candidate.SourceURL, imageURL: candidate.ImageURL, matchReason: candidate.MatchReason}
	}
}

func findTheatricalPoster(ctx context.Context, finder PosterFinder, movie plex.Movie, includeBytes bool) (posterfinder.Candidate, error) {
	if resolver, ok := finder.(posterResolver); ok {
		return resolver.ResolveTheatricalPoster(ctx, movie, includeBytes)
	}
	return finder.FindTheatricalPoster(ctx, movie)
}

func updatedResultLine(movie plex.Movie, candidate posterfinder.Candidate) string {
	line := fmt.Sprintf("updated %s (%d)", movie.Title, movie.Year)
	if match := visualMatchSummary(candidate.MatchReason); match != "" {
		line += ", " + match
	}
	return line
}

func visualMatchSummary(reason string) string {
	idx := strings.Index(reason, "visual match ")
	if idx < 0 {
		return ""
	}
	rest := reason[idx:]
	if end := strings.Index(rest, ";"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest)
}

func wikiFallbackResultLine(movie plex.Movie, candidate posterfinder.Candidate) string {
	line := fmt.Sprintf("wiki-fallback %s (%d): %s", movie.Title, movie.Year, candidate.SourceURL)
	if candidate.ImageURL != "" && candidate.ImageURL != candidate.SourceURL {
		line += " | image: " + candidate.ImageURL
	}
	if candidate.MatchReason != "" {
		line += " | reason: " + candidate.MatchReason
	}
	return line
}

func dryRunResultLine(movie plex.Movie, candidate posterfinder.Candidate) string {
	line := fmt.Sprintf("dry-run %s (%d): %s", movie.Title, movie.Year, candidate.SourceURL)
	if candidate.ImageURL != "" {
		line += " | image: " + candidate.ImageURL
	}
	if candidate.MatchReason != "" {
		line += " | reason: " + candidate.MatchReason
	}
	return line
}
