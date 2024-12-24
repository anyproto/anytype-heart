package objectstore

import (
	"context"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const bindKey = "b"

type SpaceNameGetter interface {
	GetSpaceName(spaceId string) string
}

type SpaceIdBinder interface {
	BindSpaceId(spaceId, objectId string) error
	GetSpaceId(objectId string) (spaceId string, err error)
}

func (d *dsObjectStore) GetSpaceName(spaceId string) string {
	records, err := d.SpaceIndex(d.techSpaceId).Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
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

func (d *dsObjectStore) BindSpaceId(spaceId, objectId string) error {
	return d.modifyBind(d.componentCtx, objectId, spaceId)
}

func (d *dsObjectStore) GetSpaceId(objectId string) (spaceId string, err error) {
	doc, err := d.bindId.FindId(d.componentCtx, objectId)
	if err != nil {
		return "", err
	}
	return doc.Value().GetString(bindKey), nil
}

func (d *dsObjectStore) modifyBind(ctx context.Context, objectId, spaceId string) error {
	tx, err := d.bindId.WriteTx(ctx)
	if err != nil {
		return err
	}
	arena := d.arenaPool.Get()
	defer d.arenaPool.Put(arena)
	mod := query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		v.Set(bindKey, arena.NewString(spaceId))
		return v, true, nil
	})
	_, err = d.bindId.UpsertId(tx.Context(), objectId, mod)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
