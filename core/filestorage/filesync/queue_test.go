package filesync

import (
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func generateItem(fileId string, ts int64) *QueueItem {
	return &QueueItem{
		SpaceId:     "spaceId",
		FileId:      domain.FileId(fileId),
		Timestamp:   ts,
		AddedByUser: true,
		Imported:    true,
	}
}

func newTestQueue(t *testing.T) *queue {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)

	q, err := newQueue(db, uploadKeyPrefix, uploadKey)
	require.NoError(t, err)

	return q
}

func TestQueue(t *testing.T) {

	t.Run("length", func(t *testing.T) {
		q := newTestQueue(t)

		ctx := context.Background()

		now := time.Now().UnixMilli()
		err := q.add(ctx, generateItem("id1", now))
		require.NoError(t, err)
		err = q.add(ctx, generateItem("id2", now+1))
		require.NoError(t, err)
		err = q.add(ctx, generateItem("id3", now+2))
		require.NoError(t, err)

		length := q.length()
		assert.Equal(t, 3, length)
	})

	t.Run("add and get", func(t *testing.T) {
		q := newTestQueue(t)

		item := generateItem("id1", time.Now().UnixMilli())

		err := q.add(ctx, generateItem("id1", time.Now().UnixMilli()))
		require.NoError(t, err)

		ok := q.has(item.FullFileId())
		assert.True(t, ok)

		got, err := q.getNext(ctx)
		require.NoError(t, err)
		assert.Equal(t, item, got)

		ok = q.has(item.FullFileId())
		assert.False(t, ok)
	})

	t.Run("add, remove and get", func(t *testing.T) {
		q := newTestQueue(t)

		item := generateItem("id1", time.Now().UnixMilli())

		err := q.add(ctx, generateItem("id1", time.Now().UnixMilli()))
		require.NoError(t, err)

		err = q.remove(item.FullFileId())
		require.NoError(t, err)

		ok := q.has(item.FullFileId())
		assert.False(t, ok)

		_, err = q.getNext(ctx)
		require.Error(t, err)
	})

	t.Run("remove, add and get: expect", func(t *testing.T) {
		q := newTestQueue(t)

		item := generateItem("id1", time.Now().UnixMilli())

		err := q.remove(item.FullFileId())

		err = q.add(ctx, generateItem("id1", time.Now().UnixMilli()))
		require.NoError(t, err)

		got, err := q.getNext(ctx)
		require.NoError(t, err)

		assert.Equal(t, item, got)
	})

	t.Run("wait for item", func(t *testing.T) {
		q := newTestQueue(t)

		item := generateItem("id1", time.Now().UnixMilli())
		done := make(chan struct{})
		go func() {
			time.Sleep(1 * time.Millisecond)
			err := q.add(ctx, item)
			require.NoError(t, err)
			close(done)
		}()

		got, err := q.getNext(ctx)
		require.NoError(t, err)

		assert.Equal(t, got, item)

		select {
		case <-done:
		case <-time.After(10 * time.Millisecond):
			t.Fatal("timeout")
		}
	})
}

func TestQueueRestore(t *testing.T) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)

	q, err := newQueue(db, uploadKeyPrefix, uploadKey)
	require.NoError(t, err)

	now := time.Now().UnixMilli()
	items := []*QueueItem{
		generateItem("id1", now+2),
		generateItem("id2", now),
		generateItem("id3", now+1),
	}

	for _, item := range items {
		err = q.add(ctx, item)
		require.NoError(t, err)
	}

	err = q.close()
	require.NoError(t, err)

	q, err = newQueue(db, uploadKeyPrefix, uploadKey)
	require.NoError(t, err)

	// Chronological order
	want := []*QueueItem{
		generateItem("id2", now),
		generateItem("id3", now+1),
		generateItem("id1", now+2),
	}
	for i := 0; i < len(items); i++ {
		got, err := q.getNext(ctx)
		require.NoError(t, err)
		assert.Equal(t, want[i], got)
	}

}
