package command

import (
	"context"
	"regexp"

	"github.com/go-git/go-git/v5/plumbing"
	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// MergeOptions defines the rules of a merge operation
type MergeOptions struct {
	// Name of the branch to merge with.
	BranchName string
	// Be verbose.
	Verbose bool
	// With true do not show a diffstat at the end of the merge.
	NoStat bool
	// Mode is the command mode
	CommandMode Mode
}

// Merge incorporates changes from the named commits or branches into the
// current branch
func Merge(r *git.Repository, options *MergeOptions) (string, error) {
	return MergeWithContext(context.Background(), r, options)
}

// MergeWithContext executes merge honouring the provided context for cancellation and deadlines.
func MergeWithContext(ctx context.Context, r *git.Repository, options *MergeOptions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := make([]string, 0)
	args = append(args, "merge")
	if len(options.BranchName) > 0 {
		args = append(args, options.BranchName)
	}
	if options.Verbose {
		args = append(args, "-v")
	}
	if options.NoStat {
		args = append(args, "-n")
	}

	ref, _ := r.Repo.Head()
	if out, err := RunWithContext(ctx, r.AbsPath, "git", args); err != nil {
		return "", gerr.ParseGitError(out, err)
	}

	newref, _ := r.Repo.Head()

	msg, err := getMergeMessage(r, mergeReferenceHash(ref), mergeReferenceHash(newref))
	if err != nil {
		msg = "couldn't get stat"
	}
	return msg, nil
}

func mergeReferenceHash(ref *plumbing.Reference) string {
	if ref == nil {
		return ""
	}
	return ref.Hash().String()
}

func getMergeMessage(r *git.Repository, ref1, ref2 string) (string, error) {
	var msg string
	if ref1 == ref2 {
		msg = "already up-to-date"
	} else {
		out, err := DiffStatRefs(r, ref1, ref2)
		if err != nil {
			return "", err
		}
		re := regexp.MustCompile(`\r?\n`)
		lines := re.Split(out, -1)
		last := lines[len(lines)-1]
		if len(last) > 0 {
			msg = lines[len(lines)-1][1:]
		}
	}
	return msg, nil
}
