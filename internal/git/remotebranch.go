package git

import (
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
)

// RemoteBranch is the wrapper of go-git's Reference struct. In addition to
// that, it also holds name of the remote branch
type RemoteBranch struct {
	Name      string
	Reference *plumbing.Reference
}

// loadAllRemoteBranches fetches all remote branches using git for-each-ref
// and returns them grouped by remote name.
func (r *Repository) loadAllRemoteBranches() (map[string][]*RemoteBranch, error) {
	args := []string{
		"for-each-ref",
		"--format=%(refname)|%(objectname)",
		"refs/remotes",
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.AbsPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]*RemoteBranch)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		refName := parts[0]
		hash := parts[1]

		// refName is like refs/remotes/origin/HEAD or refs/remotes/origin/master
		shortName := strings.TrimPrefix(refName, "refs/remotes/")
		remoteParts := strings.SplitN(shortName, "/", 2)
		if len(remoteParts) < 2 {
			continue
		}
		remoteName := remoteParts[0]
		// branchName := remoteParts[1]

		// Skip HEAD refs
		if strings.HasSuffix(refName, "/HEAD") {
			continue
		}

		rb := &RemoteBranch{
			Name:      shortName,
			Reference: plumbing.NewHashReference(plumbing.ReferenceName(refName), plumbing.NewHash(hash)),
		}

		if _, ok := result[remoteName]; !ok {
			result[remoteName] = make([]*RemoteBranch, 0)
		}
		result[remoteName] = append(result[remoteName], rb)
	}

	return result, nil
}
