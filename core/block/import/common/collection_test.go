package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestMakeImportCollection(t *testing.T) {
	tests := []struct {
		name              string
		needToAddDate     bool
		shouldBeFavorite  bool
		shouldAddRelation bool
	}{
		{"all false", false, false, false},
		{"add date", true, false, false},
		{"add favorite", false, true, false},
		{"add relations", false, false, true},
		{"all True", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importer := NewImportCollection(collection.New())

			req := NewImportCollectionSetting(
				WithCollectionName("My Collection"),
				WithTargetObjects([]string{"obj1", "obj2"}),
				WithIcon("icon.png"),
			)

			req.needToAddDate = tt.needToAddDate
			req.shouldBeFavorite = tt.shouldBeFavorite
			req.shouldAddRelations = tt.shouldAddRelation

			root, err := importer.MakeImportCollection(req)

			assert.NoError(t, err)
			assert.NotNil(t, root)

			if tt.needToAddDate {
				assert.Contains(t, root.FileName, time.Now().Format("2006"))
			} else {
				assert.Equal(t, "My Collection", root.FileName)
			}

			if tt.shouldBeFavorite {
				assert.Equal(t, domain.Bool(true), root.Snapshot.Data.Details.Get(bundle.RelationKeyIsFavorite))
			} else {
				assert.Equal(t, domain.Bool(false), root.Snapshot.Data.Details.Get(bundle.RelationKeyIsFavorite))
			}

		})
	}
}
