package spaceindex

import (
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
)

// deletionAuditMarkId is the single document the deletionAudit collection holds. The collection is
// already per-space, so one row is enough.
const deletionAuditMarkId = "mark"

// GetDeletionAuditMark returns the space settings tree heads as of the last completed deletion audit
// materialization, or "" when nothing has been materialized yet.
//
// Heads rather than a change id on purpose. Resuming an iteration from the last change seen is only
// correct while history grows at the end, and a settings tree does not: a change synced from another
// device attaches wherever its parent is, which can be behind the change we stopped at. Heads can
// only answer "same tree or not" — which is the question worth asking, because what matters is
// whether a walk is needed at all, and heads answer it from a single primary-key lookup in head
// storage, without building the tree first.
func (s *dsObjectStore) GetDeletionAuditMark() (string, error) {
	doc, err := s.deletionAudit.FindId(s.componentCtx, deletionAuditMarkId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find deletion audit mark: %w", err)
	}
	return string(doc.Value().GetStringBytes("value")), nil
}

// SetDeletionAuditMark records the settings tree heads that have been fully materialized. Losing it
// costs one redundant walk, never correctness: replaying a change writes the values it wrote before.
func (s *dsObjectStore) SetDeletionAuditMark(heads string) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	obj := arena.NewObject()
	obj.Set("id", arena.NewString(deletionAuditMarkId))
	obj.Set("value", arena.NewString(heads))
	if err := s.deletionAudit.UpsertOne(s.componentCtx, obj); err != nil {
		return fmt.Errorf("upsert deletion audit mark: %w", err)
	}
	return nil
}
