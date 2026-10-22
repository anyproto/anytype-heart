package v2service

// chat_space_stream.go is the service half of the space-wide chat stream: it
// admits a client (space and grant first, then the stream cap) onto the
// space's chat hub (apicore.ChatSubscriptionService.OpenSpaceChats) and
// renders the hub's changes into the v2 envelope. The hub renders each change
// once for all its clients, with the renderer of the open that created it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// maxConcurrentSpaceChatStreams caps open space chat streams process-wide,
// independently of maxConcurrentChatStreams. It bounds concurrent watches,
// not retained chat state managers.
const maxConcurrentSpaceChatStreams = 16

// SpaceChatStreamQuery is the stream's request surface.
type SpaceChatStreamQuery struct {
	// IncludeDiscussions adds object discussions; without it the stream
	// carries chats only.
	IncludeDiscussions bool
}

// SpaceChatStream is one open space chat stream: the hub subscription plus
// the stream slot. Close is safe to call more than once.
type SpaceChatStream struct {
	apicore.SpaceChatSubscription
	release   func()
	closeOnce sync.Once
}

// Close leaves the hub and returns the stream slot.
func (s *SpaceChatStream) Close() {
	s.closeOnce.Do(func() {
		s.SpaceChatSubscription.Close()
		s.release()
	})
}

// OpenSpaceChatStream opens the space-wide chat stream. The space and the
// key's grant are checked before anything subscribes; every refusal is a C6
// error, returned before the stream's first byte.
func (s *Service) OpenSpaceChatStream(ctx context.Context, spaceId string, q SpaceChatStreamQuery) (*SpaceChatStream, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, err
	}
	if s.chatSub == nil {
		return nil, fmt.Errorf("open space chat stream in %s: chat subscriptions are not configured", spaceId)
	}
	release, ok := s.spaceChatStreams.acquire(maxConcurrentSpaceChatStreams)
	if !ok {
		return nil, v2model.NewError(http.StatusTooManyRequests, v2model.CodeTooManyStreams,
			fmt.Sprintf("this process already holds %d open space chat streams", maxConcurrentSpaceChatStreams),
			v2model.Issue{
				Message: "the cap is on streams held at once, not on how fast they are opened, so retrying the same request cannot succeed",
				Hint:    "close a space chat stream you no longer read before opening another",
			})
	}
	handedOff := false
	defer func() {
		if !handedOff {
			release()
		}
	}()

	sub, err := s.chatSub.OpenSpaceChats(ctx, apicore.SpaceChatOpen{
		SpaceId:            spaceId,
		IncludeDiscussions: q.IncludeDiscussions,
		Render:             s.spaceChatRenderer(spaceId),
	})
	if err != nil {
		var attachErr *apicore.SpaceChatAttachError
		if errors.As(err, &attachErr) {
			return nil, v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError,
				fmt.Sprintf("chat %s of space %s cannot be attached, so the space chat stream cannot open", attachErr.ChatId, spaceId),
				v2model.Issue{
					Message: fmt.Sprintf("chat %s: %v", attachErr.ChatId, attachErr.Err),
					Hint:    "the chat's state failed to initialize and stays failed until the app restarts; the per-chat routes of the other chats still work",
				})
		}
		return nil, fmt.Errorf("open space chats in %s: %w", spaceId, err)
	}
	handedOff = true
	return &SpaceChatStream{SpaceChatSubscription: sub, release: release}, nil
}

// spaceChatRenderer renders the hub's changes for one space. It is shared by
// every client of the space's hub and called from more than one goroutine.
func (s *Service) spaceChatRenderer(spaceId string) apicore.SpaceChatRender {
	names := &participantNames{index: s.store.SpaceIndex(spaceId), names: map[string]string{}}
	opts := v2model.ChatMessageOptions{SpaceId: spaceId, ParticipantName: names.lookup}
	return func(change apicore.SpaceChatChange) (apicore.SpaceChatFrame, error) {
		event, err := spaceChatEventOf(change, opts)
		if err != nil {
			return apicore.SpaceChatFrame{}, err
		}
		data, err := json.Marshal(event)
		if err != nil {
			return apicore.SpaceChatFrame{}, fmt.Errorf("marshal %s event: %w", event.Type, err)
		}
		return apicore.SpaceChatFrame{Type: event.Type, Data: data}, nil
	}
}

// participantNames resolves participant display names for a renderer that
// lives as long as its hub. Only found names are remembered: a member whose
// participant object has not synced yet resolves on a later message.
type participantNames struct {
	index spaceindex.Store
	mu    sync.Mutex
	names map[string]string
}

func (p *participantNames) lookup(participantId string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if name, ok := p.names[participantId]; ok {
		return name
	}
	details, err := p.index.GetDetails(participantId)
	if err != nil {
		return ""
	}
	name := details.GetString(bundle.RelationKeyName)
	if name != "" {
		p.names[participantId] = name
	}
	return name
}

// spaceChatEventOf maps one hub change onto the v2 envelope.
func spaceChatEventOf(change apicore.SpaceChatChange, opts v2model.ChatMessageOptions) (*v2model.SpaceChatEvent, error) {
	event := &v2model.SpaceChatEvent{SpaceId: change.SpaceId}
	chat := change.Chat
	switch change.Type {
	case apicore.SpaceChatAdded:
		event.Type = v2model.SpaceChatEventChatAdded
		event.Chat = streamChatRow(chat, change.State)
	case apicore.SpaceChatSnapshotComplete:
		event.Type = v2model.SpaceChatEventSnapshotComplete
	case apicore.SpaceChatUpdated:
		event.Type = v2model.SpaceChatEventChatUpdated
		event.Chat = streamChatRow(chat, change.State)
	case apicore.SpaceChatRemoved:
		event.Type = v2model.SpaceChatEventChatRemoved
		event.ChatId = chat.Id
	case apicore.SpaceChatStateUpdated:
		event.Type = v2model.ChatEventStateUpdated
		event.ChatId = chat.Id
		event.State = chatCounters(change.State)
	case apicore.SpaceChatMessageAdded, apicore.SpaceChatMessageUpdated:
		event.Type = v2model.ChatEventMessageAdded
		if change.Type == apicore.SpaceChatMessageUpdated {
			event.Type = v2model.ChatEventMessageUpdated
		}
		event.ChatId, event.Kind, event.ParentId = chat.Id, v2model.ChatKind(chat.Discussion), chat.ParentId
		message := v2model.ChatMessageFromProto(change.Message, opts)
		event.Message = &message
	case apicore.SpaceChatMessageDeleted:
		event.Type = v2model.ChatEventMessageDeleted
		event.ChatId, event.Kind, event.ParentId = chat.Id, v2model.ChatKind(chat.Discussion), chat.ParentId
		event.MessageId = change.MessageId
	case apicore.SpaceChatReactionsUpdated:
		event.Type = v2model.ChatEventReactionsUpdated
		event.ChatId, event.MessageId = chat.Id, change.MessageId
		event.Reactions = v2model.ReactionCounts(change.Message.GetReactions())
	case apicore.SpaceChatPinnedUpdated:
		event.Type = v2model.ChatEventPinnedUpdated
		event.ChatId, event.MessageId = chat.Id, change.MessageId
		pinned := change.Message.GetPinned()
		event.Pinned = &pinned
	default:
		return nil, fmt.Errorf("render space chat change: unknown change type %d", change.Type)
	}
	return event, nil
}

func streamChatRow(chat apicore.SpaceChat, state *model.ChatState) *v2model.StreamChatRow {
	return &v2model.StreamChatRow{
		Id:             chat.Id,
		Name:           chat.Name,
		Kind:           v2model.ChatKind(chat.Discussion),
		IsMain:         chat.IsMain,
		ParentId:       chat.ParentId,
		UnreadMessages: int(state.GetMessages().GetCounter()),
		UnreadMentions: int(state.GetMentions().GetCounter()),
		LastStateId:    state.GetLastStateId(),
	}
}

func chatCounters(state *model.ChatState) *v2model.ChatCounters {
	return &v2model.ChatCounters{
		UnreadMessages: int(state.GetMessages().GetCounter()),
		UnreadMentions: int(state.GetMentions().GetCounter()),
		LastStateId:    state.GetLastStateId(),
	}
}
