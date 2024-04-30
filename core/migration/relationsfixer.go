package migration

import (
	"fmt"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type readonlyRelationsFixer struct{}

func (readonlyRelationsFixer) Name() string {
	return "ReadonlyRelationsFixer"
}

func (readonlyRelationsFixer) Run(store objectstore.ObjectStore, space clientspace.Space) (toMigrate, migrated int, err error) {
	var relations []database.Record
	relations, _, err = listReadonlyTagAndStatusRelations(store, space)
	toMigrate = len(relations)

	if err != nil {
		return toMigrate, 0, fmt.Errorf("failed to list all relations with tag and status format in space %s: %v", space.Id(), err)
	}

	if toMigrate != 0 {
		log.Infof("space %s contains %d relations of tag and status format with relationReadonlyValue=true", space.Id(), toMigrate)
	}

	for _, r := range relations {
		var (
			name = pbtypes.GetString(r.Details, bundle.RelationKeyName.String())
			uk   = pbtypes.GetString(r.Details, bundle.RelationKeyUniqueKey.String())
		)

		format, ok := model.RelationFormat_name[int32(pbtypes.GetInt64(r.Details, bundle.RelationKeyRelationFormat.String()))]
		if !ok {
			format = "<unknown>"
		}

		log.Infof("setting relationReadonlyValue to FALSE for relation %s (uniqueKey='%s', format='%s')", name, uk, format)

		det := []*model.Detail{{
			Key:   bundle.RelationKeyRelationReadonlyValue.String(),
			Value: pbtypes.Bool(false),
		}}
		e := space.Do(pbtypes.GetString(r.Details, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, det, false)
			}
			return nil
		})
		if e != nil {
			err = multierror.Append(err, fmt.Errorf("failed to set readOnlyValue=true to relation %s in space %s: %v", uk, space.Id(), e))
		} else {
			migrated++
		}
	}
	return
}

func listReadonlyTagAndStatusRelations(store objectstore.ObjectStore, space clientspace.Space) ([]database.Record, int, error) {
	return store.Query(database.Query{Filters: []*model.BlockContentDataviewFilter{
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
}
