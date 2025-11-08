package job

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestStart(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	mockJob1 := &Job{
		JobType:    PullJob,
		Repository: th.Repository,
	}
	mockJob2 := &Job{
		JobType:    FetchJob,
		Repository: th.Repository,
	}
	mockJob3 := &Job{
		JobType:    MergeJob,
		Repository: th.Repository,
	}
	mockJob4 := &Job{
		JobType:    RebaseJob,
		Repository: th.Repository,
	}
	mockJob5 := &Job{
		JobType:    PushJob,
		Repository: th.Repository,
	}

	var tests = []struct {
		input *Job
	}{
		{mockJob1},
		{mockJob2},
		{mockJob3},
		{mockJob4},
		{mockJob5},
	}
	for _, test := range tests {
		if test.input.JobType == PushJob {
			test.input.Repository.State.Remote = nil
		}
		err := test.input.start()
		require.NoError(t, err)
	}
}
