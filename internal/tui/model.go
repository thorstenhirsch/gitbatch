package tui

import (
	"time"

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
	spinnerIndex int
	version      string

	// UI state
	cursor                   int
	width                    int
	height                   int
	ready                    bool
	initialStateProbeStarted bool
	loading                  bool
	jobsRunning              bool
	err                      error

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

	// Performance caching
	cachedColWidths columnWidths
	cachedWidth     int
	cachedRepoCount int

	// Styles
	styles *Styles
}

// ViewType represents the current view mode
type ViewType int

const (
	OverviewView ViewType = iota
	FocusView
)

// columnWidths holds the calculated widths for table columns
type columnWidths struct {
	repo      int
	branch    int
	commitMsg int
}

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
	pullMode   = Mode{ID: PullMode, DisplayString: "Pull | m: switch", CommandString: "pull --ff-only"}
	mergeMode  = Mode{ID: MergeMode, DisplayString: "Merge | m: switch", CommandString: "merge"}
	rebaseMode = Mode{ID: RebaseMode, DisplayString: "Rebase | m: switch", CommandString: "pull --rebase"}
	pushMode   = Mode{ID: PushMode, DisplayString: "Push | m: switch", CommandString: "push"}

	modes = []Mode{pullMode, mergeMode, rebaseMode, pushMode}
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

var tagHighlightColor = lipgloss.AdaptiveColor{Light: "#1565C0", Dark: "#42A5F5"}

// Styles holds all lipgloss styles for the UI
type Styles struct {
	Title                         lipgloss.Style
	StatusBarPull                 lipgloss.Style
	StatusBarMerge                lipgloss.Style
	StatusBarRecoverable          lipgloss.Style
	StatusBarCredentials          lipgloss.Style
	StatusBarRebase               lipgloss.Style
	StatusBarPush                 lipgloss.Style
	StatusBarDisabled             lipgloss.Style
	StatusBarError                lipgloss.Style
	Help                          lipgloss.Style
	List                          lipgloss.Style
	ListItem                      lipgloss.Style
	RecoverableFailedSelectedItem lipgloss.Style
	CredentialsSelectedItem       lipgloss.Style
	SelectedItem                  lipgloss.Style
	DisabledSelectedItem          lipgloss.Style
	CommonSelectedItem            lipgloss.Style
	FailedSelectedItem            lipgloss.Style
	RecoverableFailedItem         lipgloss.Style
	CredentialsItem               lipgloss.Style
	QueuedItem                    lipgloss.Style
	PendingItem                   lipgloss.Style
	WorkingItem                   lipgloss.Style
	SuccessItem                   lipgloss.Style
	FailedItem                    lipgloss.Style
	DisabledItem                  lipgloss.Style
	BranchInfo                    lipgloss.Style
	KeyBinding                    lipgloss.Style
	Panel                         lipgloss.Style
	PanelTitle                    lipgloss.Style
	Error                         lipgloss.Style
	TableBorder                   lipgloss.Style
}

// DefaultStyles returns the default style set
func DefaultStyles() *Styles {
	return &Styles{
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
		StatusBarDisabled: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4A3728", Dark: "#F5F5F5"}).
			Background(lipgloss.AdaptiveColor{Light: "#D7CCC8", Dark: "#4A3728"}).
			Padding(0, 1),
		StatusBarError: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#C62828"}).
			Padding(0, 1),
		StatusBarPush: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1B1B1B", Dark: "#1B1B1B"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFF59D", Dark: "#FDD835"}).
			Padding(0, 1),
		StatusBarRecoverable: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4A3728", Dark: "#FFF3E0"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFE0B2", Dark: "#FB8C00"}).
			Padding(0, 1),
		StatusBarCredentials: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#EC407A", Dark: "#D81B60"}).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"}),
		List:     lipgloss.NewStyle(),
		ListItem: lipgloss.NewStyle(),
		SelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0B0B0B", Dark: "#F5F5F5"}).
			Background(lipgloss.AdaptiveColor{Light: "#90CAF9", Dark: "#1976D2"}).
			Bold(true),
		CommonSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1B1B1B", Dark: "#1B1B1B"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFCC80", Dark: "#FB8C00"}).
			Bold(true),
		DisabledSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4A3728", Dark: "#F5F5F5"}).
			Background(lipgloss.AdaptiveColor{Light: "#D7CCC8", Dark: "#4A3728"}).
			Bold(true),
		FailedSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#C62828"}).
			Bold(true),
		RecoverableFailedSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#3E2723", Dark: "#FFF3E0"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFCC80", Dark: "#FB8C00"}).
			Bold(true),
		CredentialsSelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#EC407A", Dark: "#D81B60"}).
			Bold(true),
		QueuedItem: lipgloss.NewStyle().
			Foreground(tagHighlightColor),
		PendingItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9E9E9E", Dark: "#757575"}),
		WorkingItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0097A7", Dark: "#4DD0E1"}),
		SuccessItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#388E3C", Dark: "#81C784"}),
		FailedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#E57373"}),
		RecoverableFailedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#EF6C00", Dark: "#FFA726"}),
		CredentialsItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#EC407A", Dark: "#F48FB1"}),
		DisabledItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4A3728", Dark: "#9A7B4F"}),
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
	return tea.Batch(loadRepositoriesCmd(m.directories), listenRepositoryUpdatesCmd(), tickCmd())
}

func (m *Model) terminalTooSmall() bool {
	return m.width < minTerminalWidth || m.height < minTerminalHeight
}

// repositoriesLoadedMsg is sent when all repositories are loaded
type repositoriesLoadedMsg struct {
	repos []*git.Repository
}

// repositoryLoadedMsg streams repositories as they finish initializing.
type repositoryLoadedMsg struct {
	repo      *git.Repository
	err       error
	nextIndex int
}

// repositoryStateChangedMsg notifies the TUI that a repository triggered a RepositoryUpdated event.
type repositoryStateChangedMsg struct{}

// repositoriesWaitingMsg signals that repositories should render in waiting state immediately after scheduling state probes.
type repositoriesWaitingMsg struct{}

// errMsg is sent when an error occurs
type errMsg struct {
	err error
}

func (e errMsg) Error() string { return e.err.Error() }

// lazygitClosedMsg is sent when lazygit exits
type lazygitClosedMsg struct {
	repo            *git.Repository
	originalModTime time.Time
	originalState   git.RepositoryState
}

// jobCompletedMsg is sent when a job completes (success or failure)
type jobCompletedMsg struct{}

// autoFetchFailedMsg signals non-fatal fetch failures during initial load
type autoFetchFailedMsg struct {
	names []string
}

// repoActionResultMsg is sent when a focus view action updates repository state
type repoActionResultMsg struct {
	panel SidePanelType
}
