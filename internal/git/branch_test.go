package git

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNextBranch(t *testing.T) {

}

func TestPreviousBranch(t *testing.T) {

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

func TestRevListCount(t *testing.T) {
	th := InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	r := th.Repository
	headref, err := r.Repo.Head()
	require.NoError(t, err)

	head := headref.Hash().String()

	// Test pullables count (should be 0 for up-to-date branch)
	pullCount, err := RevListCount(r, RevListOptions{
		Ref1: head,
		Ref2: r.State.Branch.Upstream.Reference.Hash().String(),
	})
	require.NoError(t, err)
	require.Equal(t, 0, pullCount)

	// Test pushables count (should be 0 for up-to-date branch)
	pushCount, err := RevListCount(r, RevListOptions{
		Ref1: r.State.Branch.Upstream.Reference.Hash().String(),
		Ref2: head,
	})
	require.NoError(t, err)
	require.Equal(t, 0, pushCount)
}

func TestSyncRemoteAndBranchResilience(t *testing.T) {
	th := InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	r := th.Repository

	// Test with valid upstream
	err := r.SyncRemoteAndBranch(r.State.Branch)
	require.NoError(t, err)
	require.NotEqual(t, "?", r.State.Branch.Pullables)
	require.NotEqual(t, "?", r.State.Branch.Pushables)

	// Test with nil upstream
	originalUpstream := r.State.Branch.Upstream
	r.State.Branch.Upstream = nil
	err = r.SyncRemoteAndBranch(r.State.Branch)
	require.NoError(t, err)
	require.Equal(t, "?", r.State.Branch.Pullables)
	require.Equal(t, "?", r.State.Branch.Pushables)

	// Restore upstream
	r.State.Branch.Upstream = originalUpstream
}
