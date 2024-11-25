package spaceindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGetObjectType(t *testing.T) {
	t.Run("get bundled type", func(t *testing.T) {
		s := NewStoreFixture(t)

		id := bundle.TypeKeyTask.BundledURL()
		got, err := s.GetObjectType(id)
		require.NoError(t, err)

		want := bundle.MustGetType(bundle.TypeKeyTask)
		assert.Equal(t, want, got)
	})

	t.Run("with object is not type expect error", func(t *testing.T) {
		s := NewStoreFixture(t)

		id := "id1"
		obj := TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyType: domain.String(bundle.TypeKeyNote.URL()),
		}
		s.AddObjects(t, []TestObject{obj})

		_, err := s.GetObjectType(id)
		require.Error(t, err)
	})

	t.Run("with object is type", func(t *testing.T) {
		// Given
		s := NewStoreFixture(t)

		id := "id1"
		relationID := "derivedFrom(assignee)"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, "note")
		require.NoError(t, err)
		obj := TestObject{
			bundle.RelationKeyId:                   domain.String(id),
			bundle.RelationKeyType:                 domain.String(bundle.TypeKeyObjectType.URL()),
			bundle.RelationKeyName:                 domain.String("my note"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{relationID}),
			bundle.RelationKeyRecommendedLayout:    domain.Int64(int64(model.ObjectType_note)),
			bundle.RelationKeyIconEmoji:            domain.String("📝"),
			bundle.RelationKeyIsArchived:           domain.Bool(true),
			bundle.RelationKeyUniqueKey:            domain.String(uniqueKey.Marshal()),
		}
		relObj := TestObject{
			bundle.RelationKeyId:          domain.String(relationID),
			bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeyAssignee.String()),
			bundle.RelationKeyType:        domain.String(bundle.TypeKeyRelation.URL()),
		}
		s.AddObjects(t, []TestObject{obj, relObj})

		// When
		got, err := s.GetObjectType(id)
		require.NoError(t, err)

		// Then
		want := &model.ObjectType{
			Url:        id,
			Name:       "my note",
			Layout:     model.ObjectType_note,
			IconEmoji:  "📝",
			IsArchived: true,
			Types:      []model.SmartBlockType{model.SmartBlockType_Page},
			Key:        "note",
			RelationLinks: []*model.RelationLink{
				{
					Key:    bundle.RelationKeyAssignee.String(),
					Format: model.RelationFormat_longtext,
				},
			},
		}

		assert.Equal(t, want, got)
	})
}
