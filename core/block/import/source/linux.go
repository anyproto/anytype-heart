//go:build linux || darwin
// +build linux darwin

package source

import (
	"os"
	"syscall"
	"time"

	oserror "github.com/anyproto/anytype-heart/util/os"
)

func ExtractFileTimes(fileName string) (int64, int64) {
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		log.Warnf("failed to get file info from path: %s", oserror.TransformError(err))
		return 0, 0
	}

	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		creationTime := time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec)
		modTime := fileInfo.ModTime().Unix()
		return creationTime.Unix(), modTime
	}
	return 0, 0
}
