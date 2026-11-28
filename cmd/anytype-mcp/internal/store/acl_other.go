//go:build !windows && !darwin

package store

import "os"

// A POSIX ACL shows up in the group bits (its mask): a mask that lets a
// named entry write is a group-writable mode, which the folder and file
// rules already refuse. So there is nothing more to read here.
func pathACL(string) (aclInfo, error) { return aclInfo{}, nil }

func fileACL(*os.File) (aclInfo, error) { return aclInfo{}, nil }
