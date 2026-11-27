//go:build !teststore

package store

// TestBuild is false in every shipped binary.
const TestBuild = false

// Open returns this OS's key store for the namespace. The directory
// argument exists only in the test build.
func Open(namespace string, _ string) Store { return openPlatform(NormalizeNamespace(namespace)) }
