package apicore

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

// The live object search port (ObjectSearchService.OpenObjectSearch): one
// unbounded live query over one space, delivered as an opening snapshot and
// then every change of the matching set. It backs the API v2 search stream.

// ObjectSearchOpen is one live search over one space.
type ObjectSearchOpen struct {
	SpaceId string
	Filters []database.FilterRequest
	// Sorts order the opening snapshot. Later changes carry no position.
	Sorts []database.SortRequest
	// Keys are the details a change carries. The search always carries id,
	// name, type and discussionId besides them, so a change to an object's
	// discussion is reported as an update.
	Keys []string
}

// ObjectSearchChangeType names what an ObjectSearchChange reports.
type ObjectSearchChangeType uint8

const (
	// ObjectSearchAdded reports a matching object: in the opening snapshot,
	// or when an object enters the set later.
	ObjectSearchAdded ObjectSearchChangeType = iota + 1
	// ObjectSearchSnapshotComplete closes the opening snapshot.
	ObjectSearchSnapshotComplete
	// ObjectSearchUpdated reports a carried detail of a member that changed.
	ObjectSearchUpdated
	// ObjectSearchRemoved reports an object that left the set; it may still
	// exist.
	ObjectSearchRemoved
)

// ObjectSearchChange is one change of the matching set.
type ObjectSearchChange struct {
	Type ObjectSearchChangeType
	// Id is the object's id; empty on ObjectSearchSnapshotComplete.
	Id string
	// Details are the object's carried details after the change, on added
	// and updated. They are never mutated once delivered.
	Details *domain.Details
}

// ObjectSearchSubscription is one open live search. It never closes on its
// own: a client that reads slowly only costs memory.
type ObjectSearchSubscription interface {
	// Snapshot is the opening snapshot: an added change per matching object,
	// in the sort order, then the snapshot complete change. Live changes
	// queued meanwhile follow it.
	Snapshot() []ObjectSearchChange
	// Ready is signalled whenever live changes are queued.
	Ready() <-chan struct{}
	// Drain returns and removes every queued live change, oldest first.
	Drain() []ObjectSearchChange
	// Close ends the search. It is safe to call more than once.
	Close()
}
