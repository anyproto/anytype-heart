//go:build windows

package filetimes

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

	if stat, ok := fileInfo.Sys().(*syscall.Win32FileAttributeData); ok {
		creationTime := time.Unix(0, stat.CreationTime.Nanoseconds()).Unix()
		modTime := fileInfo.ModTime().Unix()
		return creationTime, modTime
	}
	return 0, 0
}
