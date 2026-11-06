package chatmodel

import (
	"strings"

	anystore "github.com/anyproto/any-store/v2"
)

// The layout of a chat's messages collection in the per-space CRDT database. The chat repository
// opens the collection with it and the v1 to v2 database migration builds it, so both live here.

// messagesCollectionSuffix ends the name of a chat's messages collection: <chatObjectId>chats.
const messagesCollectionSuffix = "chats"

// MessagesCollectionName is the name of the messages collection of the chat chatObjectId.
func MessagesCollectionName(chatObjectId string) string {
	return chatObjectId + messagesCollectionSuffix
}

// IsMessagesCollection reports whether a CRDT database collection named name holds the messages of
// a chat. The other collections of a chat (<chatObjectId>chatMessageHistory, <chatObjectId>editor)
// and of the other store objects do not end with the suffix.
func IsMessagesCollection(name string) bool {
	return len(name) > len(messagesCollectionSuffix) && strings.HasSuffix(name, messagesCollectionSuffix)
}

// MessagesCollectionOptions are the options a chat messages collection is created with: it is
// keyed by the message storage key (StorageKey), not by the message id.
func MessagesCollectionOptions() anystore.CollectionOptions {
	return anystore.CollectionOptions{PrimaryKey: StorageKeyField}
}

// MessagesCollectionIndexes are the indexes of a chat messages collection, named as
// anystorehelper.AddIndexes names them (the fields joined by commas).
//
// The sparse ones are only usable by a filter that guarantees the field is present and non-null:
// since any-store v1.0.2 the planner refuses a sparse index for anything weaker, $exists included
// (it also matches an explicit null, which the index skips).
//
// There is no index on the order id: the primary key (the storage key) is order-prefixed, so every
// ordered walk and every order-id range is a walk of the collection itself.
func MessagesCollectionIndexes() []anystore.IndexInfo {
	return []anystore.IndexInfo{
		// messages are named by id in changes and in the API. Unique, so that two documents never
		// hold the same message.
		{Name: "id", Fields: []string{"id"}, Unique: true},
		{Name: PinnedKey, Fields: []string{PinnedKey}, Sparse: true},
		{Name: ReactionUnreadOrderIdKey, Fields: []string{ReactionUnreadOrderIdKey}, Sparse: true},
		// serves the newest messages of a creator: creator Eq + a walk in storage-key (order) order
		// with early exit. The trailing field is the storage key, so the walk has the order the
		// query sorts by and needs no sort.
		{Name: CreatorKey + "," + StorageKeyField, Fields: []string{CreatorKey, StorageKeyField}},
		// only tombstones carry the field: counting them reads just them
		{Name: DeletedKey, Fields: []string{DeletedKey}, Sparse: true},
	}
}
