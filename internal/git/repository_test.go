package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshModTime_BranchSwitch(t *testing.T) {
	th := InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	r := th.Repository

	// Get initial modtime
	modTime1 := r.RefreshModTime()

	// Get current HEAD to know what to copy
	headRef, err := r.Repo.Head()
	require.NoError(t, err)

	currentBranchName := headRef.Name().Short()

	// Create a new branch 'feature-modtime-test' pointing to same commit
	// We simulate this by copying the ref file
	gitDir := filepath.Join(r.AbsPath, ".git")
	currentRefPath := filepath.Join(gitDir, "refs", "heads", currentBranchName)
	featureRefPath := filepath.Join(gitDir, "refs", "heads", "feature-modtime-test")

	// Read current ref
	refContent, err := os.ReadFile(currentRefPath)
	require.NoError(t, err)

	// Write feature ref
	err = os.WriteFile(featureRefPath, refContent, 0644)
	require.NoError(t, err)

	// Now switch HEAD to feature by writing to .git/HEAD
	// This simulates an external tool (like lazygit) changing the branch
	headPath := filepath.Join(gitDir, "HEAD")

	newHeadContent := "ref: refs/heads/feature-modtime-test\n"
	err = os.WriteFile(headPath, []byte(newHeadContent), 0644)
	require.NoError(t, err)

	// Get modtime again
	modTime2 := r.RefreshModTime()

	assert.True(t, modTime2.After(modTime1), "ModTime should increase when HEAD symbolic ref changes")
}
