package durability

import (
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

type flushCall struct {
	timeout     time.Duration
	waitPending bool
	mode        anystore.FlushMode
}

type flusherMock struct {
	calls []flushCall
}

func (f *flusherMock) Flush(timeout time.Duration, waitPending bool, mode anystore.FlushMode) {
	f.calls = append(f.calls, flushCall{timeout: timeout, waitPending: waitPending, mode: mode})
}

func TestStateChange(t *testing.T) {
	t.Run("background uses blocking fsync, not checkpoint", func(t *testing.T) {
		// given
		spaceCore := &flusherMock{}
		provider := &flusherMock{}
		d := &durability{spaceCore: spaceCore, anystoreProvider: provider}
		want := []flushCall{{timeout: time.Second * 10, waitPending: true, mode: anystore.FlushModeFsync}}

		// when
		d.StateChange(int(domain.CompStateAppWentBackground))

		// then
		assert.Equal(t, want, spaceCore.calls)
		assert.Equal(t, want, provider.calls)
	})

	t.Run("closing uses best-effort passive checkpoint", func(t *testing.T) {
		// given
		spaceCore := &flusherMock{}
		provider := &flusherMock{}
		d := &durability{spaceCore: spaceCore, anystoreProvider: provider}
		want := []flushCall{{timeout: time.Second * 3, waitPending: false, mode: anystore.FlushModeCheckpointPassive}}

		// when
		d.StateChange(int(domain.CompStateAppClosingInitiated))

		// then
		assert.Equal(t, want, spaceCore.calls)
		assert.Equal(t, want, provider.calls)
	})

	t.Run("other states do not flush", func(t *testing.T) {
		// given
		spaceCore := &flusherMock{}
		provider := &flusherMock{}
		d := &durability{spaceCore: spaceCore, anystoreProvider: provider}

		// when
		d.StateChange(int(domain.CompStateAppWentForeground))

		// then
		require.Empty(t, spaceCore.calls)
		require.Empty(t, provider.calls)
	})
}
