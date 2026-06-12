package editor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

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
var archiveRequiredRelations = []domain.RelationKey{}

type Archive struct {
	smartblock.SmartBlock
	blockcollection.Collection
	objectStore spaceindex.Store
	reconcile   reconcileRunner
}

func NewArchive(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
) *Archive {
	return &Archive{
		SmartBlock:  sb,
		Collection:  blockcollection.NewCollection(sb, objectStore),
		objectStore: objectStore,
	}
}

func (p *Archive) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, archiveRequiredRelations...)
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.AddHook(p.updateObjects, smartblock.HookAfterApply)

	return p.updateObjects(smartblock.ApplyInfo{})
}

func (p *Archive) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 2,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithEmpty,
				template.WithNoDuplicateLinks(),
				template.WithNoObjectTypes(),
				template.WithDetailName("Archive"),
				template.WithDetailIconEmoji("🗑"),
				template.WithForcedDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
			)
		},
	}
}

func (p *Archive) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{{
		Version: 2,
		Proc:    template.WithForcedDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
	}})
}

func (p *Archive) updateObjects(_ smartblock.ApplyInfo) (err error) {
	// snapshot under the smartblock lock: seq order matches apply order and the marker
	// describes exactly the tree state the ids were read from
	archivedIds, err := p.GetIds()
	if err != nil {
		return
	}
	marker := headsMarker(p)
	seq := p.reconcile.nextSeq()
	go p.reconcile.run(seq, func() {
		p.reconcileInStore(archivedIds, marker)
	})
	return nil
}

// headsMarker fingerprints the current tree heads of a link-collection editor; empty when the
// object has no tree (virtual objects, tests), in which case no marker is persisted.
func headsMarker(sb smartblock.SmartBlock) string {
	tree := sb.Tree()
	if tree == nil {
		return ""
	}
	return spaceindex.HashIds(tree.Heads())
}

// reconcileInStore aligns isArchived details with the archive tree and, once every write
// succeeded, persists the heads fingerprint of the reconciled tree state so the indexer can
// prove on the next space load that no reconcile is needed without building this tree
// (reconcileLinkDerivedDetails).
func (p *Archive) reconcileInStore(archivedIds []string, marker string) {
	if err := p.updateInStore(archivedIds); err != nil {
		// no marker write: the next space load triggers reconciliation again
		log.Errorf("archive: can't update in store: %v", err)
		return
	}
	if marker == "" {
		return
	}
	if err := p.objectStore.SaveReconcileMarker(context.Background(), p.Id(), marker); err != nil {
		log.Errorf("archive: can't save reconcile marker: %v", err)
	}
}

func (p *Archive) updateInStore(archivedIds []string) error {
	records, err := p.objectStore.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyIsArchived,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Bool(true),
		},
	}}, 0, 0)
	if err != nil {
		return fmt.Errorf("query archived objects: %w", err)
	}

	var storeArchivedIds = make([]string, 0, len(records))
	for _, rec := range records {
		storeArchivedIds = append(storeArchivedIds, rec.Details.GetString(bundle.RelationKeyId))
	}

	removedIds, addedIds := slice.DifferenceRemovedAdded(storeArchivedIds, archivedIds)
	var (
		wg     sync.WaitGroup
		failed atomic.Int32
	)
	setIsArchived := func(id string, isArchived bool) {
		defer wg.Done()
		err := p.ModifyLocalDetails(id, func(current *domain.Details) (*domain.Details, error) {
			if current == nil {
				current = domain.NewDetails()
			}
			current.SetBool(bundle.RelationKeyIsArchived, isArchived)
			return current, nil
		})
		// a missing or deleted target has no details to carry the flag; not a failure
		if err != nil && !isMissingObjectError(err) {
			log.Errorf("archive: can't set detail to object: %v", err)
			failed.Add(1)
		}
	}
	for _, removedId := range removedIds {
		wg.Add(1)
		go setIsArchived(removedId, false)
	}
	for _, addedId := range addedIds {
		wg.Add(1)
		go setIsArchived(addedId, true)
	}
	wg.Wait()
	if n := failed.Load(); n > 0 {
		return fmt.Errorf("set isArchived detail: %d objects failed", n)
	}
	return nil
}
