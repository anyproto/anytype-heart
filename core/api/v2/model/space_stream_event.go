package v2model

// space_stream_event.go is the wire contract of the spaces stream
// (GET /v2/spaces/stream): one discriminated envelope per Server-Sent Event.
// It is the SPACES stream — the set of spaces a key can see — not the
// per-space chat stream (space_chat_event.go).

// The spaces stream's event types. snapshot_complete is the space chat
// stream's name for the same moment.
const (
	SpaceStreamEventSpaceAdded        = "space_added"
	SpaceStreamEventSnapshotComplete  = SpaceChatEventSnapshotComplete
	SpaceStreamEventSpaceUpdated      = "space_updated"
	SpaceStreamEventSpaceRemoved      = "space_removed"
	SpaceStreamEventNotGrantedUpdated = "not_granted_updated"
	SpaceStreamEventStreamClosed      = "stream_closed"
)

// Why a space left the visible set (space_removed.reason).
const (
	// SpaceRemovedAccessRemoved: the key's grant no longer covers the space.
	// It wins over the other two, so a space the key lost never reveals its
	// later state.
	SpaceRemovedAccessRemoved = "access_removed"
	// SpaceRemovedGone: this account no longer holds the space — deleted,
	// left, removed from it, or a join that was declined.
	SpaceRemovedGone = "gone"
	// SpaceRemovedUnavailable: the account still holds the space but it is
	// not usable on this device — its load failed, its deletion is pending on
	// the network, or the account is still joining it. It can come back.
	SpaceRemovedUnavailable = "unavailable"
)

// Why the server ended the stream (stream_closed.reason): what the next
// request with this key would be refused for.
const (
	StreamClosedKeyRevoked = "key_revoked"
	StreamClosedKeyExpired = "key_expired"
)

// SpaceStreamEvent is one event of the spaces stream. Type is also the SSE
// event name; members an event does not use are absent.
type SpaceStreamEvent struct {
	Type string `json:"type"`
	// Space is the GET /v2/spaces row, always with the full id, on
	// space_added and space_updated.
	Space *SpaceRow `json:"space,omitempty"`
	// SpaceId is the full id of the space that left, on space_removed.
	SpaceId string `json:"space_id,omitempty"`
	// Reason is set on space_removed and stream_closed.
	Reason string `json:"reason,omitempty"`
	// HasNotGrantedSpaces is present, true or false, on snapshot_complete and
	// not_granted_updated: whether the key's grant hides other live spaces.
	HasNotGrantedSpaces *bool `json:"has_not_granted_spaces,omitempty"`
}
