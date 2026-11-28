//go:build !windows

package store

import (
	"os"
	"path/filepath"
)

// FinalPath is where an open file really lies: its path with every link
// resolved.
func FinalPath(fh *os.File) (string, error) { return filepath.EvalSymlinks(fh.Name()) }
