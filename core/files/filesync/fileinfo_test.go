package filesync

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
)

func TestMarshalUnmarshal(t *testing.T) {
	arena := &anyenc.Arena{}

	fi := givenFileInfo()

	doc := marshalFileInfo(fi, arena)

	got, err := unmarshalFileInfo(doc)

	require.NoError(t, err)
	assert.Equal(t, fi, got)
}

func newTestRepo(t *testing.T) *anystoreFileRepository {
	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()

	db := provider.GetCommonDb()
	coll, err := db.Collection(ctx, "queue")
	require.NoError(t, err)

	repo := newAnystoreFileRepository(coll)
	go repo.runSubscriptions()

	return repo
}

func TestRepositorySubscriptions(t *testing.T) {
	repo := newTestRepo(t)

	updatesCh1 := repo.subscribe()
	updatesCh2 := repo.subscribe()

	want := givenFileInfo()

	go func() {
		for range 2 {
			got := <-updatesCh1
			assert.Equal(t, want, got)
		}
	}()
	go func() {
		for range 2 {
			got := <-updatesCh2
			assert.Equal(t, want, got)
		}
	}()

	for range 2 {
		err := repo.upsert(want)
		require.NoError(t, err)
	}
}

func TestRepositoryUnsubscribe(t *testing.T) {
	repo := newTestRepo(t)

	updatesCh := repo.subscribe()
	repo.unsubscribe(updatesCh)

	err := repo.upsert(givenFileInfo())
	require.NoError(t, err)

	timeout := time.After(50 * time.Millisecond)
	select {
	case _, ok := <-updatesCh:
		assert.False(t, ok)
	case <-timeout:
		t.Fatal("unsubscribe timeout")
	}
}

func TestRepositoryQuery(t *testing.T) {
	t.Run("no rows", func(t *testing.T) {
		repo := newTestRepo(t)

		_, err := repo.queryOne(nil, nil)
		assert.ErrorIs(t, err, errNoRows)
	})

	t.Run("no filters", func(t *testing.T) {
		repo := newTestRepo(t)

		want := givenFileInfo()

		err := repo.upsert(want)
		require.NoError(t, err)

		got, err := repo.queryOne(nil, nil)
		require.NoError(t, err)
		assert.Equal(t, want, *got)
	})

	t.Run("state filter", func(t *testing.T) {
		repo := newTestRepo(t)

		want := givenFileInfo()

		err := repo.upsert(want)
		require.NoError(t, err)

		got, err := repo.queryOne(query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(FileStateUploading)),
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, want, *got)
	})

	t.Run("state filter and sort", func(t *testing.T) {
		repo := newTestRepo(t)

		infos := make([]FileInfo, 3)
		for i := range 3 {
			want := givenFileInfo()
			want.ObjectId = fmt.Sprintf("object%d", i)
			want.HandledAt = want.HandledAt.Add(time.Duration(i) * time.Minute)

			err := repo.upsert(want)
			require.NoError(t, err)

			infos[i] = want
		}

		got, err := repo.queryOne(query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(FileStateUploading)),
		}, []query.Sort{
			&query.SortField{
				Field:   "handledAt",
				Path:    []string{"handledAt"},
				Reverse: false,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, infos[0], *got)

		got, err = repo.queryOne(query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(FileStateUploading)),
		}, []query.Sort{
			&query.SortField{
				Field:   "handledAt",
				Path:    []string{"handledAt"},
				Reverse: true,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, infos[2], *got)
	})
}

func givenFileInfo() FileInfo {

	testFileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")

	return FileInfo{
		FileId:        testFileId,
		SpaceId:       "space1",
		ObjectId:      "object1",
		State:         FileStateUploading,
		ScheduledAt:   time.Date(2021, time.December, 31, 12, 55, 12, 0, time.UTC),
		HandledAt:     time.Date(2022, time.January, 1, 13, 56, 13, 0, time.UTC),
		Variants:      []domain.FileId{"variant1", "variant2"},
		AddedByUser:   true,
		Imported:      true,
		BytesToUpload: 123,
		CidsToBind: map[cid.Cid]struct{}{
			cid.MustParse(testFileId.String()): {},
		},
	}
}
