package git

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamBranchName(t *testing.T) {
	tests := []struct {
		name string
		repo *Repository
		want string
	}{
		{
			name: "nil repo",
			repo: nil,
			want: "",
		},
		{
			name: "nil state",
			repo: &Repository{State: nil},
			want: "",
		},
		{
			name: "nil branch",
			repo: &Repository{State: &RepositoryState{}},
			want: "",
		},
		{
			name: "no upstream falls back to branch name",
			repo: &Repository{State: &RepositoryState{
				Branch: &Branch{Name: "main"},
			}},
			want: "main",
		},
		{
			name: "upstream with remote/branch format",
			repo: &Repository{State: &RepositoryState{
				Branch: &Branch{
					Name:     "main",
					Upstream: &RemoteBranch{Name: "origin/main"},
				},
			}},
			want: "main",
		},
		{
			name: "upstream with nested branch name",
			repo: &Repository{State: &RepositoryState{
				Branch: &Branch{
					Name:     "feature/test",
					Upstream: &RemoteBranch{Name: "origin/feature/test"},
				},
			}},
			want: "feature/test",
		},
		{
			name: "upstream with empty name falls back to branch name",
			repo: &Repository{State: &RepositoryState{
				Branch: &Branch{
					Name:     "develop",
					Upstream: &RemoteBranch{Name: ""},
				},
			}},
			want: "develop",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpstreamBranchName(tt.repo)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRevlistNew(t *testing.T) {
	th := InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	r := th.Repository
	// HEAD..@{u}
	headref, err := r.Repo.Head()
	if err != nil {
		t.Fatalf("Test Failed. error: %s", err.Error())
	}

	head := headref.Hash().String()

	pullables, err := RevList(r, RevListOptions{
		Ref1: head,
		Ref2: r.State.Branch.Upstream.Reference.Hash().String(),
	})
	require.NoError(t, err)
	require.Empty(t, pullables)

	pushables, err := RevList(r, RevListOptions{
		Ref1: r.State.Branch.Upstream.Reference.Hash().String(),
		Ref2: head,
	})
	require.NoError(t, err)
	require.Empty(t, pushables)
}
