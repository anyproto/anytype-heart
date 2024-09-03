package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/files/mock_files"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestService_AddObjectsToNotionCollection(t *testing.T) {
	t.Run("pages were in Notion workspace", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype: map[string]string{"id1": "anytypeID1", "id2": "anytypeID2"},
		}
		notionPages := []page.Page{
			{
				ID: "id1",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, nil, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID1"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID2"))
	})
	t.Run("2 pages were in Notion workspace, 1 page is child in Page - 2 pages in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype: map[string]string{"id1": "anytypeID1", "id2": "anytypeID2", "id3": "anytypeID3"},
		}
		notionPages := []page.Page{
			{
				ID: "id1",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:   "page",
					PageID: "id3",
				},
			},
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, nil, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID1"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
	})

	t.Run("1 pages were in Notion workspace, 1 page is child in Page, but parent page wasn't imported - both pages in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype: map[string]string{"id1": "anytypeID1", "id2": "anytypeID2"},
		}
		notionPages := []page.Page{
			{
				ID: "id1",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:   "page",
					PageID: "id3",
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, nil, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID1"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID2"))
	})

	t.Run("1 page and 1 database were in Notion workspace, 1 page is in database - 1 page and 1 db in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype:     map[string]string{"id1": "anytypeID1", "id2": "anytypeID2"},
			NotionDatabaseIdsToAnytype: map[string]string{"id3": "anytypeID3"},
		}
		notionPages := []page.Page{
			{
				ID: "id1",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:       "database",
					DatabaseID: "id3",
				},
			},
		}

		notionDB := []Database{
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, notionDB, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID1"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
	})

	t.Run("2 database were in Notion workspace, 1 page is in database, but parent db isn't imported - 1 db is in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype:     map[string]string{"id1": "anytypeID1"},
			NotionDatabaseIdsToAnytype: map[string]string{"id3": "anytypeID3"},
		}
		notionPages := []page.Page{
			{
				ID: "id1",
				Parent: api.Parent{
					Type:       "database",
					DatabaseID: "id2",
				},
			},
		}
		notionDB := []Database{
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, notionDB, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID1"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
	})

	t.Run("1 database and 1 page were in Notion workspace, 1 db is a child in page - 1 db and 1 page are in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype:     map[string]string{"id1": "anytypeID1"},
			NotionDatabaseIdsToAnytype: map[string]string{"id3": "anytypeID3", "id2": "anytypeID2"},
		}
		notionPages := []page.Page{
			{
				ID: "id1",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
		}
		notionDB := []Database{
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:   "page",
					PageID: "id1",
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, notionDB, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID1"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
	})

	t.Run("1 database was in Notion workspace, 1 db is a child in page, but page weren't imported - 1 db and 1 page are in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &api.NotionImportContext{
			NotionDatabaseIdsToAnytype: map[string]string{"id3": "anytypeID3", "id2": "anytypeID2"},
		}
		notionDB := []Database{
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:   "page",
					PageID: "id1",
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, notionDB, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID2"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
	})

	t.Run("1 page was in Notion workspace, 1 page is a child in block, 1 page is in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		pt := api.NewPageTree()
		pt.ParentPageToChildIDs = map[string][]string{"blockID": {"id2"}}
		bp := api.NewBlockToPage()
		bp.ParentBlockToPage = map[string]string{"blockID": "id3"}
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype: map[string]string{"id3": "anytypeID3", "id2": "anytypeID2"},
			PageTree:               pt,
			BlockToPage:            bp,
		}
		notionPages := []page.Page{
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:    "page",
					BlockID: "blockID",
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, nil, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 1)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
	})

	t.Run("1 page was in Notion workspace, 1 page is a child in block, but parent page is absent - 2 pages are in root collection", func(t *testing.T) {
		// given
		service := New(nil)
		pt := api.NewPageTree()
		pt.ParentPageToChildIDs = map[string][]string{"blockID": {"id2"}}
		notionImportContext := &api.NotionImportContext{
			NotionPageIdsToAnytype: map[string]string{"id3": "anytypeID3", "id2": "anytypeID2"},
			PageTree:               pt,
			BlockToPage:            api.NewBlockToPage(),
		}
		notionPages := []page.Page{
			{
				ID: "id3",
				Parent: api.Parent{
					Type:      "workspace",
					Workspace: true,
				},
			},
			{
				ID: "id2",
				Parent: api.Parent{
					Type:    "page",
					BlockID: "blockID",
				},
			},
		}

		// when
		collection, err := service.AddObjectsToNotionCollection(notionImportContext, nil, notionPages)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.NotNil(t, collection.Snapshot.Data.Collections)
		assert.NotNil(t, collection.Snapshot.Data.Collections.GetFields()["objects"])
		assert.Len(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), 2)
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID3"))
		assert.Contains(t, collection.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues(), pbtypes.String("anytypeID2"))
	})
}

func Test_makeDatabaseSnapshot(t *testing.T) {
	t.Run("Databases have properties with same name and relation - don't create additional relation", func(t *testing.T) {
		// given
		p := property.DatabaseSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Name",
			},
		}
		pr := property.DatabaseProperties{"Name": &p}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		req.AddSnapshotByNameAndFormat(p.Name, int64(p.GetFormat()), &model.SmartBlockSnapshotBase{})
		db := Database{Properties: pr}

		// when
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))
		assert.Nil(t, err)

		// then
		assert.Len(t, snapshot, 1)
		assert.NotEqual(t, sb.SmartBlockTypeRelation, snapshot[0].SbType)
	})

	t.Run("Database has Select property with Tag name", func(t *testing.T) {
		// given
		p := property.DatabaseSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Tag",
			},
		}
		pr := property.DatabaseProperties{"Tag": &p}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		db := Database{Properties: pr}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[p.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Database has Select property with Tags name", func(t *testing.T) {
		// given
		selectProperty := property.DatabaseSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Tags",
			},
		}
		properties := property.DatabaseProperties{"Tags": &selectProperty}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		db := Database{Properties: properties}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tags name", func(t *testing.T) {
		multiSelectProperty := property.DatabaseMultiSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Tags",
			},
		}
		selectProperty := property.DatabaseProperties{"Tags": &multiSelectProperty}
		dbService := New(nil)
		properties := property.NewPropertiesStore()
		db := Database{Properties: selectProperty}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), properties, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, properties.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(properties.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tag name", func(t *testing.T) {
		multiSelectProperty := property.DatabaseMultiSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Tag",
			},
		}
		selectProperty := property.DatabaseProperties{"Tag": &multiSelectProperty}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		db := Database{Properties: selectProperty}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tag name and Select property with Tags name - MultiSelect is mapped to Tag relation", func(t *testing.T) {
		multiSelectProperty := property.DatabaseMultiSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Tag",
			},
		}
		selectProperty := property.DatabaseSelect{
			Property: property.Property{
				ID:   "id1",
				Name: "Tags",
			},
		}
		properties := property.DatabaseProperties{"Tag": &multiSelectProperty, "Tags": &selectProperty}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		db := Database{Properties: properties}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, req.PropertyIdsToSnapshots, 2)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
		assert.NotEqual(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Page has MultiSelect property with Tag name and Select property with tags name - MultiSelect is mapped to Tag relation", func(t *testing.T) {
		multiSelectProperty := property.DatabaseMultiSelect{
			Property: property.Property{
				ID:   "id",
				Name: "Tag",
			},
		}
		selectProperty := property.DatabaseSelect{
			Property: property.Property{
				ID:   "id1",
				Name: "tags",
			},
		}
		properties := property.DatabaseProperties{"Tag": &multiSelectProperty, "tags": &selectProperty}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		db := Database{Properties: properties}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, req.PropertyIdsToSnapshots, 2)
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[multiSelectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
		assert.NotEqual(t, bundle.RelationKeyTag.String(), pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyRelationKey.String()))
	})

	t.Run("Database has icon emoji - details have relation iconEmoji", func(t *testing.T) {
		dbService := New(nil)
		emoji := "😘"
		db := Database{Icon: &api.Icon{
			Emoji: &emoji,
		}}

		// when
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), nil, mock_files.NewMockDownloader(t))

		// then
		assert.Nil(t, err)
		assert.Len(t, snapshot, 1)
		icon := pbtypes.GetString(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyIconEmoji.String())
		assert.Equal(t, emoji, icon)
	})
	t.Run("Database has custom external icon - details have relation iconImage", func(t *testing.T) {
		dbService := New(nil)
		db := Database{Icon: &api.Icon{
			Type: api.External,
			External: &api.FileProperty{
				URL: "url",
			},
		}}

		// when
		downloader := mock_files.NewMockDownloader(t)
		downloader.EXPECT().QueueFileForDownload(mock.Anything).Return(nil, true)
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), nil, downloader)

		// then
		assert.Nil(t, err)
		assert.Len(t, snapshot, 1)
		icon := pbtypes.GetString(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyIconImage.String())
		assert.Equal(t, "url", icon)
	})
	t.Run("Database has custom file icon - details have relation iconImage", func(t *testing.T) {
		dbService := New(nil)
		db := Database{Icon: &api.Icon{
			Type: api.File,
			File: &api.FileProperty{
				URL: "url",
			},
		}}

		// when
		downloader := mock_files.NewMockDownloader(t)
		downloader.EXPECT().QueueFileForDownload(mock.Anything).Return(nil, true)
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), nil, downloader)

		// then
		assert.Nil(t, err)
		assert.Len(t, snapshot, 1)
		icon := pbtypes.GetString(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyIconImage.String())
		assert.Equal(t, "url", icon)
	})
	t.Run("Database doesn't have icon - details don't have neither iconImage nor iconEmoji", func(t *testing.T) {
		dbService := New(nil)
		db := Database{}

		// when
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), nil, mock_files.NewMockDownloader(t))

		// then
		assert.Nil(t, err)
		assert.Len(t, snapshot, 1)
		icon := pbtypes.GetString(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyIconImage.String())
		assert.Equal(t, "", icon)
	})
	t.Run("Database has property without name - return relation with name Untitled", func(t *testing.T) {
		selectProperty := property.DatabaseSelect{
			Property: property.Property{
				ID:   "id1",
				Name: "",
			},
		}
		properties := property.DatabaseProperties{"": &selectProperty}
		dbService := New(nil)
		req := property.NewPropertiesStore()
		db := Database{Properties: properties}

		// when
		dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), req, mock_files.NewMockDownloader(t))

		// then
		assert.Len(t, req.PropertyIdsToSnapshots, 1)
		assert.Equal(t, property.UntitledProperty, pbtypes.GetString(req.PropertyIdsToSnapshots[selectProperty.ID].GetDetails(), bundle.RelationKeyName.String()))
	})
	t.Run("Database has cover file icon - details have relations coverId and coverType", func(t *testing.T) {
		dbService := New(nil)
		db := Database{Cover: &api.FileObject{
			Type: api.File,
			File: api.FileProperty{
				URL: "url",
			},
		}}

		// when
		downloader := mock_files.NewMockDownloader(t)
		downloader.EXPECT().QueueFileForDownload(mock.Anything).Return(nil, true)
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), nil, downloader)

		// then
		assert.Nil(t, err)
		assert.Len(t, snapshot, 1)
		cover := pbtypes.GetString(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyCoverId.String())
		coverType := pbtypes.GetInt64(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyCoverType.String())
		assert.Equal(t, "url", cover)
		assert.Equal(t, int64(1), coverType)
	})
	t.Run("Database doesn't have cover - details don't have neither coverType nor coverId", func(t *testing.T) {
		dbService := New(nil)
		db := Database{}

		// when
		snapshot, err := dbService.makeDatabaseSnapshot(db, api.NewNotionImportContext(), nil, mock_files.NewMockDownloader(t))

		// then
		assert.Nil(t, err)
		assert.Len(t, snapshot, 1)
		cover := pbtypes.GetString(snapshot[0].Snapshot.Data.Details, bundle.RelationKeyCoverId.String())
		assert.Equal(t, "", cover)
	})
}
