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
	cursor      int
	width       int
	height      int
	ready       bool
	loading     bool
	jobsRunning bool
	err         error

	// View state
	currentView        ViewType
	sidePanel          SidePanelType
	showHelp           bool
	branchCursor       int
	remoteBranchCursor int
	commitCursor       int
	commitOffset       int

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

var tagHighlightColor = lipgloss.AdaptiveColor{Light: "#F57C00", Dark: "#FFB74D"}

// Styles holds all lipgloss styles for the UI
type Styles struct {
	App            lipgloss.Style
	Title          lipgloss.Style
	StatusBarFetch lipgloss.Style
	StatusBarPull  lipgloss.Style
	StatusBarMerge lipgloss.Style
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
	TableBorder    lipgloss.Style
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
		StatusBarFetch: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#90CAF9", Dark: "#1E88E5"}).
			Padding(0, 1),
		StatusBarPull: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#A5D6A7", Dark: "#43A047"}).
			Padding(0, 1),
		StatusBarMerge: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFE082", Dark: "#FFA000"}).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"}),
		List: lipgloss.NewStyle().
			Padding(1, 2),
		ListItem: lipgloss.NewStyle(),
		SelectedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1976D2", Dark: "#64B5F6"}).
			Bold(true),
		QueuedItem: lipgloss.NewStyle().
			Foreground(tagHighlightColor),
		WorkingItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#0097A7", Dark: "#4DD0E1"}),
		SuccessItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#388E3C", Dark: "#81C784"}),
		FailedItem: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#E57373"}),
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

// repoActionResultMsg is sent when a focus view action updates repository state
type repoActionResultMsg struct {
	panel SidePanelType
}
