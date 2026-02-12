package git


// File represents the status of a file in an index or work tree
type File struct {
	Name    string
	AbsPath string
	X       FileStatus
	Y       FileStatus
}

// FileStatus is the short representation of state of a file
type FileStatus byte

const (
	// StatusNotupdated says file not updated
	StatusNotupdated FileStatus = ' '
	// StatusModified says file is modifed
	StatusModified FileStatus = 'M'
	// StatusModifiedUntracked says file is modifed and un-tracked
	StatusModifiedUntracked FileStatus = 'm'
	// StatusAdded says file is added to index
	StatusAdded FileStatus = 'A'
	// StatusDeleted says file is deleted
	StatusDeleted FileStatus = 'D'
	// StatusRenamed says file is renamed
	StatusRenamed FileStatus = 'R'
	// StatusCopied says file is copied
	StatusCopied FileStatus = 'C'
	// StatusUpdated says file is updated
	StatusUpdated FileStatus = 'U'
	// StatusUntracked says file is untraced
	StatusUntracked FileStatus = '?'
	// StatusIgnored says file is ignored
	StatusIgnored FileStatus = '!'
)

// FilesAlphabetical slice is the re-ordered *File slice that sorted according
// to alphabetical order (A-Z)
type FilesAlphabetical []*File

// Len is the interface implementation for Alphabetical sorting function
func (s FilesAlphabetical) Len() int { return len(s) }

// Swap is the interface implementation for Alphabetical sorting function
func (s FilesAlphabetical) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less is the interface implementation for Alphabetical sorting function
func (s FilesAlphabetical) Less(i, j int) bool {
	return CompareNamesInsensitive(s[i].Name, s[j].Name) < 0
}
