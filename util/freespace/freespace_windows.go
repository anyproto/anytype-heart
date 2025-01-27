//go:build windows

package freespace

import "golang.org/x/sys/windows"

func GetFreeDiskSpace(path string) (uint64, error) {
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	lpDirectoryName := windows.StringToUTF16Ptr(path)
	err := windows.GetDiskFreeSpaceEx(lpDirectoryName, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes)
	if err != nil {
		return 0, err
	}
	return freeBytesAvailable, nil
}
