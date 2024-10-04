package objectcreator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type objKey interface {
	URL() string
	BundledURL() string
}

func TestInstaller_queryDeletedObjects(t *testing.T) {
	// given
	var (
		spaceId         = "spaceId"
		sourceObjectIds = []string{}
		validObjectIds  = []string{}
	)

	store := objectstore.NewStoreFixture(t)

	for _, obj := range []struct {
		isDeleted, isArchived bool
		spaceId               string
		key                   objKey
	}{
		{false, false, spaceId, bundle.TypeKeyGoal},
		{false, false, spaceId, bundle.RelationKeyGenre},
		{true, false, spaceId, bundle.TypeKeyTask},
		{true, false, spaceId, bundle.RelationKeyLinkedProjects},
		{false, true, spaceId, bundle.TypeKeyBook},
		{false, true, spaceId, bundle.RelationKeyStarred},
		{true, true, spaceId, bundle.TypeKeyProject},    // not valid, but we should handle this
		{true, true, spaceId, bundle.RelationKeyArtist}, // not valid, but we should handle this
		{false, true, "otherSpaceId", bundle.TypeKeyDiaryEntry},
		{true, false, "otherSpaceId", bundle.RelationKeyAudioAlbum},
	} {
		store.AddObjects(t, obj.spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:           pbtypes.String(obj.key.URL()),
			bundle.RelationKeySpaceId:      pbtypes.String(obj.spaceId),
			bundle.RelationKeySourceObject: pbtypes.String(obj.key.BundledURL()),
			bundle.RelationKeyIsDeleted:    pbtypes.Bool(obj.isDeleted),
			bundle.RelationKeyIsArchived:   pbtypes.Bool(obj.isArchived),
			bundle.RelationKeyLayout:       pbtypes.Int64(int64(model.ObjectType_relation)),
		}})
		sourceObjectIds = append(sourceObjectIds, obj.key.BundledURL())
		if obj.spaceId == spaceId && (obj.isDeleted || obj.isArchived) {
			validObjectIds = append(validObjectIds, obj.key.URL())
		}
	}

	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().Id().Return(spaceId)

	s := service{objectStore: store}

	// when
	records, err := s.queryDeletedObjects(spc, sourceObjectIds)

	// then
	assert.NoError(t, err)
	assert.Len(t, records, 6)
	for _, det := range records {
		assert.Contains(t, validObjectIds, pbtypes.GetString(det.Details, bundle.RelationKeyId.String()))
	}
}
