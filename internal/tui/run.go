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

func (m Model) startRun() (tea.Model, tea.Cmd) {
	selected := m.selectedMovies()
	pending, skippedUpdated, skippedBlacklisted, err := m.pendingMovies(selected)
	if err != nil {
		return m.fail(err)
	}
	m.runningQueue = pending
	m.runningTotal = len(pending)
	m.runningDone = 0
	m.runningNext = 0
	m.runningActive = 0
	m.runningCurrent = nil
	m.progressCh = nil
	m.runStats = runStats{Skipped: skippedUpdated, Blacklisted: skippedBlacklisted}
	m.reportItems = skippedReportItems(selected, m.store)
	m.reportPath = ""
	m.reportCSVPath = ""
	m.log = nil
	m.details = nil
	m.cursor = 0
	m.screen = screenRunning
	if len(pending) == 0 {
		if skippedUpdated > 0 && skippedBlacklisted > 0 {
			m.log = []string{fmt.Sprintf("all selected movies skipped (%d already updated, %d blacklisted)", skippedUpdated, skippedBlacklisted)}
		} else if skippedUpdated > 0 {
			m.log = []string{fmt.Sprintf("all selected movies already updated locally (%d skipped)", skippedUpdated)}
		} else if skippedBlacklisted > 0 {
			m.log = []string{fmt.Sprintf("all selected movies blacklisted (%d skipped)", skippedBlacklisted)}
		} else {
			m.log = []string{"no movies selected"}
		}
		m = m.finishRun(false)
		m.screen = screenDone
		return m, nil
	}
	var ctx context.Context
	var opID int
	m, ctx, opID = m.startOp()
	m.progressCh = make(chan updatePhaseMsg, posterUpdateConcurrency*4)
	cmds := []tea.Cmd{m.spinner.Tick, m.bar.SetPercent(0), waitUpdatePhase(ctx, opID, m.progressCh)}
	m, cmds = m.launchUpdateBatch(ctx, opID, cmds)
	return m, tea.Batch(cmds...)
}

func (m Model) launchUpdateBatch(ctx context.Context, opID int, cmds []tea.Cmd) (Model, []tea.Cmd) {
	limit := min(posterUpdateConcurrency, m.runningTotal)
	for m.runningActive < limit && m.runningNext < m.runningTotal {
		movie := m.runningQueue[m.runningNext]
		m.runningNext++
		m.runningActive++
		m.runningCurrent = append(m.runningCurrent, runningMovie{Movie: movie, Phase: "Matching"})
		cmds = append(cmds, updateMovie(ctx, opID, m.store, m.plex, m.finder, m.server, movie, m.force, m.dryRun, m.wikiFallback, m.progressCh))
	}
	return m, cmds
}

func removeRunningCurrent(current []runningMovie, done plex.Movie) []runningMovie {
	out := current[:0]
	removed := false
	for _, entry := range current {
		if !removed && sameMovie(entry.Movie, done) {
			removed = true
			continue
		}
		out = append(out, entry)
	}
	return out
}

func updateRunningPhase(current []runningMovie, movie plex.Movie, phase string) []runningMovie {
	phase = titlePhase(phase)
	for i := range current {
		if sameMovie(current[i].Movie, movie) {
			current[i].Phase = phase
			break
		}
	}
	return current
}

func sameMovie(a, b plex.Movie) bool {
	if a.RatingKey != "" || b.RatingKey != "" {
		return a.RatingKey == b.RatingKey
	}
	return a.Title == b.Title && a.Year == b.Year
}

func (m Model) toggleBlacklist(movie plex.Movie) (tea.Model, tea.Cmd) {
	blacklisted, err := m.store.MovieBlacklisted(movie.RatingKey)
	if err != nil {
		return m.fail(err)
	}
	if blacklisted {
		if err := m.store.UnblacklistMovie(movie.RatingKey); err != nil {
			return m.fail(err)
		}
		m.notice = fmt.Sprintf("Removed %s (%d) from blacklist.", movie.Title, movie.Year)
		return m, nil
	}
	if err := m.store.BlacklistMovie(config.BlacklistItem{RatingKey: movie.RatingKey, Title: movie.Title, Year: movie.Year}); err != nil {
		return m.fail(err)
	}
	delete(m.chosen, movie.RatingKey)
	m.notice = fmt.Sprintf("Blacklisted %s (%d).", movie.Title, movie.Year)
	return m, nil
}

func movieBlacklisted(store *config.Store, ratingKey string) bool {
	blacklisted, err := store.MovieBlacklisted(ratingKey)
	return err == nil && blacklisted
}

func (m Model) pendingMovies(selected []plex.Movie) ([]plex.Movie, int, int, error) {
	metadata, err := m.store.LoadMetadata()
	if err != nil {
		return nil, 0, 0, err
	}
	pending := make([]plex.Movie, 0, len(selected))
	skippedUpdated := 0
	skippedBlacklisted := 0
	for _, movie := range selected {
		if _, ok := metadata.Blacklist[movie.RatingKey]; ok {
			skippedBlacklisted++
			continue
		}
		if m.force {
			pending = append(pending, movie)
			continue
		}
		if _, ok := metadata.Items[movie.RatingKey]; ok {
			skippedUpdated++
			continue
		}
		pending = append(pending, movie)
	}
	return pending, skippedUpdated, skippedBlacklisted, nil
}

func (m Model) selectedMovies() []plex.Movie {
	if m.mode == modeAll {
		return m.movies
	}
	selected := []plex.Movie{}
	for _, movie := range m.movies {
		if m.chosen[movie.RatingKey] {
			selected = append(selected, movie)
		}
	}
	return selected
}

func (m Model) blacklistedRatingKeys() map[string]bool {
	if m.store == nil {
		return nil
	}
	metadata, err := m.store.LoadMetadata()
	if err != nil {
		return nil
	}
	blacklist := map[string]bool{}
	for key := range metadata.Blacklist {
		blacklist[key] = true
	}
	return blacklist
}

func skippedReportItems(selected []plex.Movie, store *config.Store) []config.ReportItem {
	metadata, err := store.LoadMetadata()
	if err != nil {
		return nil
	}
	items := []config.ReportItem{}
	for _, movie := range selected {
		if _, ok := metadata.Blacklist[movie.RatingKey]; ok {
			items = append(items, config.ReportItem{RatingKey: movie.RatingKey, Title: movie.Title, Year: movie.Year, Status: "blacklisted", Message: "blacklisted"})
		}
	}
	return items}

func (m *Model) recordUpdateResult(msg updateOneMsg) {
	item := config.ReportItem{RatingKey: msg.movie.RatingKey, Title: msg.movie.Title, Year: msg.movie.Year, Message: msg.line, SourceURL: msg.sourceURL, ImageURL: msg.imageURL, MatchReason: msg.matchReason}
	if msg.err == nil {
		if strings.HasPrefix(msg.line, "wiki-fallback ") {
			m.runStats.WikiFallback++
			item.Status = "wiki-fallback"
			m.reportItems = append(m.reportItems, item)
			return
		}
		if strings.HasPrefix(msg.line, "dry-run ") {
			m.runStats.DryRun++
			item.Status = "dry-run"
			m.reportItems = append(m.reportItems, item)
			return
		}
		m.runStats.Updated++
		item.Status = "updated"
		m.reportItems = append(m.reportItems, item)
		return
	}
	item.Error = msg.err.Error()
	var ambiguous *posterfinder.AmbiguousMatchError
	if errors.As(msg.err, &ambiguous) {
		m.runStats.Ambiguous++
		item.Status = "ambiguous"
		item.Message = ambiguous.Summary()
		item.Error = ""
		item.Candidates = make([]config.CandidateInfo, len(ambiguous.Candidates))
		for i, c := range ambiguous.Candidates {
			item.Candidates[i] = config.CandidateInfo{
				PageURL:        c.PageURL,
				ImageURL:       c.ImageURL,
				Version:        c.Version,
				Canonical:      c.Canonical,
				VisualScore:    c.VisualScore,
				HasVisualScore: c.HasVisualScore,
			}
		}
		m.reportItems = append(m.reportItems, item)
		m.details = append(m.details, formatAmbiguousDetails(msg.movie, ambiguous)...)
		return
	}
	if isNoIMPPosterError(msg.err) {
		m.runStats.Skipped++
		item.Status = "skipped"
		item.Message = "no IMP poster available"
		item.Error = ""
		m.reportItems = append(m.reportItems, item)
		return
	}
	m.runStats.Failed++
	item.Status = "failed"
	if item.Message == "" {
		item.Message = formatUpdateError(msg.movie, msg.err)
	}
	m.reportItems = append(m.reportItems, item)
}

func (m Model) finishRun(cancelled bool) Model {
	m = m.cancelOp()
	m.cursor = 0
	m.runStats.Cancelled = m.runStats.Cancelled || cancelled
	jsonPath, csvPath, err := m.store.SaveRunReport(config.RunReport{Stats: reportStats(m.runStats), Items: m.reportItems})
	if err != nil {
		m.log = append(m.log, "report failed: "+err.Error())
		return m
	}
	m.reportPath = jsonPath
	m.reportCSVPath = csvPath
	m.log = append(m.log, "report: "+jsonPath)
	return m
}

func reportStats(stats runStats) config.ReportStats {
	return config.ReportStats{Updated: stats.Updated, DryRun: stats.DryRun, WikiFallback: stats.WikiFallback, Skipped: stats.Skipped, Blacklisted: stats.Blacklisted, Ambiguous: stats.Ambiguous, Failed: stats.Failed, Cancelled: stats.Cancelled}
}

func (m Model) fail(err error) (tea.Model, tea.Cmd) {
	if plex.IsUnauthorized(err) {
		return m.reauthenticate("Plex session expired or was rejected. Press Enter to log in again.")
	}
	m = m.cancelOp()
	m.err, m.screen = err, screenError
	return m, nil
}

func (m Model) reauthenticate(notice string) (tea.Model, tea.Cmd) {
	m = m.cancelOp()
	if err := m.store.ClearPlexToken(); err != nil {
		m.err, m.screen = fmt.Errorf("clear Plex token: %w", err), screenError
		return m, nil
	}
	m.pin = plex.Pin{}
	m.authURL = ""
	m.servers = nil
	m.libs = nil
	m.movies = nil
	m.server = plex.Server{}
	m.library = plex.Library{}
	m.cursor = 0
	m.notice = notice
	m.screen = screenLogin
	return m, nil
}

func (m Model) startOp() (Model, context.Context, int) {
	m = m.cancelOp()
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx, m.cancel = ctx, cancel
	m.opID++
	return m, ctx, m.opID
}

func (m Model) cancelOp() Model {
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = nil, nil
	m.opID++
	return m
}

func (m Model) isActive(opID int) bool {
	return m.ctx != nil && opID == m.opID
}
