package anystoreprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
)

type recordingReporter struct {
	mu      sync.Mutex
	reasons []string
}

func (r *recordingReporter) Report(reason string, _ map[string]any, _ debugreporter.Capture) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reasons = append(r.reasons, reason)
}

func writeTestDB(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db, err := anystore.Open(ctx, path, &anystore.Config{})
	require.NoError(t, err)
	coll, err := db.Collection(ctx, "test")
	require.NoError(t, err)
	for i := 0; i < 500; i++ {
		doc := anyenc.MustParseJson(fmt.Sprintf(`{"id":"doc-%d","pad":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}`, i))
		require.NoError(t, coll.Insert(ctx, doc))
	}
	require.NoError(t, db.Close())
}

// A damaged v2 file must be reported as DB_CORRUPTION and replaced by a new empty database,
// just like a damaged v1 file used to be.
func TestOpenDatabaseWithReinit_DamagedV2File(t *testing.T) {
	damages := map[string]func(t *testing.T, path string){
		"header overwritten": func(t *testing.T, path string) {
			f, err := os.OpenFile(path, os.O_WRONLY, 0)
			require.NoError(t, err)
			defer f.Close()
			_, err = f.WriteAt(make([]byte, 100), 0)
			require.NoError(t, err)
		},
		"file cut in half": func(t *testing.T, path string) {
			st, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.Truncate(path, st.Size()/2))
		},
		"file cut inside the header": func(t *testing.T, path string) {
			require.NoError(t, os.Truncate(path, 40))
		},
		"text file": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("this is not a database"), 0o600))
		},
	}
	for name, damage := range damages {
		t.Run(name, func(t *testing.T) {
			// given
			path := filepath.Join(t.TempDir(), "objects.db")
			writeTestDB(t, path)
			damage(t, path)
			reporter := &recordingReporter{}

			// when
			db, err := openDatabaseWithReinit(context.Background(), &anystore.Config{}, path, reporter, nil)

			// then
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			assert.Equal(t, []string{"DB_CORRUPTION"}, reporter.reasons)
			_, err = db.OpenCollection(context.Background(), "test")
			assert.ErrorIs(t, err, anystore.ErrCollectionNotFound, "the damaged data is gone, the database is new")
		})
	}
}

func TestOpenDatabaseWithReinit_IntactFileIsKept(t *testing.T) {
	// given
	path := filepath.Join(t.TempDir(), "objects.db")
	writeTestDB(t, path)
	reporter := &recordingReporter{}

	// when
	db, err := openDatabaseWithReinit(context.Background(), &anystore.Config{}, path, reporter, nil)

	// then
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Empty(t, reporter.reasons)
	_, err = db.OpenCollection(context.Background(), "test")
	assert.NoError(t, err)
}
