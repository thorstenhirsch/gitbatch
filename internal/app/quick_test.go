package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

func TestQuick(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	var tests = []struct {
		inp1 []string
		inp2 string
	}{
		{
			[]string{th.DirtyRepoPath()},
			"fetch",
		}, {
			[]string{th.DirtyRepoPath()},
			"pull",
		},
	}
	for _, test := range tests {
		err := quick(test.inp1, test.inp2)
		require.NoError(t, err)
	}
}

func TestOperatePush(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	tempDir, err := os.MkdirTemp("", "gitbatch-quick-remote")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	_, err = command.Run(tempDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	remotePath, err := filepath.Abs(tempDir)
	require.NoError(t, err)

	_, _ = command.Run(th.Repository.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = command.Run(th.Repository.AbsPath, "git", []string{"remote", "add", "origin", remotePath})
	require.NoError(t, err)

	err = operate(th.Repository.AbsPath, "push")
	require.NoError(t, err)
}
