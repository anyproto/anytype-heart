package chatsubscription

import (
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ChatChangeKind names what a ChatChange reports.
type ChatChangeKind uint8

const (
	ChatChangeMessageAdded ChatChangeKind = iota + 1
	ChatChangeMessageUpdated
	ChatChangeMessageDeleted
	ChatChangeReactionsUpdated
	ChatChangePinnedUpdated
	ChatChangeStateUpdated
)

// ChatChange is one change a manager applied, as handed to its observers.
// The payloads are deep copies made under the manager lock, because the window
// state keeps mutating the same pointers and observers convert later on other
// goroutines. Every observer of a change shares one copy: read it, never
// mutate it.
type ChatChange struct {
	Kind ChatChangeKind
	// MessageId is set for every kind but ChatChangeStateUpdated.
	MessageId string
	// Message is set for added, updated, reactions and pinned changes.
	Message *chatmodel.Message
	// State is set for ChatChangeStateUpdated.
	State *model.ChatState
}

// ChatObserver receives every change a manager applies as it happens: added,
// updated and deleted messages, reactions, pins and chat-state updates. It is
// called inside the mutation path, under the manager lock, whether or not any
// windowed subscription exists, and BEFORE the change is committed: a change
// that rolls back has still been observed.
//
// The contract is enqueue only: never block, never call back into the manager.
// A slow observer stalls the chat it observes.
type ChatObserver func(change ChatChange)

// AddObserver registers fn under id; a second call with the same id replaces
// it. Observers do not count towards IsActive, which decides object retention.
// Call under Lock.
func (s *subscriptionManager) AddObserver(id string, fn ChatObserver) {
	if s.observers == nil {
		s.observers = make(map[string]ChatObserver)
	}
	s.observers[id] = fn
}

// RemoveObserver unregisters the observer with this id; an unknown id is
// ignored. Call under Lock.
func (s *subscriptionManager) RemoveObserver(id string) {
	delete(s.observers, id)
}

func (s *subscriptionManager) notifyMessage(kind ChatChangeKind, message *chatmodel.Message) {
	if len(s.observers) == 0 {
		return
	}
	s.notify(ChatChange{Kind: kind, MessageId: message.Id, Message: message.Clone()})
}

func (s *subscriptionManager) notify(change ChatChange) {
	for _, fn := range s.observers {
		fn(change)
	}
}
