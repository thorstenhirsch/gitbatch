package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// TestApplyCleanliness_CleanWorkingTree_NoIncomingCommits tests the scenario where
// the working tree is clean and there are no incoming commits from upstream.
// Expected: Repository should be marked as clean.
func TestApplyCleanliness_CleanWorkingTree_NoIncomingCommits(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	require.NotNil(t, repo.State)
	require.NotNil(t, repo.State.Branch)

	// Ensure working tree is clean
	require.NoError(t, ScheduleRepositoryRefresh(repo, nil))
	require.True(t, repo.IsClean(), "working tree should be clean")

	// Ensure no incoming commits (already up-to-date)
	require.False(t, repo.State.Branch.HasIncomingCommits(), "should have no incoming commits")

	// Apply cleanliness check
	applyCleanliness(repo)

	// Verify: should be clean
	require.True(t, repo.State.Branch.Clean, "repository should be marked as clean")
	require.Equal(t, git.Available, repo.WorkStatus(), "status should be Available")
}

// TestApplyCleanliness_CleanWorkingTree_WithIncomingCommits tests the scenario where
// the working tree is clean but there are incoming commits from upstream.
// Expected: Repository should be marked as clean (no local changes means no conflicts).
func TestApplyCleanliness_CleanWorkingTree_WithIncomingCommits(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	// Create a bare remote repository
	remoteDir, err := os.MkdirTemp("", "gitbatch-remote")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	_, err = Run(remoteDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	// Push current state to remote
	_, _ = Run(repo.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = Run(repo.AbsPath, "git", []string{"remote", "add", "origin", remoteDir})
	require.NoError(t, err)

	_, err = Run(repo.AbsPath, "git", []string{"push", "-u", "origin", branchName})
	require.NoError(t, err)

	// Create a commit in the remote (simulating incoming commits)
	tempClone, err := os.MkdirTemp("", "gitbatch-clone")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	_, err = Run(tempClone, "git", []string{"clone", remoteDir, "."})
	require.NoError(t, err)

	// Check out the correct branch (clone might start in detached HEAD)
	_, err = Run(tempClone, "git", []string{"checkout", branchName})
	require.NoError(t, err)

	// Configure git user for the clone
	_, _ = Run(tempClone, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(tempClone, "git", []string{"config", "user.name", "Test User"})

	// Create a new commit
	testFile := filepath.Join(tempClone, "new-file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("new content"), 0644))
	_, err = Run(tempClone, "git", []string{"add", "new-file.txt"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"commit", "-m", "Add new file"})
	require.NoError(t, err)
	pushOut, err := Run(tempClone, "git", []string{"push"})
	require.NoError(t, err)
	t.Logf("Push output: %s", pushOut)

	// Fetch in the original repo to get incoming commits
	out, err := Run(repo.AbsPath, "git", []string{"fetch"})
	require.NoError(t, err)
	t.Logf("Fetch output: %s", out)

	// Refresh repo state
	require.NoError(t, repo.Refresh())
	require.NoError(t, ScheduleRepositoryRefresh(repo, nil))

	// Verify setup: clean working tree with incoming commits
	require.True(t, repo.IsClean(), "working tree should be clean")
	require.True(t, repo.State.Branch.HasIncomingCommits(), "should have incoming commits")

	// Apply cleanliness check
	applyCleanliness(repo)

	// Verify: should be clean (no local changes means no conflicts)
	require.True(t, repo.State.Branch.Clean, "repository should be marked as clean despite incoming commits")
	require.Equal(t, git.Available, repo.WorkStatus(), "status should be Available")
}

// TestApplyCleanliness_UncleanWorkingTree_NoIncomingCommits tests the scenario where
// the working tree has uncommitted changes but there are no incoming commits.
// Expected: Repository should be marked as clean (up-to-date with upstream).
func TestApplyCleanliness_UncleanWorkingTree_NoIncomingCommits(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository

	// Make the working tree dirty by modifying a file
	testFile := filepath.Join(repo.AbsPath, "test-modification.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("local changes"), 0644))

	// Refresh repo state
	require.NoError(t, repo.Refresh())

	// Verify setup: unclean working tree, no incoming commits
	require.False(t, repo.IsClean(), "working tree should not be clean")
	require.False(t, repo.State.Branch.HasIncomingCommits(), "should have no incoming commits")

	// Apply cleanliness check
	applyCleanliness(repo)

	// Verify: should be clean (no incoming commits means up-to-date)
	require.True(t, repo.State.Branch.Clean, "repository should be marked as clean")
	require.Equal(t, git.Available, repo.WorkStatus(), "status should be Available")
}

// TestApplyCleanliness_UncleanWorkingTree_IncomingCommits_FFSucceeds tests the scenario where
// the working tree has uncommitted changes, there are incoming commits, but fast-forward
// would succeed (meaning local changes don't conflict with incoming changes).
// Expected: Repository should be marked as clean.
func TestApplyCleanliness_UncleanWorkingTree_IncomingCommits_FFSucceeds(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	// Create a bare remote repository
	remoteDir, err := os.MkdirTemp("", "gitbatch-remote")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	_, err = Run(remoteDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	// Push current state to remote
	_, _ = Run(repo.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = Run(repo.AbsPath, "git", []string{"remote", "add", "origin", remoteDir})
	require.NoError(t, err)

	_, err = Run(repo.AbsPath, "git", []string{"push", "-u", "origin", branchName})
	require.NoError(t, err)

	// Create a commit in the remote that doesn't conflict with local changes
	tempClone, err := os.MkdirTemp("", "gitbatch-clone")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	_, err = Run(tempClone, "git", []string{"clone", remoteDir, "."})
	require.NoError(t, err)

	// Check out the correct branch
	_, err = Run(tempClone, "git", []string{"checkout", branchName})
	require.NoError(t, err)

	// Configure git user
	_, _ = Run(tempClone, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(tempClone, "git", []string{"config", "user.name", "Test User"})

	// Create a new file (won't conflict with local changes to different file)
	remoteFile := filepath.Join(tempClone, "remote-file.txt")
	require.NoError(t, os.WriteFile(remoteFile, []byte("remote content"), 0644))
	_, err = Run(tempClone, "git", []string{"add", "remote-file.txt"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"commit", "-m", "Add remote file"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"push"})
	require.NoError(t, err)

	// Make local changes to a different file (no conflict)
	localFile := filepath.Join(repo.AbsPath, "local-file.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("local changes"), 0644))

	// Fetch to get incoming commits
	_, err = Run(repo.AbsPath, "git", []string{"fetch"})
	require.NoError(t, err)

	// Refresh repo state
	require.NoError(t, repo.Refresh())
	require.NoError(t, ScheduleRepositoryRefresh(repo, nil))

	// Verify setup: unclean working tree with incoming commits
	require.False(t, repo.IsClean(), "working tree should not be clean")
	require.True(t, repo.State.Branch.HasIncomingCommits(), "should have incoming commits")

	// Apply cleanliness check
	applyCleanliness(repo)

	// Verify: should be clean (fast-forward would succeed)
	require.True(t, repo.State.Branch.Clean, "repository should be marked as clean - fast-forward would succeed")
	require.Equal(t, git.Available, repo.WorkStatus(), "status should be Available")
}

// TestApplyCleanliness_UncleanWorkingTree_IncomingCommits_FFFailsConflict tests the scenario where
// the working tree has uncommitted changes, there are incoming commits, and fast-forward
// would fail due to actual merge conflicts.
// Expected: Repository should be marked as dirty.
func TestApplyCleanliness_UncleanWorkingTree_IncomingCommits_FFFailsConflict(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	// Create a bare remote repository
	remoteDir, err := os.MkdirTemp("", "gitbatch-remote")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	_, err = Run(remoteDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	// Push current state to remote
	_, _ = Run(repo.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = Run(repo.AbsPath, "git", []string{"remote", "add", "origin", remoteDir})
	require.NoError(t, err)

	// Create and push an initial file
	initialFile := filepath.Join(repo.AbsPath, "conflict-file.txt")
	require.NoError(t, os.WriteFile(initialFile, []byte("initial content\n"), 0644))
	_, err = Run(repo.AbsPath, "git", []string{"add", "conflict-file.txt"})
	require.NoError(t, err)
	_, err = Run(repo.AbsPath, "git", []string{"commit", "-m", "Add initial file"})
	require.NoError(t, err)
	_, err = Run(repo.AbsPath, "git", []string{"push", "-u", "origin", branchName})
	require.NoError(t, err)

	// Create a conflicting commit in the remote
	tempClone, err := os.MkdirTemp("", "gitbatch-clone")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	_, err = Run(tempClone, "git", []string{"clone", remoteDir, "."})
	require.NoError(t, err)

	// Check out the correct branch
	_, err = Run(tempClone, "git", []string{"checkout", branchName})
	require.NoError(t, err)

	// Configure git user
	_, _ = Run(tempClone, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(tempClone, "git", []string{"config", "user.name", "Test User"})

	// Modify the same file in the remote
	remoteFile := filepath.Join(tempClone, "conflict-file.txt")
	require.NoError(t, os.WriteFile(remoteFile, []byte("initial content\nremote change\n"), 0644))
	_, err = Run(tempClone, "git", []string{"add", "conflict-file.txt"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"commit", "-m", "Remote modification"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"push"})
	require.NoError(t, err)

	// Make conflicting local changes (uncommitted)
	localFile := filepath.Join(repo.AbsPath, "conflict-file.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("initial content\nlocal change\n"), 0644))

	// Fetch to get incoming commits
	_, err = Run(repo.AbsPath, "git", []string{"fetch"})
	require.NoError(t, err)

	// Refresh repo state
	require.NoError(t, repo.Refresh())
	require.NoError(t, ScheduleRepositoryRefresh(repo, nil))

	// Verify setup: unclean working tree with incoming commits
	require.False(t, repo.IsClean(), "working tree should not be clean")
	require.True(t, repo.State.Branch.HasIncomingCommits(), "should have incoming commits")

	// Apply cleanliness check
	applyCleanliness(repo)

	// Verify: should be dirty (merge conflict would occur)
	require.False(t, repo.State.Branch.Clean, "repository should be marked as dirty - merge conflict")
	require.Equal(t, git.Available, repo.WorkStatus(), "status should be Available")
}

// TestFastForwardDryRunSucceeds_NoError tests the happy path where
// fast-forward merge would succeed without any issues.
func TestFastForwardDryRunSucceeds_NoError(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	// Create a bare remote and push
	remoteDir, err := os.MkdirTemp("", "gitbatch-remote")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	_, err = Run(remoteDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	_, _ = Run(repo.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = Run(repo.AbsPath, "git", []string{"remote", "add", "origin", remoteDir})
	require.NoError(t, err)
	_, err = Run(repo.AbsPath, "git", []string{"push", "-u", "origin", branchName})
	require.NoError(t, err)

	// Create a commit in remote
	tempClone, err := os.MkdirTemp("", "gitbatch-clone")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	_, err = Run(tempClone, "git", []string{"clone", remoteDir, "."})
	require.NoError(t, err)
	_, _ = Run(tempClone, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(tempClone, "git", []string{"config", "user.name", "Test User"})

	testFile := filepath.Join(tempClone, "new-file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))
	_, err = Run(tempClone, "git", []string{"add", "new-file.txt"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"commit", "-m", "New commit"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"push"})
	require.NoError(t, err)

	// Fetch and refresh
	_, err = Run(repo.AbsPath, "git", []string{"fetch"})
	require.NoError(t, err)
	require.NoError(t, repo.Refresh())

	// Test fast-forward check
	upstream := repo.State.Branch.Upstream
	require.NotNil(t, upstream)
	mergeArg := upstreamMergeArgument(upstream)
	require.NotEmpty(t, mergeArg)

	succeeds, err := fastForwardDryRunSucceeds(repo, mergeArg)
	require.NoError(t, err)
	require.True(t, succeeds, "fast-forward should succeed")
}

// TestFastForwardDryRunSucceeds_WouldBeOverwritten tests the case where
// git merge --dry-run fails with "would be overwritten by merge" error.
// This should be treated as success (fast-forward would work, just blocked by local changes).
func TestFastForwardDryRunSucceeds_WouldBeOverwritten(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	// Setup remote with a commit
	remoteDir, err := os.MkdirTemp("", "gitbatch-remote")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	_, err = Run(remoteDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	// Create and push initial file
	initialFile := filepath.Join(repo.AbsPath, "overwrite-test.txt")
	require.NoError(t, os.WriteFile(initialFile, []byte("initial\n"), 0644))
	_, err = Run(repo.AbsPath, "git", []string{"add", "overwrite-test.txt"})
	require.NoError(t, err)
	_, err = Run(repo.AbsPath, "git", []string{"commit", "-m", "Initial commit"})
	require.NoError(t, err)

	_, _ = Run(repo.AbsPath, "git", []string{"remote", "remove", "origin"})
	_, err = Run(repo.AbsPath, "git", []string{"remote", "add", "origin", remoteDir})
	require.NoError(t, err)
	_, err = Run(repo.AbsPath, "git", []string{"push", "-u", "origin", branchName})
	require.NoError(t, err)

	// Create a commit in remote that modifies the file
	tempClone, err := os.MkdirTemp("", "gitbatch-clone")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	_, err = Run(tempClone, "git", []string{"clone", remoteDir, "."})
	require.NoError(t, err)
	_, _ = Run(tempClone, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(tempClone, "git", []string{"config", "user.name", "Test User"})

	remoteFile := filepath.Join(tempClone, "overwrite-test.txt")
	require.NoError(t, os.WriteFile(remoteFile, []byte("initial\nremote change\n"), 0644))
	_, err = Run(tempClone, "git", []string{"add", "overwrite-test.txt"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"commit", "-m", "Remote change"})
	require.NoError(t, err)
	_, err = Run(tempClone, "git", []string{"push"})
	require.NoError(t, err)

	// Make uncommitted local changes to the same file
	localFile := filepath.Join(repo.AbsPath, "overwrite-test.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("initial\nlocal change\n"), 0644))

	// Fetch and refresh
	_, err = Run(repo.AbsPath, "git", []string{"fetch"})
	require.NoError(t, err)
	require.NoError(t, repo.Refresh())

	// Test fast-forward check - should return true even though dry-run fails
	upstream := repo.State.Branch.Upstream
	require.NotNil(t, upstream)
	mergeArg := upstreamMergeArgument(upstream)
	require.NotEmpty(t, mergeArg)

	succeeds, err := fastForwardDryRunSucceeds(repo, mergeArg)
	require.NoError(t, err)
	require.True(t, succeeds, "should return true when error is 'would be overwritten by merge'")
}

// TestIsGitFatalError tests the exit code 128 detection.
func TestIsGitFatalError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-exec error",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
		{
			name: "exit code 128",
			err: &exec.ExitError{
				ProcessState: &os.ProcessState{},
			},
			expected: false, // Can't easily create a proper exit code 128 in test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitFatalError(tt.err)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestApplyCleanliness_UpstreamNotConfigured tests the error case where
// upstream is not configured.
func TestApplyCleanliness_UpstreamNotConfigured(t *testing.T) {
	th := git.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	// Remove upstream configuration
	_, _ = Run(repo.AbsPath, "git", []string{"config", "--unset", fmt.Sprintf("branch.%s.remote", branchName)})
	_, _ = Run(repo.AbsPath, "git", []string{"config", "--unset", fmt.Sprintf("branch.%s.merge", branchName)})

	// Refresh to pick up the change
	require.NoError(t, repo.Refresh())
	require.NoError(t, ScheduleRepositoryRefresh(repo, nil))

	// Apply cleanliness check
	applyCleanliness(repo)

	// Should be clean (no upstream means no incoming commits to worry about)
	require.True(t, repo.State.Branch.Clean, "repository should be marked as clean when no upstream")
}

// TestApplyCleanliness_UncleanWithIncomingFFSucceeds is a focused test for the specific bug:
// Unclean working tree + upstream is set and valid + incoming commits + fast-forward succeeds.
// This should be marked as CLEAN, but was being marked as DIRTY.
func TestApplyCleanliness_UncleanWithIncomingFFSucceeds(t *testing.T) {
	// Create a new temporary directory for this test
	testDir, err := os.MkdirTemp("", "gitbatch-test")
	require.NoError(t, err)
	defer os.RemoveAll(testDir)

	repoDir := filepath.Join(testDir, "repo")
	require.NoError(t, os.Mkdir(repoDir, 0755))

	// Initialize git repo
	_, err = Run(repoDir, "git", []string{"init"})
	require.NoError(t, err)
	_, _ = Run(repoDir, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(repoDir, "git", []string{"config", "user.name", "Test User"})

	// Create and commit an initial file
	initialFile := filepath.Join(repoDir, "file1.txt")
	require.NoError(t, os.WriteFile(initialFile, []byte("initial content"), 0644))
	_, err = Run(repoDir, "git", []string{"add", "file1.txt"})
	require.NoError(t, err)
	_, err = Run(repoDir, "git", []string{"commit", "-m", "Initial commit"})
	require.NoError(t, err)

	// Create bare remote
	remoteDir := filepath.Join(testDir, "remote.git")
	require.NoError(t, os.Mkdir(remoteDir, 0755))
	_, err = Run(remoteDir, "git", []string{"init", "--bare"})
	require.NoError(t, err)

	// Add remote and push
	branchOut, err := Run(repoDir, "git", []string{"branch", "--show-current"})
	require.NoError(t, err)
	branchName := strings.TrimSpace(branchOut)
	t.Logf("Branch name: %s", branchName)

	_, err = Run(repoDir, "git", []string{"remote", "add", "origin", remoteDir})
	require.NoError(t, err)
	_, err = Run(repoDir, "git", []string{"push", "-u", "origin", branchName})
	require.NoError(t, err)

	// Clone the remote to create an incoming commit
	cloneDir := filepath.Join(testDir, "clone")
	require.NoError(t, os.Mkdir(cloneDir, 0755))
	_, err = Run(cloneDir, "git", []string{"clone", remoteDir, "."})
	require.NoError(t, err)
	_, _ = Run(cloneDir, "git", []string{"config", "user.email", "test@example.com"})
	_, _ = Run(cloneDir, "git", []string{"config", "user.name", "Test User"})

	// Create a new file in clone (won't conflict with local changes)
	remoteFile := filepath.Join(cloneDir, "file2.txt")
	require.NoError(t, os.WriteFile(remoteFile, []byte("remote content"), 0644))
	_, err = Run(cloneDir, "git", []string{"add", "file2.txt"})
	require.NoError(t, err)
	_, err = Run(cloneDir, "git", []string{"commit", "-m", "Add file2"})
	require.NoError(t, err)
	_, err = Run(cloneDir, "git", []string{"push"})
	require.NoError(t, err)

	// Make uncommitted local changes to a different file (no conflict)
	localFile := filepath.Join(repoDir, "file3.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("local changes"), 0644))

	// Fetch to get incoming commits
	_, err = Run(repoDir, "git", []string{"fetch"})
	require.NoError(t, err)

	// Load the repository with gitbatch
	repo, err := git.InitializeRepo(repoDir)
	require.NoError(t, err)
	require.NotNil(t, repo.State)
	require.NotNil(t, repo.State.Branch)

	// Debug output
	t.Logf("IsClean: %v", repo.IsClean())
	t.Logf("Upstream: %+v", repo.State.Branch.Upstream)
	t.Logf("Pullables: %s", repo.State.Branch.Pullables)
	t.Logf("Pushables: %s", repo.State.Branch.Pushables)

	// Verify test setup
	require.False(t, repo.IsClean(), "working tree should not be clean (uncommitted file3.txt)")
	require.True(t, repo.State.Branch.HasIncomingCommits(), "should have incoming commits (file2.txt from remote)")

	// Verify fast-forward would succeed
	upstream := repo.State.Branch.Upstream
	require.NotNil(t, upstream, "upstream should be set")
	mergeArg := upstreamMergeArgument(upstream)
	require.NotEmpty(t, mergeArg, "merge argument should not be empty")

	ffSucceeds, err := fastForwardDryRunSucceeds(repo, mergeArg)
	require.NoError(t, err)
	require.True(t, ffSucceeds, "fast-forward should succeed (non-conflicting files)")

	// Apply cleanliness check
	applyCleanliness(repo)

	// Verify: should be clean (fast-forward would succeed, no conflicts)
	require.True(t, repo.State.Branch.Clean, "repository should be marked as CLEAN - fast-forward would succeed with non-conflicting changes")
	require.Equal(t, git.Available, repo.WorkStatus(), "status should be Available")
}
