package app

import (
	"os"
	"path/filepath"
)

// generateDirectories returns possible git repositories to pipe into git pkg
// load function
func generateDirectories(dirs []string, depth int) []string {
	gitDirs := make([]string, 0)
	for i := 0; i < depth; i++ {
		directories, repositories := walkRecursive(dirs, gitDirs)
		dirs = directories
		gitDirs = repositories
	}
	return gitDirs
}

// returns given values, first search directories and second stands for possible
// git repositories. Call this func from a "for i := 0; i<depth; i++" loop
func walkRecursive(search, appendant []string) ([]string, []string) {
	max := len(search)
	for i := 0; i < max; i++ {
		if i >= len(search) {
			continue
		}
		// find possible repositories and remaining ones, b slice is possible ones
		a, b, err := separateDirectories(search[i])
		if err != nil {
			continue
		}
		// since we started to search let's get rid of it and remove from search
		// array
		search[i] = search[len(search)-1]
		search = search[:len(search)-1]
		// lets append what we have found to continue recursion
		// Optimize: pre-allocate capacity if we know we're adding multiple items
		if len(a) > 0 {
			if cap(search)-len(search) < len(a) {
				newSearch := make([]string, len(search), len(search)+len(a)+10)
				copy(newSearch, search)
				search = newSearch
			}
			search = append(search, a...)
		}
		if len(b) > 0 {
			if cap(appendant)-len(appendant) < len(b) {
				newAppendant := make([]string, len(appendant), len(appendant)+len(b)+10)
				copy(newAppendant, appendant)
				appendant = newAppendant
			}
			appendant = append(appendant, b...)
		}
	}
	return search, appendant
}

// separateDirectories is to find all the files in given path. This method
// does not check if the given file is a valid git repositories
func separateDirectories(directory string) ([]string, []string, error) {
	files, err := os.ReadDir(directory)
	// can we read the directory?
	if err != nil {
		return nil, nil, nil
	}

	// Pre-allocate slices with capacity based on file count to reduce reallocations
	dirs := make([]string, 0, len(files))
	gitDirs := make([]string, 0, len(files)/4) // Estimate fewer git repos than total files

	for _, f := range files {
		// Skip non-directories
		if !f.IsDir() {
			continue
		}
		
		// Use filepath.Join for more efficient path construction
		repo := filepath.Join(directory, f.Name())

		dir, err := filepath.Abs(repo)
		if err != nil {
			continue
		}

		// Check if this directory contains a .git folder/file
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			gitDirs = append(gitDirs, dir)
		} else {
			dirs = append(dirs, dir)
		}
	}
	return dirs, gitDirs, nil
}
