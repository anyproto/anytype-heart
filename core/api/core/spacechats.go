package apicore

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The per-space chat hub port (ChatSubscriptionService.OpenSpaceChats). One
// hub per space watches every chat and discussion of the space and fans their
// changes out to the clients that opened it. It renders each change once, for
// all its clients, with the Render function the opening call supplies.

// SpaceChatOpen is one client's request to watch a space's chats.
type SpaceChatOpen struct {
	SpaceId string
	// IncludeDiscussions adds object discussions to what this client
	// receives; without it the client receives chats only.
	IncludeDiscussions bool
	// Render turns a change into the frame clients receive. The hub an open
	// creates keeps it for its lifetime and calls it once per change for all
	// of its clients, from more than one goroutine, so it must be safe for
	// concurrent use. An open that joins a running hub shares that hub's.
	Render SpaceChatRender
}

// SpaceChatRender renders one change into a wire frame.
type SpaceChatRender func(change SpaceChatChange) (SpaceChatFrame, error)

// SpaceChatChangeType names what a SpaceChatChange reports.
type SpaceChatChangeType uint8

const (
	// SpaceChatAdded announces a chat: in the opening snapshot, or when it
	// becomes eligible later.
	SpaceChatAdded SpaceChatChangeType = iota + 1
	// SpaceChatSnapshotComplete closes the opening snapshot.
	SpaceChatSnapshotComplete
	// SpaceChatUpdated reports a changed name or parent mapping.
	SpaceChatUpdated
	// SpaceChatRemoved reports a chat that stopped being eligible.
	SpaceChatRemoved
	SpaceChatStateUpdated
	SpaceChatMessageAdded
	SpaceChatMessageUpdated
	SpaceChatMessageDeleted
	SpaceChatReactionsUpdated
	SpaceChatPinnedUpdated
)

// SpaceChat is one chat of a space as the hub sees it. For a discussion, Name
// is its parent object's name and ParentId the parent's id.
type SpaceChat struct {
	Id         string
	Name       string
	Discussion bool
	ParentId   string
}

// SpaceChatChange is one change the hub delivers, before rendering.
type SpaceChatChange struct {
	Type    SpaceChatChangeType
	SpaceId string
	// Chat is the chat the change concerns; zero for SpaceChatSnapshotComplete.
	Chat SpaceChat
	// State is set on chat added, chat updated and state updated.
	State *model.ChatState
	// Message is set on message added and updated, reactions updated and
	// pinned updated. It is shared by every client: read it, never mutate it.
	Message *model.ChatMessage
	// MessageId is set on every message change.
	MessageId string
}

// SpaceChatFrame is one rendered event: the event name and its JSON body.
type SpaceChatFrame struct {
	Type string
	Data []byte
}

// SpaceChatSubscription is one client's place on a space's chat hub. The hub
// never closes it: a client that reads slowly only costs memory.
type SpaceChatSubscription interface {
	// Snapshot is the opening snapshot: a chat added frame per chat, then the
	// snapshot complete frame. Live frames queued meanwhile follow it.
	Snapshot() []SpaceChatFrame
	// Ready is signalled whenever live frames are queued.
	Ready() <-chan struct{}
	// Drain returns and removes every queued live frame, oldest first.
	Drain() []SpaceChatFrame
	// Close leaves the hub; the last client to leave tears the hub down. It
	// is safe to call more than once.
	Close()
}

// SpaceChatAttachError reports an open refused because an eligible chat of
// the space cannot be attached: its chat state manager failed to initialize.
type SpaceChatAttachError struct {
	ChatId string
	Err    error
}

func (e *SpaceChatAttachError) Error() string {
	return fmt.Sprintf("attach chat %s: %v", e.ChatId, e.Err)
}

func (e *SpaceChatAttachError) Unwrap() error {
	return e.Err
}
