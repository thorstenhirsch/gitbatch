package command

// ResetType defines a string type for reset git command.
type ResetType string

const (
	// ResetHard Resets the index and working tree. Any changes to tracked
	// files in the working tree since <commit> are discarded.
	ResetHard ResetType = "hard"

	// ResetMixed Resets the index but not the working tree (i.e., the changed
	// files are preserved but not marked for commit) and reports what has not
	// been updated. This is the default action.
	ResetMixed ResetType = "mixed"

	// ResetMerge Resets the index and updates the files in the working tree
	// that are different between <commit> and HEAD, but keeps those which are
	// different between the index and working tree
	ResetMerge ResetType = "merge"

	// ResetSoft Does not touch the index file or the working tree at all
	// (but resets the head to <commit>
	ResetSoft ResetType = "soft"

	// ResetKeep Resets index entries and updates files in the working tree
	// that are different between <commit> and HEAD
	ResetKeep ResetType = "keep"
)
