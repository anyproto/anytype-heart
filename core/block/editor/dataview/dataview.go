package dataview

import (
	"context"
	"fmt"
	"sync"

	blockDB "github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	bundle "github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
)

const defaultLimit = 50

var log = logging.Logger("anytype-mw-editor")

type Dataview interface {
	GetObjectTypeURL(ctx *state.Context, blockId string) (string, error)
	GetAggregatedRelations(blockId string) ([]*pbrelation.Relation, error)

	UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error
	DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx *state.Context, blockId string, activeViewId string, limit int, offset int) error
	CreateView(ctx *state.Context, blockId string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)
	AddRelation(ctx *state.Context, blockId string, relation pbrelation.Relation, showEvent bool) (*pbrelation.Relation, error)
	DeleteRelation(ctx *state.Context, blockId string, relationKey string, showEvent bool) error
	UpdateRelation(ctx *state.Context, blockId string, relationKey string, relation pbrelation.Relation, showEvent bool) error
	AddRelationOption(ctx *state.Context, blockId string, recordId string, relationKey string, option pbrelation.RelationOption, showEvent bool) (*pbrelation.RelationOption, error)
	UpdateRelationOption(ctx *state.Context, blockId string, recordId string, relationKey string, option pbrelation.RelationOption, showEvent bool) error
	DeleteRelationOption(ctx *state.Context, blockId string, recordId string, relationKey string, optionId string, showEvent bool) error
	FillAggregatedOptions(ctx *state.Context) error

	CreateRecord(ctx *state.Context, blockId string, rec model.ObjectDetails) (*model.ObjectDetails, error)
	UpdateRecord(ctx *state.Context, blockId string, recID string, rec model.ObjectDetails) error
	DeleteRecord(ctx *state.Context, blockId string, recID string) error

	smartblock.SmartblockOpenListner
}

func NewDataview(sb smartblock.SmartBlock, objTypeGetter ObjectTypeGetter) Dataview {
	return &dataviewCollectionImpl{SmartBlock: sb, ObjectTypeGetter: objTypeGetter}
}

type dataviewImpl struct {
	blockId      string
	activeViewId string
	offset       int
	limit        int
	records      []database.Record
	mu           sync.Mutex

	recordsUpdatesSubscription database.Subscription
	depsUpdatesSubscription    database.Subscription

	recordsUpdatesCancel context.CancelFunc
}

type ObjectTypeGetter interface {
	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
}

type dataviewCollectionImpl struct {
	smartblock.SmartBlock
	ObjectTypeGetter
	dataviews []*dataviewImpl
}

func (d *dataviewCollectionImpl) AddRelation(ctx *state.Context, blockId string, relation pbrelation.Relation, showEvent bool) (*pbrelation.Relation, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return nil, err
	}

	if relation.Key == "" {
		relation.Key = bson.NewObjectId().Hex()
	} else {
		existingRelation, err := d.Anytype().ObjectStore().GetRelation(relation.Key)
		if err != nil {
			return nil, err
		}

		if !pbtypes.RelationCompatible(existingRelation, &relation) {
			return nil, fmt.Errorf("provided relation not compatible with the same-key existing aggregated relation")
		}
	}

	if relation.Format == pbrelation.RelationFormat_file && relation.ObjectTypes == nil {
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

func (d *dataviewCollectionImpl) UpdateRelation(ctx *state.Context, blockId string, relationKey string, relation pbrelation.Relation, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if relation.Format == pbrelation.RelationFormat_file && relation.ObjectTypes == nil {
		relation.ObjectTypes = bundle.FormatFilePossibleTargetObjectTypes
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
func (d *dataviewCollectionImpl) AddRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, option pbrelation.RelationOption, showEvent bool) (*pbrelation.RelationOption, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return nil, err
	}

	var db database.Database
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return nil, fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(""); err != nil {
		return nil, err
	} else {
		db = target
	}

	optionId, err := db.AddRelationOption(recordId, relationKey, option)
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

func (d *dataviewCollectionImpl) UpdateRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, option pbrelation.RelationOption, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	for _, rel := range tb.Model().GetDataview().Relations {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == option.Id {
				rel.SelectDict[i] = &option
				if showEvent {
					return d.Apply(s)
				}
				return d.Apply(s, smartblock.NoEvent)
			}
		}

		return fmt.Errorf("relation option not found")
	}

	return fmt.Errorf("relation not found")
}

func (d *dataviewCollectionImpl) DeleteRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, optionId string, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	for _, rel := range tb.Model().GetDataview().Relations {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == optionId {
				rel.SelectDict = append(rel.SelectDict[:i], rel.SelectDict[i+1:]...)
				if showEvent {
					return d.Apply(s)
				}
				return d.Apply(s, smartblock.NoEvent)
			}
		}
		// todo: should we remove option and value from all objects within type?

		return fmt.Errorf("relation option not found")
	}

	return fmt.Errorf("relation not found")
}

func (d *dataviewCollectionImpl) GetAggregatedRelations(blockId string) ([]*pbrelation.Relation, error) {
	st := d.NewState()
	tb, err := getDataviewBlock(st, blockId)
	if err != nil {
		return nil, err
	}

	objectType, err := d.GetObjectType(tb.GetSource())
	if err != nil {
		return nil, err
	}
	hasRelations := func(rels []*pbrelation.Relation, key string) bool {
		for _, rel := range rels {
			if rel.Key == key {
				return true
			}
		}
		return false
	}

	rels := objectType.Relations
	for _, rel := range tb.Model().GetDataview().GetRelations() {
		if hasRelations(rels, rel.Key) {
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	agRels, err := d.Anytype().ObjectStore().ListRelations(tb.GetSource())
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

func (d *dataviewCollectionImpl) getDataviewImpl(block dataview.Block) *dataviewImpl {
	for _, dv := range d.dataviews {
		if dv.blockId == block.Model().Id {
			return dv
		}
	}

	var activeViewId string
	if len(block.Model().GetDataview().Views) > 0 {
		activeViewId = block.Model().GetDataview().Views[0].Id
		block.SetActiveView(activeViewId)
	}

	dv := &dataviewImpl{blockId: block.Model().Id, activeViewId: activeViewId, offset: 0, limit: defaultLimit}
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

func (d *dataviewCollectionImpl) GetObjectTypeURL(ctx *state.Context, blockId string) (string, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return "", err
	}

	if v, ok := tb.Model().Content.(*model.BlockContentOfDataview); !ok {
		return "", fmt.Errorf("wrong dataview block content type: %T", tb.Model().Content)
	} else {
		return v.Dataview.Source, nil
	}
}

func (d *dataviewCollectionImpl) UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	oldView := dvBlock.GetView(viewId)
	var needRecordRefresh bool
	if !pbtypes.DataviewFiltersEqualSorted(oldView.Filters, view.Filters) {
		needRecordRefresh = true
	} else if !pbtypes.DataviewSortsEqualSorted(oldView.Sorts, view.Sorts) {
		needRecordRefresh = true
	}
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
	activeView := dvBlock.GetView(activeViewId)
	if activeView == nil {
		return fmt.Errorf("view not found")
	}

	dvBlock.SetActiveView(activeViewId)
	if dv.activeViewId != activeViewId {
		dv.activeViewId = activeViewId
		dv.records = nil
	}

	dv.limit = limit
	dv.offset = offset
	msgs, err := d.fetchAndGetEventsMessages(dv, dvBlock)
	if err != nil {
		return err
	}
	ctx.SetMessages(d.SmartBlock.Id(), msgs)
	d.SmartBlock.CheckSubscriptions()

	return nil
}

func (d *dataviewCollectionImpl) CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error) {
	view.Id = uuid.New().String()
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, id)
	if err != nil {
		return nil, err
	}

	if len(view.Relations) == 0 {
		objType, err := d.ObjectTypeGetter.GetObjectType(tb.GetSource())
		if err != nil {
			return nil, fmt.Errorf("object type not found")
		}

		for _, rel := range objType.Relations {
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

func (d *dataviewCollectionImpl) CreateRecord(_ *state.Context, blockId string, rec model.ObjectDetails) (*model.ObjectDetails, error) {
	var (
		source  string
		ok      bool
		dvBlock dataview.Block
	)
	if dvBlock, ok = d.Pick(blockId).(dataview.Block); !ok {
		return nil, fmt.Errorf("not a dataview block")
	} else {
		source = dvBlock.Model().GetDataview().Source
	}

	var db database.Writer
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return nil, fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(source); err != nil {
		return nil, err
	} else {
		db = target
	}

	dv := d.getDataviewImpl(dvBlock)

	created, err := db.Create(dvBlock.Model().GetDataview().Relations, database.Record{Details: rec.Details}, dv.recordsUpdatesSubscription)
	if err != nil {
		return nil, err
	}

	return &model.ObjectDetails{Details: created.Details}, nil
}

func (d *dataviewCollectionImpl) UpdateRecord(_ *state.Context, blockId string, recID string, rec model.ObjectDetails) error {
	var (
		source  string
		ok      bool
		dvBlock dataview.Block
	)

	if dvBlock, ok = d.Pick(blockId).(dataview.Block); !ok {
		return fmt.Errorf("not a dataview block")
	} else {
		source = dvBlock.Model().GetDataview().Source
	}

	var db database.Database
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(source); err != nil {
		return err
	} else {
		db = target
	}

	err := db.Update(recID, dvBlock.Model().GetDataview().Relations, database.Record{Details: rec.Details})
	if err != nil {
		return err
	}
	dv := d.getDataviewImpl(dvBlock)

	objectType, err := d.GetObjectType(source)
	if err != nil {
		return err
	}
	sch := schema.New(objectType, dvBlock.Model().GetDataview().Relations)

	var depIdsMap = map[string]struct{}{}
	var depIds []string
	if rec.Details == nil || rec.Details.Fields == nil {
		return nil
	}

	for key, item := range rec.Details.Fields {
		if key == "id" || key == "type" {
			continue
		}

		if rel, _ := sch.GetRelationByKey(key); rel != nil && (rel.GetFormat() == pbrelation.RelationFormat_object || rel.GetFormat() == pbrelation.RelationFormat_file) {
			depIdsToAdd := pbtypes.GetStringListValue(item)
			for _, depId := range depIdsToAdd {
				if _, exists := depIdsMap[depId]; !exists {
					depIds = append(depIds, depId)
					depIdsMap[depId] = struct{}{}
				}
			}
		}
	}

	depDetails, _, err := db.QueryByIdAndSubscribeForChanges(depIds, dv.depsUpdatesSubscription)
	if err != nil {
		return err
	}

	sub := dv.depsUpdatesSubscription
	go func() {
		for _, det := range depDetails {
			sub.Publish(pbtypes.GetString(det.Details, bundle.RelationKeyId.String()), det.Details)
		}
	}()
	return nil
}

func (d *dataviewCollectionImpl) DeleteRecord(_ *state.Context, blockId string, recID string) error {
	var source string
	if dvBlock, ok := d.Pick(blockId).(dataview.Block); !ok {
		return fmt.Errorf("not a dataview block")
	} else {
		source = dvBlock.Model().GetDataview().Source
	}

	var db database.Writer
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(source); err != nil {
		return err
	} else {
		db = target
	}

	return db.Delete(recID)
}

func (d *dataviewCollectionImpl) FillAggregatedOptions(ctx *state.Context) error {
	st := d.NewStateCtx(ctx)
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(dataview.Block); !ok {
			return true
		} else {
			d.fillAggregatedOptions(dvBlock)
			st.Set(b)
			return true
		}
	})
	return d.Apply(st)
}

func (d *dataviewCollectionImpl) fillAggregatedOptions(b dataview.Block) {
	dvc := b.Model().GetDataview()

	for _, rel := range dvc.Relations {
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			continue
		}

		options, err := d.Anytype().ObjectStore().GetAggregatedOptions(rel.Key, rel.Format, dvc.Source)
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
	d.fetchAllDataviewsRecordsAndSendEvents(ctx)
}

func (d *dataviewCollectionImpl) fetchAndGetEventsMessages(dv *dataviewImpl, dvBlock dataview.Block) ([]*pb.EventMessage, error) {
	source := dvBlock.Model().GetDataview().Source
	activeView := dvBlock.GetView(dv.activeViewId)

	var db database.Reader
	if dbRouter, ok := d.SmartBlock.(blockDB.Router); !ok {
		return nil, fmt.Errorf("unexpected smart block type: %T", d.SmartBlock)
	} else if target, err := dbRouter.Get(source); err != nil {
		return nil, err
	} else {
		db = target
	}

	// todo: inject schema
	objectType, err := d.GetObjectType(source)
	if err != nil {
		return nil, err
	}
	sch := schema.New(objectType, dvBlock.Model().GetDataview().Relations)

	dv.mu.Lock()
	if dv.recordsUpdatesCancel != nil {
		dv.recordsUpdatesCancel()
	}

	dv.mu.Unlock()

	recordsCh := make(chan *types.Struct)
	depRecordsCh := make(chan *types.Struct)
	recordsSub := database.NewSubscription(nil, recordsCh)
	depRecordsSub := database.NewSubscription(nil, depRecordsCh)

	entries, cancelRecordSubscription, total, err := db.QueryAndSubscribeForChanges(&sch, database.Query{
		Relations: activeView.Relations,
		Filters:   activeView.Filters,
		Sorts:     activeView.Sorts,
		Limit:     dv.limit,
		Offset:    dv.offset,
	}, recordsSub)
	if err != nil {
		return nil, err
	}
	dv.recordsUpdatesSubscription = recordsSub

	var currentEntriesIds, depIds []string
	var depIdsMap = map[string]struct{}{}

	for _, entry := range dv.records {
		currentEntriesIds = append(currentEntriesIds, getEntryID(entry))
	}

	var records []*types.Struct
	for _, entry := range entries {
		for key, item := range entry.Details.Fields {
			if key == "id" || key == "type" {
				continue
			}

			if rel, _ := sch.GetRelationByKey(key); rel != nil && (rel.GetFormat() == pbrelation.RelationFormat_object || rel.GetFormat() == pbrelation.RelationFormat_file) {
				depIdsToAdd := pbtypes.GetStringListValue(item)
				for _, depId := range depIdsToAdd {
					if _, exists := depIdsMap[depId]; !exists {
						depIds = append(depIds, depId)
						depIdsMap[depId] = struct{}{}
					}
				}
			}
		}
		records = append(records, entry.Details)
	}

	depEntries, cancelDepsSubscripton, err := db.QueryByIdAndSubscribeForChanges(depIds, depRecordsSub)
	if err != nil {
		return nil, err
	}
	dv.depsUpdatesSubscription = depRecordsSub
	dv.recordsUpdatesCancel = func() {
		cancelDepsSubscripton()
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

	for _, depEntry := range depEntries {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetDetails{BlockSetDetails: &pb.EventBlockSetDetails{Id: pbtypes.GetString(depEntry.Details, bundle.RelationKeyId.String()), Details: depEntry.Details}}})
	}

	log.Debugf("db query for %s {filters: %+v, sorts: %+v, limit: %d, offset: %d} got %d records, total: %d, msgs: %d", source, activeView.Filters, activeView.Sorts, dv.limit, dv.offset, len(entries), total, len(msgs))
	dv.records = entries

	go func() {
		for {
			select {
			case rec, ok := <-recordsCh:
				if !ok {
					return
				}
				d.SendEvent([]*pb.EventMessage{
					{Value: &pb.EventMessageValueOfBlockDataviewRecordsUpdate{
						&pb.EventBlockDataviewRecordsUpdate{
							Id:      dv.blockId,
							ViewId:  activeView.Id,
							Records: []*types.Struct{rec},
						}}}})
			case rec, ok := <-depRecordsCh:
				if !ok {
					return
				}
				d.SendEvent([]*pb.EventMessage{
					{Value: &pb.EventMessageValueOfBlockSetDetails{
						BlockSetDetails: &pb.EventBlockSetDetails{
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
