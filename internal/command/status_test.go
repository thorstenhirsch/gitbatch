package command

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

func TestStatusWithGit(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	_, err := testFile(th.RepoPath, "file")
	require.NoError(t, err)

	var tests = []struct {
		input *git.Repository
	}{
		{th.Repository},
	}
	for _, test := range tests {
		output, err := statusWithGit(test.input)
		require.NoError(t, err)
		require.NotEmpty(t, output)
	}
}
