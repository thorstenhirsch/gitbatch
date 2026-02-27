package job

import (
	"fmt"
	"sync"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// Queue holds the slice of Jobs
type Queue struct {
	series []*Job
	index  map[string]*Job
	mu     sync.Mutex
}

func jobKey(repoID string, jobType Type) string {
	return repoID + ":" + string(jobType)
}

// CreateJobQueue creates a jobqueue struct and initialize its slice then return
// its pointer
func CreateJobQueue() (jq *Queue) {
	return &Queue{
		series: make([]*Job, 0),
		index:  make(map[string]*Job),
	}
}

// AddJob adds a job to the queue
func (jq *Queue) AddJob(j *Job) error {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	key := jobKey(j.Repository.RepoID, j.JobType)
	if _, exists := jq.index[key]; exists {
		return fmt.Errorf("same job already is in the queue")
	}
	jq.series = append(jq.series, j)
	jq.index[key] = j
	return nil
}

// StartNext starts the next job in the queue
func (jq *Queue) StartNext() (j *Job, finished bool, err error) {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	finished = false
	if len(jq.series) < 1 {
		finished = true
		return nil, finished, nil
	}
	i := len(jq.series) - 1
	lastJob := jq.series[i]
	jq.series = jq.series[:i]
	key := jobKey(lastJob.Repository.RepoID, lastJob.JobType)
	delete(jq.index, key)
	if err = lastJob.Start(); err != nil {
		return lastJob, finished, err
	}
	return lastJob, finished, nil
}

// RemoveFromQueue deletes the given entity and its job from the queue
func (jq *Queue) RemoveFromQueue(r *git.Repository) error {
	jq.mu.Lock()
	defer jq.mu.Unlock()

	removed := false
	newSeries := jq.series[:0]
	for _, job := range jq.series {
		if job.Repository.RepoID == r.RepoID {
			key := jobKey(job.Repository.RepoID, job.JobType)
			delete(jq.index, key)
			removed = true
		} else {
			newSeries = append(newSeries, job)
		}
	}
	jq.series = newSeries
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

	for _, job := range jq.index {
		if job.Repository.RepoID == r.RepoID {
			return true, job
		}
	}
	return false, nil
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
