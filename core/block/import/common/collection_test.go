package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMakeImportCollection(t *testing.T) {
	tests := []struct {
		name              string
		needToAddDate     bool
		shouldBeFavorite  bool
		shouldAddRelation bool
		widgetSnapshot    bool
	}{
		{"all false", false, false, false, false},
		{"add date", true, false, false, false},
		{"add favorite", false, true, false, false},
		{"add relations", false, false, true, false},
		{"with existing widget snapshot", false, false, false, true},
		{"all True", true, true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importer := NewImportCollection(collection.New())

			var widget *Snapshot
			if tt.widgetSnapshot {
				widget = &Snapshot{
					Id:       "widget-id",
					FileName: "existing",
					Snapshot: &SnapshotModel{
						Data: &StateSnapshot{
							Blocks: []*model.Block{
								{
									Id:          "root-block",
									ChildrenIds: []string{},
									Content: &model.BlockContentOfSmartblock{
										Smartblock: &model.BlockContentSmartblock{},
									},
								},
							},
						},
					},
				}
			}

			req := NewImportCollectionSetting(
				WithCollectionName("My Collection"),
				WithTargetObjects([]string{"obj1", "obj2"}),
				WithIcon("icon.png"),
				WithWidgetSnapshot(widget),
			)

			req.needToAddDate = tt.needToAddDate
			req.shouldBeFavorite = tt.shouldBeFavorite
			req.shouldAddRelations = tt.shouldAddRelation

			root, widgetSnap, err := importer.MakeImportCollection(req)

			assert.NoError(t, err)
			assert.NotNil(t, root)
			assert.NotNil(t, widgetSnap)

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

			if tt.widgetSnapshot {
				assert.Equal(t, "existing", widgetSnap.FileName)
			} else {
				assert.Equal(t, "rootWidget", widgetSnap.FileName)
			}
		})
	}
}
