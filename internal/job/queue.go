package job

import (
	"fmt"
	"sync"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// Queue holds the slice of Jobs
type Queue struct {
	series []*Job
	mu     sync.Mutex
}

// CreateJobQueue creates a jobqueue struct and initialize its slice then return
// its pointer
func CreateJobQueue() (jq *Queue) {
	s := make([]*Job, 0)
	return &Queue{
		series: s,
	}
}

// AddJob adds a job to the queue
func (jq *Queue) AddJob(j *Job) error {
	if j == nil || j.Repository == nil {
		return fmt.Errorf("job or repository not initialized")
	}
	jq.mu.Lock()
	defer jq.mu.Unlock()
	for _, job := range jq.series {
		if job == nil || job.Repository == nil {
			continue
		}
		if job.Repository.RepoID == j.Repository.RepoID && job.JobType == j.JobType {
			return fmt.Errorf("same job already is in the queue")
		}
	}
	jq.series = append(jq.series, nil)
	copy(jq.series[1:], jq.series[0:])
	jq.series[0] = j
	return nil
}

// StartNext starts the next job in the queue
func (jq *Queue) StartNext() (j *Job, finished bool, err error) {
	jq.mu.Lock()
	if len(jq.series) < 1 {
		jq.mu.Unlock()
		return nil, true, nil
	}
	i := len(jq.series) - 1
	lastJob := jq.series[i]
	jq.series = jq.series[:i]
	jq.mu.Unlock()

	if lastJob == nil {
		return nil, false, fmt.Errorf("unexpected nil job in queue")
	}
	if err = lastJob.Start(); err != nil {
		return lastJob, finished, err
	}
	return lastJob, finished, nil
}

// RemoveFromQueue deletes the given entity and its job from the queue
func (jq *Queue) RemoveFromQueue(r *git.Repository) error {
	if r == nil {
		return fmt.Errorf("repository not initialized")
	}
	jq.mu.Lock()
	defer jq.mu.Unlock()
	removed := false
	for i := len(jq.series) - 1; i >= 0; i-- {
		job := jq.series[i]
		if job == nil || job.Repository == nil {
			continue
		}
		if job.Repository.RepoID == r.RepoID {
			if job.Repository.WorkStatus() == git.Working {
				return fmt.Errorf("cannot remove a job that already started")
			}
			jq.series = append(jq.series[:i], jq.series[i+1:]...)
			removed = true
		}
	}
	if !removed {
		return fmt.Errorf("there is no job with given repoID")
	}
	return nil
}

// IsInTheQueue function; since the job and entity is not tied with its own
// struct, this function returns true if that entity is in the queue along with
// the jobs type
func (jq *Queue) IsInTheQueue(r *git.Repository) (inTheQueue bool, j *Job) {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	inTheQueue = false
	if r == nil {
		return inTheQueue, nil
	}
	jq.mu.Lock()
	defer jq.mu.Unlock()
	for _, job := range jq.series {
		if job == nil || job.Repository == nil {
			continue
		}
		if job.Repository.RepoID == r.RepoID {
			inTheQueue = true
			j = job
		}
	}
	return inTheQueue, j
}

// StartJobsAsync starts all jobs in the queue asynchronously.
// Concurrency is limited by the git queue semaphore in internal/git/repository.go,
// which caps concurrent git operations globally across all repositories.
func (jq *Queue) StartJobsAsync() map[*Job]error {
	jq.mu.Lock()
	if len(jq.series) == 0 {
		jq.mu.Unlock()
		return make(map[*Job]error)
	}
	jobCount := len(jq.series)
	jq.mu.Unlock()

	fails := make(map[*Job]error)
	var mx sync.Mutex
	var wg sync.WaitGroup

	// Start all jobs; git queue will handle concurrency limiting
	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			j, finished, err := jq.StartNext()
			if finished {
				return
			}
			if err != nil {
				mx.Lock()
				fails[j] = err
				mx.Unlock()
			}
		}()
	}

	wg.Wait()
	return fails
}
