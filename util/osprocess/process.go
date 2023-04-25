package osprocess

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nightlyone/lockfile"
	"github.com/samber/lo"
	"github.com/shirou/gopsutil/v3/process"
)

func Lock() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	cwd := filepath.Dir(exePath)
	lock, err := lockfile.New(filepath.Join(cwd, "lock.lck"))
	if err != nil {
		return err
	}

	killOldProcess(lock, exePath)

	return lock.TryLock()
}

func ProcessByPid(pid int) (*process.Process, error) {
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

func isMyProcess(exePath string, process *process.Process) bool {
	processPath, err := process.Exe()
	if err != nil {
		return false
	}
	return processPath == exePath
}

func killOldProcess(lock lockfile.Lockfile, exePath string) {
	oldProcess, _ := lock.GetOwner() //nolint:errcheck
	if oldProcess != nil {
		proc, err := ProcessByPid(oldProcess.Pid)
		if err != nil {
			return
		}

		isNotCurrentRun := os.Getpid() != oldProcess.Pid

		if isNotCurrentRun && isMyProcess(exePath, proc) {
			_ = proc.Kill() //nolint:errcheck
		}
	}
}
