package git

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"golang.org/x/sync/semaphore"
)

// Reference is the interface for commits, remotes and branches
type Reference interface {
	Next() *Reference
	Previous() *Reference
}

// Repository is the main entity of the application. The repository name is
// actually the name of its folder in the host's filesystem. It holds the go-git
// repository entity along with critic entities such as remote/branches and commits
type Repository struct {
	RepoID   string
	Name     string
	AbsPath  string
	ModTime  time.Time
	Repo     git.Repository
	Branches []*Branch
	Remotes  []*Remote
	Stasheds []*StashedItem
	State    *RepositoryState

	mutex     *sync.RWMutex
	listeners map[string][]RepositoryListener
	queues    map[eventQueueType]*eventQueue
}

// RepositoryState is the current pointers of a repository
type RepositoryState struct {
	workStatus       WorkStatus
	Branch           *Branch
	Remote           *Remote
	Message          string
	RecoverableError bool
}

// RepositoryListener is a type for listeners
type RepositoryListener func(event *RepositoryEvent) error

// RepositoryEvent is used to transfer event-related data.
// It is passed to listeners when Publish() is called
type RepositoryEvent struct {
	Name    string
	Data    interface{}
	Context context.Context
}

// WorkStatus is the state of the repository for an operation
type WorkStatus struct {
	Status uint8
	Ready  bool
}

var (
	// Available implies repo is ready for the operation
	Available = WorkStatus{Status: 0, Ready: true}
	// Pending indicates repo is waiting for an operation to start
	Pending = WorkStatus{Status: 1, Ready: false}
	// Queued means repo is queued for a operation
	Queued = WorkStatus{Status: 2, Ready: false}
	// Working means an operation is just started for this repository
	Working = WorkStatus{Status: 3, Ready: false}
	// Paused is expected when a user interaction is required
	Paused = WorkStatus{Status: 4, Ready: true}
	// Success is the expected outcome of the operation
	Success = WorkStatus{Status: 5, Ready: true}
	// Fail is the unexpected outcome of the operation
	Fail = WorkStatus{Status: 6, Ready: false}
)

const (
	// RepositoryUpdated defines the topic for an updated repository.
	RepositoryUpdated = "repository.updated"
	// BranchUpdated defines the topic for an updated branch.
	BranchUpdated = "branch.updated"
	// RepositoryRefreshRequested signals that a repository should reload its metadata.
	RepositoryRefreshRequested = "repository.refresh.requested"
	// RepositoryEvaluationRequested signals that a repository's state should be re-evaluated.
	RepositoryEvaluationRequested = "repository.evaluation.requested"
	// RepositoryGitCommandRequested schedules execution of a git command on the git queue.
	RepositoryGitCommandRequested = "repository.git.command.requested"
	// RepositoryEventTraced is an internal event used to route trace logging messages.
	RepositoryEventTraced = "repository.event.traced"
)

type eventQueueType uint8

const (
	queueGit eventQueueType = iota
	queueState
	queueLog
)

type eventQueue struct {
	repo    *Repository
	kind    eventQueueType
	mu      sync.Mutex
	events  chan *RepositoryEvent
	started bool
	handler func(*RepositoryEvent)
}

const maxConcurrentGitWorkers = 10

var (
	gitQueueOnce      sync.Once
	gitQueueSemaphore *semaphore.Weighted
)

func gitSemaphore() *semaphore.Weighted {
	gitQueueOnce.Do(func() {
		workers := int64(runtime.GOMAXPROCS(0))
		if workers < 1 {
			workers = 1
		}
		if workers > maxConcurrentGitWorkers {
			workers = maxConcurrentGitWorkers
		}
		gitQueueSemaphore = semaphore.NewWeighted(workers)
	})
	return gitQueueSemaphore
}

// RepositoryHook represents a function that is executed after a repository has been initialized.
type RepositoryHook func(*Repository)

var (
	repositoryHooksMu sync.RWMutex
	repositoryHooks   []RepositoryHook
)

// RegisterRepositoryHook registers a hook that will run whenever a repository is created.
func RegisterRepositoryHook(h RepositoryHook) {
	if h == nil {
		return
	}
	repositoryHooksMu.Lock()
	repositoryHooks = append(repositoryHooks, h)
	repositoryHooksMu.Unlock()
}

func runRepositoryHooks(r *Repository) {
	repositoryHooksMu.RLock()
	defer repositoryHooksMu.RUnlock()
	for _, hook := range repositoryHooks {
		if hook != nil {
			hook(r)
		}
	}
}

// FastInitializeRepo initializes a Repository struct without its belongings.
func FastInitializeRepo(dir string) (r *Repository, err error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// get status of the file
	fstat, _ := f.Stat()
	rp, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	// initialize Repository with minimum viable fields
	r = &Repository{RepoID: RandomString(8),
		Name:    fstat.Name(),
		AbsPath: dir,
		ModTime: fstat.ModTime(),
		Repo:    *rp,
		State: &RepositoryState{
			workStatus:       Pending,
			Message:          "waiting",
			RecoverableError: false,
		},
		mutex:     &sync.RWMutex{},
		listeners: make(map[string][]RepositoryListener),
	}
	r.initEventQueues()
	runRepositoryHooks(r)
	return r, nil
}

// InitializeRepo initializes a Repository struct with its belongings.
func InitializeRepo(dir string) (r *Repository, err error) {
	r, err = FastInitializeRepo(dir)
	if err != nil {
		return nil, err
	}
	// need nothing extra but loading additional components
	return r, r.loadComponents(true)
}

// loadComponents initializes the fields of a repository such as branches,
// remotes, commits etc. If reset, reload commit, remote pointers too
func (r *Repository) loadComponents(reset bool) error {
	if err := r.initRemotes(); err != nil {
		return err
	}

	if err := r.initBranches(); err != nil {
		return err
	}

	if err := r.SyncRemoteAndBranch(r.State.Branch); err != nil {
		return err
	}
	return r.loadStashedItems()
}

// Refresh the belongings of a repository, this function is called right after
// fetch/pull/merge operations
func (r *Repository) Refresh() error {
	// if the Repository is only fast initialized, no need to refresh because
	// it won't contain its belongings
	if r.State.Branch == nil {
		return nil
	}

	// re-initialize the go-git repository struct
	rp, err := git.PlainOpen(r.AbsPath)
	if err != nil {
		return err
	}
	r.Repo = *rp

	if fstat, err := os.Stat(r.AbsPath); err == nil {
		r.ModTime = fstat.ModTime()
	}

	if err := r.loadComponents(false); err != nil {
		return err
	}

	return nil
}

// RequestRefresh schedules a metadata refresh via the repository's event queue.
func (r *Repository) RequestRefresh() error {
	return r.Publish(RepositoryRefreshRequested, nil)
}

func (r *Repository) initEventQueues() {
	if r.queues == nil {
		r.queues = make(map[eventQueueType]*eventQueue)
	}
	r.queues[queueGit] = newEventQueue(r, queueGit)
	r.queues[queueState] = newEventQueue(r, queueState)
	if isTraceEnabled() {
		r.queues[queueLog] = newEventQueue(r, queueLog)
	}
}

func newEventQueue(repo *Repository, kind eventQueueType) *eventQueue {
	q := &eventQueue{
		repo: repo,
		kind: kind,
	}
	switch kind {
	case queueGit:
		q.handler = q.handleGitEvent
		q.events = make(chan *RepositoryEvent, 64)
		go q.run()
	case queueState:
		// State queue needs async processing to avoid blocking during batch evaluation
		q.handler = q.handleStateEvent
		q.events = make(chan *RepositoryEvent, 64)
		go q.run()
	case queueLog:
		q.handler = q.handleLogEvent
		q.events = make(chan *RepositoryEvent, 128)
		go q.run()
	default:
		// Unknown queue types fall back to synchronous dispatch
	}
	return q
}

// On adds new listener.
// listener is a callback function that will be called when event emits
func (r *Repository) On(event string, listener RepositoryListener) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	// add listener to the specific event topic
	r.listeners[event] = append(r.listeners[event], listener)
}

// Publish publishes the data to a certain event by its name.
// Events are either queued for async dispatch or handled synchronously.
func (r *Repository) Publish(eventName string, data interface{}) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	if eventName == "" {
		return fmt.Errorf("event name required")
	}
	event := &RepositoryEvent{Name: eventName, Data: data, Context: context.Background()}

	queue := r.queueForEvent(eventName)
	if queue == nil {
		// Handle synchronously for events with lightweight listeners
		listeners := r.listenersFor(eventName)
		for _, listener := range listeners {
			if err := listener(event); err != nil {
				return err
			}
		}
		r.traceEvent(eventName, queueState, data)
		return nil
	}

	if err := queue.enqueue(event); err != nil {
		return err
	}
	r.traceEvent(eventName, queue.kind, data)
	return nil
}

func (r *Repository) listenersFor(eventName string) []RepositoryListener {
	if r == nil || r.mutex == nil {
		return nil
	}
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	listeners := r.listeners[eventName]
	if len(listeners) == 0 {
		return nil
	}
	cloned := make([]RepositoryListener, len(listeners))
	copy(cloned, listeners)
	return cloned
}

func (r *Repository) queueForEvent(eventName string) *eventQueue {
	if r == nil {
		return nil
	}
	switch eventName {
	case RepositoryGitCommandRequested:
		return r.queues[queueGit]
	case RepositoryEvaluationRequested:
		return r.queues[queueState]
	case RepositoryEventTraced:
		return r.queues[queueLog]
	default:
		// RepositoryRefreshRequested, RepositoryUpdated, BranchUpdated
		// These events have lightweight listeners (non-blocking channel sends)
		// so we handle them synchronously without a queue
		return nil
	}
}

func (q *eventQueue) enqueue(event *RepositoryEvent) error {
	if q == nil {
		return fmt.Errorf("event queue not initialized")
	}
	if q.events != nil {
		if event.Context == nil {
			event.Context = context.Background()
		}
		q.events <- event
		return nil
	}
	// For queues without channels, dispatch synchronously
	if event.Context == nil {
		event.Context = context.Background()
	}
	return q.dispatch(event)
}

func (q *eventQueue) run() {
	for event := range q.events {
		if q.handler != nil {
			q.handler(event)
		}
	}
}

func (q *eventQueue) handleGitEvent(event *RepositoryEvent) {
	sem := gitSemaphore()
	if err := sem.Acquire(context.Background(), 1); err != nil {
		log.Printf("git queue acquire failed: %v", err)
		return
	}
	go func() {
		defer sem.Release(1)
		event.Context = context.Background()
		if err := q.dispatch(event); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			log.Printf("git queue event %s failed: %v", event.Name, err)
		}
	}()
}

func (q *eventQueue) handleLogEvent(event *RepositoryEvent) {
	if event.Context == nil {
		event.Context = context.Background()
	}
	if err := q.dispatch(event); err != nil && err != context.Canceled {
		log.Printf("log queue event %s failed: %v", event.Name, err)
	}
}

func (q *eventQueue) handleStateEvent(event *RepositoryEvent) {
	if event.Context == nil {
		event.Context = context.Background()
	}
	if err := q.dispatch(event); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Printf("state queue event %s failed: %v", event.Name, err)
	}
}

func (q *eventQueue) dispatch(event *RepositoryEvent) error {
	if event.Context == nil {
		event.Context = context.Background()
	}
	listeners := q.repo.listenersFor(event.Name)
	if len(listeners) == 0 {
		return nil
	}
	for _, listener := range listeners {
		if err := listener(event); err != nil {
			return err
		}
	}
	return nil
}

// WorkStatus returns the state of the repository such as queued, failed etc.
func (r *Repository) WorkStatus() WorkStatus {
	return r.State.workStatus
}

// SetWorkStatus sets the state of repository and sends repository updated event
func (r *Repository) SetWorkStatus(ws WorkStatus) {
	if r.State == nil {
		return
	}
	prev := r.State.workStatus
	r.State.workStatus = ws
	if ws != Fail {
		r.State.RecoverableError = false
	}
	if prev == ws {
		return
	}
	r.NotifyRepositoryUpdated()
}

// NotifyRepositoryUpdated emits a repository.updated event without mutating state.
func (r *Repository) NotifyRepositoryUpdated() {
	if r == nil {
		return
	}
	_ = r.Publish(RepositoryUpdated, nil)
}

// MarkFailure is preserved for backward compatibility. Prefer using
// MarkCriticalError or MarkRecoverableError for clarity.
func (r *Repository) MarkFailure(message string, recoverable bool) {
	r.markErrorState(message, recoverable)
}

func (r *Repository) String() string {
	return r.Name
}

func Create(dir string) (*Repository, error) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	_, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return InitializeRepo(dir)
}
