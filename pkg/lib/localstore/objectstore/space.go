package objectstore

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type SpaceNameGetter interface {
	GetSpaceName(spaceId string) string
}

func (d *dsObjectStore) GetSpaceName(spaceId string) string {
	records, err := d.Query(spaceId, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to get details: %s", err)
	}
	var spaceName string
	if len(records) > 0 {
		spaceName = pbtypes.GetString(records[0].Details, bundle.RelationKeyName.String())
	}
	return spaceName
}
