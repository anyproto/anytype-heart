package spaceindex

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *dsObjectStore) DeleteDetails(ctx context.Context, ids []string) error {
	txn, err := s.db.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("write txn: %w", err)
	}
	defer func() {
		_ = txn.Rollback()
	}()
	for _, id := range ids {
		err := s.objects.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return fmt.Errorf("delete object %s: %w", id, err)
		}

		err = s.headsState.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return fmt.Errorf("delete headsState %s: %w", id, err)
		}
	}
	return txn.Commit()
}

// SnapshotOnDelete are the relations captured into the deletedSnapshot map when an object is
// deleted. Everything else is dropped.
//
// Deletion is irreversible and takes the object's tree with it (objecttree storage.Delete removes
// every change, root included), so whatever is not listed here is gone for good — there is nothing
// left for a client to resolve against. The set is deliberately narrow: it answers "who made this,
// when, where, and what was it" for the deletion audit, and carries no user-authored content.
// Notably absent: name, description and snippet.
//
// A derived object (a type, a property) also keeps its IDENTITY keys — uniqueKey, relationKey and
// apiObjectKey. They are what a cold recovery on another device would carry anyway (a derived object
// is uninstalled, never removed from the tree), and a value an object still holds under a removed
// property is served under that apiObjectKey; without them the local index alone, right after the
// delete and until the next space load, could spell the property only by its stored key.
//
// They are captured into a nested map rather than left under their own keys because four of them —
// resolvedLayout, type, lastModifiedDate and fileId — are indexed on the objects collection. Keeping
// them top-level would file every tombstone under a live value: fileId's index is sparse, so
// tombstones would add rows to an index that had none of them, and the other three would move out of
// the null bucket into the ranges real queries scan. any-store indexes exact declared paths only
// (["resolvedLayout"]), so nothing nested is reachable from any index — and no QueryRaw caller, which
// bypasses the implicit isDeleted filter, can match a tombstone by layout or type by accident.
var SnapshotOnDelete = []domain.RelationKey{
	bundle.RelationKeyCreator,
	bundle.RelationKeyCreatedDate,
	bundle.RelationKeyAddedDate,
	bundle.RelationKeyCreatedInContext,
	bundle.RelationKeyCreatedInContextRef,
	bundle.RelationKeyLastModifiedBy,
	bundle.RelationKeyLastModifiedDate,
	bundle.RelationKeyType,
	bundle.RelationKeyResolvedLayout,
	bundle.RelationKeySizeInBytes,
	bundle.RelationKeyFileId,
	bundle.RelationKeyUniqueKey,
	bundle.RelationKeyRelationKey,
	bundle.RelationKeyApiObjectKey,
}

// preservedOnDelete are the top-level relations that survive a delete. All of them are unindexed,
// which is the point — see SnapshotOnDelete.
//
// The deletion-side keys are written after the fact by the deletion audit materializer, and
// reindexDeletedObjects re-runs DeleteObject for every deleted tree id on each reindex; without them
// here the audit data would be wiped on every reindex.
var preservedOnDelete = []domain.RelationKey{
	bundle.RelationKeyDeletedBy,
	bundle.RelationKeyDeletedDate,
	bundle.RelationKeyDeletionChangeId,
	bundle.RelationKeyDeletedSnapshot,
	bundle.RelationKeyDeletedLayout,
}

// derivedLayouts are the layouts whose tombstones keep deletedLayout at the
// TOP level (sparse-indexed, see store.go): a type or a property that was
// deleted is still addressed by the slug its objects serve, and answering
// "is this spelling a removed type" must be an exact query, never a scan of
// every deleted row — a bounded scan that misses one lets the spelling fall
// through to a live type that merely shares its name.
var derivedLayouts = map[int64]bool{
	int64(model.ObjectType_objectType): true,
	int64(model.ObjectType_relation):   true,
}

// snapshotOnDelete captures SnapshotOnDelete out of an object's live details. It returns false when
// there is nothing to capture, which is how re-deleting an already-tombstoned row leaves the existing
// snapshot alone instead of replacing it with an empty one.
func snapshotOnDelete(details *domain.Details) (domain.Value, bool) {
	captured := make(map[string]domain.Value, len(SnapshotOnDelete))
	for _, key := range SnapshotOnDelete {
		if value, ok := details.TryGet(key); ok {
			captured[key.String()] = value
		}
	}
	if len(captured) == 0 {
		return domain.Value{}, false
	}
	return domain.NewValueMap(captured), true
}

// DeleteObject removes all details, leaving only id, isDeleted, the preservedOnDelete relations and
// a deletedSnapshot of what the object was. Re-running it on an existing tombstone is a no-op: the
// preserved values are read back from the tombstone itself.
func (s *dsObjectStore) DeleteObject(id string) error {
	// s.WriteTx (not s.db.WriteTx): the tombstone notification must reach
	// subscribers only after the tx commits
	txn, err := s.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("write txn: %w", err)
	}
	defer func() {
		txn.Rollback()
	}()

	oldDetails, err := s.getDetails(txn.Context(), id)
	if err != nil {
		return fmt.Errorf("delete: get current details: %w", err)
	}

	newDetails := oldDetails.CopyOnlyKeys(preservedOnDelete...)
	if snapshot, ok := snapshotOnDelete(oldDetails); ok {
		newDetails.Set(bundle.RelationKeyDeletedSnapshot, snapshot)
	}
	// the marker is derived from the RETAINED snapshot, fresh or old, so
	// re-running the delete on a tombstone written before the marker
	// existed backfills it (reindexDeletedObjects re-runs every tombstone)
	if snapshot, ok := newDetails.TryMapValue(bundle.RelationKeyDeletedSnapshot); ok {
		if layout := snapshot.GetInt64(bundle.RelationKeyResolvedLayout.String()); derivedLayouts[layout] {
			newDetails.SetInt64(bundle.RelationKeyDeletedLayout, layout)
		}
	}
	newDetails.SetString(bundle.RelationKeyId, id)
	newDetails.SetString(bundle.RelationKeySpaceId, s.spaceId)
	newDetails.SetBool(bundle.RelationKeyIsDeleted, true)

	// do not completely remove object details, so we can distinguish links to deleted and not-yet-loaded objects
	err = s.UpdateObjectDetails(txn.Context(), id, newDetails)
	if err != nil {
		return fmt.Errorf("delete: update details: %w", err)
	}

	err = s.headsState.DeleteId(txn.Context(), id)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return fmt.Errorf("delete: heads state delete: %w", err)
	}
	err = s.eraseLinksForObject(txn.Context(), id)
	if err != nil {
		return fmt.Errorf("delete: erase links: %w", err)
	}
	// add to ft index queue in order to remove the object
	// it will find the object not found error and remove all the docs
	_, _, err = s.fulltextQueue.AddToIndexQueue(s.componentCtx, domain.FullID{
		ObjectID: id,
		SpaceID:  s.spaceId,
	})
	if err != nil {
		log.Errorf("delete object %s: add to fulltext queue: %v", id, err)
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("delete object info: %w", err)
	}

	return nil
}

func (s *dsObjectStore) DeleteLinks(ids []string) error {
	txn, err := s.links.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("read txn: %w", err)
	}
	defer func() {
		_ = txn.Rollback()
	}()
	for _, id := range ids {
		err := s.eraseLinksForObject(txn.Context(), id)
		if err != nil {
			return fmt.Errorf("erase links for %s: %w", id, err)
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) eraseLinksForObject(ctx context.Context, from string) error {
	err := s.links.DeleteId(ctx, from)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return err
	}
	return nil
}
