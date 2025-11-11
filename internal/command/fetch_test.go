package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

var (
	testFetchopts1 = &FetchOptions{
		RemoteName: "origin",
	}

	testFetchopts2 = &FetchOptions{
		RemoteName: "origin",
		Prune:      true,
	}

	testFetchopts3 = &FetchOptions{
		RemoteName: "origin",
		DryRun:     true,
	}
)

func TestFetchWithGit(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	var tests = []struct {
		inp1 *git.Repository
		inp2 *FetchOptions
	}{
		{th.Repository, testFetchopts1},
		{th.Repository, testFetchopts2},
		{th.Repository, testFetchopts3},
	}
	for _, test := range tests {
		_, err := fetchWithGit(context.Background(), test.inp1, test.inp2)
		require.NoError(t, err)
	}
}
