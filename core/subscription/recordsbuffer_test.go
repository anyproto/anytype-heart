package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

func bufferRecord(id string, name string) database.Record {
	return database.Record{Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String(id),
		bundle.RelationKeyName: domain.String(name),
	})}
}

func TestRecordsBuffer(t *testing.T) {
	t.Run("coalesces multiple updates of one object to the latest", func(t *testing.T) {
		// given
		b := newRecordsBuffer()

		// when
		require.NoError(t, b.Add(bufferRecord("id1", "v1")))
		require.NoError(t, b.Add(bufferRecord("id2", "other")))
		require.NoError(t, b.Add(bufferRecord("id1", "v2")))
		require.NoError(t, b.Add(bufferRecord("id1", "v3")))

		// then
		batch, err := b.Wait(context.Background())
		require.NoError(t, err)
		require.Len(t, batch, 2)
		assert.Equal(t, "v3", batch[0].Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "id1", batch[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "id2", batch[1].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("coalescing restarts after a batch is consumed", func(t *testing.T) {
		// given
		b := newRecordsBuffer()
		require.NoError(t, b.Add(bufferRecord("id1", "v1")))
		_, err := b.Wait(context.Background())
		require.NoError(t, err)

		// when
		require.NoError(t, b.Add(bufferRecord("id1", "v2")))

		// then
		batch, err := b.Wait(context.Background())
		require.NoError(t, err)
		require.Len(t, batch, 1)
		assert.Equal(t, "v2", batch[0].Details.GetString(bundle.RelationKeyName))
	})

	t.Run("wait is unblocked by an asynchronous add", func(t *testing.T) {
		// given
		b := newRecordsBuffer()
		go func() {
			time.Sleep(10 * time.Millisecond)
			_ = b.Add(bufferRecord("id1", "v1"))
		}()

		// when
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		batch, err := b.Wait(ctx)

		// then
		require.NoError(t, err)
		require.Len(t, batch, 1)
	})

	t.Run("wait respects context cancellation", func(t *testing.T) {
		// given
		b := newRecordsBuffer()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		// when
		_, err := b.Wait(ctx)

		// then
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("close unblocks wait and rejects adds", func(t *testing.T) {
		// given
		b := newRecordsBuffer()
		done := make(chan error, 1)
		go func() {
			_, err := b.Wait(context.Background())
			done <- err
		}()

		// when
		time.Sleep(10 * time.Millisecond)
		b.Close()

		// then
		select {
		case err := <-done:
			require.ErrorIs(t, err, errRecordsBufferClosed)
		case <-time.After(5 * time.Second):
			t.Fatal("wait was not unblocked by close")
		}
		require.ErrorIs(t, b.Add(bufferRecord("id1", "v1")), errRecordsBufferClosed)
	})
}
