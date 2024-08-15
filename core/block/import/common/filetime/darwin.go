//go:build darwin

package filetime

import (
	"os"
	"syscall"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

var log = logging.Logger("import")

func ExtractFileTimes(fileName string) (int64, int64) {
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		log.Warnf("failed to get file info from path: %s", anyerror.CleanupError(err))
		return 0, 0
	}

	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		creationTime := time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
		modTime := fileInfo.ModTime().Unix()
		return creationTime.Unix(), modTime
	}
	return 0, 0
}
