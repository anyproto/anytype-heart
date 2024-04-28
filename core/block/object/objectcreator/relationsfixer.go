package objectcreator

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const logHeader = "relationsFixer"

func (s *service) fixReadonlyInRelations(space clientspace.Space) {
	rels, err := s.listTagAndStatusRelations(space)
	if err != nil {
		log.With(logHeader).Errorf("failed to list all relations with tag and status format in space %s: %v", space.Id(), err)
		return
	}

	if len(rels) != 0 {
		log.With(logHeader).Infof("space %s contains %d relations of tag and status format with relationReadonlyValue=true", space.Id(), len(rels))
	}

	for _, r := range rels {
		var (
			name = pbtypes.GetString(r.Details, bundle.RelationKeyName.String())
			uk   = pbtypes.GetString(r.Details, bundle.RelationKeyUniqueKey.String())
		)

		format, ok := model.RelationFormat_name[int32(pbtypes.GetInt64(r.Details, bundle.RelationKeyRelationFormat.String()))]
		if !ok {
			format = "<unknown>"
		}

		log.With(logHeader).Infof("setting relationReadonlyValue to FALSE for relation %s (uniqueKey='%s', format='%s')", name, uk, format)

		det := []*pb.RpcObjectSetDetailsDetail{{
			Key:   bundle.RelationKeyRelationReadonlyValue.String(),
			Value: pbtypes.Bool(false),
		}}
		if err = space.Do(pbtypes.GetString(r.Details, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, det, false)
			}
			return nil
		}); err != nil {
			log.With(logHeader).Errorf("failed to set readOnlyValue=true to relation %s in space %s: %v", uk, space.Id(), err)
		}
	}
}

func (s *service) listTagAndStatusRelations(space clientspace.Space) (records []database.Record, err error) {
	records, _, err = s.objectStore.Query(database.Query{Filters: []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyRelationFormat.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.IntList(int(model.RelationFormat_status), int(model.RelationFormat_tag)),
		},
		{
			RelationKey: bundle.RelationKeySpaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(space.Id()),
		},
		{
			RelationKey: bundle.RelationKeyRelationReadonlyValue.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Bool(true),
		},
	}})
	return
}
