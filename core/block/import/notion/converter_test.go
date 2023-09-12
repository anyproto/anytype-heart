package notion

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	converter2 "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestNotion_getUniqueProperties(t *testing.T) {
	t.Run("Page and Database have the same property - 1 unique item", func(t *testing.T) {
		// given
		converter := &Notion{}

		databases := []database.Database{
			{
				Properties: map[string]property.DatabasePropertyHandler{
					"Name": &property.DatabaseTitle{},
				},
			},
		}
		pages := []page.Page{
			{
				Properties: map[string]property.Object{
					"Name": &property.TitleItem{},
				},
			},
		}

		// when
		properties := converter.getUniqueProperties(databases, pages)

		// then
		assert.Len(t, properties, 1)
	})
	t.Run("Page and Database have the different properties - 2 unique item", func(t *testing.T) {
		// given
		converter := &Notion{}
		db := []database.Database{
			{
				Properties: map[string]property.DatabasePropertyHandler{
					"Name": &property.DatabaseTitle{},
				},
			},
		}
		pages := []page.Page{
			{
				Properties: map[string]property.Object{
					"Name1": &property.TitleItem{},
				},
			},
		}

		// when
		properties := converter.getUniqueProperties(db, pages)

		// then
		assert.Len(t, properties, 2)
	})
	t.Run("Page and Database have the 2 different properties and 1 same property - 3 unique item", func(t *testing.T) {
		// given
		converter := &Notion{}
		databases := []database.Database{
			{
				Properties: map[string]property.DatabasePropertyHandler{
					"Name":   &property.DatabaseTitle{},
					"Name 1": &property.DatabaseTitle{},
				},
			},
		}
		pages := []page.Page{
			{
				Properties: map[string]property.Object{
					"Name":   &property.TitleItem{},
					"Name 2": &property.TitleItem{},
				},
			},
		}

		// when
		properties := converter.getUniqueProperties(databases, pages)

		// then
		assert.Len(t, properties, 3)
	})
}

func TestNotion_addImportTimestamp(t *testing.T) {
	t.Run("No root objects - all snapshots doesn't have relationImportDate", func(t *testing.T) {
		// given
		converter := &Notion{}
		databases := []*converter2.Snapshot{
			{
				Id: "id1",
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{}},
					},
				},
			},
		}

		pages := []*converter2.Snapshot{
			{
				Id: "id2",
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{}},
					},
				},
			},
		}

		// when
		converter.injectImportTimestamp(databases, pages, nil, 1)

		// then
		for _, snapshot := range databases {
			assert.NotContains(t, snapshot.Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())
		}
		for _, snapshot := range pages {
			assert.NotContains(t, snapshot.Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())
		}
	})
	t.Run("Page is root a objects - page has relationImportDate", func(t *testing.T) {
		// given
		converter := &Notion{}
		databases := []*converter2.Snapshot{
			{
				Id: "id1",
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{}},
					},
				},
			},
		}

		pages := []*converter2.Snapshot{
			{
				Id: "id2",
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{}},
					},
				},
			},
		}

		rootObjects := []string{"id2"}

		// when
		converter.injectImportTimestamp(databases, pages, rootObjects, 1)

		// then
		assert.NotContains(t, databases[0].Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())

		assert.Contains(t, pages[0].Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())
		assert.Equal(t, int64(1), pbtypes.GetInt64(pages[0].Snapshot.Data.Details, bundle.RelationKeyImportDate.String()))
	})
	t.Run("Database is a root objects - database has relationImportDate", func(t *testing.T) {
		// given
		converter := &Notion{}
		databases := []*converter2.Snapshot{
			{
				Id: "id1",
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{}},
					},
				},
			},
		}

		pages := []*converter2.Snapshot{
			{
				Id: "id2",
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{}},
					},
				},
			},
		}

		rootObjects := []string{"id1"}

		// when
		converter.injectImportTimestamp(databases, pages, rootObjects, 1)

		// then
		assert.NotContains(t, pages[0].Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())

		assert.Contains(t, databases[0].Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())
		assert.Equal(t, int64(1), pbtypes.GetInt64(databases[0].Snapshot.Data.Details, bundle.RelationKeyImportDate.String()))
	})
}
