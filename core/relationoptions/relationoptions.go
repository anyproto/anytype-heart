package relationoptions

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName    = "relationOptionsDeleter"
	interval = time.Second * 5
)

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
	optionsID   chan string
	close       chan struct{}
}

func NewRelationOptionsDeleter() Deleter {
	return &relationOptionsDeleter{
		optionsID: make(chan string),
		close:     make(chan struct{}),
	}
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
		for {
			select {
			case <-time.After(interval):
				relationOptions, _, err := r.objectStore.QueryObjectIDs(database.Query{
					Filters: []*model.BlockContentDataviewFilter{
						{
							RelationKey: bundle.RelationKeyIsUninstalled.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.Bool(true),
						},
						{
							RelationKey: bundle.RelationKeyLayout.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
						},
					},
				})
				if err != nil {
					log.Errorf("failed to get options: %s", err)
					return
				}
				for _, option := range relationOptions {
					err = r.deleter.DeleteObject(option)
					if err != nil {
						log.Errorf("failed to delete option: %s", err)
					}
				}
			case <-ctx.Done():
				return
			case <-r.close:
				return
			}
		}
	}()
	return nil
}

func (r *relationOptionsDeleter) Close(ctx context.Context) (err error) {
	close(r.close)
	return nil
}
