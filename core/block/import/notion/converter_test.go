package notion

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api/database"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/page"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api/property"
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
