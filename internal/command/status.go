package command

import (
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func shortStatus(r *git.Repository, option string) string {
	args := make([]string, 0)
	args = append(args, "status")
	args = append(args, option)
	args = append(args, "--short")
	out, err := Run(r.AbsPath, "git", args)
	if err != nil {
		return "?"
	}
	return out
}

// Status returns the dirty files
func Status(r *git.Repository) ([]*git.File, error) {
	return statusWithGit(r)
}

// PlainStatus returns the plain status
func PlainStatus(r *git.Repository) (string, error) {
	args := make([]string, 0)
	args = append(args, "status")
	args = append(args, "--short")
	output, err := Run(r.AbsPath, "git", args)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`\r?\n`)
	output = re.ReplaceAllString(output, "\n")
	return output, err
}

// LoadFiles function simply commands a git status and collects output in a
// structured way
func statusWithGit(r *git.Repository) ([]*git.File, error) {
	files := make([]*git.File, 0)
	output := shortStatus(r, "--untracked-files=all")
	if len(output) == 0 {
		return files, nil
	}
	fileslist := strings.Split(output, "\n")
	for _, file := range fileslist {
		x := byte(file[0])
		y := byte(file[1])
		relativePathRegex := regexp.MustCompile(`[(\w|/|.|\-)]+`)
		path := relativePathRegex.FindString(file[2:])

		files = append(files, &git.File{
			Name:    path,
			AbsPath: r.AbsPath + string(os.PathSeparator) + path,
			X:       git.FileStatus(x),
			Y:       git.FileStatus(y),
		})
	}
	sort.Sort(git.FilesAlphabetical(files))
	return files, nil
}
