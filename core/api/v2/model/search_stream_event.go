package v2model

// search_stream_event.go is the wire contract of the search stream
// (POST /v2/spaces/{space_id}/search/stream): one discriminated envelope per
// Server-Sent Event, carrying space_id on every event.

// The search stream's event types. snapshot_complete is the space chat
// stream's name for the same moment.
const (
	SearchStreamEventObjectAdded      = "object_added"
	SearchStreamEventObjectUpdated    = "object_updated"
	SearchStreamEventObjectRemoved    = "object_removed"
	SearchStreamEventSnapshotComplete = SpaceChatEventSnapshotComplete
)

// SearchStreamEvent is one event of the search stream. Type is also the SSE
// event name; members an event does not use are absent.
type SearchStreamEvent struct {
	Type    string `json:"type"`
	SpaceId string `json:"space_id"`
	// Object is the search row, fields applied, on object_added and
	// object_updated.
	Object *ObjectRow `json:"object,omitempty"`
	// ObjectId is set on object_removed: the object left the matching set,
	// which does not mean it was deleted.
	ObjectId string `json:"object_id,omitempty"`
	// Warnings rides snapshot_complete: what the search request's own
	// validation warned about, as POST …/search returns it.
	Warnings []Issue `json:"warnings,omitempty"`
}
