package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m = m.cancelOp()
		return m, tea.Quit
	case "esc":
		if m.screen == screenMovies {
			m.screen = screenMode
			m.cursor = 0
		} else if m.screen == screenLibraries {
			m.screen = screenServers
			m.cursor = 0
		} else if m.screen == screenBlacklist {
			m.screen = m.prev
			m.cursor = 0
		} else if m.screen == screenAuthWait {
			m = m.cancelOp()
			m.screen, m.authURL = screenLogin, ""
		} else if m.screen == screenRunning {
			m = m.finishRun(true)
			m.log = append(m.log, "cancelled")
			m.screen = screenDone
		}
		return m, nil
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
		return m, nil
	case "down", "j":
		m.cursor = min(m.cursor+1, max(0, m.cursorLimit()-1))
		return m, nil
	case " ":
		if m.screen == screenMovies && len(m.movies) > 0 {
			movie := m.movies[m.cursor%len(m.movies)]
			if movieBlacklisted(m.store, movie.RatingKey) {
				return m, nil
			}
			m.chosen[movie.RatingKey] = !m.chosen[movie.RatingKey]
		}
		return m, nil
	case "b":
		if m.screen == screenMovies && len(m.movies) > 0 {
			movie := m.movies[m.cursor%len(m.movies)]
			return m.toggleBlacklist(movie)
		}
		if m.screen == screenBlacklist {
			items := blacklistItems(m.store)
			if len(items) == 0 {
				return m, nil
			}
			item := items[m.cursor%len(items)]
			if err := m.store.UnblacklistMovie(item.RatingKey); err != nil {
				return m.fail(err)
			}
			m.notice = fmt.Sprintf("Removed %s (%d) from blacklist.", item.Title, item.Year)
			m.cursor = min(m.cursor, max(0, len(items)-2))
			return m, nil
		}
		if m.screen == screenMode || m.screen == screenStatus {
			m.prev = m.screen
			m.screen = screenBlacklist
			m.cursor = 0
		}
		return m, nil
	case "f":
		if m.screen == screenMode || m.screen == screenMovies {
			m.force = !m.force
		}
		return m, nil
	case "d":
		if m.screen == screenMode || m.screen == screenMovies {
			m.dryRun = !m.dryRun
		}
		return m, nil
	case "w":
		if m.screen == screenMode || m.screen == screenMovies {
			m.wikiFallback = !m.wikiFallback
		}
		return m, nil
	case "r":
		if m.screen == screenError || m.screen == screenLogin {
			return m.reauthenticate("Plex login cleared. Press Enter to log in again.")
		}
		return m, nil
	case "s":
		if m.screen == screenStatus {
			m.screen = m.prev
			return m, nil
		}
		m.prev = m.screen
		m.screen = screenStatus
		return m, nil
	case "enter":
		switch m.screen {
		case screenLogin:
			m.notice = ""
			var ctx context.Context
			var opID int
			m, ctx, opID = m.startOp()
			m.screen = screenAuthWait
			if hasSavedToken(m.store) {
				return m, tea.Batch(m.spinner.Tick, loadServers(ctx, opID, m.plex))
			}
			return m, tea.Batch(m.spinner.Tick, startPIN(ctx, opID, m.plex))
		case screenAuthWait:
			if m.ctx == nil {
				return m, nil
			}
			return m, pollPIN(m.ctx, m.opID, m.plex, m.pin.ID)
		case screenServers:
			if len(m.servers) == 0 {
				return m.fail(fmt.Errorf("no Plex servers found"))
			}
			m.server = m.servers[m.cursor%len(m.servers)]
			var ctx context.Context
			var opID int
			m, ctx, opID = m.startOp()
			m.authURL, m.screen = "", screenAuthWait
			return m, tea.Batch(m.spinner.Tick, loadLibraries(ctx, opID, m.plex, m.server))
		case screenLibraries:
			if len(m.libs) == 0 {
				return m.fail(fmt.Errorf("no movie libraries found"))
			}
			m.library = m.libs[m.cursor%len(m.libs)]
			if err := m.store.SaveLastSelection(m.server.ClientID, m.server.Name, m.server.URI, m.library.Key, m.library.Title); err != nil {
				return m.fail(err)
			}
			var ctx context.Context
			var opID int
			m, ctx, opID = m.startOp()
			m.authURL, m.screen = "", screenAuthWait
			return m, tea.Batch(m.spinner.Tick, loadMovies(ctx, opID, m.plex, m.server, m.library))
		case screenMode:
			if m.cursor%2 == 0 {
				m.mode = modeAll
				return m.startRun()
			}
			m.mode, m.screen, m.cursor = modeSpecific, screenMovies, 0
			return m, nil
		case screenMovies:
			return m.startRun()
		case screenStatus:
			m.screen = m.prev
			return m, nil
		case screenBlacklist:
			m.screen = m.prev
			return m, nil
		case screenDone, screenError:
			return m, tea.Quit
		}
	}
	return m, nil
}
