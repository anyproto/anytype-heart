package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// An empty spaceId is a caller bug, not a backend failure: the proto declares BAD_INPUT for it, and
// the handler must reject it before reaching the space service.
func TestObjectDeletionAudit_EmptySpaceId_BadInput(t *testing.T) {
	mw := &Middleware{}

	resp := mw.ObjectDeletionAudit(context.Background(), &pb.RpcObjectDeletionAuditRequest{SpaceId: ""})

	assert.Equal(t, pb.RpcObjectDeletionAuditResponseError_BAD_INPUT, resp.Error.Code)
	assert.Empty(t, resp.Records)
	assert.Zero(t, resp.Total)
}

// A negative limit or offset would reach any-store as a huge uint. Reject it up front.
func TestObjectDeletionAudit_NegativePaging_BadInput(t *testing.T) {
	mw := &Middleware{}

	for _, req := range []*pb.RpcObjectDeletionAuditRequest{
		{SpaceId: "space1", Limit: -1},
		{SpaceId: "space1", Offset: -1},
	} {
		resp := mw.ObjectDeletionAudit(context.Background(), req)
		assert.Equal(t, pb.RpcObjectDeletionAuditResponseError_BAD_INPUT, resp.Error.Code)
	}
}

// The tombstone nests the creation side under deletedSnapshot to keep those keys out of the
// objectstore indexes. A response Struct has no indexes, so the handler flattens it back out and the
// caller's keys project it as if it had been stored flat.
func TestFlattenDeletedSnapshot(t *testing.T) {
	t.Run("lifts snapshot fields to the top level", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyId, "id1")
		details.SetString(bundle.RelationKeyDeletedBy, "_participant_test_deleter")
		details.Set(bundle.RelationKeyDeletedSnapshot, domain.NewValueMap(map[string]domain.Value{
			"creator":        domain.String("_participant_test_creator"),
			"resolvedLayout": domain.Int64(1),
		}))

		// when
		got := flattenDeletedSnapshot(details)

		// then
		assert.Equal(t, "_participant_test_creator", got.GetString(bundle.RelationKeyCreator))
		assert.Equal(t, int64(1), got.GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Equal(t, "_participant_test_deleter", got.GetString(bundle.RelationKeyDeletedBy))
		assert.False(t, got.Has(bundle.RelationKeyDeletedSnapshot), "the wrapper itself is not returned")
	})

	t.Run("leaves a tombstone without a snapshot alone", func(t *testing.T) {
		// an object deleted before snapshot preservation shipped
		// given
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyId, "id1")
		details.SetString(bundle.RelationKeyDeletedBy, "_participant_test_deleter")

		// when
		got := flattenDeletedSnapshot(details)

		// then
		assert.Equal(t, "_participant_test_deleter", got.GetString(bundle.RelationKeyDeletedBy))
		assert.False(t, got.Has(bundle.RelationKeyCreator))
	})

	t.Run("does not mutate the stored details", func(t *testing.T) {
		// List hands out the records the store returned; flattening must not write through to them
		// given
		details := domain.NewDetails()
		details.Set(bundle.RelationKeyDeletedSnapshot, domain.NewValueMap(map[string]domain.Value{
			"creator": domain.String("_participant_test_creator"),
		}))

		// when
		flattenDeletedSnapshot(details)

		// then
		assert.True(t, details.Has(bundle.RelationKeyDeletedSnapshot))
		assert.False(t, details.Has(bundle.RelationKeyCreator))
	})
}

func TestDeletionAuditKeys_EmptyRequest_UsesDefaultsPlusForced(t *testing.T) {
	keys := deletionAuditKeys(nil)

	assert.Subset(t, keys, defaultDeletionAuditKeys)
	assert.Subset(t, keys, forcedDeletionAuditKeys)
}

func TestDeletionAuditKeys_CallerKeys_ForcedAlwaysIncluded(t *testing.T) {
	keys := deletionAuditKeys([]string{"creator"})

	assert.Contains(t, keys, bundle.RelationKeyCreator)
	assert.Contains(t, keys, bundle.RelationKeyId)
	assert.Contains(t, keys, bundle.RelationKeyDeletedBy)
	assert.Contains(t, keys, bundle.RelationKeyDeletedDate)
	// caller keys do not pull in the default set
	assert.NotContains(t, keys, bundle.RelationKeySizeInBytes)
}

func TestDeletionAuditKeys_Deduplicates(t *testing.T) {
	keys := deletionAuditKeys([]string{"deletedBy", "deletedBy"})

	count := 0
	for _, k := range keys {
		if k == bundle.RelationKeyDeletedBy {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

// deletionAuditKeys must not mutate the package-level default slice: append() on a slice with spare
// capacity would write through to the backing array and corrupt it for every later call.
func TestDeletionAuditKeys_DoesNotMutateDefaults(t *testing.T) {
	before := append([]domain.RelationKey{}, defaultDeletionAuditKeys...)

	deletionAuditKeys(nil)
	deletionAuditKeys(nil)

	assert.Equal(t, before, defaultDeletionAuditKeys)
}
