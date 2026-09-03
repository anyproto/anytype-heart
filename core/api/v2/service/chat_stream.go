package v2service

// chat_stream.go is the subscribe half of the chat surface: the initial
// window a stream opens with, and what a RECONNECTING stream owes a client
// that tells it where it left off.

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// C10 bounds for the stream's opening window, the same shape the paginated
// chat read uses. v1 parsed this with a bare Atoi and passed it through, so
// the document's 1..1000 was decoration.
const (
	defaultChatStreamLimit = 50
	maxChatStreamLimit     = 1000
	chatStreamSinkBuffer   = 256
)

// ChatStreamQuery is the stream's request surface: the opening window size
// and the cursor a RECONNECTING client sends back (Last-Event-ID).
type ChatStreamQuery struct {
	Limit       int
	LastEventId string
}

// ChatStream is one open subscription. Events arrive on Events; Close
// releases the subscription and is safe to call more than once, because the
// handler defers it while the transport may also abandon the stream.
type ChatStream struct {
	// Initial is the opening window, already narrowed to what a resuming
	// client has not seen.
	Initial []v2model.ChatEvent
	// Resync reports that the window could not be proven to cover the
	// client's gap; it rides the first event so the client reconciles
	// instead of assuming continuity.
	Resync bool
	Events <-chan *pb.Event

	sub       apicore.ChatSubscriptionService
	chatId    string
	subId     string
	closeOnce sync.Once
}

// Close releases the subscription. A leaked one keeps a sliding window alive
// for a client that is already gone.
func (s *ChatStream) Close() {
	s.closeOnce.Do(func() {
		if s.sub != nil {
			_ = s.sub.Unsubscribe(s.chatId, s.subId)
		}
	})
}

// SetChatSubscription installs the chat subscription dependency. It is a
// setter rather than a ninth constructor parameter: the eight positional
// arguments NewService already takes are exactly the hazard a reviewer
// flagged on UploadFile, and the stream route is skipped entirely when the
// dependency is absent, the way RouteDeps already gates create and edit.
func (s *Service) SetChatSubscription(sub apicore.ChatSubscriptionService) {
	s.chatSub = sub
}

// ChatStreamMessageOptions is the render vocabulary for LIVE events, which
// arrive one at a time after the opening window. It exists so the transport
// resolves participant names once per connection instead of per event.
func (s *Service) ChatStreamMessageOptions(ctx context.Context, spaceId string) v2model.ChatMessageOptions {
	return v2model.ChatMessageOptions{
		SpaceId:         spaceId,
		ParticipantName: s.participantNameLookup(spaceId),
	}
}

// OpenChatStream is the scoped entry point v1 never had. Its first statement
// resolves the chat WITHIN the space, so a chat id from another space is a
// 404 and a key whose grant excludes the space is refused before anything
// subscribes. v1's handler took space_id only to enrich author names and
// subscribed to whatever chat id it was handed.
func (s *Service) OpenChatStream(ctx context.Context, spaceId, chatId string, q ChatStreamQuery) (*ChatStream, error) {
	if err := s.ensureChat(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	if s.chatSub == nil {
		return nil, v2model.NewError(501, v2model.CodeNotImplemented,
			"chat streaming is unavailable in this build")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultChatStreamLimit
	}
	if limit > maxChatStreamLimit {
		limit = maxChatStreamLimit
	}

	sink := make(chan *pb.Event, chatStreamSinkBuffer)
	subId := "v2-sse-" + uuid.New().String()
	window, err := s.chatSub.SubscribeLastMessages(ctx, chatId, limit, subId, sink)
	if err != nil {
		return nil, fmt.Errorf("subscribe last messages: %w", err)
	}

	replay, resync := replayFromState(window, q.LastEventId)
	opts := s.ChatStreamMessageOptions(ctx, spaceId)
	initial := make([]v2model.ChatEvent, 0, len(replay))
	for _, msg := range replay {
		message := v2model.ChatMessageFromProto(msg, opts)
		initial = append(initial, v2model.ChatEvent{
			Id:      msg.StateId,
			Type:    v2model.ChatEventMessageAdded,
			Message: &message,
		})
	}
	return &ChatStream{
		Initial: initial,
		Resync:  resync,
		Events:  sink,
		sub:     s.chatSub,
		chatId:  chatId,
		subId:   subId,
	}, nil
}

// replayFromState decides what a reconnecting stream replays. The
// subscription hands back a sliding window of the newest messages; a client's
// Last-Event-ID is a chat state id, which every message carries and which
// sorts, so everything above it is what the client missed.
//
// resync reports that the window CANNOT prove it covers the gap: the cursor
// predates the oldest row still held, so messages may have passed through and
// been evicted while the client was away. Saying so is the whole point — a
// partial window delivered silently looks exactly like a complete catch-up,
// and the client would carry the hole forever.
//
// Deletions are never replayable at all: a delete removes the row, so nothing
// survives to carry a state id. That gap is documented on the route rather
// than papered over here.
func replayFromState(window []*model.ChatMessage, lastEventId string) (replay []*model.ChatMessage, resync bool) {
	if lastEventId == "" {
		return window, false // a fresh connection has no history to reconcile
	}
	if len(window) == 0 {
		// the chat holds nothing now; whether the client's copy is stale is
		// unknowable from here
		return nil, true
	}
	replay = make([]*model.ChatMessage, 0, len(window))
	for _, msg := range window {
		// a row with no state id predates the cursor scheme and cannot be
		// positioned against it; replay it and flag the window unprovable
		if msg.StateId == "" {
			resync = true
			replay = append(replay, msg)
			continue
		}
		if msg.StateId > lastEventId {
			replay = append(replay, msg)
		}
	}
	if resync {
		return window, true
	}
	// the cursor predates everything still held: the gap is unrecoverable
	if len(replay) == len(window) {
		return window, true
	}
	return replay, false
}
