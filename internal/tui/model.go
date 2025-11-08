package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/job"
)

// Model represents the main application state for Bubbletea
type Model struct {
	// Application state
	repositories []*git.Repository
	directories  []string
	mode         Mode
	queue        *job.Queue
	spinnerIndex int
	version      string

	// UI state
	cursor              int
	width               int
	height              int
	ready               bool
	initialFetchStarted bool
	loading             bool
	jobsRunning         bool
	err                 error

	// View state
	currentView            ViewType
	sidePanel              SidePanelType
	showHelp               bool
	branchCursor           int
	remoteBranchCursor     int
	commitCursor           int
	commitOffset           int
	commitScrollOffsets    map[string]int
	commitDetailScroll     map[string]int
	branchOffset           int
	remoteOffset           int
	forcePromptQueue       []*forcePushPrompt
	activeForcePrompt      *forcePushPrompt
	credentialPromptQueue  []*credentialPrompt
	activeCredentialPrompt *credentialPrompt
	credentialInputField   credentialField
	credentialInputBuffer  string

	// Styles
	styles *Styles
}

// ViewType represents the current view mode
type ViewType int

const (
	OverviewView ViewType = iota
	FocusView
)

// SidePanelType represents which side panel is active
type SidePanelType int

const (
	NonePanel SidePanelType = iota
	BranchPanel
	RemotePanel
	RemoteBranchPanel
	CommitPanel
	StashPanel
	StatusPanel
)

// Mode represents the operation mode
type Mode struct {
	ID            ModeID
	DisplayString string
	CommandString string
}

type forcePushPrompt struct {
	repo *git.Repository
}

type credentialPrompt struct {
	repo     *git.Repository
	job      *job.Job
	username string
	password string
}

type credentialField int

const (
	credentialFieldUsername credentialField = iota
	credentialFieldPassword
)

// ModeID identifies the mode
type ModeID string

const (
	PullMode   ModeID = "pull"
	MergeMode  ModeID = "merge"
	RebaseMode ModeID = "rebase"
	PushMode   ModeID = "push"
)

var (
	pullMode   = Mode{ID: PullMode, DisplayString: "Pull (ff-only)", CommandString: "pull --ff-only"}
	mergeMode  = Mode{ID: MergeMode, DisplayString: "Merge", CommandString: "merge"}
	rebaseMode = Mode{ID: RebaseMode, DisplayString: "Rebase", CommandString: "pull --rebase"}
	pushMode   = Mode{ID: PushMode, DisplayString: "Push", CommandString: "push"}

	modes = []Mode{pullMode, mergeMode, rebaseMode, pushMode}
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

var tagHighlightColor = lipgloss.AdaptiveColor{Light: "#1565C0", Dark: "#42A5F5"}
var tagWarningColor = lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#E57373"}

// Styles holds all lipgloss styles for the UI
type Styles struct {
	App                lipgloss.Style
	Title              lipgloss.Style
	StatusBarPull      lipgloss.Style
	StatusBarMerge     lipgloss.Style
	StatusBarRebase    lipgloss.Style
	StatusBarPush      lipgloss.Style
	StatusBarDirty     lipgloss.Style
	StatusBarError     lipgloss.Style
	Help               lipgloss.Style
	List               lipgloss.Style
	ListItem           lipgloss.Style
	SelectedItem       lipgloss.Style
	DirtySelectedItem  lipgloss.Style
	CommonSelectedItem lipgloss.Style
	FailedSelectedItem lipgloss.Style
	QueuedItem         lipgloss.Style
	WorkingItem        lipgloss.Style
	SuccessItem        lipgloss.Style
	FailedItem         lipgloss.Style
	DisabledItem       lipgloss.Style
	BranchInfo         lipgloss.Style
	KeyBinding         lipgloss.Style
	Panel              lipgloss.Style
	PanelTitle         lipgloss.Style
	Error              lipgloss.Style
	TableBorder        lipgloss.Style
}

// DefaultStyles returns the default style set
func DefaultStyles() *Styles {
	return &Styles{
		App: lipgloss.NewStyle(),
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#5E35B1", Dark: "#7E57C2"}).
			Padding(0, 1),
		StatusBarPull: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#90CAF9", Dark: "#1E88E5"}).
			Padding(0, 1),
		StatusBarMerge: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#041419", Dark: "#E0F7FA"}).
			Background(lipgloss.AdaptiveColor{Light: "#4DD0E1", Dark: "#00ACC1"}).
			Padding(0, 1),
		StatusBarRebase: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#A5D6A7", Dark: "#43A047"}).
			Padding(0, 1),
		StatusBarDirty: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4E342E", Dark: "#D7CCC8"}).
			Background(lipgloss.AdaptiveColor{Light: "#D7CCC8", Dark: "#4E342E"}).
			Padding(0, 1),
		StatusBarError: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#C62828"}).
			Padding(0, 1),
		StatusBarPush: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1B1B1B", Dark: "#1B1B1B"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFF59D", Dark: "#FDD835"}).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"}),
		List: lipgloss.NewStyle().
			Padding(1, 2),
		ListItem: lipgloss.NewStyle(),
		SelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0B0B0B", Dark: "#F5F5F5"}).
			Background(lipgloss.AdaptiveColor{Light: "#90CAF9", Dark: "#1976D2"}).
			Bold(true),
		CommonSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1B1B1B", Dark: "#1B1B1B"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFCC80", Dark: "#FB8C00"}).
			Bold(true),
		DirtySelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#424242", Dark: "#BDBDBD"}).
			Background(lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#424242"}).
			Bold(true),
		FailedSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#C62828"}).
			Bold(true),
		QueuedItem: lipgloss.NewStyle().
			Foreground(tagHighlightColor),
		WorkingItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0097A7", Dark: "#4DD0E1"}),
		SuccessItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#388E3C", Dark: "#81C784"}),
		FailedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#E57373"}),
		DisabledItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9E9E9E", Dark: "#616161"}).
			Faint(true),
		BranchInfo: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#00796B", Dark: "#4DB6AC"}),
		KeyBinding: lipgloss.NewStyle().
			Foreground(tagHighlightColor).
			Bold(true),
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#7E57C2", Dark: "#9575CD"}).
			Padding(0, 1),
		PanelTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#424242", Dark: "#E0E0E0"}),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#E57373"}).
			Bold(true),
		TableBorder: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9E9E9E", Dark: "#616161"}),
	}
}

// New creates a new Model with the given configuration
func New(mode string, directories []string) *Model {
	initialMode := pullMode
	for _, m := range modes {
		if string(m.ID) == mode {
			initialMode = m
			break
		}
	}

	return &Model{
		directories:         directories,
		mode:                initialMode,
		queue:               job.CreateJobQueue(),
		repositories:        make([]*git.Repository, 0),
		currentView:         OverviewView,
		sidePanel:           NonePanel,
		styles:              DefaultStyles(),
		loading:             true,
		spinnerIndex:        0,
		version:             Version,
		commitScrollOffsets: make(map[string]int),
		commitDetailScroll:  make(map[string]int),
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return loadRepositoriesCmd(m.directories)
}

func (m *Model) terminalTooSmall() bool {
	return m.width < minTerminalWidth || m.height < minTerminalHeight
}

// repositoriesLoadedMsg is sent when all repositories are loaded
type repositoriesLoadedMsg struct {
	repos []*git.Repository
}

// errMsg is sent when an error occurs
type errMsg struct {
	err error
}

func (e errMsg) Error() string { return e.err.Error() }

// lazygitClosedMsg is sent when lazygit exits
type lazygitClosedMsg struct{}

// jobCompletedMsg is sent when a job completes (success or failure)
type jobCompletedMsg struct{}

// jobQueueResultMsg delivers the outcome of an async job queue execution
type jobQueueResultMsg struct {
	resetMainQueue bool
	failures       map[*job.Job]error
}

// autoFetchFailedMsg signals non-fatal fetch failures during initial load
type autoFetchFailedMsg struct {
	names []string
}

// repoActionResultMsg is sent when a focus view action updates repository state
type repoActionResultMsg struct {
	panel SidePanelType
}
