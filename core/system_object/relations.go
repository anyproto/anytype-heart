package system_object

import (
	"fmt"

	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/core/system_object/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) GetRelationByID(id string) (*model.Relation, error) {
	det, err := s.objectStore.GetDetails(id)
	if err != nil {
		return nil, err
	}

	if pbtypes.GetString(det.GetDetails(), bundle.RelationKeyRelationKey.String()) == "" {
		return nil, fmt.Errorf("object %s is not a relation", id)
	}

	rel := relationutils.RelationFromStruct(det.GetDetails())
	return rel.Relation, nil
}

func (s *service) GetRelationByKey(key string) (*model.Relation, error) {
	// todo: should pass workspace
	q := database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.String(key),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyLayout.String(),
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
	}

	records, _, err := s.objectStore.Query(q)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ds.ErrNotFound
	}

	rel := relationutils.RelationFromStruct(records[0].Details)

	return rel.Relation, nil
}
