package dataview

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
)

const defaultLimit = 100

var log = logging.Logger("anytype-mw-editor")

type Dataview interface {
	UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error
	DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx *state.Context, blockId string, activeViewId string, limit int, offset int) error
	CreateView(ctx *state.Context, blockId string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)

	CreateRecord(ctx *state.Context, blockId string, rec model.PageDetails) (*model.PageDetails, error)
	UpdateRecord(ctx *state.Context, blockId string, recID string, rec model.PageDetails) error
	DeleteRecord(ctx *state.Context, blockId string, recID string) error

	smartblock.SmartblockOpenListner
}

func NewDataview(sb smartblock.SmartBlock) Dataview {
	return &dataviewCollectionImpl{SmartBlock: sb}
}

type dataviewImpl struct {
	blockId      string
	activeViewId string
	offset       int
	limit        int
	records      []database.Record
	mu           sync.Mutex
}

type dataviewCollectionImpl struct {
	smartblock.SmartBlock
	dataviews []*dataviewImpl
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

func (d *dataviewCollectionImpl) UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = tb.SetView(viewId, view); err != nil {
		return err
	}

	dv := d.getDataviewImpl(tb)
	if dv.activeViewId == viewId {
		dv.offset = 0
		msgs, err := d.fetchAndGetEventsMessages(d.getDataviewImpl(tb), tb)
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
		sch, err := schema.Get(tb.Model().GetDataview().SchemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema %s for dataview: %s", tb.Model().GetDataview().SchemaURL, err.Error())
		}

		view.Relations = getDefaultRelations(sch)
	}

	if len(view.Sorts) == 0 {
		// todo: set depends on the view type
		view.Sorts = []*model.BlockContentDataviewSort{{
			RelationId: "id",
			Type:       model.BlockContentDataviewSort_Asc,
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

func (d *dataviewCollectionImpl) CreateRecord(ctx *state.Context, blockId string, rec model.PageDetails) (*model.PageDetails, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return nil, err
	}

	dbId := tb.Model().GetDataview().GetDatabaseId()
	db, err := d.Anytype().DatabaseByID(dbId)
	if err != nil {
		return nil, err
	}

	createdRec, err := db.Create(database.Record{Details: rec.Details})
	if err != nil {
		return nil, err
	}

	return &model.PageDetails{Details: createdRec.Details}, nil
}

func (d *dataviewCollectionImpl) UpdateRecord(ctx *state.Context, blockId string, recID string, rec model.PageDetails) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	dbId := tb.Model().GetDataview().GetDatabaseId()
	db, err := d.Anytype().DatabaseByID(dbId)
	if err != nil {
		return err
	}

	return db.Update(recID, database.Record{Details: rec.Details})
}

func (d *dataviewCollectionImpl) DeleteRecord(ctx *state.Context, blockId string, recID string) error {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	dbId := tb.Model().GetDataview().GetDatabaseId()
	db, err := d.Anytype().DatabaseByID(dbId)
	if err != nil {
		return err
	}

	if err := db.Delete(recID); err != nil {
		return err
	}

	d.fetchAllDataviewsRecordsAndSendEvents(ctx)
	return nil
}

func (d *dataviewCollectionImpl) SmartblockOpened(ctx *state.Context) {
	d.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(dataview.Block); !ok {
			return true
		} else {
			// reset state after block was reopened
			// getDataviewImpl will also set activeView to the fist one in case the smartblock wasn't opened in this session before
			dv := d.getDataviewImpl(dvBlock)
			dv.records = nil
		}
		return true
	})

	d.fetchAllDataviewsRecordsAndSendEvents(ctx)
}

func (d *dataviewCollectionImpl) fetchAndGetEventsMessages(dv *dataviewImpl, dvBlock dataview.Block) ([]*pb.EventMessage, error) {
	databaseId := dvBlock.Model().GetDataview().DatabaseId
	activeView := dvBlock.GetView(dv.activeViewId)

	db, err := d.Anytype().DatabaseByID(databaseId)
	if err != nil {
		return nil, err
	}

	var msgs []*pb.EventMessage

	entries, total, err := db.Query(database.Query{
		Relations: activeView.Relations,
		Filters:   activeView.Filters,
		Sorts:     activeView.Sorts,
		Limit:     dv.limit,
		Offset:    dv.offset,
	})

	var currentEntriesIds []string
	for _, entry := range dv.records {
		currentEntriesIds = append(currentEntriesIds, getEntryID(entry))
	}

	var records []*types.Struct
	for _, entry := range entries {
		records = append(records, entry.Details)
	}

	msgs = append(msgs, &pb.EventMessage{&pb.EventMessageValueOfBlockSetDataviewRecords{
		&pb.EventBlockSetDataviewRecords{
			Id:             dv.blockId,
			ViewId:         activeView.Id,
			Updated:        nil,
			Removed:        currentEntriesIds,
			Inserted:       records,
			InsertPosition: 0,
			Total:          uint32(total),
		},
	}})

	log.Debugf("db query for %s {filters: %+v, sorts: %+v, limit: %d, offset: %d} got %d records, total: %d, msgs: %d", databaseId, activeView.Filters, activeView.Sorts, dv.limit, dv.offset, len(entries), total, len(msgs))
	dv.records = entries

	return msgs, nil
}

func getDefaultRelations(schema *schema.Schema) []*model.BlockContentDataviewRelation {
	var relations []*model.BlockContentDataviewRelation

	for _, rel := range schema.Default {
		if rel.IsHidden {
			continue
		}
		relations = append(relations,
			&model.BlockContentDataviewRelation{
				Id:        rel.ID,
				IsVisible: true,
			})
	}

	return relations
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

	return entry.Details.Fields["id"].GetStringValue()
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
