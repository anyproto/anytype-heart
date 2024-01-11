package clientds

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

// test for backupJob
func TestBackupJob(t *testing.T) {
	tempDir, err := os.MkdirTemp(os.TempDir(), "anytypetestds*")
	require.NoError(t, err)
	fmt.Printf("tempDir: %s\n", tempDir)
	dsDir := filepath.Join(tempDir, "ds")
	opts := DefaultConfig.Spacestore
	opts.Dir = dsDir
	opts.ValueDir = dsDir
	db, err := badger.Open(opts)
	require.NoError(t, err)
	err = initBackupDir(opts.Dir)
	require.NoError(t, err)

	for i := 0; i < 10000; i++ {
		err = db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(fmt.Sprintf("key/%d", i)), []byte("firstValue"))
		})
		require.NoError(t, err)
	}
	err = backupJob(db)
	require.NoError(t, err)
	for i := 0; i < 10000; i++ {
		err = db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(fmt.Sprintf("key/%d", i)), []byte("secondValue"))
		})
		require.NoError(t, err)
	}
	err = backupJob(db)

	err = db.Sync()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)

	// corrupt db
	nulloutBytesRangeInFile(t, filepath.Join(dsDir, "000001.sst"), 100000, 900000)

	_, err = openBadgerWithRecover(opts)
	require.NotNil(t, err)

	db, err = restoreBadger(opts, false)
	require.NoError(t, err)
	for i := 0; i < 10000; i++ {
		// iterate over all keys
		err = db.View(func(txn *badger.Txn) error {
			v, err := txn.Get([]byte(fmt.Sprintf("key/%d", i)))
			require.NoError(t, err)
			err = v.Value(func(val []byte) error {
				require.Equal(t, []byte("secondValue"), val)
				return nil
			})
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
	}

}

func TestBackupJob2(t *testing.T) {
	tempDir, err := os.MkdirTemp(os.TempDir(), "anytypetestds*")
	require.NoError(t, err)
	fmt.Printf("tempDir: %s\n", tempDir)
	dsDir := filepath.Join(tempDir, "ds")
	opts := DefaultConfig.Spacestore
	opts.Dir = dsDir
	opts.ValueDir = dsDir
	db, err := badger.Open(opts)
	require.NoError(t, err)
	err = initBackupDir(opts.Dir)
	require.NoError(t, err)

	FullBackupEvery = 10
	for j := 0; j <= 12; j++ {
		for i := 0; i < 1000; i++ {
			err = db.Update(func(txn *badger.Txn) error {
				return txn.Set([]byte(fmt.Sprintf("key/%d", i)), []byte(fmt.Sprintf("Value-%d", j)))
			})
			require.NoError(t, err)
		}
		err = backupJob(db)
		require.NoError(t, err)

		backups, err := getAllBackupFiles(getBackupPathFromDbPath(opts.Dir))
		require.NoError(t, err)
		require.Equal(t, j%10+1, len(backups))
	}

	err = db.Sync()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)

	// corrupt db
	nulloutBytesRangeInFile(t, filepath.Join(dsDir, "000001.sst"), 100000, 900000)

	_, err = openBadgerWithRecover(opts)
	require.NotNil(t, err)

	db, err = restoreBadger(opts, false)
	require.NoError(t, err)
	for i := 0; i < 1000; i++ {
		// iterate over all keys
		err = db.View(func(txn *badger.Txn) error {
			v, err := txn.Get([]byte(fmt.Sprintf("key/%d", i)))
			require.NoError(t, err)
			err = v.Value(func(val []byte) error {
				require.Equal(t, []byte("Value-12"), val)
				return nil
			})
			require.NoError(t, err)
			return nil
		})
		require.NoError(t, err)
	}

}

func nulloutBytesRangeInFile(t *testing.T, path string, from, to int64) {
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Seek(from, 0)
	require.NoError(t, err)

	buf := make([]byte, to-from)
	_, err = file.Write(buf)
	require.NoError(t, err)
	file.Sync()
}
