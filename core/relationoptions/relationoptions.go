package relationoptions

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "relationOptionsDeleter"

var log = logging.Logger("relationOptionsDeleter")

type Deleter interface {
	app.ComponentRunnable
}
type ObjectDeleter interface {
	DeleteObject(objectId string) (err error)
}

type relationOptionsDeleter struct {
	deleter     ObjectDeleter
	objectStore objectstore.ObjectStore
}

func NewRelationOptionsDeleter() Deleter {
	return &relationOptionsDeleter{}
}

func (r *relationOptionsDeleter) Init(a *app.App) (err error) {
	r.deleter = app.MustComponent[*block.Service](a)
	r.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (r *relationOptionsDeleter) Name() (name string) {
	return CName
}

func (r *relationOptionsDeleter) Run(ctx context.Context) (err error) {
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			r.deleteRelationOptions()
		}
	}()
	return nil
}

func (r *relationOptionsDeleter) deleteRelationOptions() {
	deletedRelations, _, err := r.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyIsUninstalled.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(true),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyRelationFormat.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.RelationFormat_tag), int(model.RelationFormat_status)),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to get uninstalled relations: %s", err)
	}
	for _, relation := range deletedRelations {
		relationKey := pbtypes.GetString(relation.Details, bundle.RelationKeyRelationKey.String())
		relationOptions, _, err := r.objectStore.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIsUninstalled.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Bool(false),
				},
				{
					RelationKey: bundle.RelationKeyRelationKey.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(relationKey),
				},
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
				},
			},
		})
		if err != nil {
			log.Errorf("failed to get relation options: %s", err)
			continue
		}
		for _, option := range relationOptions {
			err = r.deleter.DeleteObject(option)
			if err != nil {
				log.Errorf("failed to delete option: %s", err)
			}
		}
	}
}

func (r *relationOptionsDeleter) Close(ctx context.Context) (err error) {
	return nil
}
