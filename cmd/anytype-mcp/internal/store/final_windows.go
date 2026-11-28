//go:build windows

package store

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// GetFinalPathNameByHandle's volume forms: a drive letter, else the
// volume's GUID (a volume mounted in a folder has no drive letter).
const (
	volumeNameDOS  = 0x0
	volumeNameGUID = 0x1
)

// FinalPath is where an open file really lies, asked of its handle:
// junctions and mount points resolved, which filepath.EvalSymlinks leaves
// alone. A drive path comes back without its \\?\ prefix.
func FinalPath(fh *os.File) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	var n uint32
	var err error
	for _, form := range []uint32{volumeNameDOS, volumeNameGUID} {
		n, err = windows.GetFinalPathNameByHandle(windows.Handle(fh.Fd()), &buf[0], windows.MAX_LONG_PATH, form)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", err
	}
	if n == 0 || n >= windows.MAX_LONG_PATH {
		return "", errors.New("the path is too long")
	}
	path := windows.UTF16ToString(buf[:n])
	if rest, ok := strings.CutPrefix(path, `\\?\UNC\`); ok {
		return `\\` + rest, nil
	}
	if len(path) > 6 && strings.HasPrefix(path, `\\?\`) && path[5] == ':' {
		return path[4:], nil
	}
	return path, nil
}
