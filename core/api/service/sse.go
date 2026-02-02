package service

import (
	"sync"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var sseLog = logging.Logger("api-sse")

// SSESession represents an active SSE connection
type SSESession struct {
	SubId  string
	ChatId string
	Events chan *apimodel.SSEChatEvent
	Done   chan struct{}
}

// SSESessionManager manages active SSE sessions
type SSESessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*SSESession // subId -> session
}

// NewSSESessionManager creates a new SSE session manager
func NewSSESessionManager() *SSESessionManager {
	return &SSESessionManager{
		sessions: make(map[string]*SSESession),
	}
}

// Register adds a new SSE session
func (m *SSESessionManager) Register(session *SSESession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.SubId] = session
	sseLog.Infof("SSE session registered: subId=%s chatId=%s", session.SubId, session.ChatId)
}

// Unregister removes an SSE session
func (m *SSESessionManager) Unregister(subId string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[subId]; ok {
		close(session.Done)
		delete(m.sessions, subId)
		sseLog.Infof("SSE session unregistered: subId=%s", subId)
	}
}

// RouteEvent sends an event to all sessions matching the given subIds
func (m *SSESessionManager) RouteEvent(subIds []string, event *apimodel.SSEChatEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, subId := range subIds {
		if session, ok := m.sessions[subId]; ok {
			select {
			case session.Events <- event:
				// Event sent successfully
			default:
				// Channel full, log warning
				sseLog.Warnf("SSE event channel full for subId=%s, dropping event", subId)
			}
		}
	}
}

// CloseAll closes all active sessions
func (m *SSESessionManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for subId, session := range m.sessions {
		close(session.Done)
		delete(m.sessions, subId)
	}
	sseLog.Info("All SSE sessions closed")
}

// SSEEventDispatcher processes events and routes them to SSE sessions
type SSEEventDispatcher struct {
	sessionManager *SSESessionManager
	service        *Service
}

// NewSSEEventDispatcher creates a new event dispatcher
func NewSSEEventDispatcher(sessionManager *SSESessionManager, service *Service) *SSEEventDispatcher {
	return &SSEEventDispatcher{
		sessionManager: sessionManager,
		service:        service,
	}
}

// HandleEvent processes an event and routes it to appropriate SSE sessions
func (d *SSEEventDispatcher) HandleEvent(event *pb.Event) {
	if event == nil || len(event.Messages) == 0 {
		return
	}

	for _, msg := range event.Messages {
		d.processEventMessage(msg)
	}
}

func (d *SSEEventDispatcher) processEventMessage(msg *pb.EventMessage) {
	if msg == nil {
		return
	}

	// Handle chat add event
	if chatAdd := msg.GetChatAdd(); chatAdd != nil {
		d.handleChatAdd(chatAdd)
		return
	}

	// Handle chat update event
	if chatUpdate := msg.GetChatUpdate(); chatUpdate != nil {
		d.handleChatUpdate(chatUpdate)
		return
	}

	// Handle chat delete event
	if chatDelete := msg.GetChatDelete(); chatDelete != nil {
		d.handleChatDelete(chatDelete)
		return
	}

	// Handle chat reactions update
	if reactionsUpdate := msg.GetChatUpdateReactions(); reactionsUpdate != nil {
		d.handleChatUpdateReactions(reactionsUpdate)
		return
	}

	// Handle chat state update
	if stateUpdate := msg.GetChatStateUpdate(); stateUpdate != nil {
		d.handleChatStateUpdate(stateUpdate)
		return
	}
}

func (d *SSEEventDispatcher) handleChatAdd(chatAdd *pb.EventChatAdd) {
	if chatAdd.Message == nil || len(chatAdd.SubIds) == 0 {
		return
	}

	apiMsg := d.service.ProtoMessageToApiMessage(chatAdd.Message)
	event := &apimodel.SSEChatEvent{
		Type:    apimodel.SSEEventTypeAdd,
		Message: &apiMsg,
	}
	d.sessionManager.RouteEvent(chatAdd.SubIds, event)
}

func (d *SSEEventDispatcher) handleChatUpdate(chatUpdate *pb.EventChatUpdate) {
	if chatUpdate.Message == nil || len(chatUpdate.SubIds) == 0 {
		return
	}

	apiMsg := d.service.ProtoMessageToApiMessage(chatUpdate.Message)
	event := &apimodel.SSEChatEvent{
		Type:    apimodel.SSEEventTypeUpdate,
		Message: &apiMsg,
	}
	d.sessionManager.RouteEvent(chatUpdate.SubIds, event)
}

func (d *SSEEventDispatcher) handleChatDelete(chatDelete *pb.EventChatDelete) {
	if chatDelete.Id == "" || len(chatDelete.SubIds) == 0 {
		return
	}

	event := &apimodel.SSEChatEvent{
		Type:    apimodel.SSEEventTypeDelete,
		Deleted: chatDelete.Id,
	}
	d.sessionManager.RouteEvent(chatDelete.SubIds, event)
}

func (d *SSEEventDispatcher) handleChatUpdateReactions(reactionsUpdate *pb.EventChatUpdateReactions) {
	if reactionsUpdate.Reactions == nil || len(reactionsUpdate.SubIds) == 0 {
		return
	}

	// Create a minimal message with just the ID and reactions
	reactions := protoReactionsToApiReactions(reactionsUpdate.Reactions)
	msg := apimodel.ChatMessage{
		Id:        reactionsUpdate.Id,
		Reactions: reactions,
	}
	event := &apimodel.SSEChatEvent{
		Type:    apimodel.SSEEventTypeReactions,
		Message: &msg,
	}
	d.sessionManager.RouteEvent(reactionsUpdate.SubIds, event)
}

func (d *SSEEventDispatcher) handleChatStateUpdate(stateUpdate *pb.EventChatUpdateState) {
	if stateUpdate.State == nil || len(stateUpdate.SubIds) == 0 {
		return
	}

	apiState := d.service.ProtoChatStateToApiChatState(stateUpdate.State)
	event := &apimodel.SSEChatEvent{
		Type:  apimodel.SSEEventTypeState,
		State: apiState,
	}
	d.sessionManager.RouteEvent(stateUpdate.SubIds, event)
}

// protoReactionsToApiReactions converts protobuf reactions to API format
func protoReactionsToApiReactions(reactions *model.ChatMessageReactions) map[string][]string {
	if reactions == nil || reactions.Reactions == nil {
		return make(map[string][]string)
	}
	result := make(map[string][]string)
	for emoji, identityList := range reactions.Reactions {
		result[emoji] = identityList.Ids
	}
	return result
}
