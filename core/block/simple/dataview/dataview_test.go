package dataview

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDataview_FillSmartIds(t *testing.T) {
	obj1 := "obj1"
	obj2 := "obj2"
	obj3 := "obj3"

	t.Run("object ids should be added from filter", func(t *testing.T) {
		// given
		var ids []string
		d := Dataview{content: &model.BlockContentDataview{
			Views: []*model.BlockContentDataviewView{{
				Filters: []*model.BlockContentDataviewFilter{{
					Format: model.RelationFormat_object,
					Value:  pbtypes.StringList([]string{obj1, obj2}),
				}, {
					Format: model.RelationFormat_tag,
					Value:  pbtypes.String(obj3),
				}, {
					Format: model.RelationFormat_number,
					Value:  pbtypes.Int64(555),
				}, {
					Format: model.RelationFormat_longtext,
					Value:  pbtypes.String("hello"),
				}},
			}},
		}}

		// when
		ids = d.FillSmartIds(ids)

		// then
		assert.Contains(t, ids, obj1)
		assert.Contains(t, ids, obj2)
		assert.Contains(t, ids, obj3)
		assert.Len(t, ids, 3)
	})
}

func TestDataview_MigrateFile(t *testing.T) {
	fileId := "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"
	fileIdMigrated := "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku-migrated"
	migrator := func(id string) string {
		if domain.IsFileId(id) {
			return fileId + "-migrated"
		}
		return id
	}
	dv := Dataview{content: &model.BlockContentDataview{
		Views: []*model.BlockContentDataviewView{{
			Filters: []*model.BlockContentDataviewFilter{
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.StringList([]string{"object1", "object2"}),
				},
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.StringList([]string{fileId, "object2"}),
				},
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.String("object3"),
				},
				{
					Format: model.RelationFormat_longtext,
					Value:  pbtypes.String("hello"),
				},
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.String(fileId),
				},
				{
					Format: model.RelationFormat_file,
					Value:  pbtypes.String(fileId),
				},
				{
					Format: model.RelationFormat_file,
					Value:  pbtypes.String("object3"),
				},
			},
		}},
	}}

	dv.MigrateFile(migrator)

	want := Dataview{content: &model.BlockContentDataview{
		Views: []*model.BlockContentDataviewView{{
			Filters: []*model.BlockContentDataviewFilter{
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.StringList([]string{"object1", "object2"}),
				},
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.StringList([]string{fileIdMigrated, "object2"}),
				},
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.String("object3"),
				},
				{
					Format: model.RelationFormat_longtext,
					Value:  pbtypes.String("hello"),
				},
				{
					Format: model.RelationFormat_object,
					Value:  pbtypes.String(fileIdMigrated),
				},
				{
					Format: model.RelationFormat_file,
					Value:  pbtypes.String(fileIdMigrated),
				},
				{
					Format: model.RelationFormat_file,
					Value:  pbtypes.String("object3"),
				},
			},
		}},
	}}

	assert.Equal(t, want, dv)
}
