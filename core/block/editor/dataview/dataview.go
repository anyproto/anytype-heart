package dataview

import (
	"fmt"
	"sync"

	blockDB "github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
)

const defaultLimit = 100

var log = logging.Logger("anytype-mw-editor")

type Dataview interface {
	GetObjectTypeURL(ctx *state.Context, blockId string) (string, error)
	UpdateView(ctx *state.Context, blockId string, viewId string, view model.BlockContentDataviewView, showEvent bool) error
	DeleteView(ctx *state.Context, blockId string, viewId string, showEvent bool) error
	SetActiveView(ctx *state.Context, blockId string, activeViewId string, limit int, offset int) error
	CreateView(ctx *state.Context, blockId string, view model.BlockContentDataviewView) (*model.BlockContentDataviewView, error)

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
	database.Database
}

type ObjectTypeGetter interface {
	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
}

type dataviewCollectionImpl struct {
	smartblock.SmartBlock
	ObjectTypeGetter
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

func (d *dataviewCollectionImpl) GetObjectTypeURL(ctx *state.Context, blockId string) (string, error) {
	s := d.NewStateCtx(ctx)
	tb, err := getDataviewBlock(s, blockId)
	if err != nil {
		return "", err
	}

	if v, ok := tb.Model().Content.(*model.BlockContentOfDataview); !ok {
		return "", fmt.Errorf("wrong dataview block content type: %T", tb.Model().Content)
	} else {
		return v.Dataview.SchemaURL, nil
	}
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
		// todo: fill with required relations
		/*sch, err := schema.Get(tb.Model().GetDataview().SchemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema %s for dataview: %s", tb.Model().GetDataview().SchemaURL, err.Error())
		}

		view.Relations = getDefaultRelations(sch)*/
	}

	if len(view.Sorts) == 0 {
		// todo: set depends on the view type
		view.Sorts = []*model.BlockContentDataviewSort{{
			RelationKey: "id",
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
	var source string
	if dvBlock, ok := d.Pick(blockId).(dataview.Block); !ok {
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

	created, err := db.Create(database.Record{Details: rec.Details})
	if err != nil {
		return nil, err
	}

	return &model.ObjectDetails{Details: created.Details}, nil
}

func (d *dataviewCollectionImpl) UpdateRecord(_ *state.Context, blockId string, recID string, rec model.ObjectDetails) error {
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

	return db.Update(recID, database.Record{Details: rec.Details})
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
	source := dvBlock.Model().GetDataview().Source
	if source == "" && dvBlock.Model().GetDataview().SchemaURL == "pages" {
		// compatibility with older versions
		source = "https://anytype.io/schemas/object/bundled/page"
	}
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
	sch := schema.New(objectType)
	entries, total, err := db.Query(&sch, database.Query{
		Relations: activeView.Relations,
		Filters:   activeView.Filters,
		Sorts:     activeView.Sorts,
		Limit:     dv.limit,
		Offset:    dv.offset,
	})
	if err != nil {
		return nil, err
	}

	var currentEntriesIds []string
	for _, entry := range dv.records {
		currentEntriesIds = append(currentEntriesIds, getEntryID(entry))
	}

	var records []*types.Struct
	for _, entry := range entries {
		records = append(records, entry.Details)
	}

	var msgs = []*pb.EventMessage{
		{Value: &pb.EventMessageValueOfBlockSetDataviewRecords{
			BlockSetDataviewRecords: &pb.EventBlockSetDataviewRecords{
				Id:             dv.blockId,
				ViewId:         activeView.Id,
				Updated:        nil,
				Removed:        currentEntriesIds,
				Inserted:       records,
				InsertPosition: 0,
				Total:          uint32(total),
			},
		}},
	}

	log.Debugf("db query for %s {filters: %+v, sorts: %+v, limit: %d, offset: %d} got %d records, total: %d, msgs: %d", source, activeView.Filters, activeView.Sorts, dv.limit, dv.offset, len(entries), total, len(msgs))
	dv.records = entries

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
