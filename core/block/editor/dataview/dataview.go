package dataview

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"

	blockDB "github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	bundle "github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
)

const defaultLimit = 50

var log = logging.Logger("anytype-mw-editor")
var ErrMultiupdateWasNotAllowed = fmt.Errorf("multiupdate was not allowed")
var defaultDataviewRelations = append(bundle.RequiredInternalRelations, bundle.RelationKeyDone)

type Dataview interface {
	SetSource(ctx *state.Context, blockId string, source []string) (err error)

	GetAggregatedRelations(blockId string) ([]*model.Relation, error)
	GetDataviewRelations(blockId string) ([]*model.Relation, error)

	UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error
	DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx *state.Context, blockId string, activeViewId string, limit int, offset int) error
	CreateView(ctx *state.Context, blockId string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)
	SetViewPosition(ctx *state.Context, blockId string, viewId string, position uint32) error
	AddRelation(ctx *state.Context, blockId string, relation model.Relation, showEvent bool) (*model.Relation, error)
	DeleteRelation(ctx *state.Context, blockId string, relationKey string, showEvent bool) error
	UpdateRelation(ctx *state.Context, blockId string, relationKey string, relation model.Relation, showEvent bool) error
	AddRelationOption(ctx *state.Context, blockId string, recordId string, relationKey string, option model.RelationOption, showEvent bool) (*model.RelationOption, error)
	UpdateRelationOption(ctx *state.Context, blockId string, recordId string, relationKey string, option model.RelationOption, showEvent bool) error
	DeleteRelationOption(ctx *state.Context, allowMultiupdate bool, blockId string, recordId string, relationKey string, optionId string, showEvent bool) error
	FillAggregatedOptions(ctx *state.Context) error

	CreateRecord(ctx *state.Context, blockId string, rec model.ObjectDetails, templateId string) (*model.ObjectDetails, error)
	UpdateRecord(ctx *state.Context, blockId string, recID string, rec model.ObjectDetails) error
	DeleteRecord(ctx *state.Context, blockId string, recID string) error

	WithSystemObjects(yes bool)
	SetNewRecordDefaultFields(blockId string, defaultRecordFields *types.Struct) error

	smartblock.SmartblockOpenListner
}

func NewDataview(sb smartblock.SmartBlock) Dataview {
	return &dataviewCollectionImpl{SmartBlock: sb}
}

type dataviewImpl struct {
	blockId                    string
	activeViewId               string
	offset                     int
	limit                      int
	records                    []database.Record
	mu                         sync.Mutex
	defaultRecordFields        *types.Struct // will be always set to the new record
	recordsUpdatesSubscription database.Subscription
	depsUpdatesSubscription    database.Subscription
	depIds                     []string

	recordsUpdatesCancel context.CancelFunc
}

type dataviewCollectionImpl struct {
	smartblock.SmartBlock
	dataviews         []*dataviewImpl
	withSystemObjects bool
}

func (d *dataviewCollectionImpl) SetSource(ctx *state.Context, blockId string, source []string) (err error) {
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
		// todo: we should move d.dataviews cleanup somewhere globally to support direct dv block unlink
		filtered := d.dataviews[:0]
		for _, dv := range d.dataviews {
			if dv.blockId == blockId {
				dv.recordsUpdatesCancel()
				continue
			}
			filtered = append(filtered, dv)
		}

		d.dataviews = filtered
		s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
		return d.Apply(s, smartblock.NoRestrictions)
	}

	dvContent, _, err := DataviewBlockBySource(d.Anytype().ObjectStore(), source)
	if err != nil {
		return
	}

	if len(dvContent.Dataview.Views) > 0 {
		dvContent.Dataview.ActiveView = dvContent.Dataview.Views[0].Id
	}
	blockNew := simple.New(&model.Block{Content: &dvContent, Id: blockId}).(dataview.Block)
	d.fillAggregatedOptions(blockNew)
	s.Set(blockNew)
	if block == nil {
		s.InsertTo("", 0, blockId)
	}

	dv := d.getDataviewImpl(blockNew)
	dv.activeViewId = ""

	s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
	return d.Apply(s, smartblock.NoRestrictions)
}

func (d *dataviewCollectionImpl) SetNewRecordDefaultFields(blockId string, defaultRecordFields *types.Struct) error {
	var (
		dvBlock dataview.Block
		ok      bool
	)
	if dvBlock, ok = d.Pick(blockId).(dataview.Block); !ok {
		return fmt.Errorf("not a dataview block")
	}

	dv := d.getDataviewImpl(dvBlock)
	if dv == nil {
		return fmt.Errorf("block not found")
	}

	dv.defaultRecordFields = defaultRecordFields
	return nil
}

func (d *dataviewCollectionImpl) WithSystemObjects(yes bool) {
	d.withSystemObjects = yes
}

func (d *dataviewCollectionImpl) AddRelation(ctx *state.Context, blockId string, relation model.Relation, showEvent bool) (*model.Relation, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return nil, err
	}

	if relation.Key == "" {
		relation.Creator = d.Anytype().ProfileID()
		relation.Key = bson.NewObjectId().Hex()
	} else {
		existingRelation, err := d.Anytype().ObjectStore().GetRelation(relation.Key)
		if err != nil {
			log.Errorf("existingRelation failed to get: %s", err.Error())
		}

		if existingRelation != nil && (relation.ReadOnlyRelation || relation.Name == "") {
			relation = *existingRelation
		} else if existingRelation != nil && !pbtypes.RelationCompatible(existingRelation, &relation) {
			return nil, fmt.Errorf("provided relation not compatible with the same-key existing aggregated relation")
		}
	}

	if relation.Format == model.RelationFormat_file && relation.ObjectTypes == nil {
		relation.ObjectTypes = bundle.FormatFilePossibleTargetObjectTypes
	}

	// reset SelectDict because it is supposed to be aggregated and injected on-the-fly
	relation.SelectDict = nil
	tb.AddRelation(relation)
	err = d.Apply(s)
	if err != nil {
		return nil, err
	}

	return &relation, nil
}

func (d *dataviewCollectionImpl) DeleteRelation(ctx *state.Context, blockId string, relationKey string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = tb.DeleteRelation(relationKey); err != nil {
		return err
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *dataviewCollectionImpl) UpdateRelation(ctx *state.Context, blockId string, relationKey string, relation model.Relation, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	ot := d.getObjectTypeSource(tb)
	if relation.Format == model.RelationFormat_file && relation.ObjectTypes == nil {
		relation.ObjectTypes = bundle.FormatFilePossibleTargetObjectTypes
	}

	ex, _ := tb.GetRelation(relationKey)
	if ex != nil {
		if ex.Format != relation.Format {
			return fmt.Errorf("changing format of existing relation is retricted")
		}
		if ex.DataSource != relation.DataSource {
			return fmt.Errorf("changing data source of existing relation is retricted")
		}

		if ex.Hidden != relation.Hidden {
			return fmt.Errorf("changing hidden flag of existing relation is retricted")
		}
	}

	if relation.Format == model.RelationFormat_status || relation.Format == model.RelationFormat_tag {
		// reinject relation options
		options, err := d.Anytype().ObjectStore().GetAggregatedOptions(relationKey, ot)
		if err != nil {
			log.Errorf("failed to GetAggregatedOptionsForRelation %s", err.Error())
		} else {
			relation.SelectDict = options
		}
	}

	if err = tb.UpdateRelation(relationKey, relation); err != nil {
		return err
	}
	if showEvent {
		err = d.Apply(s)
	} else {
		err = d.Apply(s, smartblock.NoEvent)
	}
	if err != nil {
		return err
	}

	return nil
}

// AddRelationOption adds a new option to the select dict. It returns existing option for the relation key in case there is a one with the same text
func (d *dataviewCollectionImpl) AddRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, option model.RelationOption, showEvent bool) (*model.RelationOption, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return nil, err
	}

	var db database.Database
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return nil, fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(nil); err != nil {
		return nil, err
	} else {
		db = target
	}

	rel := pbtypes.GetRelation(tb.Model().GetDataview().Relations, relationKey)
	if rel == nil {
		return nil, fmt.Errorf("relation not found in dataview")
	}
	err = db.Update(recordId, []*model.Relation{rel}, database.Record{})
	if err != nil {
		return nil, err
	}

	if option.Id == "" {
		existingOptions, err := d.Anytype().ObjectStore().GetAggregatedOptions(rel.Key, s.ObjectType())
		if err != nil {
			log.Errorf("failed to get existing aggregated options: %s", err.Error())
		} else {
			for _, exOpt := range existingOptions {
				if strings.EqualFold(exOpt.Text, option.Text) {
					option.Id = exOpt.Id
					option.Color = exOpt.Color
					break
				}
			}
		}
	}

	optionId, err := db.UpdateRelationOption(recordId, relationKey, option)
	if err != nil {
		return nil, err
	}

	option.Id = optionId
	err = tb.AddRelationOption(relationKey, option)
	if err != nil {
		return nil, err
	}

	if showEvent {
		err = d.Apply(s)
	} else {
		err = d.Apply(s, smartblock.NoEvent)
	}

	return &option, err
}

func (d *dataviewCollectionImpl) UpdateRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, option model.RelationOption, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	var db database.Database
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(nil); err != nil {
		return err
	} else {
		db = target
	}

	if option.Id == "" {
		return fmt.Errorf("option id is empty")
	}

	err = tb.UpdateRelationOption(relationKey, option)
	if err != nil {
		log.Errorf("UpdateRelationOption error: %s", err.Error())
		return err
	}

	if showEvent {
		err = d.Apply(s)
	} else {
		err = d.Apply(s, smartblock.NoEvent)
	}

	rel := pbtypes.GetRelation(tb.Model().GetDataview().Relations, relationKey)
	if rel == nil {
		return fmt.Errorf("relation not found in dataview")
	}

	return db.Update(recordId, []*model.Relation{rel}, database.Record{})
}

func (d *dataviewCollectionImpl) DeleteRelationOption(ctx *state.Context, allowMultiupdate bool, blockId, recordId string, relationKey string, optionId string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	var db database.Database
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(nil); err != nil {
		return err
	} else {
		db = target
	}

	if optionId == "" {
		return fmt.Errorf("option id is empty")
	}

	objIds, err := db.AggregateObjectIdsForOptionAndRelation(relationKey, optionId)
	if err != nil {
		return err
	}

	if len(objIds) > 1 && !allowMultiupdate {
		return ErrMultiupdateWasNotAllowed
	}

	if slice.FindPos(objIds, recordId) == -1 {
		// just in case we have some indexing lag
		objIds = append(objIds, recordId)
	}

	err = tb.DeleteRelationOption(relationKey, optionId)
	if err != nil {
		return err
	}

	rel := pbtypes.GetRelation(tb.Model().GetDataview().Relations, relationKey)
	if rel == nil {
		return fmt.Errorf("relation not found in dataview")
	}

	for _, objId := range objIds {
		err = db.DeleteRelationOption(objId, relationKey, optionId)
		if err != nil {
			if objId != recordId {
				// not sure if it is a right approach here, but we may face some ACL problems later otherwise
				log.Errorf("DeleteRelationOption failed to multiupdate %s: %s", objId, err.Error())
			} else {
				return err
			}
		}
		log.Debugf("DeleteRelationOption updated %s", objId)
	}
	// todo: remove after source refactoring
	time.Sleep(time.Second * 1)

	if showEvent {
		err = d.Apply(s)
	} else {
		err = d.Apply(s, smartblock.NoEvent)
	}

	return nil
}

func (d *dataviewCollectionImpl) GetAggregatedRelations(blockId string) ([]*model.Relation, error) {
	st := d.NewState()
	tb, err := getDataviewBlock(st, blockId)
	if err != nil {
		return nil, err
	}

	sch, err := d.getSchema(tb)
	if err != nil {
		return nil, err
	}

	hasRelations := func(rels []*model.Relation, key string) bool {
		for _, rel := range rels {
			if rel.Key == key {
				return true
			}
		}
		return false
	}

	rels := sch.ListRelations()
	for _, rel := range tb.Model().GetDataview().GetRelations() {
		if hasRelations(rels, rel.Key) {
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	agRels, err := d.Anytype().ObjectStore().ListRelations(sch.ObjectType().GetUrl())
	if err != nil {
		return nil, err
	}

	for _, rel := range agRels {
		if hasRelations(rels, rel.Key) {
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	return rels, nil
}

func (d *dataviewCollectionImpl) GetDataviewRelations(blockId string) ([]*model.Relation, error) {
	st := d.NewState()
	tb, err := getDataviewBlock(st, blockId)
	if err != nil {
		return nil, err
	}

	return tb.Model().GetDataview().GetRelations(), nil
}

func (d *dataviewCollectionImpl) getDataviewImpl(block dataview.Block) *dataviewImpl {
	for _, dv := range d.dataviews {
		if dv.blockId == block.Model().Id {
			return dv
		}
	}

	dv := &dataviewImpl{blockId: block.Model().Id, activeViewId: "", offset: 0, limit: defaultLimit}
	d.dataviews = append(d.dataviews, dv)
	return dv
}

func (d *dataviewCollectionImpl) DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error {
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

	dv := d.getDataviewImpl(tb)
	if dv.activeViewId == viewId {
		views := tb.Model().GetDataview().Views
		if len(views) > 0 {
			dv.activeViewId = views[0].Id
			dv.offset = 0
			msgs, err := d.fetchAndGetEventsMessages(d.getDataviewImpl(tb), tb)
			if err != nil {
				return err
			}

			ctx.SetMessages(d.Id(), msgs)
		}
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *dataviewCollectionImpl) UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	oldView, err := dvBlock.GetView(viewId)
	if err != nil {
		return err
	}
	var needRecordRefresh bool
	if !pbtypes.DataviewFiltersEqualSorted(oldView.Filters, view.Filters) {
		needRecordRefresh = true
	} else if !pbtypes.DataviewSortsEqualSorted(oldView.Sorts, view.Sorts) {
		needRecordRefresh = true
	}
	d.fillAggregatedOptions(dvBlock)
	if err = dvBlock.SetView(viewId, view); err != nil {
		return err
	}

	dv := d.getDataviewImpl(dvBlock)
	if needRecordRefresh && dv.activeViewId == viewId {
		dv.offset = 0
		msgs, err := d.fetchAndGetEventsMessages(d.getDataviewImpl(dvBlock), dvBlock)
		if err != nil {
			return err
		}

		defer ctx.AddMessages(d.Id(), msgs)
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *dataviewCollectionImpl) SetActiveView(ctx *state.Context, id string, activeViewId string, limit int, offset int) error {
	var dvBlock dataview.Block
	var ok bool
	if dvBlock, ok = d.Pick(id).(dataview.Block); !ok {
		return fmt.Errorf("not a dataview block")
	}

	dv := d.getDataviewImpl(dvBlock)
	_, err := dvBlock.GetView(activeViewId)
	if err != nil {
		return err
	}
	dvBlock.SetActiveView(activeViewId)
	if dv.activeViewId != activeViewId {
		dv.activeViewId = activeViewId
		dv.records = nil
	}

	dv.limit = limit
	dv.offset = offset
	d.fillAggregatedOptions(dvBlock)
	msgs, err := d.fetchAndGetEventsMessages(dv, dvBlock)
	if err != nil {
		return err
	}

	d.SmartBlock.CheckSubscriptions()
	ctx.SetMessages(d.SmartBlock.Id(), msgs)
	return nil
}

func (d *dataviewCollectionImpl) SetViewPosition(ctx *state.Context, blockId string, viewId string, position uint32) (err error) {
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

func (d *dataviewCollectionImpl) CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error) {
	view.Id = uuid.New().String()
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, id)
	if err != nil {
		return nil, err
	}

	sch, err := d.getSchema(tb)
	if err != nil {
		return nil, err
	}

	if len(view.Relations) == 0 {
		// by default use list of relations from the schema
		for _, rel := range sch.ListRelations() {
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: !rel.Hidden})
		}
	}

	if len(view.Sorts) == 0 {
		// todo: set depends on the view type
		view.Sorts = []*model.BlockContentDataviewSort{{
			RelationKey: "name",
			Type:        model.BlockContentDataviewSort_Asc,
		}}
	}

	tb.AddView(view)
	return &view, d.Apply(s)
}

func (d *dataviewCollectionImpl) fetchAllDataviewsRecordsAndSendEvents(ctx *state.Context) {
	for _, dv := range d.dataviews {
		block := d.Pick(dv.blockId)
		if dvBlock, ok := block.(dataview.Block); !ok {
			continue
		} else {
			msgs, err := d.fetchAndGetEventsMessages(dv, dvBlock)
			if err != nil {
				log.Errorf("fetchAndGetEventsMessages for dataview block %s failed: %s", dv.blockId, err.Error())
				continue
			}

			if len(msgs) > 0 {
				ctx.AddMessages(d.SmartBlock.Id(), msgs)
			}
		}
	}
}

func (d *dataviewCollectionImpl) CreateRecord(ctx *state.Context, blockId string, rec model.ObjectDetails, templateId string) (*model.ObjectDetails, error) {
	dvBlock, db, err := d.getDatabase(blockId)
	if err != nil {
		return nil, err
	}

	dv := d.getDataviewImpl(dvBlock)
	dvBlock.Model().GetDataview().GetActiveView()
	if dv.defaultRecordFields != nil && dv.defaultRecordFields.Fields != nil {
		if rec.Details == nil || rec.Details.Fields == nil {
			rec.Details = dv.defaultRecordFields
		} else {
			for k, v := range dv.defaultRecordFields.Fields {
				if !pbtypes.HasField(rec.Details, k) {
					rec.Details.Fields[k] = pbtypes.CopyVal(v)
				}
			}
		}
	}
	callerCtx := context.WithValue(context.Background(), smartblock.CallerKey, d.Id())
	created, err := db.Create(callerCtx, dvBlock.Model().GetDataview().Relations, database.Record{Details: rec.Details}, dv.recordsUpdatesSubscription, templateId)
	if err != nil {
		return nil, err
	}

	return &model.ObjectDetails{Details: created.Details}, nil
}

func (d *dataviewCollectionImpl) UpdateRecord(_ *state.Context, blockId string, recID string, rec model.ObjectDetails) error {
	dvBlock, db, err := d.getDatabase(blockId)
	if err != nil {
		return err
	}

	relationsFiltered := pbtypes.RelationsFilterKeys(dvBlock.Model().GetDataview().Relations, pbtypes.StructNotNilKeys(rec.Details))
	err = db.Update(recID, relationsFiltered, database.Record{Details: rec.Details})
	if err != nil {
		return err
	}
	dv := d.getDataviewImpl(dvBlock)

	sch, err := d.getSchema(dvBlock)
	if err != nil {
		return err
	}

	var depIdsMap = map[string]struct{}{}
	var depIds []string
	if rec.Details == nil || rec.Details.Fields == nil {
		return nil
	}

	for key, item := range rec.Details.Fields {
		if key == "id" || key == "type" {
			continue
		}

		if rel := pbtypes.GetRelation(sch.ListRelations(), key); rel != nil && (rel.GetFormat() == model.RelationFormat_object || rel.GetFormat() == model.RelationFormat_file) {
			depIdsToAdd := pbtypes.GetStringListValue(item)
			for _, depId := range depIdsToAdd {
				if _, exists := depIdsMap[depId]; !exists {
					depIds = append(depIds, depId)
					depIdsMap[depId] = struct{}{}
				}
			}
		}
	}

	var depIdsNew []string
	for _, depId := range depIds {
		if slice.FindPos(dv.depIds, depId) == -1 {
			depIdsNew = append(depIdsNew, depId)
		}
	}

	depDetails, _, err := db.QueryByIdAndSubscribeForChanges(depIdsNew, dv.depsUpdatesSubscription)
	if err != nil {
		return err
	}

	sub := dv.depsUpdatesSubscription
	go func() {
		for _, det := range depDetails {
			sub.Publish(pbtypes.GetString(det.Details, bundle.RelationKeyId.String()), det.Details)
		}
	}()

	// replace dependent ids
	dv.depIds = depIds

	return nil
}

func (d *dataviewCollectionImpl) getDatabase(blockId string) (dataview.Block, database.Database, error) {
	if dvBlock, ok := d.Pick(blockId).(dataview.Block); !ok {
		return nil, nil, fmt.Errorf("not a dataview block")
	} else {
		sch, err := d.getSchema(dvBlock)
		if err != nil {
			return nil, nil, err
		}

		var db database.Database
		if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
			return nil, nil, fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
		} else if target, err := dbRouter.Get(sch.ObjectType()); err != nil {
			return nil, nil, err
		} else {
			db = target
		}

		return dvBlock, db, nil
	}
}

func (d *dataviewCollectionImpl) DeleteRecord(_ *state.Context, blockId string, recID string) error {
	_, db, err := d.getDatabase(blockId)
	if err != nil {
		return err
	}

	return db.Delete(recID)
}

func (d *dataviewCollectionImpl) FillAggregatedOptions(ctx *state.Context) error {
	st := d.NewStateCtx(ctx)
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(dataview.Block); !ok {
			return true
		} else {
			dvBlock = b.Copy().(dataview.Block)
			d.fillAggregatedOptions(dvBlock)
			st.Set(dvBlock)
			return true
		}
	})
	return d.Apply(st)
}

func (d *dataviewCollectionImpl) fillAggregatedOptions(b dataview.Block) {
	dvc := b.Model().GetDataview()
	ot := d.getObjectTypeSource(b)
	for _, rel := range dvc.Relations {
		if rel.Format != model.RelationFormat_status && rel.Format != model.RelationFormat_tag {
			continue
		}

		options, err := d.Anytype().ObjectStore().GetAggregatedOptions(rel.Key, ot)
		if err != nil {
			log.Errorf("failed to GetAggregatedOptionsForRelation %s", err.Error())
			continue
		}

		rel.SelectDict = options
	}
}

func (d *dataviewCollectionImpl) SmartblockOpened(ctx *state.Context) {
	st := d.NewStateCtx(ctx)
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(dataview.Block); !ok {
			return true
		} else {
			dv := d.getDataviewImpl(dvBlock)
			// reset state after block was reopened
			// getDataviewImpl will also set activeView to the fist one in case the smartblock wasn't opened in this session before
			d.fillAggregatedOptions(dvBlock)
			st.Set(b)
			dv.records = nil
		}
		return true
	})
	err := d.Apply(st)
	if err != nil {
		log.Errorf("failed to GetAggregatedOptionsForRelation %s", err.Error())
	}
}

func (d *dataviewCollectionImpl) updateAggregatedOptionsForRelation(st *state.State, dvBlock dataview.Block, rel *model.Relation) error {
	ot := d.getObjectTypeSource(dvBlock)
	options, err := d.Anytype().ObjectStore().GetAggregatedOptions(rel.Key, ot)
	if err != nil {
		return fmt.Errorf("failed to aggregate: %s", err.Error())
	}

	rel.SelectDict = options
	dvBlock.UpdateRelation(rel.Key, *rel)

	st.Set(dvBlock)
	return d.Apply(st)
}

// returns empty string
func (d *dataviewCollectionImpl) getObjectTypeSource(dvBlock dataview.Block) string {
	sources := dvBlock.Model().GetDataview().Source
	if len(sources) > 1 {
		return ""
	}

	for _, source := range sources {
		sbt, err := smartblock2.SmartBlockTypeFromID(source)
		if err != nil {
			return ""
		}

		if sbt == smartblock2.SmartBlockTypeObjectType || sbt == smartblock2.SmartBlockTypeBundledObjectType {
			return source
		}
		return ""
	}

	return ""
}

func SchemaBySources(sources []string, store objectstore.ObjectStore, optionalRelations []*model.Relation) (schema.Schema, error) {
	var hasRelations, hasType bool

	for _, source := range sources {
		sbt, err := smartblock2.SmartBlockTypeFromID(source)
		if err != nil {
			return nil, err
		}

		if sbt == smartblock2.SmartBlockTypeObjectType || sbt == smartblock2.SmartBlockTypeBundledObjectType {
			if hasRelations {
				return nil, fmt.Errorf("dataview source contains both type and relation")
			}
			if hasType {
				return nil, fmt.Errorf("dataview source contains more than one object type")
			}
			hasType = true
		}

		if sbt == smartblock2.SmartBlockTypeIndexedRelation || sbt == smartblock2.SmartBlockTypeBundledRelation {
			if hasType {
				return nil, fmt.Errorf("dataview source contains both type and relation")
			}
			hasRelations = true
		}
	}
	if hasType {
		objectType, err := objectstore.GetObjectType(store, sources[0])
		if err != nil {
			return nil, err
		}
		sch := schema.NewByType(objectType, optionalRelations)
		return sch, nil
	}

	if hasRelations {
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
			smartblock2.SmartBlockTypeObjectType,
		})
		if err != nil {
			return nil, err
		}

		var relations []*model.Relation
		for _, relId := range sources {
			relKey, err := pbtypes.RelationIdToKey(relId)
			if err != nil {
				return nil, fmt.Errorf("failed to get relation key from id %s: %s", relId, err.Error())
			}

			rel, err := store.GetRelation(relKey)
			if err != nil {
				return nil, fmt.Errorf("failed to get relation %s: %s", relKey, err.Error())
			}

			relations = append(relations, rel)
		}
		sch := schema.NewByRelations(ids, relations, optionalRelations)
		return sch, nil
	}

	return nil, fmt.Errorf("relation or type not found")
}

func (d *dataviewCollectionImpl) getSchema(dvBlock dataview.Block) (schema.Schema, error) {
	return SchemaBySources(dvBlock.Model().GetDataview().Source, d.Anytype().ObjectStore(), dvBlock.Model().GetDataview().Relations)
}

func (d *dataviewCollectionImpl) fetchAndGetEventsMessages(dv *dataviewImpl, dvBlock dataview.Block) ([]*pb.EventMessage, error) {
	activeView, err := dvBlock.GetView(dv.activeViewId)
	if err != nil {
		return nil, err
	}

	sch, err := d.getSchema(dvBlock)
	if err != nil {
		return nil, err
	}

	var db database.Database
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return nil, fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(sch.ObjectType()); err != nil {
		return nil, err
	} else {
		db = target
	}

	dv.mu.Lock()
	if dv.recordsUpdatesCancel != nil {
		dv.recordsUpdatesCancel()
	}

	dv.mu.Unlock()

	recordsCh := make(chan *types.Struct)
	depRecordsCh := make(chan *types.Struct)
	recordsSub := database.NewSubscription(nil, recordsCh)
	depRecordsSub := database.NewSubscription(nil, depRecordsCh)
	q := database.Query{
		Relations:         activeView.Relations,
		Filters:           activeView.Filters,
		Sorts:             activeView.Sorts,
		Limit:             dv.limit,
		Offset:            dv.offset,
		WithSystemObjects: d.withSystemObjects,
	}
	entries, cancelRecordSubscription, total, err := db.QueryAndSubscribeForChanges(sch, q, recordsSub)
	if err != nil {
		return nil, err
	}
	dv.recordsUpdatesSubscription = recordsSub

	var currentEntriesIds, depIds []string
	var depIdsMap = map[string]struct{}{}

	for _, entry := range dv.records {
		currentEntriesIds = append(currentEntriesIds, getEntryID(entry))
	}

	updateDepIds := func(ids []string) (newDepEntries []database.Record, close func(), err error) {
		var newDepIds []string
		for _, depId := range ids {
			if _, exists := depIdsMap[depId]; !exists {
				newDepIds = append(newDepIds, depId)
				depIdsMap[depId] = struct{}{}
			}
		}

		// todo: implement ref counter in order to unsubscribe from deps that are no longer used
		depEntries, cancelDepsSubscripton, err := db.QueryByIdAndSubscribeForChanges(newDepIds, depRecordsSub)
		if err != nil {
			return nil, nil, err
		}
		return depEntries, cancelDepsSubscripton, nil
	}

	getDepsFromRecord := func(rec *types.Struct) []string {
		if rec == nil || rec.Fields == nil {
			return nil
		}
		depsMap := make(map[string]struct{}, len(rec.Fields))
		var depIds []string
		for key, item := range rec.Fields {
			if key == "id" || key == "type" {
				continue
			}

			if rel := pbtypes.GetRelation(sch.ListRelations(), key); rel != nil && (rel.GetFormat() == model.RelationFormat_object || rel.GetFormat() == model.RelationFormat_file) && (len(rel.ObjectTypes) == 0 || rel.ObjectTypes[0] != bundle.TypeKeyRelation.URL()) {
				for _, depId := range pbtypes.GetStringListValue(item) {
					if _, exists := depsMap[depId]; exists {
						continue
					}
					depIds = append(depIds, depId)
					depsMap[depId] = struct{}{}
				}
			}
		}

		return depIds
	}

	var records []*types.Struct
	for _, entry := range entries {
		records = append(records, entry.Details)
		depIds = append(depIds, getDepsFromRecord(entry.Details)...)
	}

	depsEntries, cancelDepsSubscription, err := updateDepIds(depIds)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe dep entries: %s", err.Error())
	}

	dv.depsUpdatesSubscription = depRecordsSub
	dv.recordsUpdatesCancel = func() {
		cancelDepsSubscription()
		cancelRecordSubscription()
	}

	var msgs = []*pb.EventMessage{
		{Value: &pb.EventMessageValueOfBlockDataviewRecordsSet{
			BlockDataviewRecordsSet: &pb.EventBlockDataviewRecordsSet{
				Id:      dv.blockId,
				ViewId:  activeView.Id,
				Records: records,
				Total:   uint32(total),
			},
		}},
	}

	depEntriesToEvents := func(depsEntries []database.Record) []*pb.EventMessage {
		var msgs []*pb.EventMessage
		for _, depEntry := range depsEntries {
			msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfObjectDetailsSet{ObjectDetailsSet: &pb.EventObjectDetailsSet{Id: pbtypes.GetString(depEntry.Details, bundle.RelationKeyId.String()), Details: depEntry.Details}}})
		}
		return msgs
	}

	msgs = append(msgs, depEntriesToEvents(depsEntries)...)

	log.Debugf("db query for %s {filters: %+v, sorts: %+v, limit: %d, offset: %d} got %d records, total: %d, msgs: %d", sch.String(), activeView.Filters, activeView.Sorts, dv.limit, dv.offset, len(entries), total, len(msgs))
	dv.records = entries
	qFilter, err := filter.MakeAndFilter(activeView.Filters)
	if err != nil {
		return nil, err
	}

	filters := filter.AndFilters{sch.Filters(), qFilter}
	go func(dvBlockId string) {
		for {
			select {
			case rec, ok := <-recordsCh:
				if !ok {
					return
				}
				vg := pbtypes.ValueGetter(rec)
				if !filters.FilterObject(vg) {
					d.SendEvent([]*pb.EventMessage{
						{Value: &pb.EventMessageValueOfBlockDataviewRecordsDelete{
							&pb.EventBlockDataviewRecordsDelete{
								Id:      dv.blockId,
								ViewId:  activeView.Id,
								Removed: []string{pbtypes.GetString(rec, bundle.RelationKeyId.String())},
							}}}})

				} else {
					d.Lock()
					st := d.NewState()
					tb, err := getDataviewBlock(st, dvBlockId)
					if err != nil {
						log.Errorf("fetchAndGetEventsMessages subscription getDataviewBlock failed: %s", err.Error())
						d.Unlock()
						continue
					}
					rels := tb.Model().GetDataview().GetRelations()
					if rec != nil && rels != nil {
						for k, v := range rec.Fields {
							rel := pbtypes.GetRelation(rels, k)
							if rel == nil {
								// we don't have the dataview relation for this struct key, this means we can ignore it
								// todo: should we omit value when we don't have explicit relation in a dataview for it?
								continue
							}
							if rel.Format == model.RelationFormat_tag || rel.Format == model.RelationFormat_status {
								for _, opt := range pbtypes.GetStringListValue(v) {
									var found bool
									for _, existingOpt := range rel.SelectDict {
										if existingOpt.Id == opt {
											found = true
											break
										}
									}
									if !found {
										err = d.updateAggregatedOptionsForRelation(st, tb, rel)
										if err != nil {
											log.Errorf("failed to update dv relation: %s", err.Error())
										}
										break
									}
								}
							}
						}
					}
					d.Unlock()
					depsEntries, _, err := updateDepIds(getDepsFromRecord(rec))
					if err != nil {
						log.Errorf("failed to subscribe dep records of updated record: %s", err.Error())
					} else {
						d.SendEvent(depEntriesToEvents(depsEntries))
					}

					d.SendEvent([]*pb.EventMessage{
						{Value: &pb.EventMessageValueOfBlockDataviewRecordsUpdate{
							&pb.EventBlockDataviewRecordsUpdate{
								Id:      dv.blockId,
								ViewId:  activeView.Id,
								Records: []*types.Struct{rec},
							}}}})
				}
			}

		}
	}(dvBlock.Model().Id)
	go func() {
		for {
			select {
			case rec, ok := <-depRecordsCh:
				if !ok {
					return
				}
				d.SendEvent([]*pb.EventMessage{
					{Value: &pb.EventMessageValueOfObjectDetailsSet{
						ObjectDetailsSet: &pb.EventObjectDetailsSet{
							Id:      pbtypes.GetString(rec, bundle.RelationKeyId.String()),
							Details: rec,
						}}}})
			}

		}
	}()

	return msgs, nil
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

func DataviewBlockBySource(store objectstore.ObjectStore, source []string) (res model.BlockContentOfDataview, schema schema.Schema, err error) {
	if schema, err = SchemaBySources(source, store, nil); err != nil {
		return
	}

	var (
		relations     []*model.Relation
		viewRelations []*model.BlockContentDataviewRelation
	)

	for _, rel := range schema.RequiredRelations() {
		// other relations should be added with
		if pbtypes.HasRelation(relations, rel.Key) {
			continue
		}

		relations = append(relations, rel)
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: true})
	}

	for _, rel := range schema.ListRelations() {
		// other relations should be added with
		if pbtypes.HasRelation(relations, rel.Key) {
			continue
		}

		relations = append(relations, rel)
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
	}

	schemaRelations := schema.ListRelations()
	if !pbtypes.HasRelation(schemaRelations, bundle.RelationKeyName.String()) {
		schemaRelations = append([]*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)}, schemaRelations...)
	}

	for _, relKey := range defaultDataviewRelations {
		if pbtypes.HasRelation(relations, relKey.String()) {
			continue
		}
		rel := bundle.MustGetRelation(relKey)
		if rel.Hidden {
			continue
		}
		relations = append(relations, rel)
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
	}

	res = model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Relations: relations,
			Source:    source,
			Views: []*model.BlockContentDataviewView{
				{
					Id:   bson.NewObjectId().Hex(),
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Filters:   nil,
					Relations: viewRelations,
				},
			},
		},
	}
	return
}
