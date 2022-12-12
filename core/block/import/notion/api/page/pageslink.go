package page

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// TODO tests
func SetPageLinksInDatabase(databaseSnaphots *converter.Response,
	pages []Page,
	databases []database.Database,
	notionPageIdsToAnytype, notionDatabaseIdsToAnytype map[string]string) {
	snapshots := makeSnapshotMapFromArray(databaseSnaphots.Snapshots)

	for _, p := range pages {
		if p.Parent.DatabaseID != "" {
			if parentID, ok := notionDatabaseIdsToAnytype[p.Parent.DatabaseID]; ok {
				addLinkBlockToDatabase(snapshots[parentID], notionPageIdsToAnytype[p.ID])
			}
		}
	}
	for _, d := range databases {
		if d.Parent.DatabaseID != "" {
			if parentID, ok := notionDatabaseIdsToAnytype[d.Parent.DatabaseID]; ok {
				addLinkBlockToDatabase(snapshots[parentID], notionDatabaseIdsToAnytype[d.ID])
			}
		}
	}
}

func addLinkBlockToDatabase(snapshots *converter.Snapshot, targetID string) {
	id := bson.NewObjectId().Hex()
	link := &model.Block{
		Id:          id,
		ChildrenIds: []string{},
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetID,
			},
		}}
	snapshots.Snapshot.Blocks = append(snapshots.Snapshot.Blocks, link)
}

func makeSnapshotMapFromArray(snapshots []*converter.Snapshot) map[string]*converter.Snapshot {
	snapshotsMap := make(map[string]*converter.Snapshot, len(snapshots))
	for _, s := range snapshots {
		snapshotsMap[s.Id] = s
	}
	return snapshotsMap
}
