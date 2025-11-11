package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

var (
	testPullopts1 = &PullOptions{
		RemoteName: "origin",
	}

	testPullopts2 = &PullOptions{
		RemoteName: "origin",
		Force:      true,
	}
)

func TestPullWithGit(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	var tests = []struct {
		inp1 *git.Repository
		inp2 *PullOptions
	}{
		{th.Repository, testPullopts1},
		{th.Repository, testPullopts2},
	}
	for _, test := range tests {
		_, err := pullWithGit(context.Background(), test.inp1, test.inp2)
		require.NoError(t, err)
	}
}
