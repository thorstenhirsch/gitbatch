package job

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestStartNextEmptyQueue(t *testing.T) {
	queue := CreateJobQueue()

	started, err := queue.StartNext()

	require.NoError(t, err)
	require.Nil(t, started)
}

func TestStartNextReleasesQueueLock(t *testing.T) {
	queue := CreateJobQueue()
	first := &Job{
		JobType: Type("unknown"),
		Repository: &git.Repository{
			RepoID: "repo-1",
			State:  &git.RepositoryState{},
		},
	}
	require.NoError(t, queue.AddJob(first))

	started, err := queue.StartNext()
	require.NoError(t, err)
	require.Same(t, first, started)

	done := make(chan error, 1)
	go func() {
		done <- queue.AddJob(&Job{
			JobType: Type("unknown"),
			Repository: &git.Repository{
				RepoID: "repo-2",
				State:  &git.RepositoryState{},
			},
		})
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("AddJob blocked after StartNext")
	}
}

func TestStartJobsAsyncCollectsFailures(t *testing.T) {
	queue := CreateJobQueue()
	successJob := &Job{
		JobType: Type("unknown"),
		Repository: &git.Repository{
			RepoID: "repo-success",
			State:  &git.RepositoryState{},
		},
	}
	failingJob := &Job{
		JobType: Type("unknown"),
		Repository: &git.Repository{
			RepoID: "repo-fail",
		},
	}

	require.NoError(t, queue.AddJob(successJob))
	require.NoError(t, queue.AddJob(failingJob))

	failures := queue.StartJobsAsync()

	require.Len(t, failures, 1)
	require.EqualError(t, failures[failingJob], "repository state not initialized")
}
