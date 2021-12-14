package dataview

import (
	"context"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"

	blockDB "github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	bundle "github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
)

const DefaultDetailsFieldName = "_defaultRecordFields"

var log = logging.Logger("anytype-mw-editor")
var ErrMultiupdateWasNotAllowed = fmt.Errorf("multiupdate was not allowed")
var DefaultDataviewRelations = append(bundle.RequiredInternalRelations, bundle.RelationKeyDone)

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
	FillAggregatedOptionsState(s *state.State) error

	CreateRecord(ctx *state.Context, blockId string, rec model.ObjectDetails, templateId string) (*model.ObjectDetails, error)
}

func NewDataview(sb smartblock.SmartBlock) Dataview {
	return &sdataview{SmartBlock: sb}
}

type sdataview struct {
	smartblock.SmartBlock
}

func (d *sdataview) SetSource(ctx *state.Context, blockId string, source []string) (err error) {
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

	s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
	return d.Apply(s, smartblock.NoRestrictions)
}

func (d *sdataview) AddRelation(ctx *state.Context, blockId string, relation model.Relation, showEvent bool) (*model.Relation, error) {
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

func (d *sdataview) DeleteRelation(ctx *state.Context, blockId string, relationKey string, showEvent bool) error {
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

func (d *sdataview) UpdateRelation(ctx *state.Context, blockId string, relationKey string, relation model.Relation, showEvent bool) error {
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
func (d *sdataview) AddRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, option model.RelationOption, showEvent bool) (*model.RelationOption, error) {
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

func (d *sdataview) UpdateRelationOption(ctx *state.Context, blockId, recordId string, relationKey string, option model.RelationOption, showEvent bool) error {
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

func (d *sdataview) DeleteRelationOption(ctx *state.Context, allowMultiupdate bool, blockId, recordId string, relationKey string, optionId string, showEvent bool) error {
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

	if showEvent {
		err = d.Apply(s)
	} else {
		err = d.Apply(s, smartblock.NoEvent)
	}

	return nil
}

func (d *sdataview) GetAggregatedRelations(blockId string) ([]*model.Relation, error) {
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

func (d *sdataview) GetDataviewRelations(blockId string) ([]*model.Relation, error) {
	st := d.NewState()
	tb, err := getDataviewBlock(st, blockId)
	if err != nil {
		return nil, err
	}

	return tb.Model().GetDataview().GetRelations(), nil
}

func (d *sdataview) DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error {
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

func (d *sdataview) UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	dvBlock, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	d.fillAggregatedOptions(dvBlock)
	if err = dvBlock.SetView(viewId, view); err != nil {
		return err
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *sdataview) SetActiveView(ctx *state.Context, id string, activeViewId string, limit int, offset int) error {
	s := d.NewStateCtx(ctx)

	dvBlock, err := getDataviewBlock(s, id)
	if err != nil {
		return err
	}

	if _, err = dvBlock.GetView(activeViewId); err != nil {
		return err
	}
	dvBlock.SetActiveView(activeViewId)

	d.fillAggregatedOptions(dvBlock)

	d.SmartBlock.CheckSubscriptions()
	return d.Apply(s)
}

func (d *sdataview) SetViewPosition(ctx *state.Context, blockId string, viewId string, position uint32) (err error) {
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

func (d *sdataview) CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error) {
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
		relsM := make(map[string]struct{}, len(view.Relations))
		// by default use list of relations from the schema
		for _, rel := range sch.ListRelations() {
			relsM[rel.Key] = struct{}{}
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: !rel.Hidden})
		}
		for _, relKey := range DefaultDataviewRelations {
			if _, exists := relsM[relKey.String()]; exists {
				continue
			}
			rel := bundle.MustGetRelation(relKey)
			if rel.Hidden {
				continue
			}
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
		}
	}

	if len(view.Sorts) == 0 {
		// todo: set depends on the view type
		view.Sorts = []*model.BlockContentDataviewSort{{
			RelationKey: "name",
			Type:        model.BlockContentDataviewSort_Asc,
		}}
	}

	sbType, err := smartblock2.SmartBlockTypeFromID(d.Id())
	if err != nil {
		return nil, err
	}
	if sbType == smartblock2.SmartBlockTypeWorkspace && d.Id() != d.Anytype().PredefinedBlocks().Account {
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

func (d *sdataview) CreateRecord(ctx *state.Context, blockId string, rec model.ObjectDetails, templateId string) (*model.ObjectDetails, error) {
	dvBlock, db, err := d.getDatabase(blockId)
	if err != nil {
		return nil, err
	}
	if defaultRecordFields := pbtypes.GetStruct(dvBlock.Model().Fields, DefaultDetailsFieldName); defaultRecordFields != nil && defaultRecordFields.Fields != nil {
		if rec.Details == nil || rec.Details.Fields == nil {
			rec.Details = defaultRecordFields
		} else {
			for k, v := range defaultRecordFields.Fields {
				if !pbtypes.HasField(rec.Details, k) {
					rec.Details.Fields[k] = pbtypes.CopyVal(v)
				}
			}
		}
	}
	callerCtx := context.WithValue(context.Background(), smartblock.CallerKey, d.Id())
	created, err := db.Create(callerCtx, dvBlock.Model().GetDataview().Relations, database.Record{Details: rec.Details}, nil, templateId)
	if err != nil {
		return nil, err
	}

	return &model.ObjectDetails{Details: created.Details}, nil
}

func (d *sdataview) getDatabase(blockId string) (dataview.Block, database.Database, error) {
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

func (d *sdataview) FillAggregatedOptions(ctx *state.Context) error {
	st := d.NewStateCtx(ctx)
	if err := d.FillAggregatedOptionsState(st); err != nil {
		return err
	}
	return d.Apply(st)
}

func (d *sdataview) FillAggregatedOptionsState(s *state.State) error {
	return s.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(dataview.Block); !ok {
			return true
		} else {
			b = s.Get(b.Model().Id)
			d.fillAggregatedOptions(dvBlock)
			return true
		}
	})
}

func (d *sdataview) fillAggregatedOptions(b dataview.Block) {
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

func (d *sdataview) updateAggregatedOptionsForRelation(st *state.State, dvBlock dataview.Block, rel *model.Relation) error {
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
func (d *sdataview) getObjectTypeSource(dvBlock dataview.Block) string {
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

func (d *sdataview) getSchema(dvBlock dataview.Block) (schema.Schema, error) {
	return SchemaBySources(dvBlock.Model().GetDataview().Source, d.Anytype().ObjectStore(), dvBlock.Model().GetDataview().Relations)
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

	for _, relKey := range DefaultDataviewRelations {
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
