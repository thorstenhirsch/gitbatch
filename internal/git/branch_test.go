package git

import (
	"fmt"
	"os/exec"
	"strings"
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

func TestGetUpstreamHandlesMissingRemoteBranch(t *testing.T) {
	th := InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	r := th.Repository
	branchName := r.State.Branch.Name
	remoteName := r.State.Remote.Name

	// Capture original merge config to restore after the test.
	readCmd := exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.merge", branchName))
	readCmd.Dir = r.AbsPath
	originalMergeBytes, err := readCmd.CombinedOutput()
	require.NoError(t, err)
	originalMerge := strings.TrimSpace(string(originalMergeBytes))

	defer func() {
		restore := exec.Command("git", "config", fmt.Sprintf("branch.%s.merge", branchName), originalMerge)
		restore.Dir = r.AbsPath
		_ = restore.Run()
	}()

	missingBranch := "__gitbatch_missing_branch__"
	updateCmd := exec.Command("git", "config", fmt.Sprintf("branch.%s.merge", branchName), "refs/heads/"+missingBranch)
	updateCmd.Dir = r.AbsPath
	require.NoError(t, updateCmd.Run())

	rb, err := getUpstream(r, branchName)
	require.NoError(t, err)
	require.NotNil(t, rb)
	require.Equal(t, fmt.Sprintf("%s/%s", remoteName, missingBranch), rb.Name)
	require.Nil(t, rb.Reference)

	// Assign the placeholder upstream and ensure diff helpers degrade gracefully.
	r.State.Branch.Upstream = rb
	_, err = r.State.Branch.pullDiffsToUpstream(r)
	require.NoError(t, err)
	_, err = r.State.Branch.pushDiffsToUpstream(r)
	require.NoError(t, err)
}

func TestNormalizeMergeBranchName(t *testing.T) {
	cases := []struct {
		remote   string
		branch   string
		mergeRef string
		expected string
	}{
		{remote: "origin", branch: "main", mergeRef: "refs/heads/main", expected: "main"},
		{remote: "origin", branch: "main", mergeRef: "refs/remotes/origin/feature/foo", expected: "feature/foo"},
		{remote: "", branch: "develop", mergeRef: "refs/heads/release", expected: "release"},
		{remote: "origin", branch: "trunk", mergeRef: "origin/custom", expected: "custom"},
	}

	for _, tc := range cases {
		result := normalizeMergeBranchName(tc.remote, tc.branch, tc.mergeRef)
		require.Equal(t, tc.expected, result)
	}
}
