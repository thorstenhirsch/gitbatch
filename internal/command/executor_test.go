package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

func TestNormalizePushOptionsUsesRepositoryDefaults(t *testing.T) {
	repo := &git.Repository{
		State: &git.RepositoryState{
			Remote: &git.Remote{Name: "upstream"},
			Branch: &git.Branch{Name: "feature/refactor"},
		},
	}

	opts := normalizePushOptions(nil, repo)

	require.Equal(t, "upstream", opts.RemoteName)
	require.Equal(t, "feature/refactor", opts.ReferenceName)
}

func TestPrepareFetchWithoutUpstreamProducesNoUpstreamOutcome(t *testing.T) {
	repo := &git.Repository{
		RepoID: "repo-1",
		State: &git.RepositoryState{
			Branch: &git.Branch{Name: "main"},
			Remote: &git.Remote{Name: "origin"},
		},
	}

	plan := NewExecutor(repo).prepareFetch(nil)

	require.NotNil(t, plan.immediate)
	require.Equal(t, OperationNoUpstream, plan.immediate.Operation)
	require.EqualError(t, plan.immediate.Err, "upstream not configured")
}

func TestExecutorRunPushUsesRepositoryDefaults(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	tempDir, err := os.MkdirTemp("", "gitbatch-executor-remote")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	_, err = Run(tempDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	remotePath, err := filepath.Abs(tempDir)
	require.NoError(t, err)

	_, _ = Run(th.Repository.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = Run(th.Repository.AbsPath, "git", []string{"remote", "add", "origin", remotePath})
	require.NoError(t, err)

	th.Repository.State.Remote = &git.Remote{Name: "origin"}

	err = NewExecutor(th.Repository).RunPush(nil, nil, false)
	require.NoError(t, err)
}
