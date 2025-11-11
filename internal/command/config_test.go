package command

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func TestConfigWithGit(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	testConfigopt := &ConfigOptions{
		Section: "remote.origin",
		Option:  "url",
		Site:    ConfigSiteLocal,
	}

	var tests = []struct {
		inp1     *git.Repository
		inp2     *ConfigOptions
		expected string
	}{
		{th.Repository, testConfigopt, "https://gitlab.com/isacikgoz/test-data.git"},
	}
	for _, test := range tests {
		output, err := configWithGit(test.inp1, test.inp2)
		require.NoError(t, err)
		require.Equal(t, test.expected, output)
	}
}
