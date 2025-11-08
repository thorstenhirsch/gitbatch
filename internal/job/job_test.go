package job

import (
	"fmt"
	"os/exec"
	"strings"
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

func TestFetchJobPreservesRecoverableState(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	readCmd := exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.merge", branchName))
	readCmd.Dir = repo.AbsPath
	originalMergeBytes, err := readCmd.CombinedOutput()
	require.NoError(t, err)
	originalMerge := strings.TrimSpace(string(originalMergeBytes))

	missingBranch := "__gitbatch_missing_branch__"
	updateCmd := exec.Command("git", "config", fmt.Sprintf("branch.%s.merge", branchName), "refs/heads/"+missingBranch)
	updateCmd.Dir = repo.AbsPath
	require.NoError(t, updateCmd.Run())

	defer func() {
		restore := exec.Command("git", "config", fmt.Sprintf("branch.%s.merge", branchName), originalMerge)
		restore.Dir = repo.AbsPath
		_ = restore.Run()
	}()

	require.NoError(t, repo.ForceRefresh())
	require.NotNil(t, repo.State.Branch.Upstream)
	require.Nil(t, repo.State.Branch.Upstream.Reference)

	job := &Job{JobType: FetchJob, Repository: repo}
	require.NoError(t, job.start())

	status := repo.WorkStatus()
	require.Equal(t, git.Fail, status)
	require.True(t, repo.State.RecoverableError)
	require.Contains(t, strings.ToLower(repo.State.Message), "upstream")
}
