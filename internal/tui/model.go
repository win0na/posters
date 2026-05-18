package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
	posterfinder "github.com/win0na/posters/internal/posters"
)

var urlRE = regexp.MustCompile(`https?://[^\s|]+`)

const horizontalContentPadding = 10
const lipglossHorizontalPadding = horizontalContentPadding - 2
const verticalContentPadding = 2

const (
	panelGap           = 2
	dashboardWideWidth = 72
	panelHorizontalPad = 1
	panelVerticalPad   = 0
	accentCyan         = lipgloss.Color("45")
	accentMagenta      = lipgloss.Color("201")
	accentGreen        = lipgloss.Color("83")
	accentAmber        = lipgloss.Color("214")
	accentBlue         = lipgloss.Color("39")
	accentRed          = lipgloss.Color("203")
	frameBorderColor   = lipgloss.Color("63")
	frameTextColor     = lipgloss.Color("255")
	frameMutedColor    = lipgloss.Color("245")
	frameDimColor      = lipgloss.Color("240")
	framePanelBg       = lipgloss.Color("236")
	framePanelBgAlt    = lipgloss.Color("235")
)

type uiTheme struct {
	title       lipgloss.Style
	subtitle    lipgloss.Style
	frame       lipgloss.Style
	frameTitle  lipgloss.Style
	panel       lipgloss.Style
	panelTitle  lipgloss.Style
	panelValue  lipgloss.Style
	muted       lipgloss.Style
	dim         lipgloss.Style
	good        lipgloss.Style
	warn        lipgloss.Style
	bad         lipgloss.Style
	accent      lipgloss.Style
	accent2     lipgloss.Style
	help        lipgloss.Style
	chip        lipgloss.Style
	selected    lipgloss.Style
	divider     lipgloss.Style
	headerLabel lipgloss.Style
	footer      lipgloss.Style
	code        lipgloss.Style
}

var ui = newUITheme()

func newUITheme() uiTheme {
	base := lipgloss.NewStyle().Foreground(frameTextColor)
	muted := lipgloss.NewStyle().Foreground(frameMutedColor)
	dim := lipgloss.NewStyle().Foreground(frameDimColor)
	return uiTheme{
		title:       base.Bold(true).Foreground(accentMagenta),
		subtitle:    muted,
		frame:       lipgloss.NewStyle().Padding(verticalContentPadding, lipglossHorizontalPadding).Width(0).AlignVertical(lipgloss.Center).Border(lipgloss.RoundedBorder()).BorderForeground(frameBorderColor).Foreground(frameTextColor),
		frameTitle:  base.Bold(true).Foreground(accentCyan),
		panel:       lipgloss.NewStyle().Padding(panelVerticalPad, panelHorizontalPad).Border(lipgloss.RoundedBorder()).BorderForeground(frameBorderColor).Foreground(frameTextColor).Background(framePanelBg),
		panelTitle:  base.Bold(true).Foreground(accentCyan),
		panelValue:  base,
		muted:       muted,
		dim:         dim,
		good:        base.Bold(true).Foreground(accentGreen),
		warn:        base.Bold(true).Foreground(accentAmber),
		bad:         base.Bold(true).Foreground(accentRed),
		accent:      base.Bold(true).Foreground(accentCyan),
		accent2:     base.Bold(true).Foreground(accentMagenta),
		help:        dim,
		chip:        base.Bold(true).Foreground(accentBlue).Background(framePanelBgAlt),
		selected:    base.Bold(true).Foreground(accentAmber),
		divider:     dim,
		headerLabel: base.Bold(true).Foreground(accentBlue),
		footer:      muted,
		code:        base.Bold(true).Foreground(accentAmber),
	}
}

type screen int

const (
	screenLogin screen = iota
	screenAuthWait
	screenServers
	screenLibraries
	screenMode
	screenMovies
	screenRunning
	screenStatus
	screenDone
	screenError
)

type mode int

const (
	modeAll mode = iota
	modeSpecific
)

type runStats struct {
	Updated   int
	DryRun    int
	Skipped   int
	Ambiguous int
	Failed    int
	Cancelled bool
}

type Plex interface {
	StartPIN(context.Context) (plex.Pin, string, error)
	PollPIN(context.Context, int) (string, error)
	ListServers(context.Context) ([]plex.Server, error)
	ListLibraries(context.Context, plex.Server) ([]plex.Library, error)
	ListMovies(context.Context, plex.Server, plex.Library) ([]plex.Movie, error)
	UploadPoster(context.Context, plex.Server, plex.Movie, string, []byte, string) error
}

type PosterFinder interface {
	FindTheatricalPoster(context.Context, plex.Movie) (posterfinder.Candidate, error)
}

type Options struct {
	Force  bool
	DryRun bool
}

type Model struct {
	store   *config.Store
	plex    Plex
	finder  PosterFinder
	screen  screen
	prev    screen
	spinner spinner.Model
	bar     progress.Model
	width   int
	height  int

	pin     plex.Pin
	authURL string
	server  plex.Server
	servers []plex.Server
	library plex.Library
	libs    []plex.Library
	movies  []plex.Movie
	mode    mode
	force   bool
	dryRun  bool
	cursor  int
	chosen  map[string]bool

	runningTotal  int
	runningDone   int
	runningQueue  []plex.Movie
	runStats      runStats
	reportItems   []config.ReportItem
	reportPath    string
	reportCSVPath string
	log           []string
	details       []string
	notice        string
	err           error
	selection     selectionState

	ctx    context.Context
	cancel context.CancelFunc
	opID   int
}

type pinStartedMsg struct {
	opID int
	pin  plex.Pin
	url  string
	err  error
}
type authPollMsg struct {
	opID  int
	token string
	err   error
}
type serversMsg struct {
	opID    int
	servers []plex.Server
	err     error
}
type librariesMsg struct {
	opID int
	libs []plex.Library
	err  error
}
type moviesMsg struct {
	opID   int
	movies []plex.Movie
	err    error
}
type updateOneMsg struct {
	opID        int
	movie       plex.Movie
	line        string
	sourceURL   string
	imageURL    string
	matchReason string
	err         error
}
type doneMsg struct{}
type selectionCopiedMsg struct{ err error }

func New(store *config.Store, client Plex) Model {
	return NewWithOptions(store, client, Options{})
}

func NewWithOptions(store *config.Store, client Plex, options Options) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	initialScreen := screenLogin
	var ctx context.Context
	var cancel context.CancelFunc
	opID := 0
	if hasSavedToken(store) {
		initialScreen = screenAuthWait
		ctx, cancel = context.WithCancel(context.Background())
		opID = 1
	}
	return Model{
		store:   store,
		plex:    client,
		finder:  posterfinder.NewService(),
		screen:  initialScreen,
		spinner: sp,
		bar:     progress.New(progress.WithDefaultGradient()),
		chosen:  map[string]bool{},
		force:   options.Force,
		dryRun:  options.DryRun,
		ctx:     ctx,
		cancel:  cancel,
		opID:    opID,
	}
}

func (m Model) Init() tea.Cmd {
	if hasSavedToken(m.store) && m.ctx != nil {
		return tea.Batch(m.spinner.Tick, loadServers(m.ctx, m.opID, m.plex))
	}
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.bar.Width = progressBarWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		m.selection = selectionState{}
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		progressModel, cmd := m.bar.Update(msg)
		m.bar = progressModel.(progress.Model)
		return m, cmd
	case selectionCopiedMsg:
		if msg.err != nil {
			m.notice = "Selection copy failed: " + msg.err.Error()
		}
		return m, nil
	case pinStartedMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.pin, m.authURL, m.screen = msg.pin, msg.url, screenAuthWait
		return m, tea.Batch(m.spinner.Tick, pollPIN(m.ctx, msg.opID, m.plex, msg.pin.ID))
	case authPollMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		if msg.err != nil {
			return m.fail(msg.err)
		}
		if msg.token == "" {
			return m, tea.Batch(m.spinner.Tick, waitAndPollPIN(m.ctx, msg.opID, m.plex, m.pin.ID))
		}
		m.authURL = ""
		return m, loadServers(m.ctx, msg.opID, m.plex)
	case serversMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		if msg.err != nil {
			return m.fail(msg.err)
		}
		if len(msg.servers) == 0 {
			return m.fail(fmt.Errorf("no Plex servers found"))
		}
		m.servers, m.cursor, m.screen = msg.servers, preferredServerCursor(m.store, msg.servers), screenServers
		return m, nil
	case librariesMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.libs = msg.libs
		m.cursor, m.screen = preferredLibraryCursor(m.store, msg.libs), screenLibraries
		return m, nil
	case moviesMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.movies, m.cursor, m.screen = msg.movies, 0, screenMode
		return m, nil
	case updateOneMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		m.runningDone++
		m.recordUpdateResult(msg)
		if msg.err != nil {
			m.log = append(m.log, formatUpdateError(msg.movie, msg.err))
		} else {
			m.log = append(m.log, msg.line)
		}
		m.cursor = max(0, m.cursorLimit()-1)
		if m.runningDone >= m.runningTotal {
			m = m.finishRun(false)
			m.screen = screenDone
			return m, nil
		}
		return m, tea.Batch(
			m.bar.SetPercent(float64(m.runningDone)/float64(max(1, m.runningTotal))),
			updateMovie(m.ctx, msg.opID, m.store, m.plex, m.finder, m.server, m.runningQueue[m.runningDone], m.dryRun),
		)
	case doneMsg:
		m.screen = screenDone
		return m, nil
	}
	return m, nil
}

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
			m.chosen[movie.RatingKey] = !m.chosen[movie.RatingKey]
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
		case screenDone, screenError:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) startRun() (tea.Model, tea.Cmd) {
	selected := m.selectedMovies()
	pending, skipped, err := m.pendingMovies(selected)
	if err != nil {
		return m.fail(err)
	}
	m.runningQueue = pending
	m.runningTotal = len(pending)
	m.runningDone = 0
	m.runStats = runStats{Skipped: len(skipped)}
	m.reportItems = nil
	m.reportPath = ""
	m.reportCSVPath = ""
	m.log = skipped
	m.details = nil
	m.cursor = 0
	m.screen = screenRunning
	if len(pending) == 0 {
		if len(m.log) == 0 {
			m.log = []string{"no movies selected"}
		}
		m = m.finishRun(false)
		m.screen = screenDone
		return m, nil
	}
	var ctx context.Context
	var opID int
	m, ctx, opID = m.startOp()
	return m, tea.Batch(m.spinner.Tick, m.bar.SetPercent(0), updateMovie(ctx, opID, m.store, m.plex, m.finder, m.server, pending[0], m.dryRun))
}

func (m Model) pendingMovies(selected []plex.Movie) ([]plex.Movie, []string, error) {
	metadata, err := m.store.LoadMetadata()
	if err != nil {
		return nil, nil, err
	}
	pending := make([]plex.Movie, 0, len(selected))
	skipped := []string{}
	for _, movie := range selected {
		if m.force {
			pending = append(pending, movie)
			continue
		}
		if _, ok := metadata.Items[movie.RatingKey]; ok {
			skipped = append(skipped, fmt.Sprintf("skip %s (%d): already updated", movie.Title, movie.Year))
			continue
		}
		pending = append(pending, movie)
	}
	return pending, skipped, nil
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

func (m *Model) recordUpdateResult(msg updateOneMsg) {
	item := config.ReportItem{RatingKey: msg.movie.RatingKey, Title: msg.movie.Title, Year: msg.movie.Year, Message: msg.line, SourceURL: msg.sourceURL, ImageURL: msg.imageURL, MatchReason: msg.matchReason}
	if msg.err == nil {
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
		m.reportItems = append(m.reportItems, item)
		m.details = append(m.details, formatAmbiguousDetails(msg.movie, ambiguous)...)
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
	return config.ReportStats{Updated: stats.Updated, DryRun: stats.DryRun, Skipped: stats.Skipped, Ambiguous: stats.Ambiguous, Failed: stats.Failed, Cancelled: stats.Cancelled}
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

func (m Model) View() string {
	view := m.baseView()
	if m.selection.active {
		return highlightRenderedSelection(view, m.selection.start, m.selection.end)
	}
	return view
}

func (m Model) baseView() string {
	return centerView(shellSized(m.body(), m.width, m.height), m.width, m.height)
}

func (m Model) body() string {
	body := ""
	switch m.screen {
	case screenLogin:
		body = "Update Plex movie posters to original theatrical posters."
		if m.notice != "" {
			body += "\n\n" + m.notice
		}
		body += "\n\nEnter: login to Plex\ns: status\nr: clear saved login\nq: quit"
	case screenAuthWait:
		if m.authURL != "" {
			body = fmt.Sprintf("%s Waiting for Plex login\n\nOpen:\n%s\n\nCode: %s\n\nEnter: poll now\nEsc: cancel", m.spinner.View(), m.authURL, m.pin.Code)
		} else {
			body = m.spinner.View() + " Loading...\n\nEsc: cancel"
		}
	case screenServers:
		body = "Choose Plex server:\n\n" + renderChoices(m.servers, m.cursor, func(s plex.Server) string { return serverLabel(s) }) + "\n\ns: status"
	case screenLibraries:
		body = "Choose movie library:\n\n" + renderChoices(m.libs, m.cursor, func(l plex.Library) string { return l.Title }) + "\n\nEsc: servers\ns: status"
	case screenMode:
		body = "Update mode:\n\n" + renderLines([]string{"All posters (default)", "Specific posters"}, m.cursor) + "\n\n" + optionLines(m.force, m.dryRun)
	case screenMovies:
		body = "Select movies (space toggles, enter starts, esc back):\n\n" + renderMovies(m.movies, m.cursor, m.chosen, movieListRows(m.height)) + "\n\n" + optionLines(m.force, m.dryRun)
	case screenStatus:
		body = m.statusView() + "\n\nEnter/s: back"
	case screenRunning:
		percent := float64(m.runningDone) / float64(max(1, m.runningTotal))
		body = m.runningView(percent)
	case screenDone:
		body = m.doneView(doneRows(m.height))
	case screenError:
		body = fmt.Sprintf("Error: %v\n\nr: clear saved login and reauthenticate\nEnter/q: quit", m.err)
	}
	return body
}

type dashboardPane struct {
	title    string
	subtitle string
	icon     string
	accent   lipgloss.Color
	body     string
}

func (m Model) loginBody() string {
	left := dashboardPane{
		title:    "Login",
		subtitle: "Plex auth · local config only",
		icon:     "◆",
		accent:   accentMagenta,
		body: strings.Join(filterEmptyStrings([]string{
			"Update Plex movie posters to original theatrical posters.",
			m.notice,
		}), "\n\n"),
	}
	right := dashboardPane{
		title:    "Flow",
		subtitle: "No API keys · no env vars",
		icon:     "↗",
		accent:   accentCyan,
		body: strings.Join([]string{
			"1. Log in with Plex.",
			"2. Pick server and library.",
			"3. Choose all posters or specific movies.",
			"4. Review progress and report.",
			"",
			"Enter: login",
			"s: status",
			"r: clear saved login",
			"q / ctrl+c: quit",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) authBody() string {
	body := ""
	if m.authURL != "" {
		body = strings.Join([]string{
			m.spinner.View() + " Waiting for Plex login",
			"",
			"Open:",
			m.authURL,
			"",
			"Code: " + m.pin.Code,
		}, "\n")
	} else {
		body = m.spinner.View() + " Loading..."
	}
	left := dashboardPane{
		title:    "Authenticate",
		subtitle: "Watch the code and return here",
		icon:     "●",
		accent:   accentAmber,
		body:     body,
	}
	right := dashboardPane{
		title:    "Steps",
		subtitle: "Fast path",
		icon:     "→",
		accent:   accentCyan,
		body: strings.Join([]string{
			"1. Open the link.",
			"2. Enter the code.",
			"3. Wait for the token.",
			"",
			"Enter: poll now",
			"Esc: cancel",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) choiceBody(title, subtitle, help string, choices string) string {
	left := dashboardPane{
		title:    title,
		subtitle: subtitle,
		icon:     "▸",
		accent:   accentCyan,
		body:     choices,
	}
	right := dashboardPane{
		title:    "Help",
		subtitle: "Keep it fast",
		icon:     "⌁",
		accent:   accentMagenta,
		body: strings.Join(filterEmptyStrings([]string{
			help,
			"↑/↓ or j/k move",
			"Enter selects",
			"q / ctrl+c quits",
		}), "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) modeBody() string {
	left := dashboardPane{
		title:    "Update mode",
		subtitle: "All posters is the default",
		icon:     "◆",
		accent:   accentMagenta,
		body:     renderLines([]string{"All posters (default)", "Specific posters"}, m.cursor),
	}
	right := dashboardPane{
		title:    "Toggles",
		subtitle: "Apply to either mode",
		icon:     "◌",
		accent:   accentCyan,
		body: strings.Join([]string{
			optionLines(m.force, m.dryRun),
			"",
			"f: force refresh",
			"d: dry run",
			"Enter starts",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) moviesBody() string {
	left := dashboardPane{
		title:    "Select movies",
		subtitle: "Space toggles · Enter starts",
		icon:     "☑",
		accent:   accentCyan,
		body:     renderMovies(m.movies, m.cursor, m.chosen, movieListRows(m.height)),
	}
	right := dashboardPane{
		title:    "Run settings",
		subtitle: "Selection summary",
		icon:     "↳",
		accent:   accentAmber,
		body: strings.Join([]string{
			fmt.Sprintf("Selected: %d / %d", chosenCount(m.chosen), len(m.movies)),
			optionLines(m.force, m.dryRun),
			"",
			"Space: toggle row",
			"Enter: update now",
			"Esc: back",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) statusBody() string {
	state, _ := m.store.LoadState()
	metadata, _ := m.store.LoadMetadata()
	left := dashboardPane{
		title:    "Status",
		subtitle: "Local store is source of truth",
		icon:     "◐",
		accent:   accentMagenta,
		body: strings.Join([]string{
			"Config: " + m.store.Dir(),
			"Plex token: " + tokenStatus(state.PlexToken),
			fmt.Sprintf("Metadata items: %d", len(metadata.Items)),
			"Server: " + chooseText(serverLabel(m.server), state.LastServerName),
			"Library: " + chooseText(m.library.Title, state.LastLibraryTitle),
			forceLine(m.force),
			dryRunLine(m.dryRun),
		}, "\n"),
	}
	right := dashboardPane{
		title:    "Controls",
		subtitle: "Quick status toggles",
		icon:     "⌘",
		accent:   accentCyan,
		body: strings.Join([]string{
			forceLine(m.force),
			dryRunLine(m.dryRun),
			"",
			"s: back",
			"q / ctrl+c: quit",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func (m Model) runningBody(percent float64) string {
	header := m.runningHeader(percent)
	activity := viewportLines(runningActivityLines(m.log, 0, contentWidth(m.width)), m.cursor, m.runningViewportRows())
	if activity == "" {
		activity = "  waiting for first update..."
	}
	bodyWidth := contentWidth(m.width)
	var body string
	if bodyWidth < dashboardWideWidth {
		body = strings.Join(filterEmptyStrings([]string{
			header,
			activity,
		}), "\n\n")
	} else {
		left := dashboardPane{
			title:    "Progress",
			subtitle: resultSummary(m.runStats),
			icon:     "▣",
			accent:   accentGreen,
			body:     header,
		}
		right := dashboardPane{
			title:    "Activity",
			subtitle: "Newest updates land at the bottom",
			icon:     "↟",
			accent:   accentCyan,
			body:     activity,
		}
		body = dashboardLayout(m.width, left, right)
	}
	footer := ui.footer.Render("Esc: cancel")
	if body == "" {
		return footer
	}
	return body + "\n\n" + footer
}

func (m Model) doneBody(maxRows int) string {
	footer := ui.footer.Render("Enter/q: quit")
	contentRows := max(3, maxRows-2)
	view := viewportLines(m.doneFullLines(), m.cursor, contentRows)
	if view == "" {
		return footer
	}
	return view + "\n\n" + footer
}

func (m Model) errorBody() string {
	left := dashboardPane{
		title:    "Error",
		subtitle: "Saved login can be cleared",
		icon:     "!",
		accent:   accentRed,
		body:     fmt.Sprintf("%v", m.err),
	}
	right := dashboardPane{
		title:    "Recovery",
		subtitle: "Quick reset",
		icon:     "↺",
		accent:   accentAmber,
		body: strings.Join([]string{
			"r: clear saved login",
			"Enter: retry",
			"q / ctrl+c: quit",
		}, "\n"),
	}
	return dashboardLayout(m.width, left, right)
}

func dashboardLayout(width int, left, right dashboardPane) string {
	if right.body == "" {
		return renderDashboardPane(width, left)
	}
	bodyWidth := contentWidth(width)
	if bodyWidth < dashboardWideWidth {
		return renderDashboardPane(bodyWidth, left)
	}
	leftWidth := max(28, (bodyWidth-panelGap)/2)
	rightWidth := max(28, bodyWidth-panelGap-leftWidth)
	if leftWidth+rightWidth+panelGap > bodyWidth {
		return renderDashboardPane(bodyWidth, left)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderDashboardPane(leftWidth, left), strings.Repeat(" ", panelGap), renderDashboardPane(rightWidth, right))
}

func renderDashboardPane(width int, pane dashboardPane) string {
	width = max(20, width)
	innerWidth := max(10, width-2-(panelHorizontalPad*2))
	body := wrapBody(pane.body, innerWidth)
	parts := []string{}
	if pane.title != "" {
		header := ui.panelTitle.Foreground(pane.accent).Render(iconTitle(pane.icon, pane.title))
		if pane.subtitle != "" {
			header += " " + ui.subtitle.Render("· "+pane.subtitle)
		}
		parts = append(parts, header)
		parts = append(parts, ui.divider.Render(strings.Repeat("─", min(innerWidth, max(8, lipgloss.Width(stripANSI(header)))))))
	}
	if body != "" {
		parts = append(parts, body)
	}
	content := strings.Join(parts, "\n")
	return ui.panel.Copy().Width(width).BorderForeground(pane.accent).Render(content)
}

func iconTitle(icon, title string) string {
	if icon == "" {
		return title
	}
	return icon + " " + title
}

func filterEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func chooseText(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "none"
}

func tokenStatus(token string) string {
	if strings.TrimSpace(token) == "" {
		return "absent"
	}
	return "present"
}

func chosenCount(chosen map[string]bool) int {
	count := 0
	for _, selected := range chosen {
		if selected {
			count++
		}
	}
	return count
}

func (m Model) cursorLimit() int {
	switch m.screen {
	case screenServers:
		return len(m.servers)
	case screenLibraries:
		return len(m.libs)
	case screenMode:
		return 2
	case screenMovies:
		return len(m.movies)
	case screenRunning:
		return runningCursorLimit(len(runningActivityLines(m.log, 0, contentWidth(m.width))), m.runningViewportRows())
	case screenDone:
		return reportCursorLimit(len(m.doneFullLines()), doneViewportRows(doneRows(m.height)))
	}
	return m.cursor + 2
}

func hasSavedToken(store *config.Store) bool {
	state, err := store.LoadState()
	return err == nil && state.PlexToken != ""
}

func preferredServerCursor(store *config.Store, servers []plex.Server) int {
	state, err := store.LoadState()
	if err != nil {
		return 0
	}
	for i, server := range servers {
		if state.LastServerID != "" && server.ClientID == state.LastServerID {
			return i
		}
		if state.LastServerURI != "" && server.URI == state.LastServerURI {
			return i
		}
	}
	return 0
}

func preferredLibraryCursor(store *config.Store, libs []plex.Library) int {
	state, err := store.LoadState()
	if err != nil {
		return 0
	}
	for i, lib := range libs {
		if state.LastLibraryKey != "" && lib.Key == state.LastLibraryKey {
			return i
		}
	}
	return 0
}

func (m Model) statusView() string {
	state, _ := m.store.LoadState()
	metadata, _ := m.store.LoadMetadata()
	token := "absent"
	if state.PlexToken != "" {
		token = "present"
	}
	server := serverLabel(m.server)
	if server == "" {
		server = state.LastServerName
	}
	if server == "" {
		server = "none"
	}
	library := m.library.Title
	if library == "" {
		library = state.LastLibraryTitle
	}
	if library == "" {
		library = "none"
	}
	return strings.Join([]string{
		"Status",
		"",
		"Config: " + m.store.Dir(),
		"Plex token: " + token,
		fmt.Sprintf("Metadata items: %d", len(metadata.Items)),
		"Server: " + server,
		"Library: " + library,
		forceLine(m.force),
		dryRunLine(m.dryRun),
	}, "\n")
}

func startPIN(ctx context.Context, opID int, client Plex) tea.Cmd {
	return func() tea.Msg {
		pin, url, err := client.StartPIN(ctx)
		return pinStartedMsg{opID: opID, pin: pin, url: url, err: err}
	}
}

func pollPIN(ctx context.Context, opID int, client Plex, pinID int) tea.Cmd {
	return func() tea.Msg {
		token, err := client.PollPIN(ctx, pinID)
		return authPollMsg{opID: opID, token: token, err: err}
	}
}

func waitAndPollPIN(ctx context.Context, opID int, client Plex, pinID int) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		token, err := client.PollPIN(ctx, pinID)
		return authPollMsg{opID: opID, token: token, err: err}
	})
}

func loadServers(ctx context.Context, opID int, client Plex) tea.Cmd {
	return func() tea.Msg {
		servers, err := client.ListServers(ctx)
		return serversMsg{opID: opID, servers: servers, err: err}
	}
}

func loadLibraries(ctx context.Context, opID int, client Plex, server plex.Server) tea.Cmd {
	return func() tea.Msg {
		libs, err := client.ListLibraries(ctx, server)
		if err != nil {
			return librariesMsg{opID: opID, err: err}
		}
		return librariesMsg{opID: opID, libs: libs, err: err}
	}
}

func serverLabel(server plex.Server) string {
	return server.Name
}

func optionLines(force bool, dryRun bool) string {
	return forceLine(force) + "\n" + dryRunLine(dryRun)
}

func resultSummary(stats runStats) string {
	parts := []string{
		fmt.Sprintf("updated: %d", stats.Updated),
		fmt.Sprintf("dry-run: %d", stats.DryRun),
		fmt.Sprintf("skipped: %d", stats.Skipped),
		fmt.Sprintf("ambiguous: %d", stats.Ambiguous),
		fmt.Sprintf("failed: %d", stats.Failed),
	}
	if stats.Cancelled {
		parts = append(parts, "cancelled")
	}
	return strings.Join(parts, " · ")
}

func resultSummaryBlock(stats runStats) string {
	lines := []string{
		fmt.Sprintf("Updated:   %d", stats.Updated),
		fmt.Sprintf("Dry runs:  %d", stats.DryRun),
		fmt.Sprintf("Skipped:   %d", stats.Skipped),
		fmt.Sprintf("Ambiguous: %d", stats.Ambiguous),
		fmt.Sprintf("Failed:    %d", stats.Failed),
	}
	if stats.Cancelled {
		lines = append(lines, "Status:    cancelled")
	}
	return strings.Join(lines, "\n")
}

func (m Model) runningView(percent float64) string {
	header := m.runningHeader(percent)
	activity := viewportLines(runningActivityLines(m.log, 0, contentWidth(m.width)), m.cursor, m.runningViewportRows())
	if activity == "" {
		activity = "  waiting for first update..."
	}
	return header + "\n" + activity + "\n\nEsc: cancel"
}

func (m Model) runningHeader(percent float64) string {
	return fmt.Sprintf("%s Updating posters %d/%d\n%s\n\n%s\n\nActivity:", m.spinner.View(), m.runningDone, m.runningTotal, resultSummary(m.runStats), m.bar.ViewAs(percent))
}

func (m Model) runningViewportRows() int {
	return runningRows(m.height, contentWidth(m.width), m.runningHeader(0))
}

func (m Model) doneView(maxRows int) string {
	footer := "Enter/q: quit"
	view := viewportLines(m.doneFullLines(), m.cursor, doneViewportRows(maxRows))
	if view == "" {
		return footer
	}
	return view + "\n\n" + footer
}

func doneViewportRows(maxRows int) int {
	return max(3, maxRows-2)
}

func (m Model) doneFullLines() []string {
	sections := []string{"Done.", "Summary:\n" + indentBlock(resultSummaryBlock(m.runStats), "  ")}
	if m.reportPath != "" || m.reportCSVPath != "" {
		lines := []string{}
		if m.reportPath != "" {
			lines = append(lines, "JSON: "+m.reportPath)
		}
		if m.reportCSVPath != "" {
			lines = append(lines, "CSV:  "+m.reportCSVPath)
		}
		sections = append(sections, "Reports:\n"+indentBlock(strings.Join(lines, "\n"), "  "))
	}
	if results := reportItemsView(m.reportItems); results != "" {
		sections = append(sections, "Results:\n"+results)
	} else if recent := recentActivityView(m.log, 8); recent != "" {
		sections = append(sections, "Recent activity:\n"+recent)
	}
	if details := tail(m.details, 8); details != "" {
		sections = append(sections, "Ambiguous matches:\n"+indentBlock(details, "  "))
	}
	return strings.Split(strings.Join(sections, "\n\n"), "\n")
}

func reportItemsView(items []config.ReportItem) string {
	if len(items) == 0 {
		return ""
	}
	sections := make([]string, 0, len(items))
	for _, item := range items {
		sections = append(sections, formatReportItem(item))
	}
	return strings.Join(sections, "\n")
}

func formatReportItem(item config.ReportItem) string {
	status := strings.ToUpper(item.Status)
	if status == "" {
		status = "RESULT"
	}
	header := fmt.Sprintf("  %s %s", status, item.Title)
	if item.Year > 0 {
		header += fmt.Sprintf(" (%d)", item.Year)
	}
	lines := []string{header}
	if item.SourceURL != "" {
		lines = append(lines, "    IMP page: "+item.SourceURL)
	}
	if item.ImageURL != "" {
		lines = append(lines, "    Image:    "+item.ImageURL)
	}
	if item.MatchReason != "" {
		lines = append(lines, "    Match:    "+item.MatchReason)
	}
	if item.Error != "" {
		lines = append(lines, "    Error:    "+item.Error)
	} else if item.Message != "" && item.SourceURL == "" && item.ImageURL == "" {
		lines = append(lines, "    Note:     "+item.Message)
	}
	return strings.Join(lines, "\n")
}

func recentActivityView(lines []string, limit int) string {
	recent := tail(lines, limit)
	if recent == "" {
		return ""
	}
	return indentBlock(recent, "  ")
}

func runningActivityView(lines []string, limit, width int) string {
	return strings.Join(runningActivityLines(lines, limit, width), "\n")
}

func runningActivityLines(lines []string, limit, width int) []string {
	if len(lines) == 0 {
		return []string{"  waiting for first update..."}
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	formatted := []string{}
	for _, line := range lines {
		formatted = append(formatted, formatActivityEntry(line, width)...)
	}
	return formatted
}

func formatActivityEntry(line string, width int) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if strings.HasPrefix(line, "dry-run ") {
		return formatDryRunActivity(line, width)
	}
	prefix := "  • "
	if strings.HasPrefix(line, "updated ") {
		prefix = "  ✓ "
	} else if strings.HasPrefix(line, "skip ") {
		prefix = "  – "
	} else if strings.HasPrefix(line, "report") {
		prefix = "  ↳ "
	}
	return wrapWithPrefix(line, prefix, "    ", width)
}

func formatDryRunActivity(line string, width int) []string {
	parts := strings.Split(line, " | ")
	lines := []string{}
	main := strings.TrimPrefix(parts[0], "dry-run ")
	if title, source, ok := strings.Cut(main, ": "); ok {
		lines = append(lines, wrapWithPrefix(title, "  ○ DRY-RUN ", "    ", width)...)
		lines = append(lines, wrapWithPrefix(source, "    IMP:   ", "           ", width)...)
	} else {
		lines = append(lines, wrapWithPrefix(main, "  ○ DRY-RUN ", "    ", width)...)
	}
	for _, part := range parts[1:] {
		label := "    Info:  "
		value := part
		if v, ok := strings.CutPrefix(part, "image: "); ok {
			label, value = "    Image: ", v
		} else if v, ok := strings.CutPrefix(part, "reason: "); ok {
			label, value = "    Match: ", v
		}
		lines = append(lines, wrapWithPrefix(value, label, strings.Repeat(" ", lipgloss.Width(label)), width)...)
	}
	return lines
}

func wrapWithPrefix(text, firstPrefix, nextPrefix string, width int) []string {
	available := max(10, width-lipgloss.Width(firstPrefix))
	wrapped := wrapLineHard(text, available)
	if len(wrapped) == 0 {
		return []string{firstPrefix}
	}
	out := make([]string, len(wrapped))
	for i, line := range wrapped {
		prefix := firstPrefix
		if i > 0 {
			prefix = nextPrefix
		}
		out[i] = prefix + line
	}
	return out
}

func wrapLineHard(text string, width int) []string {
	soft := wrapLine(text, width)
	out := []string{}
	for _, line := range soft {
		for lipgloss.Width(line) > width {
			cut := visibleCut(line, width)
			out = append(out, strings.TrimRight(line[:cut], " "))
			line = strings.TrimLeft(line[cut:], " ")
		}
		out = append(out, line)
	}
	return out
}

func visibleCut(text string, width int) int {
	if width <= 0 {
		return 0
	}
	x := 0
	for i, r := range text {
		w := lipgloss.Width(string(r))
		if w == 0 {
			w = 1
		}
		if x+w > width {
			return i
		}
		x += w
	}
	return len(text)
}

func viewportLines(lines []string, offset, maxRows int) string {
	if len(lines) == 0 {
		return ""
	}
	if maxRows <= 0 {
		return ""
	}
	if len(lines) <= maxRows {
		return strings.Join(lines, "\n")
	}
	if maxRows == 1 {
		offset = clamp(offset, 0, len(lines)-1)
		return lines[offset]
	}
	if maxRows == 2 {
		offset = clamp(offset, 0, len(lines)-1)
		marker := ""
		if offset+1 < len(lines) {
			marker = fmt.Sprintf("… %d more (↑/↓ scroll)", len(lines)-offset-1)
		} else if offset > 0 {
			marker = fmt.Sprintf("… %d earlier", offset)
		}
		return strings.Join([]string{lines[offset], marker}, "\n")
	}
	contentRows := maxRows - 2
	offset = clamp(offset, 0, max(0, len(lines)-contentRows))
	end := min(len(lines), offset+contentRows)
	top := ""
	if offset > 0 {
		top = fmt.Sprintf("… %d earlier", offset)
	}
	bottom := ""
	if end < len(lines) {
		bottom = fmt.Sprintf("… %d more (↑/↓ scroll)", len(lines)-end)
	}
	visible := []string{top}
	visible = append(visible, lines[offset:end]...)
	visible = append(visible, bottom)
	return strings.Join(visible, "\n")
}

func reportCursorLimit(lineCount, maxRows int) int {
	if lineCount <= 0 || maxRows <= 0 || lineCount <= maxRows {
		return 1
	}
	if maxRows <= 2 {
		return lineCount
	}
	contentRows := maxRows - 2
	return max(1, lineCount-contentRows+1)
}

func runningCursorLimit(lineCount, maxRows int) int {
	return reportCursorLimit(lineCount, maxRows)
}

func runningRows(height, width int, header string) int {
	if height <= 0 {
		return 8
	}
	headerRows := len(strings.Split(wrapBody(header, width), "\n"))
	return max(1, height-8-headerRows-2)
}

func doneRows(height int) int {
	if height <= 0 {
		return 18
	}
	return max(6, height-2-(verticalContentPadding*2)-3)
}

func indentBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func forceLine(force bool) string {
	if force {
		return "Force refresh: on (f toggles)"
	}
	return "Force refresh: off (f toggles)"
}

func dryRunLine(dryRun bool) string {
	if dryRun {
		return "Dry run: on (d toggles)"
	}
	return "Dry run: off (d toggles)"
}

func loadMovies(ctx context.Context, opID int, client Plex, server plex.Server, library plex.Library) tea.Cmd {
	return func() tea.Msg {
		movies, err := client.ListMovies(ctx, server, library)
		return moviesMsg{opID: opID, movies: movies, err: err}
	}
}

func formatUpdateError(movie plex.Movie, err error) string {
	var ambiguous *posterfinder.AmbiguousMatchError
	if errors.As(err, &ambiguous) {
		return fmt.Sprintf("skip %s (%d): %s", movie.Title, movie.Year, ambiguous.Summary())
	}
	return fmt.Sprintf("skip %s (%d): %v", movie.Title, movie.Year, err)
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
		lines = append(lines, "  - "+strings.Join(bits, " · "))
	}
	return lines
}

func updateMovie(ctx context.Context, opID int, store *config.Store, client Plex, finder PosterFinder, server plex.Server, movie plex.Movie, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		candidate, err := finder.FindTheatricalPoster(ctx, movie)
		if err != nil {
			return updateOneMsg{opID: opID, movie: movie, err: err}
		}
		if dryRun {
			return updateOneMsg{opID: opID, movie: movie, line: dryRunResultLine(movie, candidate), sourceURL: candidate.SourceURL, imageURL: candidate.ImageURL, matchReason: candidate.MatchReason}
		}
		if err := client.UploadPoster(ctx, server, movie, "poster.jpg", candidate.Bytes, candidate.ImageURL); err != nil {
			return updateOneMsg{opID: opID, movie: movie, err: err}
		}
		if err := store.MarkPosterUpdated(config.PosterItem{RatingKey: movie.RatingKey, Title: movie.Title, Year: movie.Year, SourceURL: candidate.SourceURL}); err != nil {
			return updateOneMsg{opID: opID, movie: movie, err: err}
		}
		return updateOneMsg{opID: opID, movie: movie, line: fmt.Sprintf("updated %s (%d)", movie.Title, movie.Year), sourceURL: candidate.SourceURL, imageURL: candidate.ImageURL, matchReason: candidate.MatchReason}
	}
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

func shell(body string, width int) string {
	return shellSized(body, width, 0)
}

func shellSized(body string, width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("posters")
	wrapWidth := contentWidth(width)
	body = wrapBody(body, wrapWidth)
	content := title + "\n\n" + linkifyURLs(body)
	contentPlain := "posters\n\n" + body
	boxWidth := maxVisibleLineWidth(contentPlain) + (lipglossHorizontalPadding * 2)
	maxBoxWidth := max(1, cardWidth(width)-2)
	boxWidth = min(maxBoxWidth, max(1, boxWidth))
	style := lipgloss.NewStyle().Padding(verticalContentPadding, lipglossHorizontalPadding).Width(boxWidth).AlignVertical(lipgloss.Center).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63"))
	return style.Render(content)
}

func cardWidth(width int) int {
	if width <= 0 {
		return 80
	}
	return max(40, width-2)
}

func cardHeight(height int) int {
	return 0
}

func contentWidth(width int) int {
	return max(20, cardWidth(width)-2-(horizontalContentPadding*2))
}

func progressBarWidth(width int) int {
	return max(10, min(60, contentWidth(width)-8))
}

func maxVisibleLineWidth(text string) int {
	maxWidth := 0
	for _, line := range strings.Split(text, "\n") {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	return maxWidth
}

func centerView(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return view
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, view)
}

func wrapBody(body string, width int) string {
	lines := strings.Split(body, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return []string{line}
	}
	indent := leadingWhitespace(line)
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := ""
	continuation := indent
	if continuation != "" && lipgloss.Width(continuation) >= width-4 {
		continuation = ""
	}
	for _, word := range words {
		if current == "" {
			current = indent + word
			continue
		}
		candidate := current + " " + word
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = continuation + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func linkifyURLs(text string) string {
	return urlRE.ReplaceAllStringFunc(text, func(raw string) string {
		return "\x1b]8;;" + raw + "\x1b\\" + raw + "\x1b]8;;\x1b\\"
	})
}

func renderChoices[T any](items []T, cursor int, label func(T) string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		prefix := "  "
		if i == cursor%max(1, len(items)) {
			prefix = "› "
		}
		lines[i] = prefix + label(item)
	}
	if len(lines) == 0 {
		return "No choices."
	}
	return strings.Join(lines, "\n")
}

func renderLines(items []string, cursor int) string {
	return renderChoices(items, cursor, func(s string) string { return s })
}

func renderMovies(movies []plex.Movie, cursor int, chosen map[string]bool, maxRows int) string {
	if len(movies) == 0 {
		return "No choices."
	}
	if maxRows <= 0 || maxRows > len(movies) {
		maxRows = len(movies)
	}
	selected := min(max(0, cursor), len(movies)-1)
	if len(movies) <= maxRows || maxRows < 3 {
		return renderMovieRows(movies, 0, len(movies), selected, chosen)
	}
	contentRows := maxRows - 2
	start := selected - contentRows/2
	start = min(max(0, start), max(0, len(movies)-contentRows))
	end := min(len(movies), start+contentRows)
	topMarker := ""
	if start > 0 {
		topMarker = fmt.Sprintf("… %d earlier", start)
	}
	bottomMarker := ""
	if end < len(movies) {
		bottomMarker = fmt.Sprintf("… %d more", len(movies)-end)
	}
	lines := []string{topMarker}
	lines = append(lines, strings.Split(renderMovieRows(movies, start, end, selected, chosen), "\n")...)
	lines = append(lines, bottomMarker)
	return strings.Join(lines, "\n")
}

func renderMovieRows(movies []plex.Movie, start, end, selected int, chosen map[string]bool) string {
	start = max(0, start)
	end = min(len(movies), end)
	lines := []string{}
	for i := start; i < end; i++ {
		movie := movies[i]
		mark := "[ ]"
		if chosen[movie.RatingKey] {
			mark = "[x]"
		}
		prefix := "  "
		if i == selected {
			prefix = "› "
		}
		lines = append(lines, fmt.Sprintf("%s%s %s (%d)", prefix, mark, movie.Title, movie.Year))
	}
	return strings.Join(lines, "\n")
}

func movieListRows(height int) int {
	if height <= 0 {
		return 12
	}
	return max(3, height-13)
}

func tail(lines []string, n int) string {
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
