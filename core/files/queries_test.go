package files

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func givenFileVariantRecord(isDeleted bool) spaceindex.TestObject {
	obj := spaceindex.TestObject{
		bundle.RelationKeyId:                   domain.String("existingFileObjectId"),
		bundle.RelationKeyFileId:               domain.String("existingFileId"),
		bundle.RelationKeyFileSourceChecksum:   domain.String("sourceChecksum1"),
		bundle.RelationKeyFileVariantIds:       domain.StringList([]string{"variantCid1"}),
		bundle.RelationKeyFileVariantChecksums: domain.StringList([]string{"variantChecksum1"}),
		bundle.RelationKeyFileVariantMills:     domain.StringList([]string{"/blob"}),
		bundle.RelationKeyFileVariantPaths:     domain.StringList([]string{"file"}),
		bundle.RelationKeyFileVariantKeys:      domain.StringList([]string{"key1"}),
		bundle.RelationKeyFileVariantOptions:   domain.StringList([]string{"options1"}),
		bundle.RelationKeyFileVariantWidths:    domain.Int64List([]int64{0}),
	}
	if isDeleted {
		obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
	}
	return obj
}

func TestGetFileVariantBySourceChecksum(t *testing.T) {
	ctx := context.Background()

	t.Run("existing file object is reused", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.objectStore.(*objectstore.StoreFixture).AddObjects(t, spaceId, []spaceindex.TestObject{
			givenFileVariantRecord(false),
		})

		// when
		got, err := fx.Service.(*service).getFileVariantBySourceChecksum(ctx, "/blob", "sourceChecksum1", "options1")

		// then
		require.NoError(t, err)
		assert.Equal(t, domain.FileId("existingFileId"), got.fileId)
	})

	t.Run("deleted file object is not reused: its blocks may be gone from the node", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.objectStore.(*objectstore.StoreFixture).AddObjects(t, spaceId, []spaceindex.TestObject{
			givenFileVariantRecord(true),
		})

		// when
		_, err := fx.Service.(*service).getFileVariantBySourceChecksum(ctx, "/blob", "sourceChecksum1", "options1")

		// then
		require.Error(t, err)
	})

	t.Run("deleted file object is not reused by variant checksum either", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.objectStore.(*objectstore.StoreFixture).AddObjects(t, spaceId, []spaceindex.TestObject{
			givenFileVariantRecord(true),
		})

		// when
		_, _, err := fx.Service.(*service).getFileVariantByChecksum(ctx, "/blob", "variantChecksum1")

		// then
		require.Error(t, err)
	})
}
