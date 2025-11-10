package chatobject

import (
	"context"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/util/slice"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/components"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CollectionName        = "chats"
	EditorCollectionName  = "editor"
	diffManagerMessages   = "messages"
	diffManagerMentions   = "mentions"
	diffManagerSyncStatus = "syncStatus"
)

var log = logging.Logger("core.block.editor.chatobject").Desugar()

type StoreObject interface {
	smartblock.SmartBlock
	basic.DetailsUpdatable
	basic.DetailsSettable

	anystoredebug.AnystoreDebug
	components.SyncStatusHandler

	AddMessage(ctx context.Context, sessionCtx session.Context, message *chatmodel.Message) (string, error)
	GetMessages(ctx context.Context, req chatrepository.GetMessagesRequest) (*GetMessagesResponse, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*chatmodel.Message, error)
	EditMessage(ctx context.Context, messageId string, newMessage *chatmodel.Message) error
	ToggleMessageReaction(ctx context.Context, messageId string, emoji string) (bool, error)
	DeleteMessage(ctx context.Context, messageId string) error
	MarkReadMessages(ctx context.Context, req ReadMessagesRequest) (markedCount int, err error)
	MarkMessagesAsUnread(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) error
}

type AccountService interface {
	AccountID() string
	Keys() *accountdata.AccountKeys
}

type seenHeadsCollector interface {
	collectSeenHeads(ctx context.Context, afterOrderId string) ([]string, error)
}

type storeObject struct {
	anystoredebug.AnystoreDebug
	basic.DetailsSettable
	basic.DetailsUpdatable
	smartblock.SmartBlock
	locker smartblock.Locker

	seenHeadsCollector      seenHeadsCollector
	accountService          AccountService
	storeSource             source.Store
	repositoryService       chatrepository.Service
	store                   *storestate.StoreState
	chatSubscriptionService chatsubscription.Service
	subscription            chatsubscription.Manager
	crdtDb                  anystore.DB
	chatHandler             *ChatHandler
	repository              chatrepository.Repository
	detailsComponent        *detailsComponent
	statService             debugstat.StatService
	spaceIndex              spaceindex.Store

	arenaPool          *anyenc.ArenaPool
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

type UnreadStats struct {
	MessagesCount int      `json:"messagesCount"`
	MessageIds    []string `json:"messageIds"`
	StatType      string   `json:"statType"`
}

type StoreObjectStats struct {
	StoreState  any           `json:"storeState"`
	UnreadStats []UnreadStats `json:"unreadStats"`
	Heads       []string      `json:"heads"`
}

func (s *storeObject) ProvideStat() any {
	s.Lock()
	defer s.Unlock()
	stats := StoreObjectStats{}
	if statProvider, ok := s.storeSource.(debugstat.StatProvider); ok {
		stats.StoreState = statProvider.ProvideStat()
	}
	stats.Heads = make([]string, len(s.storeSource.Heads()))
	copy(stats.Heads, s.storeSource.Heads())
	statTypes := []string{diffManagerMessages, diffManagerMentions}
	msgTypes := []chatmodel.CounterType{chatmodel.CounterTypeMessage, chatmodel.CounterTypeMention}
	for i, statType := range statTypes {
		msgIds, err := s.repository.GetAllUnreadMessages(s.componentCtx, msgTypes[i])
		if err != nil {
			log.Error("get unread messages", zap.Error(err), zap.String("statType", statType))
			continue
		}
		stats.UnreadStats = append(stats.UnreadStats, UnreadStats{
			MessagesCount: len(msgIds),
			MessageIds:    msgIds[0:min(len(msgIds), 1000)],
			StatType:      statType,
		})
	}
	return stats
}

func (s *storeObject) StatId() string {
	return s.Id()
}

func (s *storeObject) StatType() string {
	return "store.object"
}

func New(
	sb smartblock.SmartBlock,
	accountService AccountService,
	crdtDb anystore.DB,
	repositoryService chatrepository.Service,
	chatSubscriptionService chatsubscription.Service,
	spaceIndex spaceindex.Store,
	layoutConverter converter.LayoutConverter,
	fileObjectService fileobject.Service,
	statService debugstat.StatService,
) StoreObject {
	ctx, cancel := context.WithCancel(context.Background())
	bs := basic.NewBasic(sb, spaceIndex, layoutConverter, fileObjectService)
	return &storeObject{
		SmartBlock:              sb,
		locker:                  sb.(smartblock.Locker),
		accountService:          accountService,
		statService:             statService,
		arenaPool:               &anyenc.ArenaPool{},
		crdtDb:                  crdtDb,
		repositoryService:       repositoryService,
		componentCtx:            ctx,
		componentCtxCancel:      cancel,
		chatSubscriptionService: chatSubscriptionService,
		DetailsSettable:         bs,
		DetailsUpdatable:        bs,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}

	var err error
	s.repository, err = s.repositoryService.Repository(storeSource.Id())
	if err != nil {
		return fmt.Errorf("get repository: %w", err)
	}

	// Use Object and Space IDs from source, because object is not initialized yet
	myParticipantId := domain.NewParticipantId(ctx.Source.SpaceID(), s.accountService.AccountID())

	// Diff managers should be added before SmartBlock.Init, because they have to be initialized in source.ReadStoreDoc
	storeSource.RegisterDiffManager(diffManagerMessages, func(removed []string) {
		markErr := s.markReadMessages(removed, chatmodel.CounterTypeMessage)
		if markErr != nil {
			log.Error("mark read messages", zap.Error(markErr))
		}
	})
	storeSource.RegisterDiffManager(diffManagerMentions, func(removed []string) {
		markErr := s.markReadMessages(removed, chatmodel.CounterTypeMention)
		if markErr != nil {
			log.Error("mark read mentions", zap.Error(markErr))
		}
	})
	storeSource.RegisterDiffManager(diffManagerSyncStatus, func(removed []string) {
		updateErr := s.setMessagesSyncStatus(removed)
		if updateErr != nil {
			log.Error("set sync status", zap.Error(updateErr))
		}
	})
	err = s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	s.storeSource = storeSource

	s.subscription, err = s.chatSubscriptionService.GetManager(storeSource.SpaceID(), storeSource.Id())
	if err != nil {
		return fmt.Errorf("get subscription manager: %w", err)
	}

	s.chatHandler = &ChatHandler{
		repository:      s.repository,
		subscription:    s.subscription,
		currentIdentity: s.accountService.AccountID(),
		myParticipantId: myParticipantId,
	}

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.crdtDb, s.chatHandler, storestate.DefaultHandler{Name: EditorCollectionName, ModifyMode: storestate.ModifyModeUpsert})
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	s.store = stateStore

	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, source.ReadStoreDocParams{
		OnUpdateHook: s.onUpdate,
		ReadStoreTreeHook: &readStoreTreeHook{
			currentIdentity: s.accountService.Keys().SignKey.GetPublic(),
			source:          s.storeSource,
		},
	})
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	s.detailsComponent = &detailsComponent{
		componentCtx:       s.componentCtx,
		collectionName:     EditorCollectionName,
		storeSource:        storeSource,
		storeState:         stateStore,
		spaceIndex:         s.spaceIndex,
		sb:                 s.SmartBlock,
		deniedRelationKeys: []domain.RelationKey{bundle.RelationKeyInternalFlags},
	}
	spaceChatId := s.Space().DerivedIDs().SpaceChat
	if s.Id() == spaceChatId {
		setDetail := func(key domain.RelationKey, val domain.Value) {
			// Set property both in parent and in the current state to avoid pushing a change
			ctx.State.ParentState().SetDetail(key, val)
			ctx.State.SetDetail(key, val)
		}
		setDetail(bundle.RelationKeyName, domain.String("General"))
		setDetail(bundle.RelationKeyIsMainChat, domain.Bool(true))
	}
	err = s.detailsComponent.init(ctx.State)
	if err != nil {
		return fmt.Errorf("init details: %w", err)
	}

	storeSource.SetPushChangeHook(s.detailsComponent.onPushOrdinaryChange)

	s.AnystoreDebug = anystoredebug.New(s.SmartBlock, stateStore)

	s.seenHeadsCollector = newTreeSeenHeadsCollector(s.Tree())
	s.statService.AddProvider(s)

	return nil
}

func (s *storeObject) onUpdate() {
	err := s.detailsComponent.onAnystoreUpdated(s.componentCtx)
	if err != nil {
		log.Error("onUpdate: on anystore updated", zap.Error(err))
	}

	s.subscription.Lock()
	defer s.subscription.Unlock()

	s.subscription.Flush()

	last, ok := s.subscription.GetLastMessage()
	if ok {
		st := s.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyLastMessageDate, domain.Int64(last.CreatedAt))
		err = s.Apply(st, smartblock.NotPushChanges)
		if err != nil {
			log.Error("onUpdate: update last message date", zap.Error(err))
		}
	}
}

func (s *storeObject) GetMessageById(ctx context.Context, id string) (*chatmodel.Message, error) {
	messages, err := s.GetMessagesByIds(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("message not found")
	}
	return messages[0], nil
}

func (s *storeObject) GetMessagesByIds(ctx context.Context, messageIds []string) ([]*chatmodel.Message, error) {
	return s.repository.GetMessagesByIds(ctx, messageIds)
}

type GetMessagesResponse struct {
	Messages  []*chatmodel.Message
	ChatState *model.ChatState
}

func (s *storeObject) GetMessages(ctx context.Context, req chatrepository.GetMessagesRequest) (*GetMessagesResponse, error) {
	msgs, err := s.repository.GetMessages(ctx, req)
	if err != nil {
		return nil, err
	}
	return &GetMessagesResponse{
		Messages:  msgs,
		ChatState: s.subscription.GetChatState(),
	}, nil
}

func (s *storeObject) AddMessage(ctx context.Context, sessionCtx session.Context, message *chatmodel.Message) (string, error) {
	err := message.Validate()
	if err != nil {
		return "", fmt.Errorf("validate: %w", err)
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	// Normalize message
	message.Read = false
	message.MentionRead = false

	obj := arena.NewObject()
	message.MarshalAnyenc(obj, arena)
	obj.Del(chatmodel.ReadKey)
	obj.Del(chatmodel.MentionReadKey)
	obj.Del(chatmodel.SyncedKey)

	builder := storestate.Builder{}
	err = builder.Create(CollectionName, storestate.IdFromChange, obj)
	if err != nil {
		return "", fmt.Errorf("create chat: %w", err)
	}

	s.subscription.Lock()
	s.subscription.SetSessionContext(sessionCtx)
	s.subscription.Unlock()
	defer func() {
		s.subscription.Lock()
		s.subscription.SetSessionContext(nil)
		s.subscription.Unlock()
	}()
	messageId, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("push change: %w", err)
	}

	if !s.chatHandler.forceNotRead {
		for _, counterType := range []chatmodel.CounterType{chatmodel.CounterTypeMessage, chatmodel.CounterTypeMention} {
			err = s.storeSource.MarkSeenHeads(ctx, counterType.DiffManagerName(), []string{messageId})
			if err != nil {
				return "", fmt.Errorf("mark read: %w", err)
			}
		}
	}

	return messageId, nil
}

func (s *storeObject) DeleteMessage(ctx context.Context, messageId string) error {
	builder := storestate.Builder{}
	builder.Delete(CollectionName, messageId)
	_, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) EditMessage(ctx context.Context, messageId string, newMessage *chatmodel.Message) error {
	err := newMessage.Validate()
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	obj := arena.NewObject()
	newMessage.MarshalAnyenc(obj, arena)

	builder := storestate.Builder{}
	err = builder.Modify(CollectionName, messageId, []string{chatmodel.ContentKey}, pb.ModifyOp_Set, obj.Get(chatmodel.ContentKey))
	if err != nil {
		return fmt.Errorf("modify content: %w", err)
	}
	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) ToggleMessageReaction(ctx context.Context, messageId string, emoji string) (bool, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	hasReaction, err := s.repository.HasMyReaction(ctx, s.accountService.AccountID(), messageId, emoji)
	if err != nil {
		return false, fmt.Errorf("check reaction: %w", err)
	}

	builder := storestate.Builder{}

	if hasReaction {
		err = builder.Modify(CollectionName, messageId, []string{chatmodel.ReactionsKey, emoji}, pb.ModifyOp_Pull, arena.NewString(s.accountService.AccountID()))
		if err != nil {
			return false, fmt.Errorf("modify content: %w", err)
		}
	} else {
		err = builder.Modify(CollectionName, messageId, []string{chatmodel.ReactionsKey, emoji}, pb.ModifyOp_AddToSet, arena.NewString(s.accountService.AccountID()))
		if err != nil {
			return false, fmt.Errorf("modify content: %w", err)
		}
	}

	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return false, fmt.Errorf("push change: %w", err)
	}
	return !hasReaction, nil
}

func (s *storeObject) TryClose(objectTTL time.Duration) (res bool, err error) {
	if !s.locker.TryLock() {
		return false, nil
	}
	s.subscription.Lock()
	defer s.subscription.Unlock()
	isActive := s.subscription.IsActive()
	s.Unlock()

	if isActive {
		return false, nil
	}
	s.statService.RemoveProvider(s)
	return s.SmartBlock.TryClose(objectTTL)
}

func (s *storeObject) Close() error {
	s.componentCtxCancel()
	s.statService.RemoveProvider(s)
	return s.SmartBlock.Close()
}

func (s *storeObject) HandleSyncStatusUpdate(heads []string, status domain.ObjectSyncStatus, syncError domain.SyncError) {
	if status == (domain.ObjectSyncStatusSynced) {
		err := s.storeSource.MarkSeenHeads(s.componentCtx, diffManagerSyncStatus, heads)
		if err != nil {
			log.Error("mark sync status heads", zap.Error(err))
		}
	}
}

type treeSeenHeadsCollector struct {
	tree objecttree.ObjectTree
}

func newTreeSeenHeadsCollector(tree objecttree.ObjectTree) *treeSeenHeadsCollector {
	return &treeSeenHeadsCollector{
		tree: tree,
	}
}

func (c *treeSeenHeadsCollector) collectSeenHeads(ctx context.Context, afterOrderId string) ([]string, error) {
	var seenHeads []string
	err := c.tree.Storage().GetAfterOrder(ctx, "", func(ctx context.Context, change objecttree.StorageChange) (shouldContinue bool, err error) {
		if change.OrderId >= afterOrderId {
			return false, nil
		}

		seenHeads = slice.DiscardFromSlice(seenHeads, func(id string) bool {
			return slices.Contains(change.PrevIds, id)
		})
		if !slices.Contains(seenHeads, change.Id) {
			seenHeads = append(seenHeads, change.Id)
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect seen heads: %w", err)
	}
	return seenHeads, err
}
