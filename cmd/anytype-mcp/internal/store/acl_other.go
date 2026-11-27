//go:build !windows && !darwin

package store

import "os"

// A POSIX ACL shows up in the group bits (its mask), which the folder and
// item rules already refuse; the 0700 and 0600 modes mask named entries to
// nothing. So there is nothing more to read here.
func pathACL(string) (aclInfo, error) { return aclInfo{}, nil }

func fileACL(*os.File) (aclInfo, error) { return aclInfo{}, nil }
