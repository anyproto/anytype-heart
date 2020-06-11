package dataview

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v2"
)

const defaultViewName = "Untitled"
const defaultLimit = 20

var log = logging.Logger("anytype-mw-editor")

type Dataview interface {
	UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error
	DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx *state.Context, blockId string, activeViewId string, limit int, offset int) error
	CreateView(ctx *state.Context, blockId string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)

	smartblock.SmartblockOpenListner
}

func NewDataview(sb smartblock.SmartBlock, sendEvent func(e *pb.Event)) Dataview {
	return &dataviewCollectionImpl{SmartBlock: sb, sendEvent: sendEvent}
}

type dataviewImpl struct {
	blockId      string
	activeViewId string
	offset       int
	limit        int
	entries      []database.Entry
	mu           sync.Mutex
}

type dataviewCollectionImpl struct {
	smartblock.SmartBlock
	dataviews []*dataviewImpl
	mu        sync.Mutex
	sendEvent func(e *pb.Event)
}

// This method is not thread-safe
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
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.mu.Lock()
	defer d.mu.Unlock()
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
		dv.entries = []database.Entry{}
		dv.activeViewId = activeViewId
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
	d.mu.Lock()
	defer d.mu.Unlock()

	view.Id = uuid.New().String()
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, id)
	if err != nil {
		return nil, err
	}

	if view.Name == "" {
		view.Name = defaultViewName
	}

	if len(view.Relations) == 0 {
		sch, err := d.Anytype().GetSchema(tb.Model().GetDataview().SchemaURL)
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

func (d *dataviewCollectionImpl) fetchAllDataviewsRecordsAndSendEvents() {
	d.mu.Lock()
	defer d.mu.Unlock()

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

			d.sendEvent(&pb.Event{
				Messages:  msgs,
				ContextId: d.SmartBlock.Id(),
				Initiator: nil,
			})
		}
	}

}

func (d *dataviewCollectionImpl) SmartblockOpened(ctx *state.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(dataview.Block); !ok {
			return true
		} else {
			// reset state after block was reopened
			// getDataviewImpl will also set activeView to the fist one in case the smartblock wasn't opened in this session before
			dv := d.getDataviewImpl(dvBlock)
			dv.entries = []database.Entry{}
		}
		return true
	})

	go d.fetchAllDataviewsRecordsAndSendEvents()
}

func (d *dataviewCollectionImpl) fetchAndGetEventsMessages(dv *dataviewImpl, dvBlock dataview.Block) ([]*pb.EventMessage, error) {
	databaseId := dvBlock.Model().GetDataview().DatabaseId
	activeView := dvBlock.GetView(dv.activeViewId)

	db, err := d.Anytype().DatabaseByID(databaseId)
	if err != nil {
		return nil, err
	}

	var msgs []*pb.EventMessage

	entries, err := db.Query(database.Query{
		Filters: activeView.Filters,
		Sorts:   activeView.Sorts,
		Limit:   dv.limit,
		Offset:  dv.offset,
	})

	updated, removed, insertedGroupedByPosition := calculateEntriesDiff(dv.entries, entries)

	var firstEventInserted []*types.Struct
	var firstEventInsertedAt int
	if len(insertedGroupedByPosition) > 0 {
		firstEventInserted = insertedGroupedByPosition[0].entries
		firstEventInsertedAt = insertedGroupedByPosition[0].position
	}

	if len(insertedGroupedByPosition) > 0 ||
		len(updated) > 0 ||
		len(removed) > 0 {

		msgs = append(msgs, &pb.EventMessage{&pb.EventMessageValueOfBlockSetDataviewRecords{
			&pb.EventBlockSetDataviewRecords{
				Id:             dv.blockId,
				ViewId:         activeView.Id,
				Updated:        updated,
				Removed:        removed,
				Inserted:       firstEventInserted,
				InsertPosition: int32(firstEventInsertedAt),
			},
		}})
	}

	if len(insertedGroupedByPosition) > 1 {
		for _, insertedPortion := range insertedGroupedByPosition[1:] {
			msgs = append(msgs, &pb.EventMessage{&pb.EventMessageValueOfBlockSetDataviewRecords{
				&pb.EventBlockSetDataviewRecords{
					Id:             dv.blockId,
					ViewId:         activeView.Id,
					Updated:        nil,
					Removed:        nil,
					Inserted:       insertedPortion.entries,
					InsertPosition: int32(insertedPortion.position),
				},
			}})
		}
	}
	log.Debugf("db query for %s {filters: %+v, sorts: %+v, limit: %d, offset: %d} got %d entries, updated: %d, removed: %d, insertedGroups: %d, msgs: %d", databaseId, activeView.Filters, activeView.Sorts, dv.limit, dv.offset, len(entries), len(updated), len(removed), len(insertedGroupedByPosition), len(msgs))

	dv.entries = entries

	return msgs, nil
}

func getDefaultRelations(schema *jsonschema.Schema) []*model.BlockContentDataviewRelation {
	var relations []*model.BlockContentDataviewRelation

	if defaults, ok := schema.Default.([]interface{}); ok {
		for _, def := range defaults {
			if v, ok := def.(map[string]interface{}); ok {
				if isHidden, exists := v["isHidden"]; exists {
					if v, ok := isHidden.(bool); ok {
						if v {
							continue
						}
					}
				}

				relations = append(relations,
					&model.BlockContentDataviewRelation{
						Id:      v["id"].(string),
						Visible: true,
					})
			}
		}
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

func getEntryID(entry database.Entry) string {
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

func calculateEntriesDiff(a []database.Entry, b []database.Entry) (updated []*types.Struct, removed []string, insertedGroupedByPosition []recordsInsertedAtPosition) {
	var inserted []recordInsertedAtPosition

	var existing = make(map[string]*types.Struct, len(a))
	for _, record := range a {
		existing[getEntryID(record)] = record.Details
	}

	var existingInNew = make(map[string]struct{}, len(b))
	for i, entry := range b {
		id := getEntryID(entry)
		if prev, exists := existing[id]; exists {
			if !prev.Equal(entry.Details) {
				updated = append(updated, entry.Details)
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
