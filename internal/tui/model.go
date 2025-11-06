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
	repositories  []*git.Repository
	directories   []string
	mode          Mode
	queue         *job.Queue
	failoverQueue *job.Queue
	targetBranch  string
	
	// UI state
	cursor        int
	width         int
	height        int
	ready         bool
	loading       bool
	err           error
	
	// View state
	currentView   ViewType
	sidePanel     SidePanelType
	showHelp      bool
	
	// Styles
	styles        *Styles
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

// ModeID identifies the mode
type ModeID string

const (
	FetchMode    ModeID = "fetch"
	PullMode     ModeID = "pull"
	MergeMode    ModeID = "merge"
	CheckoutMode ModeID = "checkout"
)

var (
	fetchMode    = Mode{ID: FetchMode, DisplayString: "Fetch", CommandString: "fetch"}
	pullMode     = Mode{ID: PullMode, DisplayString: "Pull", CommandString: "pull"}
	mergeMode    = Mode{ID: MergeMode, DisplayString: "Merge", CommandString: "merge"}
	checkoutMode = Mode{ID: CheckoutMode, DisplayString: "Checkout", CommandString: "checkout"}
	
	modes = []Mode{fetchMode, pullMode, mergeMode}
)

// Styles holds all lipgloss styles for the UI
type Styles struct {
	App            lipgloss.Style
	Title          lipgloss.Style
	StatusBar      lipgloss.Style
	Help           lipgloss.Style
	List           lipgloss.Style
	ListItem       lipgloss.Style
	SelectedItem   lipgloss.Style
	QueuedItem     lipgloss.Style
	WorkingItem    lipgloss.Style
	SuccessItem    lipgloss.Style
	FailedItem     lipgloss.Style
	BranchInfo     lipgloss.Style
	KeyBinding     lipgloss.Style
	Panel          lipgloss.Style
	PanelTitle     lipgloss.Style
	Error          lipgloss.Style
}

// DefaultStyles returns the default style set
func DefaultStyles() *Styles {
	return &Styles{
		App: lipgloss.NewStyle(),
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1),
		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FFFFFF")).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")),
		List: lipgloss.NewStyle().
			Padding(1, 2),
		ListItem: lipgloss.NewStyle().
			PaddingLeft(2),
		SelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true),
		QueuedItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")),
		WorkingItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")),
		SuccessItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")),
		FailedItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")),
		BranchInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")),
		KeyBinding: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")).
			Bold(true),
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			Padding(0, 1),
		PanelTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFDF5")),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true),
	}
}

// New creates a new Model with the given configuration
func New(mode string, directories []string) *Model {
	initialMode := fetchMode
	for _, m := range modes {
		if string(m.ID) == mode {
			initialMode = m
			break
		}
	}
	
	return &Model{
		directories:   directories,
		mode:          initialMode,
		queue:         job.CreateJobQueue(),
		failoverQueue: job.CreateJobQueue(),
		repositories:  make([]*git.Repository, 0),
		currentView:   OverviewView,
		sidePanel:     NonePanel,
		styles:        DefaultStyles(),
		loading:       true,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return loadRepositoriesCmd(m.directories)
}

// repositoriesLoadedMsg is sent when all repositories are loaded
type repositoriesLoadedMsg struct{
	repos []*git.Repository
}

// errMsg is sent when an error occurs
type errMsg struct {
	err error
}

func (e errMsg) Error() string { return e.err.Error() }
