package v2service

// chat_stream.go is the subscribe half of the chat surface: the initial
// window a stream opens with, and what a RECONNECTING stream owes a client
// that tells it where it left off.

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

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
	// maxConcurrentChatStreams caps open streams process-wide. Each one
	// pins a per-subscription clone of its opening window (up to
	// maxChatStreamLimit messages), a sink of chatStreamSinkBuffer events,
	// and goroutines in both this process and net/http — roughly a megabyte
	// at the top of the range, shared with nobody, since two subscriptions
	// on one chat clone the window twice.
	//
	// Nothing else bounds them: the shared write limiter is attached to
	// mutations only, and it is keyed on RemoteAddr, which is always
	// loopback here. Without a cap one key holds as many as it likes.
	maxConcurrentChatStreams = 64
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
	// Cursor is the HIGHEST state id in the opening window. The window is
	// ordered by order id and the cursor is a state id, and those are not
	// the same order: a message that arrived late over sync carries a fresh
	// state id at an old order-id position, so the last row emitted is not
	// necessarily the newest state. Emitting the maximum keeps the client's
	// Last-Event-ID from going backwards on the very first reconnect.
	Cursor string
	Events <-chan *pb.Event

	sub       apicore.ChatSubscriptionService
	chatId    string
	subId     string
	release   func()
	closeOnce sync.Once
}

// Close releases the subscription and the stream slot. A leaked subscription
// keeps a sliding window alive for a client that is already gone.
//
// The error is logged, not discarded: Unsubscribe resolves the space first
// and returns EARLY without removing the subscription when that fails, so a
// dropped error is precisely the leak signal. Nothing useful remains to do
// about it here, but it must not be silent.
func (s *ChatStream) Close() {
	s.closeOnce.Do(func() {
		if s.sub != nil {
			if err := s.sub.Unsubscribe(s.chatId, s.subId); err != nil {
				log.Warn("v2 chat stream: unsubscribe failed, subscription may be retained",
					zap.String("chatId", s.chatId), zap.String("subId", s.subId), zap.Error(err))
			}
		}
		if s.release != nil {
			s.release()
		}
	})
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
	limit := q.Limit
	if limit <= 0 {
		limit = defaultChatStreamLimit
	}
	if limit > maxChatStreamLimit {
		limit = maxChatStreamLimit
	}
	// the slot is taken BEFORE subscribing, so a refusal costs nothing, and
	// released through Close's sync.Once, so it cannot be double-returned
	release, ok := s.chatStreams.acquire()
	if !ok {
		return nil, v2model.NewError(http.StatusTooManyRequests, v2model.CodeRateLimitExceeded,
			fmt.Sprintf("too many open chat streams (%d); close one before opening another", maxConcurrentChatStreams))
	}

	sink := make(chan *pb.Event, chatStreamSinkBuffer)
	subId := "v2-sse-" + uuid.New().String()
	window, err := s.chatSub.SubscribeLastMessages(ctx, chatId, limit, subId, sink)
	if err != nil {
		release()
		return nil, fmt.Errorf("subscribe last messages: %w", err)
	}

	replay, resync := replayFromState(window, q.LastEventId, limit)
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
		Cursor:  maxStateId(window),
		Resync:  resync,
		Events:  sink,
		sub:     s.chatSub,
		chatId:  chatId,
		subId:   subId,
		release: release,
	}, nil
}

// maxStateId is the highest state id in a window. See ChatStream.Cursor for
// why the last row is not it.
func maxStateId(window []*model.ChatMessage) string {
	var max string
	for _, msg := range window {
		if msg.StateId > max {
			max = msg.StateId
		}
	}
	return max
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
func replayFromState(window []*model.ChatMessage, lastEventId string, limit int) (replay []*model.ChatMessage, resync bool) {
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
	// Every row is newer than the cursor, so the cursor names nothing still
	// held. That is only a GAP when the window is a truncated tail: a short
	// window (fewer rows than asked for) is the whole chat, so there is
	// nothing between the cursor and the oldest row for the client to have
	// missed, and answering resync there would fire on every reconnect to a
	// small chat.
	if len(replay) == len(window) && len(window) >= limit {
		return window, true
	}
	return replay, false
}

// chatStreamSlots is the process-wide open-stream cap. A counting semaphore
// rather than a rate limiter: what has to be bounded is how many streams are
// held AT ONCE, not how fast they are opened.
type chatStreamSlots struct {
	mu   sync.Mutex
	open int
}

// acquire takes a slot, returning the release func and whether one was free.
func (c *chatStreamSlots) acquire() (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open >= maxConcurrentChatStreams {
		return nil, false
	}
	c.open++
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.open--
		})
	}, true
}
