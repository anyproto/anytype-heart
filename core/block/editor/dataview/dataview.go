package dataview

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger("anytype-mw-editor-dataview")

var errSourceNotChanged = fmt.Errorf("source not changed")

type Dataview interface {
	SetSource(ctx session.Context, blockId string, source []string) (err error)

	// GetAggregatedRelations(blockId string) ([]*model.Relation, error)
	GetDataviewRelations(blockId string) ([]*model.Relation, error)
	GetDataview(blockID string) (*model.BlockContentDataview, error)

	DeleteView(ctx session.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx session.Context, blockId string, activeViewId string) error
	CreateView(ctx session.Context, blockID string,
		view model.BlockContentDataviewView, source []string) (*model.BlockContentDataviewView, error)
	SetViewPosition(ctx session.Context, blockId string, viewId string, position uint32) error
	AddRelations(ctx session.Context, blockId string, relationIds []string, showEvent bool) error
	DeleteRelations(ctx session.Context, blockId string, relationIds []string, showEvent bool) error
	UpdateView(ctx session.Context, blockID string, viewID string, view *model.BlockContentDataviewView, showEvent bool) error
	UpdateViewGroupOrder(ctx session.Context, blockId string, order *model.BlockContentDataviewGroupOrder) error
	UpdateViewObjectOrder(ctx session.Context, blockId string, orders []*model.BlockContentDataviewObjectOrder) error
	DataviewMoveObjectsInView(ctx session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest) error

	GetDataviewBlock(s *state.State, blockID string) (dataview.Block, error)
}

func NewDataview(sb smartblock.SmartBlock, objectStore objectstore.ObjectStore) Dataview {
	dv := &sdataview{
		SmartBlock:  sb,
		objectStore: objectStore,
	}
	sb.AddHook(dv.checkDVBlocks, smartblock.HookBeforeApply)
	sb.AddHook(dv.injectActiveViews, smartblock.HookBeforeApply)
	return dv
}

type sdataview struct {
	smartblock.SmartBlock
	objectStore objectstore.ObjectStore
}

func (d *sdataview) GetDataviewBlock(s *state.State, blockID string) (dataview.Block, error) {
	return getDataviewBlock(s, blockID)
}

func (d *sdataview) SetSource(ctx session.Context, blockId string, source []string) (err error) {
	s := d.NewStateCtx(ctx)
	if blockId == "" {
		blockId = template.DataviewBlockId
	}
	err = d.setSourceToBlock(s, blockId, source)
	if err != nil {
		if errors.Is(err, errSourceNotChanged) {
			return nil
		}
		return err
	}
	return d.Apply(s, smartblock.NoRestrictions, smartblock.KeepInternalFlags)
}

// func setSourceToSet(s *state.State, blockId string, source []string) error {
// 	setOf := pbtypes.GetStringList(s.Details(), bundle.RelationKeySetOf.String())
// 	if slice.UnsortedEqual(setOf, source) {
// 		return errSourceNotChanged
// 	}
// 	s.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(source))
//
// 	flags := internalflag.NewFromState(s)
// 	// set with source is no longer empty
// 	flags.Remove(model.InternalFlag_editorDeleteEmpty)
// 	flags.AddToState(s)
//
// 	b := s.Get(blockId)
// 	if b == nil {
// 		return fmt.Errorf("block %s not found", blockId)
// 	}
//
// 	dv := b.Model().GetDataview()
// 	if dv == nil {
// 		return fmt.Errorf("block %s is not dataview", blockId)
// 	}
// 	for _, view := range dv.Views {
// 		view.DefaultTemplateId = ""
// 		view.DefaultObjectTypeId = ""
// 	}
// 	s.Set(b)
// 	return nil
// }

func (d *sdataview) setSourceToBlock(s *state.State, blockId string, source []string) error {
	block, err := getDataviewBlock(s, blockId)
	if err != nil && blockId != template.DataviewBlockId {
		return err
	}

	if block != nil && slice.UnsortedEqual(block.GetSource(), source) {
		return errSourceNotChanged
	}

	s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))

	if len(source) == 0 {
		s.Unlink(blockId)
		return nil
	}

	newBlock, err := d.buildDataviewBlock(blockId, source)
	if err != nil {
		return err
	}
	s.Set(newBlock)

	if block == nil {
		e := s.InsertTo("", 0, blockId)
		if e != nil {
			log.With("objectId", s.RootId()).Errorf("failed to insert dataview block '%s': %v", blockId, e)
		}
	}
	return nil
}

func (d *sdataview) buildDataviewBlock(blockId string, source []string) (dataview.Block, error) {
	content, err := BlockBySource(d.objectStore, source)
	if err != nil {
		return nil, err
	}

	if len(content.Dataview.Views) > 0 {
		content.Dataview.ActiveView = content.Dataview.Views[0].Id
	}
	return simple.New(&model.Block{Content: content, Id: blockId}).(dataview.Block), nil
}

func (d *sdataview) AddRelations(ctx session.Context, blockId string, relationKeys []string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}
	for _, key := range relationKeys {
		relation, err2 := d.objectStore.FetchRelationByKey(d.SpaceID(), key)
		if err2 != nil {
			return err2
		}
		tb.AddRelation(relation.RelationLink())
	}
	if showEvent {
		return d.Apply(s)
	} else {
		return d.Apply(s, smartblock.NoEvent)
	}
}

func (d *sdataview) DeleteRelations(ctx session.Context, blockId string, relationIds []string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	for _, id := range relationIds {
		if err = tb.DeleteRelation(id); err != nil {
			return err
		}
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *sdataview) GetDataviewRelations(blockId string) ([]*model.Relation, error) {
	st := d.NewState()
	tb, err := getDataviewBlock(st, blockId)
	if err != nil {
		return nil, err
	}

	return tb.Model().GetDataview().GetRelations(), nil
}

func (d *sdataview) GetDataview(blockID string) (*model.BlockContentDataview, error) {
	st := d.NewState()
	tb, err := getDataviewBlock(st, blockID)
	if err != nil {
		return nil, err
	}

	return tb.Model().GetDataview(), nil
}

func (d *sdataview) DeleteView(ctx session.Context, blockId string, viewId string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = tb.DeleteView(viewId); err != nil {
		return err
	}
	if len(tb.Model().GetDataview().Views) == 0 {
		return fmt.Errorf("cannot remove the last view")
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *sdataview) UpdateView(ctx session.Context, blockID string, viewID string, view *model.BlockContentDataviewView, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(s, blockID)
	if err != nil {
		return err
	}

	if err = dvBlock.SetViewFields(viewID, view); err != nil {
		return err
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *sdataview) SetActiveView(ctx session.Context, id string, activeViewId string) error {
	s := d.NewStateCtx(ctx)

	dvBlock, err := getDataviewBlock(s, id)
	if err != nil {
		return err
	}

	if _, err = dvBlock.GetView(activeViewId); err != nil {
		return err
	}
	dvBlock.SetActiveView(activeViewId)

	if err = d.objectStore.SetActiveView(d.Id(), id, activeViewId); err != nil {
		return err
	}

	d.SmartBlock.CheckSubscriptions()
	return d.Apply(s, smartblock.NoHooks)
}

func (d *sdataview) SetViewPosition(ctx session.Context, blockId string, viewId string, position uint32) (err error) {
	s := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(s, blockId)
	if err != nil {
		return
	}
	var (
		curPos int
		newPos = int(position)
		found  bool
		views  = dvBlock.Model().GetDataview().Views
	)
	for i, view := range views {
		if view.Id == viewId {
			curPos = i
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("view not found")
	}
	if newPos > len(views)-1 {
		newPos = len(views) - 1
	}
	var newViews = make([]*model.BlockContentDataviewView, 0, len(views))
	for i, view := range views {
		if len(newViews) == newPos {
			newViews = append(newViews, views[curPos])
		}
		if i != curPos {
			newViews = append(newViews, view)
		}
	}
	if len(newViews) == newPos {
		newViews = append(newViews, views[curPos])
	}
	dvBlock.Model().GetDataview().Views = newViews
	return d.Apply(s)
}

func (d *sdataview) CreateView(ctx session.Context, id string,
	view model.BlockContentDataviewView, source []string) (*model.BlockContentDataviewView, error) {
	view.Id = uuid.New().String()
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, id)
	if err != nil {
		return nil, err
	}

	if len(view.Relations) == 0 {
		for _, rl := range tb.Model().GetDataview().GetRelationLinks() {
			var isVisible bool
			if rl.Key == bundle.RelationKeyName.String() {
				isVisible = true
			}
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{Key: rl.Key, IsVisible: isVisible})
		}
	}

	if len(view.Sorts) == 0 {
		// todo: set depends on the view type
		view.Sorts = template.DefaultLastModifiedDateSort()
	}
	tb.AddView(view)
	return &view, d.Apply(s)
}

func (d *sdataview) UpdateViewGroupOrder(ctx session.Context, blockId string, order *model.BlockContentDataviewGroupOrder) error {
	st := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(st, blockId)
	if err != nil {
		return err
	}

	dvBlock.SetViewGroupOrder(order)

	return d.Apply(st)
}

func (d *sdataview) UpdateViewObjectOrder(ctx session.Context, blockId string, orders []*model.BlockContentDataviewObjectOrder) error {
	st := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(st, blockId)
	if err != nil {
		return err
	}

	dvBlock.SetViewObjectOrder(orders)

	return d.Apply(st)
}

func (d *sdataview) DataviewMoveObjectsInView(ctx session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest) error {
	st := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(st, req.BlockId)
	if err != nil {
		return err
	}

	if err = dvBlock.MoveObjectsInView(req); err != nil {
		return err
	}

	return d.Apply(st)
}

func (d *sdataview) listRestrictedSources(ctx context.Context) ([]string, error) {
	keys := []domain.TypeKey{
		bundle.TypeKeyFile,
		bundle.TypeKeyImage,
		bundle.TypeKeyVideo,
		bundle.TypeKeyAudio,
		bundle.TypeKeyObjectType,
		bundle.TypeKeySet,
		bundle.TypeKeyRelation,
	}
	sources := make([]string, 0, len(keys))
	for _, key := range keys {
		id, err := d.Space().GetTypeIdByKey(ctx, key)
		if err != nil {
			return nil, err
		}
		sources = append(sources, id)
	}
	return sources, nil
}

func (d *sdataview) checkDVBlocks(info smartblock.ApplyInfo) (err error) {
	var dvChanged bool
	info.State.IterateActive(func(b simple.Block) (isContinue bool) {
		if dv := b.Model().GetDataview(); dv != nil {
			dvChanged = true
			return false
		}
		return true
	})
	if !dvChanged {
		return
	}

	restrictedSources, err := d.listRestrictedSources(context.Background())
	if err != nil {
		return fmt.Errorf("list restricted sources: %w", err)
	}
	r := d.Restrictions().Copy()
	r.Dataview = r.Dataview[:0]
	info.State.Iterate(func(b simple.Block) (isContinue bool) {
		if dv := b.Model().GetDataview(); dv != nil && len(dv.Source) == 1 {
			if slice.FindPos(restrictedSources, dv.Source[0]) != -1 {
				r.Dataview = append(r.Dataview, model.RestrictionsDataviewRestrictions{
					BlockId: b.Model().Id,
					Restrictions: []model.RestrictionsDataviewRestriction{
						model.Restrictions_DVRelation, model.Restrictions_DVCreateObject,
					},
				})
				return true
			}
			r.Dataview = append(r.Dataview, model.RestrictionsDataviewRestrictions{BlockId: b.Model().Id})
		}
		return true
	})
	return
}

func (d *sdataview) injectActiveViews(info smartblock.ApplyInfo) (err error) {
	s := info.State
	views, err := d.objectStore.GetActiveViews(d.Id())
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil
	}
	if err != nil {
		log.With("objectId", s.RootId()).Warnf("failed to get list of active views from store: %v", err)
		return
	}

	for blockId, viewId := range views {
		b := s.Pick(blockId)
		if b == nil {
			log.With("objectId", s.RootId()).Warnf("failed to get block '%s' to inject active view", blockId)
			continue
		}
		dv := b.Model().GetDataview()
		if dv == nil {
			log.With("objectId", s.RootId()).Warnf("block '%s' is not dataview, so cannot inject active view", blockId)
			continue
		}
		dv.ActiveView = viewId
	}

	return nil
}

func getDataviewBlock(s *state.State, id string) (dataview.Block, error) {
	b := s.Get(id)
	if b == nil {
		return nil, smartblock.ErrSimpleBlockNotFound
	}
	if tb, ok := b.(dataview.Block); ok {
		return tb, nil
	}
	return nil, fmt.Errorf("not a dataview block")
}

func BlockBySource(objectStore objectstore.ObjectStore, sources []string) (*model.BlockContentOfDataview, error) {
	// Empty schema
	if len(sources) == 0 {
		return template.MakeDataviewContent(false, nil, nil), nil
	}

	// Try object type
	objectType, err := objectStore.GetObjectType(sources[0])
	if err == nil {
		return template.MakeDataviewContent(false, objectType, nil), nil
	}

	// Finally, try relations
	relations := make([]*model.RelationLink, 0, len(sources))
	for _, relId := range sources {
		rel, err := objectStore.GetRelationByID(relId)
		if err != nil {
			return nil, fmt.Errorf("failed to get relation %s: %w", relId, err)
		}

		relations = append(relations, (&relationutils.Relation{Relation: rel}).RelationLink())
	}
	return template.MakeDataviewContent(false, objectType, relations), nil
}
