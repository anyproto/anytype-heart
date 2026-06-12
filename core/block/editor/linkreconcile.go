package editor

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
)

// reconcileRunner serializes the background store reconciles of a link-collection editor
// (Archive, Dashboard) and drops runs whose snapshot was superseded by a newer completed one.
// Snapshots (link ids + heads marker + seq) are taken in the after-apply hook under the
// smartblock lock, so seq order matches apply order; serialization guarantees the persisted
// reconcile marker always describes the snapshot whose detail writes landed last.
type reconcileRunner struct {
	seq     atomic.Int64
	mu      sync.Mutex
	doneSeq int64
}

func (r *reconcileRunner) nextSeq() int64 {
	return r.seq.Add(1)
}

func (r *reconcileRunner) run(seq int64, f func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if seq <= r.doneSeq {
		// a newer snapshot has already been reconciled; running this one would
		// revert details and persist a marker for a superseded tree state
		return
	}
	f()
	r.doneSeq = seq
}

// isMissingObjectError reports whether the target of a local-details write does not exist
// locally (deleted or never synced); such targets carry no details, so this is not a
// reconcile failure.
func isMissingObjectError(err error) bool {
	return errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) || errors.Is(err, treestorage.ErrUnknownTreeId)
}
