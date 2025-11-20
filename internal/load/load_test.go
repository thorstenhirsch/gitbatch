package load

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

func TestSyncLoad(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	var tests = []struct {
		input []string
	}{
		{[]string{th.BasicRepoPath(), th.DirtyRepoPath()}},
	}
	for _, test := range tests {
		output, err := SyncLoad(test.input)
		require.NoError(t, err)
		require.NotEmpty(t, output)
	}
}
