package api

// spacechathub.go is the per-space chat hub behind apicore's OpenSpaceChats:
// the backend of the API v2 space-wide chat stream.
//
// One hub per space, created by the first client and torn down by the last.
// It discovers the space's chats through two internal subscriptions (the chat
// objects, and the objects that carry a discussion), attaches an observer to
// each eligible chat's state manager, and fans every change out to its
// clients: rendered once, filtered by each client's include predicate, and
// appended to the client's own unbounded queue. A client is never
// disconnected by the hub; a slow reader only costs memory.
//
// Goroutines: per hub, a forwarder moves discovery batches from the
// subscription queue onto the hub queue, and one worker drains the hub queue
// in order, so discovery and observer changes are applied in arrival order.
//
// Lock order: a manager's observer callback runs under the manager lock and
// takes only the hub queue's lock. The hub never holds its own locks while it
// takes a manager lock or waits on a worker.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// spaceChatBackfillLimit is how many of a chat's newest messages follow its
// chat added event when the chat appears after a client's snapshot: its first
// messages can be applied before discovery reaches the hub, and the observer
// does not replay them.
const spaceChatBackfillLimit = 50

// The hub's dependencies, narrowed to what it calls.
type (
	spaceChatManagers interface {
		GetManager(spaceId string, chatObjectId string) (chatsubscription.Manager, error)
	}
	spaceChatSearcher interface {
		Search(req subscription.SubscribeRequest) (*subscription.SubscribeResponse, error)
		Unsubscribe(subIds ...string) error
	}
	spaceChatRepositories interface {
		Repository(spaceId, chatObjectId string) (chatrepository.Repository, error)
	}
)

// spaceChatHubs owns one hub slot per space.
type spaceChatHubs struct {
	managers     spaceChatManagers
	searcher     spaceChatSearcher
	repositories spaceChatRepositories

	generation atomic.Uint64

	mu     sync.Mutex
	spaces map[string]*spaceChatSlot
}

// spaceChatSlot serializes opening and closing one space's hub.
type spaceChatSlot struct {
	mu  sync.Mutex
	hub *spaceChatHub
}

func newSpaceChatHubs(managers spaceChatManagers, searcher spaceChatSearcher, repositories spaceChatRepositories) *spaceChatHubs {
	return &spaceChatHubs{
		managers:     managers,
		searcher:     searcher,
		repositories: repositories,
		spaces:       make(map[string]*spaceChatSlot),
	}
}

// Open places one client on the space's hub, creating and starting the hub
// when this is the first client. ctx bounds the start and the client's
// snapshot: a client that leaves during a slow start leaves no hub behind.
func (r *spaceChatHubs) Open(ctx context.Context, req apicore.SpaceChatOpen) (apicore.SpaceChatSubscription, error) {
	if req.SpaceId == "" {
		return nil, errors.New("open space chats: empty space id")
	}
	if req.Render == nil {
		return nil, errors.New("open space chats: no renderer")
	}
	slot := r.slot(req.SpaceId)
	slot.mu.Lock()
	hub := slot.hub
	if hub == nil {
		hub = newSpaceChatHub(r, req.SpaceId, r.generation.Add(1), req.Render)
		if err := hub.start(ctx); err != nil {
			slot.mu.Unlock()
			hub.close()
			return nil, fmt.Errorf("start space chat hub: %w", err)
		}
		slot.hub = hub
	}
	client, err := hub.join(ctx, req.IncludeDiscussions)
	if err != nil {
		var abandoned *spaceChatHub
		if hub.clientCount() == 0 {
			slot.hub = nil
			abandoned = hub
		}
		slot.mu.Unlock()
		if abandoned != nil {
			abandoned.close()
		}
		return nil, fmt.Errorf("join space chat hub: %w", err)
	}
	client.release = func() { r.leave(slot, hub, client) }
	slot.mu.Unlock()
	return client, nil
}

func (r *spaceChatHubs) slot(spaceId string) *spaceChatSlot {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot, ok := r.spaces[spaceId]
	if !ok {
		slot = &spaceChatSlot{}
		r.spaces[spaceId] = slot
	}
	return slot
}

// leave removes a client; the last one detaches the hub from its slot and
// tears it down outside the slot lock, so a new hub of the next generation
// can start meanwhile.
func (r *spaceChatHubs) leave(slot *spaceChatSlot, hub *spaceChatHub, client *spaceChatClient) {
	slot.mu.Lock()
	last := hub.removeClient(client)
	if last && slot.hub == hub {
		slot.hub = nil
	}
	slot.mu.Unlock()
	client.queue.close()
	if last {
		hub.close()
	}
}

// discoveredChat is a row of the chat-objects subscription.
type discoveredChat struct {
	layout model.ObjectTypeLayout
	name   string
	isMain bool
}

// discoveredParent is a row of the discussion-parents subscription.
type discoveredParent struct {
	name         string
	discussionId string
}

// chatCounters is what a state_updated event reports; a state change that
// leaves it unchanged (a watermark move) is not delivered.
type chatCounters struct {
	messages    int32
	mentions    int32
	lastStateId string
}

func countersOf(state *model.ChatState) chatCounters {
	return chatCounters{
		messages:    state.GetMessages().GetCounter(),
		mentions:    state.GetMentions().GetCounter(),
		lastStateId: state.GetLastStateId(),
	}
}

// hubChat is one eligible chat. manager is nil when the chat could not be
// attached; attachErr then says why, and opens that would receive the chat fail.
type hubChat struct {
	info      apicore.SpaceChat
	manager   chatsubscription.Manager
	attachErr error
	// membership identifies this attachment: observer changes carry it, so
	// changes queued for an earlier attachment of the same chat are dropped.
	membership uint64
	counters   chatCounters
}

// hubItem is one entry of the hub queue: a discovery batch or an observer change.
type hubItem struct {
	discovery  []*pb.EventMessage
	chatId     string
	membership uint64
	change     chatsubscription.ChatChange
}

type spaceChatHub struct {
	owner        *spaceChatHubs
	spaceId      string
	generation   uint64
	render       apicore.SpaceChatRender
	observerId   string
	chatsSubId   string
	parentsSubId string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// discovery receives both subscriptions' events. The hub owns it:
	// Unsubscribe does not close a caller-provided queue.
	discovery *mb.MB[*pb.EventMessage]
	queue     *eventQueue[hubItem]

	// Discovery state, touched by start and then by the worker only.
	subscribed     bool
	chatObjects    map[string]discoveredChat
	parents        map[string]discoveredParent
	nextMembership uint64

	// mu guards clients and chats. chats is written only by start and the
	// worker; join reads it.
	mu      sync.Mutex
	clients map[*spaceChatClient]struct{}
	chats   map[string]hubChat
}

func newSpaceChatHub(owner *spaceChatHubs, spaceId string, generation uint64, render apicore.SpaceChatRender) *spaceChatHub {
	ctx, cancel := context.WithCancel(context.Background())
	idPrefix := fmt.Sprintf("api-space-chats-%s-%d", spaceId, generation)
	return &spaceChatHub{
		owner:        owner,
		spaceId:      spaceId,
		generation:   generation,
		render:       render,
		observerId:   idPrefix,
		chatsSubId:   idPrefix + "-chats",
		parentsSubId: idPrefix + "-parents",
		ctx:          ctx,
		cancel:       cancel,
		discovery:    mb.New[*pb.EventMessage](0),
		queue:        newEventQueue[hubItem](),
		chatObjects:  make(map[string]discoveredChat),
		parents:      make(map[string]discoveredParent),
		clients:      make(map[*spaceChatClient]struct{}),
		chats:        make(map[string]hubChat),
	}
}

// start subscribes to discovery, attaches every eligible chat and starts the
// workers. A chat that cannot attach is recorded, not fatal here: the open
// that would receive it fails in join.
func (h *spaceChatHub) start(ctx context.Context) error {
	h.subscribed = true
	chats, err := h.owner.searcher.Search(h.chatsRequest())
	if err != nil {
		return fmt.Errorf("subscribe to chats: %w", err)
	}
	for _, details := range chats.Records {
		h.setDetails(h.chatsSubId, details.GetString(bundle.RelationKeyId), details)
	}
	parents, err := h.owner.searcher.Search(h.parentsRequest())
	if err != nil {
		return fmt.Errorf("subscribe to discussion parents: %w", err)
	}
	for _, details := range parents.Records {
		h.setDetails(h.parentsSubId, details.GetString(bundle.RelationKeyId), details)
	}
	eligible := h.eligible()
	for _, id := range sortedKeys(eligible) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("attach chats: %w", err)
		}
		hc, _, err := h.attach(eligible[id])
		if err != nil {
			hc = hubChat{info: eligible[id], attachErr: err}
		}
		h.setChat(hc)
	}
	h.wg.Add(2)
	go h.forward()
	go h.work()
	return nil
}

// close tears the hub down: refuse new attaches and stop the workers, then
// detach every observer, unsubscribe both searches and close the owned
// queues. Joining the workers first means nothing attaches after the
// observers are removed. Observer and subscription ids carry the generation,
// so this cannot touch a newer hub of the same space.
func (h *spaceChatHub) close() {
	h.cancel()
	h.wg.Wait()
	h.mu.Lock()
	chats := make([]hubChat, 0, len(h.chats))
	for _, hc := range h.chats {
		chats = append(chats, hc)
	}
	h.chats = map[string]hubChat{}
	h.mu.Unlock()
	for _, hc := range chats {
		h.detach(hc)
	}
	if h.subscribed {
		if err := h.owner.searcher.Unsubscribe(h.chatsSubId, h.parentsSubId); err != nil {
			log.Warnf("space chat hub %s: unsubscribe: %v", h.observerId, err)
		}
	}
	_ = h.discovery.Close()
	h.queue.close()
}

func (h *spaceChatHub) chatsRequest() subscription.SubscribeRequest {
	return subscription.SubscribeRequest{
		SpaceId: h.spaceId,
		SubId:   h.chatsSubId,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyResolvedLayout.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIsMainChat.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List([]model.ObjectTypeLayout{model.ObjectType_chatDerived, model.ObjectType_discussion}),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
		NoDepSubscription: true,
		Internal:          true,
		InternalQueue:     h.discovery,
	}
}

func (h *spaceChatHub) parentsRequest() subscription.SubscribeRequest {
	return subscription.SubscribeRequest{
		SpaceId: h.spaceId,
		SubId:   h.parentsSubId,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyDiscussionId.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyDiscussionId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
		NoDepSubscription: true,
		Internal:          true,
		InternalQueue:     h.discovery,
	}
}

// attach registers the hub's observer on the chat's manager and reads the
// chat state under the same lock, so every later change reaches the observer.
func (h *spaceChatHub) attach(info apicore.SpaceChat) (hubChat, *model.ChatState, error) {
	mngr, err := h.owner.managers.GetManager(h.spaceId, info.Id)
	if err != nil {
		return hubChat{}, nil, fmt.Errorf("get chat manager: %w", err)
	}
	h.nextMembership++
	membership := h.nextMembership
	mngr.Lock()
	mngr.AddObserver(h.observerId, h.observer(info.Id, membership))
	state := mngr.GetChatState()
	mngr.Unlock()
	return hubChat{info: info, manager: mngr, membership: membership, counters: countersOf(state)}, state, nil
}

func (h *spaceChatHub) detach(hc hubChat) {
	if hc.manager == nil {
		return
	}
	hc.manager.Lock()
	hc.manager.RemoveObserver(h.observerId)
	hc.manager.Unlock()
}

// observer is the manager callback: enqueue only.
func (h *spaceChatHub) observer(chatId string, membership uint64) chatsubscription.ChatObserver {
	return func(change chatsubscription.ChatChange) {
		h.queue.push(hubItem{chatId: chatId, membership: membership, change: change})
	}
}

// forward moves discovery batches onto the hub queue, behind any observer
// change already queued.
func (h *spaceChatHub) forward() {
	defer h.wg.Done()
	for {
		msgs, err := h.discovery.Wait(h.ctx)
		if err != nil {
			return
		}
		h.queue.push(hubItem{discovery: msgs})
	}
}

func (h *spaceChatHub) work() {
	defer h.wg.Done()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-h.queue.ready():
		}
		for _, item := range h.queue.drain() {
			if h.ctx.Err() != nil {
				return
			}
			if item.discovery != nil {
				h.applyDiscovery(item.discovery)
			} else {
				h.deliver(item)
			}
		}
	}
}

func (h *spaceChatHub) applyDiscovery(msgs []*pb.EventMessage) {
	for _, msg := range msgs {
		switch v := msg.Value.(type) {
		case *pb.EventMessageValueOfObjectDetailsSet:
			details := domain.NewDetailsFromProto(v.ObjectDetailsSet.Details)
			for _, subId := range v.ObjectDetailsSet.SubIds {
				h.setDetails(subId, v.ObjectDetailsSet.Id, details)
			}
		case *pb.EventMessageValueOfObjectDetailsAmend:
			for _, subId := range v.ObjectDetailsAmend.SubIds {
				for _, kv := range v.ObjectDetailsAmend.Details {
					h.amendDetail(subId, v.ObjectDetailsAmend.Id, domain.RelationKey(kv.Key), domain.ValueFromProto(kv.Value))
				}
			}
		case *pb.EventMessageValueOfObjectDetailsUnset:
			for _, subId := range v.ObjectDetailsUnset.SubIds {
				for _, key := range v.ObjectDetailsUnset.Keys {
					h.amendDetail(subId, v.ObjectDetailsUnset.Id, domain.RelationKey(key), domain.Value{})
				}
			}
		case *pb.EventMessageValueOfSubscriptionRemove:
			switch v.SubscriptionRemove.SubId {
			case h.chatsSubId:
				delete(h.chatObjects, v.SubscriptionRemove.Id)
			case h.parentsSubId:
				delete(h.parents, v.SubscriptionRemove.Id)
			}
		}
	}
	h.reconcile()
}

func (h *spaceChatHub) setDetails(subId, id string, details *domain.Details) {
	switch subId {
	case h.chatsSubId:
		h.chatObjects[id] = discoveredChat{
			// nolint: gosec
			layout: model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout)),
			name:   details.GetString(bundle.RelationKeyName),
			isMain: details.GetBool(bundle.RelationKeyIsMainChat),
		}
	case h.parentsSubId:
		h.parents[id] = discoveredParent{
			name:         details.GetString(bundle.RelationKeyName),
			discussionId: details.GetString(bundle.RelationKeyDiscussionId),
		}
	}
}

// amendDetail applies one changed (or, with a zero value, removed) detail to a
// row already in the subscription.
func (h *spaceChatHub) amendDetail(subId, id string, key domain.RelationKey, value domain.Value) {
	switch subId {
	case h.chatsSubId:
		row, ok := h.chatObjects[id]
		if !ok {
			return
		}
		switch key {
		case bundle.RelationKeyResolvedLayout:
			// nolint: gosec
			row.layout = model.ObjectTypeLayout(value.Int64())
		case bundle.RelationKeyName:
			row.name = value.String()
		case bundle.RelationKeyIsMainChat:
			row.isMain = value.Bool()
		}
		h.chatObjects[id] = row
	case h.parentsSubId:
		row, ok := h.parents[id]
		if !ok {
			return
		}
		switch key {
		case bundle.RelationKeyName:
			row.name = value.String()
		case bundle.RelationKeyDiscussionId:
			row.discussionId = value.String()
		}
		h.parents[id] = row
	}
}

// eligible joins the two subscriptions: every chat-layout object, and every
// discussion whose parent carries its id. When two parents claim one
// discussion, the smaller parent id wins, so the mapping is stable.
func (h *spaceChatHub) eligible() map[string]apicore.SpaceChat {
	parentOf := make(map[string]string, len(h.parents))
	for parentId, parent := range h.parents {
		if parent.discussionId == "" {
			continue
		}
		if current, ok := parentOf[parent.discussionId]; !ok || parentId < current {
			parentOf[parent.discussionId] = parentId
		}
	}
	out := make(map[string]apicore.SpaceChat, len(h.chatObjects))
	for id, chat := range h.chatObjects {
		switch chat.layout {
		case model.ObjectType_chatDerived:
			out[id] = apicore.SpaceChat{Id: id, Name: chat.name, IsMain: chat.isMain}
		case model.ObjectType_discussion:
			parentId, ok := parentOf[id]
			if !ok {
				continue
			}
			out[id] = apicore.SpaceChat{Id: id, Name: h.parents[parentId].name, Discussion: true, ParentId: parentId}
		}
	}
	return out
}

// reconcile moves the hub's chats to the current eligible set: removals
// first, then additions and updates.
func (h *spaceChatHub) reconcile() {
	next := h.eligible()
	h.mu.Lock()
	current := make(map[string]hubChat, len(h.chats))
	for id, hc := range h.chats {
		current[id] = hc
	}
	h.mu.Unlock()

	for _, id := range sortedKeys(current) {
		if _, ok := next[id]; !ok {
			h.removeChat(current[id])
		}
	}
	for _, id := range sortedKeys(next) {
		info := next[id]
		hc, ok := current[id]
		switch {
		case !ok:
			h.addChat(info)
		case hc.info != info:
			h.updateChat(hc, info)
		}
	}
}

// removeChat drops a chat that stopped being eligible. Deleting it from chats
// is what invalidates observer changes still queued for its attachment.
func (h *spaceChatHub) removeChat(hc hubChat) {
	h.mu.Lock()
	delete(h.chats, hc.info.Id)
	h.mu.Unlock()
	h.detach(hc)
	if hc.manager != nil {
		h.fanOut(apicore.SpaceChatChange{Type: apicore.SpaceChatRemoved, Chat: hc.info})
	}
}

// addChat attaches a chat that became eligible while the hub runs, announces
// it and backfills its newest messages. The observer is attached BEFORE the
// repository read, so a message lands in the backfill, in the observer, or in
// both; never in neither. A chat that cannot attach is logged and skipped.
func (h *spaceChatHub) addChat(info apicore.SpaceChat) {
	if h.ctx.Err() != nil {
		return // tearing down: refuse new attaches
	}
	hc, state, err := h.attach(info)
	if err != nil {
		log.With(zap.String("spaceId", h.spaceId), zap.String("chatId", info.Id), zap.Error(err)).
			Error("space chat hub: attach chat, skipping it")
		h.setChat(hubChat{info: info, attachErr: err})
		return
	}
	backfill := h.backfill(info.Id)
	h.setChat(hc)
	h.fanOut(apicore.SpaceChatChange{Type: apicore.SpaceChatAdded, Chat: info, State: state})
	for _, msg := range backfill {
		h.fanOut(apicore.SpaceChatChange{Type: apicore.SpaceChatMessageAdded, Chat: info, Message: msg.ChatMessage, MessageId: msg.Id})
	}
}

func (h *spaceChatHub) backfill(chatId string) []*chatmodel.Message {
	repo, err := h.owner.repositories.Repository(h.spaceId, chatId)
	if err != nil {
		log.With(zap.String("chatId", chatId), zap.Error(err)).Error("space chat hub: backfill: get repository")
		return nil
	}
	msgs, err := repo.GetLastMessages(h.ctx, spaceChatBackfillLimit)
	if err != nil {
		log.With(zap.String("chatId", chatId), zap.Error(err)).Error("space chat hub: backfill: read messages")
		return nil
	}
	return msgs
}

func (h *spaceChatHub) updateChat(hc hubChat, info apicore.SpaceChat) {
	hc.info = info
	h.setChat(hc)
	if hc.manager == nil {
		return
	}
	hc.manager.Lock()
	state := hc.manager.GetChatState()
	hc.manager.Unlock()
	h.fanOut(apicore.SpaceChatChange{Type: apicore.SpaceChatUpdated, Chat: info, State: state})
}

func (h *spaceChatHub) setChat(hc hubChat) {
	h.mu.Lock()
	h.chats[hc.info.Id] = hc
	h.mu.Unlock()
}

// deliver fans one observer change out, if its chat is still eligible under
// the same attachment.
func (h *spaceChatHub) deliver(item hubItem) {
	h.mu.Lock()
	hc, ok := h.chats[item.chatId]
	if !ok || hc.manager == nil || hc.membership != item.membership {
		h.mu.Unlock()
		return
	}
	change := apicore.SpaceChatChange{Chat: hc.info, MessageId: item.change.MessageId}
	if item.change.Message != nil {
		change.Message = item.change.Message.ChatMessage
	}
	switch item.change.Kind {
	case chatsubscription.ChatChangeStateUpdated:
		counters := countersOf(item.change.State)
		if counters == hc.counters {
			h.mu.Unlock()
			return
		}
		hc.counters = counters
		h.chats[item.chatId] = hc
		change.Type = apicore.SpaceChatStateUpdated
		change.State = item.change.State
	case chatsubscription.ChatChangeMessageAdded:
		change.Type = apicore.SpaceChatMessageAdded
	case chatsubscription.ChatChangeMessageUpdated:
		change.Type = apicore.SpaceChatMessageUpdated
	case chatsubscription.ChatChangeMessageDeleted:
		change.Type = apicore.SpaceChatMessageDeleted
	case chatsubscription.ChatChangeReactionsUpdated:
		change.Type = apicore.SpaceChatReactionsUpdated
	case chatsubscription.ChatChangePinnedUpdated:
		change.Type = apicore.SpaceChatPinnedUpdated
	default:
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	h.fanOut(change)
}

// fanOut renders a change once and appends it to every client that receives
// its chat. Called by the worker only, so the chat's eligibility cannot change
// between the caller's check and the append.
func (h *spaceChatHub) fanOut(change apicore.SpaceChatChange) {
	change.SpaceId = h.spaceId
	frame, err := h.render(change)
	if err != nil {
		log.With(zap.String("spaceId", h.spaceId), zap.String("chatId", change.Chat.Id), zap.Error(err)).
			Error("space chat hub: render event, dropping it")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		if client.accepts(change.Chat) {
			client.queue.push(frame)
		}
	}
}

// join registers a client and builds its snapshot. The client's queue is
// registered BEFORE the snapshot reads the chats, so a change racing the
// snapshot appears in it, in the queue, or in both; never in neither.
func (h *spaceChatHub) join(ctx context.Context, includeDiscussions bool) (*spaceChatClient, error) {
	client := &spaceChatClient{includeDiscussions: includeDiscussions, queue: newEventQueue[apicore.SpaceChatFrame]()}
	h.mu.Lock()
	h.clients[client] = struct{}{}
	chats := make([]hubChat, 0, len(h.chats))
	for _, hc := range h.chats {
		chats = append(chats, hc)
	}
	h.mu.Unlock()
	sort.Slice(chats, func(i, j int) bool { return chats[i].info.Id < chats[j].info.Id })

	snapshot, err := h.snapshot(ctx, client, chats)
	if err != nil {
		h.removeClient(client)
		client.queue.close()
		return nil, err
	}
	client.snapshot = snapshot
	return client, nil
}

func (h *spaceChatHub) snapshot(ctx context.Context, client *spaceChatClient, chats []hubChat) ([]apicore.SpaceChatFrame, error) {
	frames := make([]apicore.SpaceChatFrame, 0, len(chats)+1)
	for _, hc := range chats {
		if !client.accepts(hc.info) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("build snapshot: %w", err)
		}
		if hc.manager == nil {
			return nil, &apicore.SpaceChatAttachError{ChatId: hc.info.Id, Err: hc.attachErr}
		}
		hc.manager.Lock()
		state := hc.manager.GetChatState()
		hc.manager.Unlock()
		frame, err := h.render(apicore.SpaceChatChange{Type: apicore.SpaceChatAdded, SpaceId: h.spaceId, Chat: hc.info, State: state})
		if err != nil {
			return nil, fmt.Errorf("render chat %s: %w", hc.info.Id, err)
		}
		frames = append(frames, frame)
	}
	complete, err := h.render(apicore.SpaceChatChange{Type: apicore.SpaceChatSnapshotComplete, SpaceId: h.spaceId})
	if err != nil {
		return nil, fmt.Errorf("render snapshot complete: %w", err)
	}
	return append(frames, complete), nil
}

// removeClient unregisters a client and reports whether it was the last.
func (h *spaceChatHub) removeClient(client *spaceChatClient) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client)
	return len(h.clients) == 0
}

func (h *spaceChatHub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// spaceChatClient is one client's place on a hub (apicore.SpaceChatSubscription).
type spaceChatClient struct {
	includeDiscussions bool
	queue              *eventQueue[apicore.SpaceChatFrame]
	snapshot           []apicore.SpaceChatFrame
	release            func()
	closeOnce          sync.Once
}

func (c *spaceChatClient) accepts(chat apicore.SpaceChat) bool {
	return !chat.Discussion || c.includeDiscussions
}

func (c *spaceChatClient) Snapshot() []apicore.SpaceChatFrame { return c.snapshot }

func (c *spaceChatClient) Ready() <-chan struct{} { return c.queue.ready() }

func (c *spaceChatClient) Drain() []apicore.SpaceChatFrame { return c.queue.drain() }

func (c *spaceChatClient) Close() {
	c.closeOnce.Do(func() {
		if c.release != nil {
			c.release()
		}
	})
}

// eventQueue is an unbounded FIFO whose push never blocks: it is what an
// observer callback, running under a manager lock, may call. ready is
// signalled after every push; a consumer receives from it, then drains.
type eventQueue[T any] struct {
	mu     sync.Mutex
	items  []T
	closed bool
	signal chan struct{}
}

func newEventQueue[T any]() *eventQueue[T] {
	return &eventQueue[T]{signal: make(chan struct{}, 1)}
}

// push appends items; on a closed queue it drops them.
func (q *eventQueue[T]) push(items ...T) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, items...)
	q.mu.Unlock()
	select {
	case q.signal <- struct{}{}:
	default:
	}
}

func (q *eventQueue[T]) drain() []T {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.items
	q.items = nil
	return items
}

func (q *eventQueue[T]) ready() <-chan struct{} {
	return q.signal
}

func (q *eventQueue[T]) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.items = nil
}

func (q *eventQueue[T]) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
