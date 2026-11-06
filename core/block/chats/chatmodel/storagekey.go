package chatmodel

import (
	"github.com/anyproto/any-store/v2/query"
)

// StorageKeyField is the top-level field holding a chat message document's storage key, the
// primary key of a chat messages collection. Every document carries it; it is written once, when
// the message is created, and never changes. The message id stays in "id", under a unique index.
const StorageKeyField = "_k"

const (
	// storageKeySeparator ends the order-id part of a storage key. It must sort below every
	// character an order id can hold, so that a key orders exactly as (order id, id tail) and a
	// shorter order id that is a prefix of a longer one sorts first. Order ids are lexids over
	// lexid.CharsAllNoEscape (any-sync's tree order and storestate.LexId alike), the printable
	// ASCII range, whose lowest character is '!' (0x21).
	//
	// It is 0x01 rather than 0x00: any-store terminates strings with 0x00 and stores a 0x00
	// inside a string as an escape pair, which makes prefix bounds on such values a special case
	// in the planner; 0x01 is stored as it is.
	storageKeySeparator = "\x01"
	// storageKeyAboveSeparator is the byte right after the separator. A bound of
	// orderId+storageKeyAboveSeparator is above every key of that order id (orderId+separator+…)
	// and below every key of a greater order id, whose next character is at least '!'.
	storageKeyAboveSeparator = "\x02"
	// storageKeyIdTailLength is how much of the message id the key keeps after the order id. The
	// order id alone is not unique: changes made concurrently on the same parent get the same
	// one. A message id is a change CID; its tail is hash bits, enough to tell such messages
	// apart. A tail collision is refused at create time, never overwritten.
	storageKeyIdTailLength = 12
)

// StorageKey returns the storage key of the message with the given order id and message id:
// the order id, the separator and the last 12 characters of the message id (the whole id when
// it is shorter). It is the one place the key is built.
//
// A document without an order id (a legacy stub that an old reaction upsert created) gets an
// empty order part, so it sorts before every message.
//
// Keys are local: they never leave the device and never appear in changes, so the layout can
// change with a migration of the local store alone.
func StorageKey(orderId, messageId string) string {
	tail := messageId
	if len(tail) > storageKeyIdTailLength {
		tail = tail[len(tail)-storageKeyIdTailLength:]
	}
	return orderId + storageKeySeparator + tail
}

// The filters below express an order-id comparison on the storage key, so that a range of
// messages is a range of the primary key: the collection walks it in its natural order, with no
// secondary index and no sort. Every key is orderId+separator+tail and the separator sorts below
// every order-id character, so for any order id X:
//
//	_o.id >= X  <=>  key >= X
//	_o.id >  X  <=>  key >= X+aboveSeparator
//	_o.id <  X  <=>  key <  X
//	_o.id <= X  <=>  key <  X+aboveSeparator
//
// A legacy stub without an order id has an empty order part: it compares as the empty order id.

// OrderIdAtLeast matches messages whose order id is >= orderId.
func OrderIdAtLeast(orderId string) query.Filter {
	return storageKeyComp(query.CompOpGte, orderId)
}

// OrderIdAfter matches messages whose order id is > orderId.
func OrderIdAfter(orderId string) query.Filter {
	return storageKeyComp(query.CompOpGte, orderId+storageKeyAboveSeparator)
}

// OrderIdBefore matches messages whose order id is < orderId.
func OrderIdBefore(orderId string) query.Filter {
	return storageKeyComp(query.CompOpLt, orderId)
}

// OrderIdAtMost matches messages whose order id is <= orderId.
func OrderIdAtMost(orderId string) query.Filter {
	return storageKeyComp(query.CompOpLt, orderId+storageKeyAboveSeparator)
}

func storageKeyComp(op query.CompOp, bound string) query.Filter {
	return query.Key{Path: []string{StorageKeyField}, Filter: query.NewComp(op, bound)}
}

const (
	// SortStorageKeyAsc walks messages from the oldest: the collection's natural order.
	SortStorageKeyAsc = StorageKeyField
	// SortStorageKeyDesc walks messages from the newest: the natural order, reversed.
	SortStorageKeyDesc = "-" + StorageKeyField
)
