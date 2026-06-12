package editor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

// required relations for archive beside the bundle.RequiredInternalRelations
var dashboardRequiredRelations = []domain.RelationKey{}

type Dashboard struct {
	smartblock.SmartBlock
	basic.AllOperations
	blockcollection.Collection

	objectStore spaceindex.Store
	reconcile   reconcileRunner
}

func (f *ObjectFactory) newDashboard(sb smartblock.SmartBlock, objectStore spaceindex.Store) *Dashboard {
	return &Dashboard{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, f.layoutConverter, nil),
		Collection:    blockcollection.NewCollection(sb, objectStore),
		objectStore:   objectStore,
	}
}

func (p *Dashboard) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, dashboardRequiredRelations...)
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	return p.updateObjects(smartblock.ApplyInfo{})

}

func (p *Dashboard) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 2,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyDashboard}),
				template.WithLayout(model.ObjectType_dashboard),
				template.WithEmpty,
				template.WithDetailName("Home"),
				template.WithDetailIconEmoji("🏠"),
				template.WithNoDuplicateLinks(),
				template.WithForcedDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
			)
		},
	}
}

func (p *Dashboard) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{{
		Version: 2,
		Proc:    template.WithForcedDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
	}})
}

func (p *Dashboard) updateObjects(info smartblock.ApplyInfo) (err error) {
	// snapshot under the smartblock lock: seq order matches apply order and the marker
	// describes exactly the tree state the ids were read from
	favoritedIds, err := p.GetIds()
	if err != nil {
		return
	}
	marker := headsMarker(p)
	seq := p.reconcile.nextSeq()
	go p.reconcile.run(seq, func() {
		p.reconcileInStore(favoritedIds, marker)
	})
	return nil
}

// reconcileInStore aligns isFavorite details with the home tree and, once every write
// succeeded, persists the heads fingerprint of the reconciled tree state so the indexer can
// prove on the next space load that no reconcile is needed without building this tree
// (reconcileLinkDerivedDetails).
func (p *Dashboard) reconcileInStore(favoritedIds []string, marker string) {
	if err := p.updateInStore(favoritedIds); err != nil {
		// no marker write: the next space load triggers reconciliation again
		log.Errorf("favorite: can't update in store: %v", err)
		return
	}
	if marker == "" {
		return
	}
	if err := p.objectStore.SaveReconcileMarker(context.Background(), p.Id(), marker); err != nil {
		log.Errorf("favorite: can't save reconcile marker: %v", err)
	}
}

func (p *Dashboard) updateInStore(favoritedIds []string) error {
	records, err := p.objectStore.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyIsFavorite,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query favorited objects: %w", err)
	}
	var storeFavoritedIds = make([]string, 0, len(records))
	for _, rec := range records {
		storeFavoritedIds = append(storeFavoritedIds, rec.Details.GetString(bundle.RelationKeyId))
	}

	removedIds, addedIds := slice.DifferenceRemovedAdded(storeFavoritedIds, favoritedIds)
	var (
		wg     sync.WaitGroup
		failed atomic.Int32
	)
	setIsFavorite := func(id string, isFavorite bool) {
		defer wg.Done()
		err := p.ModifyLocalDetails(id, func(current *domain.Details) (*domain.Details, error) {
			if current == nil {
				current = domain.NewDetails()
			}
			current.SetBool(bundle.RelationKeyIsFavorite, isFavorite)
			return current, nil
		})
		// a missing or deleted target has no details to carry the flag; not a failure
		if err != nil && !isMissingObjectError(err) {
			log.Errorf("favorite: can't set detail to object: %v", err)
			failed.Add(1)
		}
	}
	for _, removedId := range removedIds {
		wg.Add(1)
		go setIsFavorite(removedId, false)
	}
	for _, addedId := range addedIds {
		wg.Add(1)
		go setIsFavorite(addedId, true)
	}
	wg.Wait()
	if n := failed.Load(); n > 0 {
		return fmt.Errorf("set isFavorite detail: %d objects failed", n)
	}
	return nil
}
