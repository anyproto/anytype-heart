package editor

import (
	"errors"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/stretchr/testify/assert"
)

func TestReconcileRunner(t *testing.T) {
	t.Run("runs snapshots in order", func(t *testing.T) {
		// given
		var r reconcileRunner
		var got []int64
		s1, s2 := r.nextSeq(), r.nextSeq()

		// when
		r.run(s1, func() { got = append(got, s1) })
		r.run(s2, func() { got = append(got, s2) })

		// then
		assert.Equal(t, []int64{s1, s2}, got)
	})

	t.Run("stale snapshot is dropped after a newer one completed", func(t *testing.T) {
		// given
		var r reconcileRunner
		var got []int64
		s1, s2 := r.nextSeq(), r.nextSeq()

		// when: the newer snapshot's goroutine wins the race
		r.run(s2, func() { got = append(got, s2) })
		r.run(s1, func() { got = append(got, s1) })

		// then: running s1 would revert details and persist a superseded marker
		assert.Equal(t, []int64{s2}, got)
	})
}

func TestIsMissingObjectError(t *testing.T) {
	assert.True(t, isMissingObjectError(spacestorage.ErrTreeStorageAlreadyDeleted))
	assert.True(t, isMissingObjectError(treestorage.ErrUnknownTreeId))
	assert.False(t, isMissingObjectError(errors.New("disk failure")))
	assert.False(t, isMissingObjectError(nil))
}
