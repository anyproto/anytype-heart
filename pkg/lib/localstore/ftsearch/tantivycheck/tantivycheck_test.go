package tantivycheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck(t *testing.T) {
	t.Run("schema field names are read in order", func(t *testing.T) {
		// given
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "meta.json"),
			[]byte(`{"schema":[{"name":"B"},{"name":"A"}],"segments":[],"opstamp":0}`), 0o600))
		want := []string{"B", "A"}

		// when
		report, err := Check(dir)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, report.SchemaFieldNames)
		assert.True(t, report.IsOk())
	})

	t.Run("lock flags survive an undecodable meta.json", func(t *testing.T) {
		// given
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{garbage"), 0o600))
		lock := flock.New(filepath.Join(dir, ".tantivy-writer.lock"))
		locked, err := lock.TryLock()
		require.NoError(t, err)
		require.True(t, locked)
		t.Cleanup(func() { _ = lock.Unlock() })

		// when
		report, err := Check(dir)

		// then
		require.ErrorIs(t, err, ErrMetaUndecodable)
		assert.True(t, report.WriterLockPresent)
		assert.False(t, report.MetaLockPresent)
		assert.Empty(t, report.SchemaFieldNames)
	})

	t.Run("missing index reports not-exist and no locks", func(t *testing.T) {
		// when
		report, err := Check(filepath.Join(t.TempDir(), "absent"))

		// then
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.NotErrorIs(t, err, ErrMetaUndecodable)
		assert.False(t, report.WriterLockPresent)
		assert.False(t, report.MetaLockPresent)
	})
}
