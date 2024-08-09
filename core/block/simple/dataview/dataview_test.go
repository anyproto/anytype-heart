package dataview

import (
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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

func TestDataview_HasEmptyContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dv     Dataview
		assert func(t assert.TestingT, value bool, msgAndArgs ...interface{}) bool
	}{
		{
			name: "new dataview block is empty",
			dv: Dataview{content: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:   bson.NewObjectId().Hex(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All",
						Sorts: []*model.BlockContentDataviewSort{{
							Id:          bson.NewObjectId().Hex(),
							RelationKey: bundle.RelationKeyLastModifiedDate.String(),
							Type:        model.BlockContentDataviewSort_Desc,
						}},
					},
				}}},
			assert: assert.True,
		},
		{
			name:   "nil dataview block is empty",
			dv:     Dataview{},
			assert: assert.True,
		},
		{
			name: "dataview block with multiple views is not empty",
			dv: Dataview{content: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:   bson.NewObjectId().Hex(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All",
						Sorts: []*model.BlockContentDataviewSort{{
							Id:          bson.NewObjectId().Hex(),
							RelationKey: bundle.RelationKeyLastModifiedDate.String(),
							Type:        model.BlockContentDataviewSort_Desc,
						}},
					},
					{
						Id:   bson.NewObjectId().Hex(),
						Type: model.BlockContentDataviewView_Kanban,
						Name: "Kanban",
					},
				}}},
			assert: assert.False,
		},
		{
			name: "dataview block with filters is not empty",
			dv: Dataview{content: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:   bson.NewObjectId().Hex(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All",
						Sorts: []*model.BlockContentDataviewSort{{
							Id:          bson.NewObjectId().Hex(),
							RelationKey: bundle.RelationKeyLastModifiedDate.String(),
							Type:        model.BlockContentDataviewSort_Desc,
						}},
						Filters: []*model.BlockContentDataviewFilter{{
							RelationKey: bundle.RelationKeyName.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String("Maria Antonietta"),
						}},
					},
				}}},
			assert: assert.False,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, tc.dv.HasEmptyContent())
		})
	}
}
