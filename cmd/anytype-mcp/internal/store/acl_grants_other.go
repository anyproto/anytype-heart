//go:build !darwin

package store

import "os"

// ACLGrants counts the ACL entries that grant access. Only macOS has an
// ACL beside the mode bits that this package reads: on Linux a POSIX ACL
// shows in the group bits, and Windows descriptors are read where they
// are judged.
func ACLGrants(*os.File) (int, error) { return 0, nil }
