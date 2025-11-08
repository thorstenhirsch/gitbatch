package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestPush(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	tempDir, err := os.MkdirTemp("", "gitbatch-remote")
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

	branchName := ""
	if th.Repository.State != nil && th.Repository.State.Branch != nil {
		branchName = th.Repository.State.Branch.Name
	}
	require.NotEmpty(t, branchName)

	opts := &PushOptions{
		RemoteName:    "origin",
		ReferenceName: branchName,
		CommandMode:   ModeLegacy,
	}

	err = Push(th.Repository, opts)
	require.NoError(t, err)
}
