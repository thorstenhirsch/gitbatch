package command

import "regexp"

// newlineRegex matches line endings for splitting command output.
var newlineRegex = regexp.MustCompile(`\r?\n`)
