//go:build !windows

package freespace

import "golang.org/x/sys/unix"

func GetFreeDiskSpace(path string) (freeSpace uint64, err error) {
	var stat unix.Statfs_t
	err = unix.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	freeSpace = stat.Bavail * uint64(stat.Bsize)
	return freeSpace, nil
}
