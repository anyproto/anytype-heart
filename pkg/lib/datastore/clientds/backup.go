package clientds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/dgraph-io/badger/v4"
	"golang.org/x/sys/unix"
)

var (
	// every FullBackupEvery incremental backups we do a full backup and remove all old backups
	FullBackupEvery = 1000
	BackupInterval  = time.Minute
)

// getAllBackupFiles returns all backup files in the repoPath sorted by timestamp suffix
func getAllBackupFiles(repoPath string) ([]string, error) {
	var files []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".tmp" {
			// means it is in not finished state - skip it
			return nil
		}

		tsStr := filepath.Base(path)
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			return nil
		}
		if ts < 0 {
			return nil
		}

		files = append(files, path)

		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i] < files[j]
	})
	return files, nil
}

func initBackupDir(dsPath string) error {
	backupPath := getBackupPathFromDbPath(dsPath)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		if err := os.Mkdir(backupPath, 0700); err != nil {
			return fmt.Errorf("failed to create backup dir: %s", err)
		}
		// write text file with info about backup dir
		if err := os.WriteFile(filepath.Join(backupPath, "README.txt"), []byte("This directory contains incremental backups of spacestore database. Do not modify or remove any files in this folder."), 0600); err != nil {
			log.Errorf("failed to write README.txt: %s", err)
		}
	}
	return nil
}

func (r *clientds) runBackup() error {
	// create backup dir if not exists
	if err := initBackupDir(r.spaceDS.Opts().Dir); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-r.closed:
				return
			case <-time.After(BackupInterval):
				err := backupJob(r.spaceDS)
				if err != nil {
					log.Errorf("failed to backup spacestore: %s", err)
				}
			}
		}
	}()
	return nil
}

func backupJob(db *badger.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if db.IsClosed() {
		return fmt.Errorf("db is closed")
	}
	var since uint64
	backupPath := getBackupPathFromDbPath(db.Opts().Dir)
	backups, err := getAllBackupFiles(backupPath)
	if err != nil {
		return fmt.Errorf("failed to get all backups: %s", err)
	}

	if len(backups) < FullBackupEvery && len(backups) > 0 {
		lastTime, err := strconv.ParseUint(filepath.Base(backups[len(backups)-1]), 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse backup max version: %s", err)
		} else {
			since = lastTime
		}
	}
	path := getSpaceStoreTempBackupPath(backupPath)
	bak, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create spacestore backup file: %s", err)
	}
	maxVersion, err := db.Backup(bak, since)
	if err != nil {
		return fmt.Errorf("failed to backup spacestore: %s", err)
	}

	if maxVersion > 0 {
		err = bak.Sync()
		if err != nil {
			// sync failed, lets log this info and continue
			log.Errorf("db backup file failed to sync: %s", err)
		}
	}

	info, _ := bak.Stat()
	if maxVersion > 0 {
		log.Debugf("spacestore backup since %d done, maxVersion: %d, size: %d", since, maxVersion, info.Size())
	}

	err = bak.Close()
	if err != nil {
		return fmt.Errorf("failed to close backup file: %s", err)
	}

	if maxVersion == 0 {
		err = os.Remove(path)
		if err != nil {
			return fmt.Errorf("failed to remove empty temp backup: %s", err)
		}
	}

	err = os.Rename(path, getSpaceStoreFinishedBackupPath(backupPath, maxVersion))
	if err != nil {
		return fmt.Errorf("failed to rename temp backup: %s", err)
	}

	if len(backups) > FullBackupEvery {
		for _, backup := range backups {
			err = os.Remove(backup)
			if err != nil {
				name := filepath.Base(backup)
				log.Errorf("failed to remove old backup %s: %s", name, err)
			}
		}
	}
	return nil
}

func getSpaceStoreTempBackupPath(backupDir string) string {
	return filepath.Join(backupDir, fmt.Sprintf("%d.tmp", time.Now().Unix()))
}

func getSpaceStoreFinishedBackupPath(backupDir string, ts uint64) string {
	return filepath.Join(backupDir, fmt.Sprintf("%d", ts))
}

func getBackupPathFromDbPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), SpaceDSBackupsDir)
}

func restoreBadger(opts badger.Options, skipBackupRestore bool) (*badger.DB, error) {
	base := filepath.Base(opts.Dir)
	var (
		backups []string
		err     error
	)
	err = os.RemoveAll(filepath.Join(opts.Dir, lock))
	if err != nil {
		return nil, err
	}
	r, err := os.Open(opts.Dir)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(r.Fd()), unix.LOCK_UN); err != nil {
		log.Fatal(err)
	}
	r.Close()

	backups, err = getAllBackupFiles(getBackupPathFromDbPath(opts.Dir))
	if err != nil {
		if !skipBackupRestore {
			return nil, err
		}
	}

	if len(backups) == 0 && !skipBackupRestore {
		return nil, fmt.Errorf("no backups found")
	}

	originalDir := opts.Dir
	if len(backups) > 0 {
		// lets start recovery in the separate dir in case it will be interrupted
		recoveryDbDir := filepath.Join(filepath.Dir(opts.Dir), fmt.Sprintf(base+"_recovery_%d", time.Now().Unix()))
		opts.Dir = recoveryDbDir
	}

	// at this
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	if len(backups) == 0 {
		return db, nil
	}

	// restore from incremental backup
	// backup files are sorted
	for _, backup := range backups {
		f, err := os.Open(backup)
		if err != nil {
			return nil, err
		}
		err = db.Load(f, 16)
		if err != nil {
			return nil, err
		}
	}
	err = db.Sync()
	if err != nil {
		return nil, err
	}
	err = db.Close()
	if err != nil {
		return nil, err
	}
	err = os.Rename(originalDir, fmt.Sprintf("%s_corrupted_%d", originalDir, time.Now().Unix()))
	if err != nil {
		return nil, err
	}
	err = os.Rename(opts.Dir, originalDir)
	if err != nil {
		return nil, err
	}
	opts.Dir = originalDir
	db, err = badger.Open(opts)
	if err != nil {
		log.Errorf("failed to open db after restore: %s", err)
	} else {
		log.Warn("badger db restored successfully after corruption")
	}
	return db, nil
}
