package dataview

import (
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const DefaultDetailsFieldName = "_defaultRecordFields"

var log = logging.Logger("anytype-mw-editor-dataview")
var ErrMultiupdateWasNotAllowed = fmt.Errorf("multiupdate was not allowed")

type Dataview interface {
	SetSource(ctx *session.Context, blockId string, source []string) (err error)

	// GetAggregatedRelations(blockId string) ([]*model.Relation, error)
	GetDataviewRelations(blockId string) ([]*model.Relation, error)
	GetDataview(blockID string) (*model.BlockContentDataview, error)

	DeleteView(ctx *session.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx *session.Context, blockId string, activeViewId string, limit int, offset int) error
	CreateView(ctx *session.Context, blockID string,
		view model.BlockContentDataviewView, source []string) (*model.BlockContentDataviewView, error)
	SetViewPosition(ctx *session.Context, blockId string, viewId string, position uint32) error
	AddRelations(ctx *session.Context, blockId string, relationIds []string, showEvent bool) error
	DeleteRelations(ctx *session.Context, blockId string, relationIds []string, showEvent bool) error
	UpdateView(ctx *session.Context, blockID string, viewID string, view *model.BlockContentDataviewView, showEvent bool) error
	UpdateViewGroupOrder(ctx *session.Context, blockId string, order *model.BlockContentDataviewGroupOrder) error
	UpdateViewObjectOrder(ctx *session.Context, blockId string, orders []*model.BlockContentDataviewObjectOrder) error
	DataviewMoveObjectsInView(ctx *session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest) error

	GetDataviewBlock(s *state.State, blockID string) (dataview.Block, error)
}

func NewDataview(
	sb smartblock.SmartBlock,
	anytype core.Service,
	objectStore objectstore.ObjectStore,
	relationService relation.Service,
	sbtProvider typeprovider.SmartBlockTypeProvider,
) Dataview {
	dv := &sdataview{
		SmartBlock:      sb,
		anytype:         anytype,
		objectStore:     objectStore,
		relationService: relationService,
		sbtProvider:     sbtProvider,
	}
	sb.AddHook(dv.checkDVBlocks, smartblock.HookBeforeApply)
	return dv
}

type sdataview struct {
	smartblock.SmartBlock
	anytype         core.Service
	objectStore     objectstore.ObjectStore
	relationService relation.Service
	sbtProvider     typeprovider.SmartBlockTypeProvider
}

func (d *sdataview) GetDataviewBlock(s *state.State, blockID string) (dataview.Block, error) {
	return getDataviewBlock(s, blockID)
}

func (d *sdataview) SetSource(ctx *session.Context, blockId string, source []string) (err error) {
	s := d.NewStateCtx(ctx)
	if blockId == "" {
		blockId = template.DataviewBlockId
	}

	block, e := getDataviewBlock(s, blockId)
	if e != nil && blockId != template.DataviewBlockId {
		return e
	}
	if block != nil && slice.UnsortedEquals(block.GetSource(), source) {
		return
	}

	if len(source) == 0 {
		s.Unlink(blockId)
		s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
		return d.Apply(s, smartblock.NoRestrictions)
	}

	dvContent, _, err := DataviewBlockBySource(d.sbtProvider, d.objectStore, source)
	if err != nil {
		return
	}

	if len(dvContent.Dataview.Views) > 0 {
		dvContent.Dataview.ActiveView = dvContent.Dataview.Views[0].Id
	}
	blockNew := simple.New(&model.Block{Content: &dvContent, Id: blockId}).(dataview.Block)
	s.Set(blockNew)
	if block == nil {
		s.InsertTo("", 0, blockId)
	}

	s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
	return d.Apply(s, smartblock.NoRestrictions)
}

func (d *sdataview) AddRelations(ctx *session.Context, blockId string, relationKeys []string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}
	for _, key := range relationKeys {
		relation, err2 := d.relationService.FetchKey(key)
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

func (d *sdataview) DeleteRelations(ctx *session.Context, blockId string, relationIds []string, showEvent bool) error {
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

func (d *sdataview) DeleteView(ctx *session.Context, blockId string, viewId string, showEvent bool) error {
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

func (d *sdataview) UpdateView(ctx *session.Context, blockID string, viewID string, view *model.BlockContentDataviewView, showEvent bool) error {
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

func (d *sdataview) SetActiveView(ctx *session.Context, id string, activeViewId string, limit int, offset int) error {
	s := d.NewStateCtx(ctx)

	dvBlock, err := getDataviewBlock(s, id)
	if err != nil {
		return err
	}

	if _, err = dvBlock.GetView(activeViewId); err != nil {
		return err
	}
	dvBlock.SetActiveView(activeViewId)

	d.SmartBlock.CheckSubscriptions()
	return d.Apply(s)
}

func (d *sdataview) SetViewPosition(ctx *session.Context, blockId string, viewId string, position uint32) (err error) {
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

func (d *sdataview) CreateView(ctx *session.Context, id string,
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
		view.Sorts = defaultLastModifiedDateSort()
	}

	sbType, err := d.sbtProvider.Type(d.Id())
	if err != nil {
		return nil, err
	}
	if sbType == smartblock2.SmartBlockTypeWorkspace && d.Id() != d.anytype.PredefinedBlocks().Account {
		view.Filters = []*model.BlockContentDataviewFilter{{
			RelationKey: bundle.RelationKeyWorkspaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(d.Id()),
		}, {
			RelationKey: bundle.RelationKeyId.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.String(d.Id()),
		}}
	}
	tb.AddView(view)
	return &view, d.Apply(s)
}

func defaultLastModifiedDateSort() []*model.BlockContentDataviewSort {
	return []*model.BlockContentDataviewSort{
		{
			Id:          bson.NewObjectId().Hex(),
			RelationKey: bundle.RelationKeyLastModifiedDate.String(),
			Type:        model.BlockContentDataviewSort_Desc,
		},
	}
}

func (d *sdataview) UpdateViewGroupOrder(ctx *session.Context, blockId string, order *model.BlockContentDataviewGroupOrder) error {
	st := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(st, blockId)
	if err != nil {
		return err
	}

	dvBlock.SetViewGroupOrder(order)

	return d.Apply(st)
}

func (d *sdataview) UpdateViewObjectOrder(ctx *session.Context, blockId string, orders []*model.BlockContentDataviewObjectOrder) error {
	st := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(st, blockId)
	if err != nil {
		return err
	}

	dvBlock.SetViewObjectOrder(orders)

	return d.Apply(st)
}

func (d *sdataview) DataviewMoveObjectsInView(ctx *session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest) error {
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

func SchemaBySources(sbtProvider typeprovider.SmartBlockTypeProvider, sources []string, store objectstore.ObjectStore, optionalRelations []*model.RelationLink) (schema.Schema, error) {
	var hasRelations, hasType bool

	for _, source := range sources {
		sbt, err := sbtProvider.Type(source)
		if err != nil {
			return nil, err
		}

		// todo: fix a bug here. we will get subobject type here so we can't depend on smartblock type
		if sbt == smartblock2.SmartBlockTypeBundledObjectType {
			if hasRelations {
				return nil, fmt.Errorf("dataview source contains both type and relation")
			}
			if hasType {
				return nil, fmt.Errorf("dataview source contains more than one object type")
			}
			hasType = true
		}

		if strings.HasPrefix(source, addr.RelationKeyToIdPrefix) {
			if hasType {
				return nil, fmt.Errorf("dataview source contains both type and relation")
			}
			hasRelations = true
		}

		if strings.HasPrefix(source, addr.ObjectTypeKeyToIdPrefix) {
			if hasRelations {
				return nil, fmt.Errorf("dataview source contains both type and relation")
			}
			hasType = true
		}
	}
	if hasType {
		objectType, err := store.GetObjectType(sources[0])
		if err != nil {
			return nil, err
		}
		sch := schema.NewByType(objectType, optionalRelations)
		return sch, nil
	}

	if hasRelations {
		// todo: fix a bug here. we will get subobject type here so we can't depend on smartblock type
		ids, _, err := store.QueryObjectIds(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyRecommendedRelations.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.StringList(sources),
				},
			},
		}, []smartblock2.SmartBlockType{
			smartblock2.SmartBlockTypeBundledObjectType,
		})
		if err != nil {
			return nil, err
		}

		var relations []*model.RelationLink
		for _, relId := range sources {
			rel, err := store.GetRelationById(relId)
			if err != nil {
				return nil, fmt.Errorf("failed to get relation %s: %s", relId, err.Error())
			}

			relations = append(relations, (&relationutils.Relation{rel}).RelationLink())
		}
		sch := schema.NewByRelations(ids, relations, optionalRelations)
		return sch, nil
	}

	return nil, fmt.Errorf("relation or type not found")
}

func (d *sdataview) getSchema(dvBlock dataview.Block, source []string) (schema.Schema, error) {
	return SchemaBySources(d.sbtProvider, source, d.objectStore, dvBlock.Model().GetDataview().RelationLinks)
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
	var restrictedSources = []string{
		bundle.TypeKeyFile.URL(),
		bundle.TypeKeyImage.URL(),
		bundle.TypeKeyVideo.URL(),
		bundle.TypeKeyAudio.URL(),
		bundle.TypeKeyObjectType.URL(),
		bundle.TypeKeySet.URL(),
		bundle.TypeKeyRelation.URL(),
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

func getEntryID(entry database.Record) string {
	if entry.Details == nil {
		return ""
	}

	return pbtypes.GetString(entry.Details, bundle.RelationKeyId.String())
}

type recordInsertedAtPosition struct {
	position int
	entry    *types.Struct
}

type recordsInsertedAtPosition struct {
	position int
	entries  []*types.Struct
}

func calculateEntriesDiff(a, b []database.Record) (updated []*types.Struct, removed []string, insertedGroupedByPosition []recordsInsertedAtPosition) {
	var inserted []recordInsertedAtPosition

	var existing = make(map[string]*types.Struct, len(a))
	for _, record := range a {
		existing[getEntryID(record)] = record.Details
	}

	var existingInNew = make(map[string]struct{}, len(b))
	for i, entry := range b {
		id := getEntryID(entry)
		if prev, exists := existing[id]; exists {
			if len(a) <= i || getEntryID(a[i]) != id {
				// todo: return as moved?
				removed = append(removed, id)
				inserted = append(inserted, recordInsertedAtPosition{i, entry.Details})
			} else {
				if !prev.Equal(entry.Details) {
					updated = append(updated, entry.Details)
				}
			}
		} else {
			inserted = append(inserted, recordInsertedAtPosition{i, entry.Details})
		}

		existingInNew[id] = struct{}{}
	}

	for id := range existing {
		if _, exists := existingInNew[id]; !exists {
			removed = append(removed, id)
		}
	}

	var insertedToTheLastPosition = recordsInsertedAtPosition{position: -1}
	var lastPos = -1

	if len(inserted) > 0 {
		insertedToTheLastPosition.position = inserted[0].position
		lastPos = inserted[0].position - 1
	}

	for _, entry := range inserted {
		if entry.position > lastPos+1 {
			// split the insert portion
			insertedGroupedByPosition = append(insertedGroupedByPosition, insertedToTheLastPosition)
			insertedToTheLastPosition = recordsInsertedAtPosition{position: entry.position}
		}

		lastPos = entry.position
		insertedToTheLastPosition.entries = append(insertedToTheLastPosition.entries, entry.entry)
	}
	if len(insertedToTheLastPosition.entries) > 0 {
		insertedGroupedByPosition = append(insertedGroupedByPosition, insertedToTheLastPosition)
	}

	return
}

func DataviewBlockBySource(sbtProvider typeprovider.SmartBlockTypeProvider, store objectstore.ObjectStore, source []string) (res model.BlockContentOfDataview, schema schema.Schema, err error) {
	if schema, err = SchemaBySources(sbtProvider, source, store, nil); err != nil {
		return
	}

	var (
		relations     []*model.RelationLink
		viewRelations []*model.BlockContentDataviewRelation
	)

	for _, rel := range schema.RequiredRelations() {
		relations = append(relations, &model.RelationLink{
			Format: rel.Format,
			Key:    rel.Key,
		})
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: true})
	}

	for _, rel := range schema.ListRelations() {
		// other relations should be added with
		if pbtypes.HasRelationLink(relations, rel.Key) {
			continue
		}

		relations = append(relations, &model.RelationLink{
			Format: rel.Format,
			Key:    rel.Key,
		})
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
	}

	schemaRelations := schema.ListRelations()
	if !pbtypes.HasRelationLink(schemaRelations, bundle.RelationKeyName.String()) {
		schemaRelations = append([]*model.RelationLink{bundle.MustGetRelationLink(bundle.RelationKeyName)}, schemaRelations...)
	}

	for _, relKey := range template.DefaultDataviewRelations {
		if pbtypes.HasRelationLink(relations, relKey.String()) {
			continue
		}
		rel := bundle.MustGetRelation(relKey)
		if rel.Hidden {
			continue
		}
		relations = append(relations, &model.RelationLink{
			Format: rel.Format,
			Key:    rel.Key,
		})
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
	}

	res = model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			RelationLinks: relations,
			Views: []*model.BlockContentDataviewView{
				{
					Id:        bson.NewObjectId().Hex(),
					Type:      model.BlockContentDataviewView_Table,
					Name:      "All",
					Sorts:     defaultLastModifiedDateSort(),
					Filters:   nil,
					Relations: viewRelations,
				},
			},
		},
	}
	return
}
