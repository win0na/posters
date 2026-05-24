package tui

import (
	"context"
	"fmt"
	"regexp"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/win0na/posters/internal/config"
	"github.com/win0na/posters/internal/plex"
	posterfinder "github.com/win0na/posters/internal/posters"
)

var urlRE = regexp.MustCompile(`https?://[^\s|]+`)

const horizontalContentPadding = 10
const lipglossHorizontalPadding = horizontalContentPadding - 2
const verticalContentPadding = 2
const posterUpdateConcurrency = 4

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
	skip        lipgloss.Style
	wiki        lipgloss.Style
	worker      lipgloss.Style
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

var ui uiTheme

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
	ui = newUITheme()
}

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
		skip:        base.Bold(true).Foreground(accentMagenta),
		wiki:        base.Bold(true).Foreground(accentBlue),
		worker:      base.Bold(true).Foreground(accentAmber),
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
	screenBlacklist
	screenDone
	screenError
)

type mode int

const (
	modeAll mode = iota
	modeSpecific
)

type runStats struct {
	Updated      int
	DryRun       int
	WikiFallback int
	Skipped      int
	Blacklisted  int
	Ambiguous    int
	Failed       int
	Cancelled    bool
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
	FindWikipediaPoster(context.Context, plex.Movie) (posterfinder.Candidate, error)
}

type posterResolver interface {
	ResolveTheatricalPoster(context.Context, plex.Movie, bool) (posterfinder.Candidate, error)
}

type Options struct {
	Force        bool
	DryRun       bool
	WikiFallback bool
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

	pin          plex.Pin
	authURL      string
	server       plex.Server
	servers      []plex.Server
	library      plex.Library
	libs         []plex.Library
	movies       []plex.Movie
	mode         mode
	force        bool
	dryRun       bool
	wikiFallback bool
	cursor       int
	chosen       map[string]bool

	runningTotal   int
	runningDone    int
	runningNext    int
	runningActive  int
	runningQueue   []plex.Movie
	runningCurrent []runningMovie
	progressCh     chan updatePhaseMsg
	runStats       runStats
	reportItems    []config.ReportItem
	reportPath     string
	reportCSVPath  string
	log            []string
	details        []string
	notice         string
	err            error
	selection      selectionState

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
type updatePhaseMsg struct {
	opID  int
	movie plex.Movie
	phase string
}
type runningMovie struct {
	Movie plex.Movie
	Phase string
}
type doneMsg struct{}
type selectionCopiedMsg struct{ err error }

func New(store *config.Store, client Plex) Model {
	return NewWithOptions(store, client, Options{})
}

func NewWithOptions(store *config.Store, client Plex, options Options) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{
		store:        store,
		plex:         client,
		finder:       posterfinder.NewService(),
		screen:       screenLogin,
		spinner:      sp,
		bar:          progress.New(progress.WithDefaultGradient()),
		chosen:       map[string]bool{},
		force:        options.Force,
		dryRun:       options.DryRun,
		wikiFallback: options.WikiFallback,
	}
}

func (m Model) Init() tea.Cmd {
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
		m.runningActive = max(0, m.runningActive-1)
		m.runningCurrent = removeRunningCurrent(m.runningCurrent, msg.movie)
		m.runningDone++
		m.recordUpdateResult(msg)
		if msg.err != nil {
			m.log = append(m.log, formatUpdateError(msg.movie, msg.err))
		} else {
			m.log = append(m.log, msg.line)
		}
		if m.runningDone >= m.runningTotal {
			m = m.finishRun(false)
			m.screen = screenDone
			return m, nil
		}
		cmds := []tea.Cmd{m.bar.SetPercent(float64(m.runningDone) / float64(max(1, m.runningTotal)))}
		m, cmds = m.launchUpdateBatch(m.ctx, msg.opID, cmds)
		m.cursor = max(0, m.cursorLimit()-1)
		return m, tea.Batch(cmds...)
	case updatePhaseMsg:
		if !m.isActive(msg.opID) {
			return m, nil
		}
		m.runningCurrent = updateRunningPhase(m.runningCurrent, msg.movie, msg.phase)
		return m, waitUpdatePhase(m.ctx, msg.opID, m.progressCh)
	case doneMsg:
		m.screen = screenDone
		return m, nil
	}
	return m, nil
}
