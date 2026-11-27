//go:build teststore

package store

// TestBuild marks a binary built with the teststore tag: its store is a
// directory of files and serve accepts overrides. Never shipped.
const TestBuild = true

// Open returns the file store rooted at dir.
func Open(_ string, dir string) Store { return &File{Dir: dir} }
