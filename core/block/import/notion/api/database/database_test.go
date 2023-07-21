package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/block"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestService_AddObjectsToNotionCollection(t *testing.T) {
	t.Run("pages were in Notion workspace", func(t *testing.T) {
		// given
		service := New(nil)
		notionImportContext := &block.NotionImportContext{
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
		notionImportContext := &block.NotionImportContext{
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
		notionImportContext := &block.NotionImportContext{
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
		notionImportContext := &block.NotionImportContext{
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
		notionImportContext := &block.NotionImportContext{
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
		notionImportContext := &block.NotionImportContext{
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
		notionImportContext := &block.NotionImportContext{
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
}
