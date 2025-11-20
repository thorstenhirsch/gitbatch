package command

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

func TestMerge(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	opts := &MergeOptions{
		BranchName: th.Repository.State.Branch.Upstream.Name,
	}
	var tests = []struct {
		inp1 *git.Repository
		inp2 *MergeOptions
	}{
		{th.Repository, opts},
	}
	for _, test := range tests {
		_, err := Merge(test.inp1, test.inp2)
		require.NoError(t, err)
	}
}
