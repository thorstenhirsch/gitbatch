package git

import (
	"unicode"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// CompareNamesInsensitive compares two strings case-insensitively, with
// case-sensitive tiebreaking for equal lowercase runes.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareNamesInsensitive(a, b string) int {
	ar, br := []rune(a), []rune(b)
	max := len(ar)
	if max > len(br) {
		max = len(br)
	}
	for i := 0; i < max; i++ {
		la, lb := unicode.ToLower(ar[i]), unicode.ToLower(br[i])
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		if ar[i] != br[i] {
			if ar[i] < br[i] {
				return -1
			}
			return 1
		}
	}
	if len(ar) < len(br) {
		return -1
	}
	if len(ar) > len(br) {
		return 1
	}
	return 0
}

// Alphabetical slice is the re-ordered *Repository slice that sorted according
// to alphabetical order (A-Z)
type Alphabetical []*Repository

// Len is the interface implementation for Alphabetical sorting function
func (s Alphabetical) Len() int { return len(s) }

// Swap is the interface implementation for Alphabetical sorting function
func (s Alphabetical) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less is the interface implementation for Alphabetical sorting function
func (s Alphabetical) Less(i, j int) bool {
	return CompareNamesInsensitive(s[i].Name, s[j].Name) < 0
}

// LastModified slice is the re-ordered *Repository slice that sorted according
// to last modified date of the repository directory
type LastModified []*Repository

// Len is the interface implementation for LastModified sorting function
func (s LastModified) Len() int { return len(s) }

// Swap is the interface implementation for LastModified sorting function
func (s LastModified) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less is the interface implementation for LastModified sorting function
func (s LastModified) Less(i, j int) bool {
	return s[i].ModTime.Unix() > s[j].ModTime.Unix()
}

// CommitTime slice is the re-ordered *object.Commit slice that sorted according
// commit date
type CommitTime []*object.Commit

// Len is the interface implementation for CommitTime sorting function
func (s CommitTime) Len() int { return len(s) }

// Swap is the interface implementation for CommitTime sorting function
func (s CommitTime) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less is the interface implementation for CommitTime sorting function
func (s CommitTime) Less(i, j int) bool {
	return s[i].Author.When.Unix() > s[j].Author.When.Unix()
}
