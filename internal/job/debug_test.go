package job

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/gittest"
)

func TestDebugFetchJob(t *testing.T) {
	th := gittest.InitTestRepositoryFromLocal(t)
	defer th.CleanUp(t)

	repo := th.Repository
	branchName := repo.State.Branch.Name

	readCmd := exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.merge", branchName))
	readCmd.Dir = repo.AbsPath
	originalMergeBytes, err := readCmd.CombinedOutput()
	require.NoError(t, err)
	originalMerge := strings.TrimSpace(string(originalMergeBytes))

	missingBranch := "__gitbatch_missing_branch__"
	updateCmd := exec.Command("git", "config", fmt.Sprintf("branch.%s.merge", branchName), "refs/heads/"+missingBranch)
	updateCmd.Dir = repo.AbsPath
	require.NoError(t, updateCmd.Run())

	defer func() {
		restore := exec.Command("git", "config", fmt.Sprintf("branch.%s.merge", branchName), originalMerge)
		restore.Dir = repo.AbsPath
		_ = restore.Run()
	}()

	require.NoError(t, command.ScheduleRepositoryRefresh(repo, nil))
	require.Eventually(t, func() bool {
		return repo.State.Branch.Upstream == nil
	}, 3*time.Second, 25*time.Millisecond, "upstream should become nil after refresh")


	job := &Job{JobType: FetchJob, Repository: repo}
	require.NoError(t, job.Start())

	require.Eventually(t, func() bool {
		return repo.WorkStatus() == git.Fail
	}, 2*time.Second, 50*time.Millisecond)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if repo.WorkStatus() == git.Fail {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	elapsed := 5*time.Second - time.Until(deadline)
	t.Logf("status=%v message=%s elapsed=%s", repo.WorkStatus(), repo.State.Message, elapsed)
}
