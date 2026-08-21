package personalfavorites

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// WidgetEntry is a single personal widgets entry stored in the tech-space
// CRDT store. Ordering is encoded in AfterId: the entry whose AfterId is
// "" is the head, and the chain is walked via AfterId links.
type WidgetEntry struct {
	Id       string
	SpaceId  string
	TargetId string
	Layout   model.BlockContentWidgetLayout
	Limit    int32
	ViewId   string
	AfterId  string
}

// WidgetUpdate is a partial WidgetEntry update. Nil pointers are left
// untouched. TargetId is immutable on a given entry — changing the widget
// target is modeled as delete + create by the editor.
type WidgetUpdate struct {
	Layout  *model.BlockContentWidgetLayout
	Limit   *int32
	ViewId  *string
	AfterId *string
}

// Observer receives per-entry change notifications for a single space.
// The per-space subscription is set up via Service.Subscribe.
type Observer interface {
	OnWidgetCreate(entry WidgetEntry)
	OnWidgetUpdate(entry WidgetEntry)
	OnWidgetDelete(wrapperId string)
}

type SubscribeParams struct {
	SpaceId  string
	Observer Observer
}

// ChangeType enumerates the kinds of CRDT changes the handler records for
// later dispatch. Kept as an int enum so it can be passed through
// dispatch batches without allocating.
type ChangeType int

const (
	ChangeCreate ChangeType = iota
	ChangeModify
	ChangeDelete
)

// WidgetChange is one accumulated store change emitted by the editor
// handler and consumed by Service.OnStoreUpdate.
type WidgetChange struct {
	Type  ChangeType
	Entry WidgetEntry
}

// StoreObject is the per-account CRDT store of personal favorites entries,
// implemented by the editor package on the SmartBlockTypeTechSpaceObject
// smartblock. The service uses this interface to invoke the store under
// its smartblock lock via techspace.DoPersonalFavoritesStore.
type StoreObject interface {
	smartblock.SmartBlock
	anystoredebug.AnystoreDebug

	CreateWidget(ctx context.Context, entry WidgetEntry) error
	DeleteWidget(ctx context.Context, id string) error
	UpdateWidget(ctx context.Context, id string, updates WidgetUpdate) error
	GetWidgets(ctx context.Context, spaceId string) ([]WidgetEntry, error)
	GetWidget(ctx context.Context, id string) (WidgetEntry, error)
}
