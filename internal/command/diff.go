package command

import (
	"regexp"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// DiffStatRefs shows diff stat of two refs  "git diff a1b2c3..e4f5g6 --stat"
func DiffStatRefs(r *git.Repository, ref1, ref2 string) (string, error) {
	args := make([]string, 0)
	args = append(args, "diff")
	args = append(args, ref1+".."+ref2)
	args = append(args, "--shortstat")
	output, err := Run(r.AbsPath, "git", args)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`\n?\r`)
	output = re.ReplaceAllString(output, "\n")
	return output, err
}
