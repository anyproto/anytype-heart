package fileobject

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestListNonIndexedFiles(t *testing.T) {
	fx := newFixture(t)

	fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
		// basic object -> ignore
		map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("id1"),
			bundle.RelationKeySpaceId:        domain.String("space1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_basic),
		},
		// file object -> no index
		givenNonIndexedFileObject("id2", "space1", model.ObjectType_file),
		// file object -> indexed
		givenIndexedFileObject("id3", "space1", model.ObjectType_audio),
	})
	fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{
		// file object -> no index
		givenNonIndexedFileObject("id3", "space2", model.ObjectType_image),
		givenNonIndexedFileObject("id4", "space2", model.ObjectType_pdf),
		givenNonIndexedFileObject("id5", "space2", model.ObjectType_video),
		givenNonIndexedFileObject("id6", "space2", model.ObjectType_audio),
	})

	got, err := fx.listNonIndexedFiles()
	require.NoError(t, err)

	want := []database.Record{
		givenNonIndexedFileObject("id2", "space1", model.ObjectType_file).Record(),
		givenNonIndexedFileObject("id3", "space2", model.ObjectType_image).Record(),
		givenNonIndexedFileObject("id4", "space2", model.ObjectType_pdf).Record(),
		givenNonIndexedFileObject("id5", "space2", model.ObjectType_video).Record(),
		givenNonIndexedFileObject("id6", "space2", model.ObjectType_audio).Record(),
	}

	assert.ElementsMatch(t, want, got)
}

func givenNonIndexedFileObject(id string, spaceId string, layout model.ObjectTypeLayout) objectstore.TestObject {
	return map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
		bundle.RelationKeyResolvedLayout: domain.Int64(layout),
	}
}

func givenIndexedFileObject(id string, spaceId string, layout model.ObjectTypeLayout) objectstore.TestObject {
	return map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:                   domain.String(id),
		bundle.RelationKeySpaceId:              domain.String(spaceId),
		bundle.RelationKeyResolvedLayout:       domain.Int64(layout),
		bundle.RelationKeyFileVariantIds:       domain.StringList([]string{"data"}),
		bundle.RelationKeyFileVariantPaths:     domain.StringList([]string{"data"}),
		bundle.RelationKeyFileVariantChecksums: domain.StringList([]string{"data"}),
		bundle.RelationKeyFileVariantMills:     domain.StringList([]string{"data"}),
		bundle.RelationKeyFileVariantWidths:    domain.Int64List([]int64{1024}),
		bundle.RelationKeyFileVariantKeys:      domain.StringList([]string{"data"}),
		bundle.RelationKeyFileVariantOptions:   domain.StringList([]string{"data"}),
	}
}
