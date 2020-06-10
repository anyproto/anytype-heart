package dataview

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v2"
)

const defaultViewName = "Untitled"

var log = logging.Logger("anytype-mw-editor")

type Dataview interface {
	UpdateDataview(ctx *state.Context, id string, showEvent bool, apply func(t dataview.Block) error) error
	SetActiveView(ctx *state.Context, id string, activeViewId string, limit int, offset int) error
	CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)

	smartblock.SmartblockOpenListner
}

func NewDataview(sb smartblock.SmartBlock) Dataview {
	return &dataviewImpl{SmartBlock: sb}
}

type dataviewImpl struct {
	smartblock.SmartBlock
	activeView string
	entries    []database.Entry
	mu         sync.Mutex
}

func (d *dataviewImpl) UpdateDataview(ctx *state.Context, id string, showEvent bool, apply func(t dataview.Block) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.NewStateCtx(ctx)
	tb, err := getDataview(s, id)
	if err != nil {
		return err
	}

	if err = apply(tb); err != nil {
		return err
	}

	if showEvent {
		return d.Apply(s)
	}
	return d.Apply(s, smartblock.NoEvent)
}

func (d *dataviewImpl) SetActiveView(ctx *state.Context, id string, activeViewId string, limit int, offset int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.NewStateCtx(ctx)
	tb, err := getDataview(s, id)
	if err != nil {
		return err
	}

	for _, view := range tb.Model().GetDataview().Views {
		if view.Id == activeViewId {
			if d.activeView != activeViewId {
				// reset state in case view have changed
				d.entries = []database.Entry{}
			}
			d.activeView = activeViewId
			msgs, err := d.getEventsMessages(id, tb.Model().GetDataview().DatabaseId, view, offset, limit)
			if err != nil {
				return err
			}
			ctx.SetMessages(d.SmartBlock.Id(), msgs)
			return nil
		}
	}

	return fmt.Errorf("view not found")
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

func (d *dataviewImpl) CreateView(ctx *state.Context, id string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	view.Id = uuid.New().String()
	s := d.NewStateCtx(ctx)
	tb, err := getDataview(s, id)
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

func (d *dataviewImpl) SmartblockOpened() {
	d.activeView = ""
	d.entries = []database.Entry{}
}

func getDataview(s *state.State, id string) (dataview.Block, error) {
	b := s.Get(id)
	if b == nil {
		return nil, smartblock.ErrSimpleBlockNotFound
	}
	if tb, ok := b.(dataview.Block); ok {
		return tb, nil
	}
	return nil, fmt.Errorf("block '%s' not a dataview block", id)
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

func (d *dataviewImpl) getEventsMessages(dataviewId string, databaseId string, activeView *model.BlockContentDataviewView, offset int, limit int) ([]*pb.EventMessage, error) {
	db, err := d.Anytype().DatabaseByID(databaseId)
	if err != nil {
		return nil, err
	}

	var msgs []*pb.EventMessage

	entries, err := db.Query(database.Query{
		Filters: activeView.Filters,
		Sorts:   activeView.Sorts,
		Limit:   limit,
		Offset:  offset,
	})

	log.Debugf("db query for %s {filters: %+v, sorts: %+v, limit: %d, offset: %d} got %d entries", databaseId, activeView.Filters, activeView.Sorts, limit, offset, len(entries))

	updated, removed, insertedGroupedByPosition := calculateEntriesDiff(d.entries, entries)

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
				Id:             dataviewId,
				ViewId:         d.activeView,
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
					Id:             d.Id(),
					ViewId:         d.activeView,
					Updated:        nil,
					Removed:        nil,
					Inserted:       insertedPortion.entries,
					InsertPosition: int32(insertedPortion.position),
				},
			}})
		}
	}

	d.entries = entries

	return msgs, nil
}
