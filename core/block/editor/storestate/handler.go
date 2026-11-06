package storestate

import (
	"context"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-store/v2/query"

	"github.com/anyproto/anytype-heart/pb"
)

type ChangeOp struct {
	Change Change
	State  *StoreState
	Value  *anyenc.Value
	Arena  *anyenc.Arena
}

type DeleteMode uint

const (
	DeleteModeDelete DeleteMode = iota
	DeleteModeMark
	// DeleteModeKeep: the handler already rewrote the document inside the apply transaction
	// (for example into a tombstone); the document stays as the handler left it.
	DeleteModeKeep
)

type ModifyMode uint

const (
	ModifyModeUpdate ModifyMode = iota
	ModifyModeUpsert
)

type Handler interface {
	CollectionName() string
	Init(ctx context.Context, s *StoreState) (err error)
	BeforeCreate(ctx context.Context, ch ChangeOp) (err error)
	BeforeModify(ctx context.Context, ch ChangeOp) (mode ModifyMode, err error)
	BeforeDelete(ctx context.Context, ch ChangeOp) (mode DeleteMode, err error)
	UpgradeKeyModifier(ch ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier
}

// KeyedHandler is a Handler whose collection is not keyed by the document id: its primary key
// (anystore.CollectionOptions.PrimaryKey) is another top-level string field of the document. A
// change still names its document by id, so the store state asks the handler for the key: it
// writes the key into the document a create makes, and resolves the id a modify or a delete
// names to the stored document's key.
//
// A keyed collection is modified in ModifyModeUpdate only: an upsert cannot know the key of a
// document that does not exist yet.
type KeyedHandler interface {
	Handler
	// CollectionOptions are the options the collection is opened with. They name the primary key.
	CollectionOptions() anystore.CollectionOptions
	// NewDocumentKey returns the key of the document a create with this id and order makes.
	// BeforeCreate must refuse (ErrIgnore) a create replayed over its stored document: the insert
	// of a key another document holds fails critically.
	NewDocumentKey(documentId, order string) string
	// DocumentKey returns the key of the stored document with this id, or an error wrapping
	// anystore.ErrDocNotFound when there is none.
	DocumentKey(ctx context.Context, coll anystore.Collection, documentId string) (string, error)
}

type DefaultHandler struct {
	Name       string
	ModifyMode ModifyMode
	DeleteMode DeleteMode
}

func (d DefaultHandler) CollectionName() string {
	return d.Name
}

func (d DefaultHandler) Init(ctx context.Context, s *StoreState) (err error) {
	_, err = s.Collection(ctx, d.Name)
	return
}

func (d DefaultHandler) BeforeCreate(ctx context.Context, ch ChangeOp) (err error) {
	return
}

func (d DefaultHandler) BeforeModify(ctx context.Context, ch ChangeOp) (mode ModifyMode, err error) {
	return d.ModifyMode, nil
}

func (d DefaultHandler) BeforeDelete(ctx context.Context, ch ChangeOp) (mode DeleteMode, err error) {
	return d.DeleteMode, nil
}

func (d DefaultHandler) UpgradeKeyModifier(ch ChangeOp, key *pb.KeyModify, mod query.Modifier) query.Modifier {
	return mod
}
