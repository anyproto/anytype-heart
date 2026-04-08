package personalfavorites

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type changeType int

const (
	changeCreate changeType = iota
	changeModify
	changeDelete
)

type pendingChange struct {
	typ   changeType
	entry WidgetEntry
}

// favoritesHandler implements storestate.Handler and accumulates change notifications.
// pending is synchronized because BeforeCreate/Modify/Delete can be called from a
// remote-update goroutine (via store.update → storestate tx) concurrently with
// FlushPendingChanges running on a local-push goroutine after its tx commit.
// After the transaction commits, FlushPendingChanges is called (from onUpdate) to dispatch them to observers.
type favoritesHandler struct {
	state *storestate.StoreState

	mu      sync.Mutex
	pending []pendingChange
}

func (h *favoritesHandler) CollectionName() string {
	return collectionName
}

func (h *favoritesHandler) Init(ctx context.Context, s *storestate.StoreState) error {
	h.state = s
	if _, err := s.Collection(ctx, collectionName); err != nil {
		return fmt.Errorf("init collection %s: %w", collectionName, err)
	}
	return nil
}

func (h *favoritesHandler) BeforeCreate(ctx context.Context, ch storestate.ChangeOp) error {
	entry := entryFromAnyencValue(ch.Value)
	h.mu.Lock()
	h.pending = append(h.pending, pendingChange{typ: changeCreate, entry: entry})
	h.mu.Unlock()
	return nil
}

func (h *favoritesHandler) BeforeModify(ctx context.Context, ch storestate.ChangeOp) (storestate.ModifyMode, error) {
	docId := ch.Change.Change.GetModify().GetDocumentId()
	entry := WidgetEntry{Id: docId}
	entry.SpaceId = h.lookupSpaceId(ctx, docId)
	h.mu.Lock()
	h.pending = append(h.pending, pendingChange{typ: changeModify, entry: entry})
	h.mu.Unlock()
	return storestate.ModifyModeUpsert, nil
}

func (h *favoritesHandler) BeforeDelete(ctx context.Context, ch storestate.ChangeOp) (storestate.DeleteMode, error) {
	docId := ch.Change.Change.GetDelete().GetDocumentId()
	entry := WidgetEntry{Id: docId}
	entry.SpaceId = h.lookupSpaceId(ctx, docId)
	h.mu.Lock()
	h.pending = append(h.pending, pendingChange{typ: changeDelete, entry: entry})
	h.mu.Unlock()
	return storestate.DeleteModeDelete, nil
}

func (h *favoritesHandler) UpgradeKeyModifier(ch storestate.ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return mod
}

// lookupSpaceId resolves the spaceId of an existing doc. Called from Before*
// callbacks inside a storestate tx. Returns empty string on any failure — the
// observer dispatcher will then fan out to all registered observers, which is
// the previous (less efficient but correct) behavior.
func (h *favoritesHandler) lookupSpaceId(ctx context.Context, docId string) string {
	if h.state == nil {
		return ""
	}
	coll, err := h.state.Collection(ctx, collectionName)
	if err != nil {
		return ""
	}
	doc, err := coll.FindId(ctx, docId)
	if err != nil {
		return ""
	}
	return doc.Value().GetString("spaceId")
}

// FlushPendingChanges returns accumulated changes and clears the buffer.
func (h *favoritesHandler) FlushPendingChanges() []pendingChange {
	h.mu.Lock()
	changes := h.pending
	h.pending = nil
	h.mu.Unlock()
	return changes
}

func entryFromAnyencValue(v *anyenc.Value) WidgetEntry {
	if v == nil {
		return WidgetEntry{}
	}
	return WidgetEntry{
		Id:       v.GetString("id"),
		SpaceId:  v.GetString("spaceId"),
		TargetId: v.GetString("targetId"),
		Layout:   model.BlockContentWidgetLayout(v.GetInt("layout")),
		Limit:    int32(v.GetInt("limit")),
		ViewId:   v.GetString("viewId"),
		AfterId:  v.GetString("afterId"),
	}
}
