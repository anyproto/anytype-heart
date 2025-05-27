//go:build !gomobile

package clientds

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	lock             = "LOCK"
	lockReleaseDelay = 300 * time.Millisecond
	noProcess        = -1
)

func RemoveExpiredLocks(path string) {
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	cleanupAfterOldProcess(exePath, filepath.Join(path, SpaceDSDir, lock))
}

func extractPid(path string) (int, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return noProcess, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(bytes)))
	if err != nil {
		return noProcess, err
	}

	return pid, nil
}

func processByPid(pid int) (*process.Process, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	item, found := lo.Find(
		processes,
		func(item *process.Process) bool { return int(item.Pid) == pid },
	)

	if found {
		return item, nil
	}
	return nil, fmt.Errorf("process not found")
}

func isSameProcessExeFilename(exePath string, process *process.Process) bool {
	processPath, err := process.Exe()
	if err != nil {
		return false
	}

	// only check the last part of the path
	return filepath.Base(processPath) == filepath.Base(exePath)
}

func cleanupAfterOldProcess(exePath string, lockfile string) {
	oldPid, err := extractPid(lockfile)
	if err != nil {
		return
	}

	proc, err := processByPid(oldPid)
	if err != nil {
		return
	}

	isNotCurrentRun := os.Getpid() != oldPid

	if isNotCurrentRun && isSameProcessExeFilename(exePath, proc) {
		// isSameProcessExeFilename is to avoid false positives, because PIDs can be reused by the OS
		log.Warnf("Killing the old process.")
		err = proc.Kill()
		if err != nil {
			log.Errorf("Failed to kill the old process: %v", err)
		} else {
			time.Sleep(lockReleaseDelay)
		}
	}
}
