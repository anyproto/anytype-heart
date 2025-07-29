package smartblock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"go.uber.org/zap"

	// nolint:misspell
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/util/anonymize"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/slice"
)

type ApplyFlag int

var (
	ErrSimpleBlockNotFound                         = errors.New("simple block not found")
	ErrCantInitExistingSmartblockWithNonEmptyState = errors.New("can't init existing smartblock with non-empty state")
	ErrApplyOnEmptyTreeDisallowed                  = errors.New("apply on empty tree disallowed")
)

const (
	NoHistory ApplyFlag = iota
	NoEvent
	NoRestrictions
	NoHooks
	DoSnapshot
	SkipIfNoChanges
	KeepInternalFlags
	IgnoreNoPermissions
	NotPushChanges // Used only for read-only actions like InitObject or OpenObject
	AllowApplyWithEmptyTree
)

type Hook int

type ApplyInfo struct {
	State             *state.State
	ParentDetails     *domain.Details
	Events            []simple.EventMessage
	Changes           []*pb.ChangeContent
	ApplyOtherObjects bool
}

type HookCallback func(info ApplyInfo) (err error)

const (
	HookOnNewState  Hook = iota
	HookBeforeApply      // runs before user changes will be applied, provides the state that can be changed
	HookAfterApply       // runs after changes applied from the user or externally via changeListener
	HookOnClose
	HookOnBlockClose
	HookOnStateRebuild
)

type key int

const CallerKey key = 0

var log = logging.Logger("anytype-mw-smartblock")

func New(
	space Space,
	currentParticipantId string,
	spaceIndex spaceindex.Store,
	objectStore objectstore.ObjectStore,
	indexer Indexer,
	eventSender event.Sender,
	spaceIdResolver idresolver.Resolver,
) SmartBlock {
	s := &smartBlock{
		currentParticipantId: currentParticipantId,
		space:                space,
		hooks:                map[Hook][]HookCallback{},
		hooksOnce:            map[string]struct{}{},
		Locker:               &sync.Mutex{},
		sessions:             map[string]session.Context{},

		spaceIndex:      spaceIndex,
		indexer:         indexer,
		eventSender:     eventSender,
		objectStore:     objectStore,
		spaceIdResolver: spaceIdResolver,
		lastDepDetails:  map[string]*domain.Details{},
	}
	return s
}

type Space interface {
	Id() string
	TreeBuilder() objecttreebuilder.TreeBuilder
	DerivedIDs() threads.DerivedSmartblockIds

	GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)
	DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)

	IsPersonal() bool

	Do(objectId string, apply func(sb SmartBlock) error) error
	DoLockedIfNotExists(objectID string, proc func() error) error // TODO Temporarily before rewriting favorites/archive mechanism
	TryRemove(objectId string) (bool, error)

	StoredIds() []string
	RefreshObjects(objectIds []string) (err error)
}

type SmartBlock interface {
	Tree() objecttree.ObjectTree
	Init(ctx *InitContext) (err error)
	Id() string
	SpaceID() string
	Type() smartblock.SmartBlockType
	UniqueKey() domain.UniqueKey
	Show() (obj *model.ObjectView, err error)
	RegisterSession(session.Context)
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	HasRelation(s *state.State, relationKey string) bool
	RemoveRelations(ctx session.Context, relationKeys []domain.RelationKey) (err error)
	SetVerticalAlign(ctx session.Context, align model.BlockVerticalAlign, ids ...string) error
	SetIsDeleted()
	IsDeleted() bool
	IsLocked() bool

	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	EnableLayouts()
	EnabledRelationAsDependentObjects()
	AddHook(f HookCallback, events ...Hook)
	AddHookOnce(id string, f HookCallback, events ...Hook)
	CheckSubscriptions() (changed bool)
	GetDocInfo() DocInfo
	Restrictions() restriction.Restrictions
	ObjectClose(ctx session.Context)
	ObjectCloseAllSessions()

	Space() Space

	ocache.Object
	state.Doc
	sync.Locker
	SetLocker(locker Locker)
}

type DocInfo struct {
	Id      string
	Space   Space
	Links   []string
	Heads   []string
	Creator string
	Type    domain.TypeKey
	Details *domain.Details

	SmartblockType smartblock.SmartBlockType
}

// TODO Maybe create constructor? Don't want to forget required fields
type InitContext struct {
	IsNewObject                  bool
	Source                       source.Source
	ObjectTypeKeys               []domain.TypeKey
	RelationKeys                 []domain.RelationKey
	RequiredInternalRelationKeys []domain.RelationKey // bundled relations that MUST be present in the state
	State                        *state.State
	Relations                    []*model.Relation
	ObjectStore                  objectstore.ObjectStore
	SpaceID                      string
	BuildOpts                    source.BuildOptions
	Ctx                          context.Context
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

type Locker interface {
	TryLock() bool
	sync.Locker
}

type Indexer interface {
	Index(info DocInfo, options ...IndexOption) error
	app.ComponentRunnable
}

type smartBlock struct {
	state.Doc
	objecttree.ObjectTree
	Locker
	currentParticipantId string
	depIds               []string // slice must be sorted
	sessions             map[string]session.Context
	undo                 undo.History
	source               source.Source
	lastDepDetails       map[string]*domain.Details
	restrictions         restriction.Restrictions
	isDeleted            bool
	enableLayouts        bool

	includeRelationObjectsAsDependents bool // used by some clients

	hooks     map[Hook][]HookCallback
	hooksOnce map[string]struct{}

	recordsSub      database.Subscription
	closeRecordsSub func()

	space Space

	// Deps
	spaceIndex      spaceindex.Store
	objectStore     objectstore.ObjectStore
	indexer         Indexer
	eventSender     event.Sender
	spaceIdResolver idresolver.Resolver
}

func (sb *smartBlock) SetLocker(locker Locker) {
	sb.Locker = locker
}

func (sb *smartBlock) Tree() objecttree.ObjectTree {
	return sb.ObjectTree
}

func (sb *smartBlock) HasRelation(s *state.State, key string) bool {
	return s.HasRelation(domain.RelationKey(key))
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) SpaceID() string {
	return sb.source.SpaceID()
}

func (sb *smartBlock) Space() Space {
	return sb.space
}

// UniqueKey returns the unique key for types that support it. For example, object types, relations and relation options
func (sb *smartBlock) UniqueKey() domain.UniqueKey {
	uk, _ := domain.NewUniqueKey(sb.Type(), sb.Doc.UniqueKeyInternal())
	return uk
}

func (sb *smartBlock) GetAndUnsetFileKeys() (keys []pb.ChangeFileKeys) {
	keys2 := sb.source.GetFileKeysSnapshot()
	for _, key := range keys2 {
		if key == nil {
			continue
		}
		keys = append(keys, pb.ChangeFileKeys{
			Hash: key.Hash,
			Keys: key.Keys,
		})
	}
	return
}

func (sb *smartBlock) Type() smartblock.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) ObjectTypeID() string {
	return sb.Doc.LocalDetails().GetString(bundle.RelationKeyType)
}

func (sb *smartBlock) Init(ctx *InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, bundle.RequiredInternalRelations...)
	if sb.Doc, err = ctx.Source.ReadDoc(ctx.Ctx, sb, ctx.State != nil); err != nil {
		return fmt.Errorf("reading document: %w", err)
	}

	sb.source = ctx.Source
	if provider, ok := sb.source.(source.ObjectTreeProvider); ok {
		sb.ObjectTree = provider.Tree()
	}
	sb.undo = undo.NewHistory(0)
	sb.restrictions = restriction.GetRestrictions(sb)
	if ctx.State != nil {
		// need to store file keys in case we have some new files in the state
		sb.storeFileKeys(ctx.State)
	}
	sb.Doc.BlocksInit(sb.Doc.(simple.DetailsService))

	if ctx.State == nil {
		ctx.State = sb.NewState()
		sb.storeFileKeys(sb.Doc)
	} else {
		if !sb.Doc.(*state.State).IsEmpty(true) {
			return ErrCantInitExistingSmartblockWithNonEmptyState
		}
		ctx.State.SetParent(sb.Doc.(*state.State))
	}

	injectRequiredRelations := func(s *state.State) {
		s.AddRelationKeys(bundle.RequiredInternalRelations...)
		s.AddRelationKeys(ctx.RequiredInternalRelationKeys...)
	}
	injectRequiredRelations(ctx.State)
	injectRequiredRelations(ctx.State.ParentState())

	ctx.State.AddRelationKeys(ctx.RelationKeys...)
	// Add bundled relations
	var relKeys []domain.RelationKey
	for k, _ := range ctx.State.Details().Iterate() {
		if bundle.HasRelation(k) {
			relKeys = append(relKeys, k)
		}
	}
	ctx.State.AddRelationKeys(relKeys...)
	if ctx.IsNewObject && ctx.State != nil {
		source.NewSubObjectsAndProfileLinksMigration(sb.Type(), sb.space, sb.currentParticipantId, sb.spaceIndex).Migrate(ctx.State)
	}

	if err = sb.injectLocalDetails(ctx.State); err != nil {
		return
	}
	sb.injectDerivedDetails(ctx.State, sb.SpaceID(), sb.Type())
	sb.resolveLayout(ctx.State)

	sb.AddHook(sb.sendObjectCloseEvent, HookOnClose, HookOnBlockClose)
	return
}

func (sb *smartBlock) sendObjectCloseEvent(_ ApplyInfo) error {
	sb.sendEvent(&pb.Event{
		ContextId: sb.Id(),
		Messages: []*pb.EventMessage{
			event.NewMessage(sb.SpaceID(), &pb.EventMessageValueOfObjectClose{
				ObjectClose: &pb.EventObjectClose{
					Id: sb.Id(),
				}}),
		}})
	return nil
}

// updateRestrictions refetch restrictions from restriction service and update them in the smartblock
func (sb *smartBlock) updateRestrictions() {
	r := restriction.GetRestrictions(sb)
	if sb.restrictions.Equal(r) {
		return
	}
	sb.restrictions = r
	sb.SendEvent([]*pb.EventMessage{
		event.NewMessage(sb.SpaceID(), &pb.EventMessageValueOfObjectRestrictionsSet{
			ObjectRestrictionsSet: &pb.EventObjectRestrictionsSet{Id: sb.Id(), Restrictions: r.Proto()},
		}),
	})
}

func (sb *smartBlock) SetIsDeleted() {
	sb.isDeleted = true
}

func (sb *smartBlock) IsDeleted() bool {
	return sb.isDeleted
}

func (sb *smartBlock) sendEvent(e *pb.Event) {
	for _, s := range sb.sessions {
		sb.eventSender.SendToSession(s.ID(), e)
	}
}

func (sb *smartBlock) SendEvent(msgs []*pb.EventMessage) {
	sb.sendEvent(&pb.Event{
		Messages:  msgs,
		ContextId: sb.Id(),
	})
}

func (sb *smartBlock) Restrictions() restriction.Restrictions {
	return sb.restrictions
}

func (sb *smartBlock) Show() (*model.ObjectView, error) {
	sb.updateRestrictions()

	details, err := sb.fetchMeta()
	if err != nil {
		return nil, err
	}

	undo, redo := sb.History().Counters()

	return &model.ObjectView{
		RootId:       sb.RootId(),
		Type:         sb.Type().ToProto(),
		Blocks:       sb.Blocks(),
		Details:      details,
		Restrictions: sb.restrictions.Proto(),
		History: &model.ObjectViewHistorySize{
			Undo: undo,
			Redo: redo,
		},
	}, nil
}

func (sb *smartBlock) fetchMeta() (details []*model.ObjectViewDetailsSet, err error) {
	if sb.closeRecordsSub != nil {
		sb.closeRecordsSub()
		sb.closeRecordsSub = nil
	}

	depIds := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true)
	sb.setDependentIDs(depIds)

	perSpace := sb.partitionIdsBySpace(sb.depIds)

	recordsCh := make(chan *domain.Details, 10)
	sb.recordsSub = database.NewSubscription(nil, recordsCh)

	var records []database.Record
	closers := make([]func(), 0, len(perSpace))

	for spaceId, perSpaceDepIds := range perSpace {
		spaceIndex := sb.objectStore.SpaceIndex(spaceId)

		recs, closeRecordsSub, err := spaceIndex.QueryByIdsAndSubscribeForChanges(perSpaceDepIds, sb.recordsSub)
		if err != nil {
			for _, closer := range closers {
				closer()
			}
			// datastore unavailable, cancel the subscription
			sb.recordsSub.Close()
			sb.closeRecordsSub = nil
			return nil, fmt.Errorf("subscribe: %w", err)
		}

		closers = append(closers, closeRecordsSub)
		records = append(records, recs...)
	}
	sb.closeRecordsSub = func() {
		for _, closer := range closers {
			closer()
		}
	}

	details = make([]*model.ObjectViewDetailsSet, 0, len(records)+1)

	// add self details
	details = append(details, &model.ObjectViewDetailsSet{
		Id:      sb.Id(),
		Details: sb.CombinedDetails().ToProto(),
	})

	for _, rec := range records {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      rec.Details.GetString(bundle.RelationKeyId),
			Details: rec.Details.ToProto(),
		})
	}
	go sb.metaListener(recordsCh)
	return
}

func (sb *smartBlock) partitionIdsBySpace(ids []string) map[string][]string {
	perSpace := map[string][]string{}
	for _, id := range ids {
		if dateObject, parseErr := dateutil.BuildDateObjectFromId(id); parseErr == nil {
			perSpace[sb.space.Id()] = append(perSpace[sb.space.Id()], dateObject.Id())
			continue
		}

		spaceId, err := sb.spaceIdResolver.ResolveSpaceID(id)
		if errors.Is(err, domain.ErrObjectNotFound) {
			perSpace[sb.space.Id()] = append(perSpace[sb.space.Id()], id)
			continue
		}

		if err != nil {
			perSpace[sb.space.Id()] = append(perSpace[sb.space.Id()], id)
			log.With("id", id).Warn("resolve space id", zap.Error(err))
			continue
		}
		perSpace[spaceId] = append(perSpace[spaceId], id)
	}
	return perSpace
}

func (sb *smartBlock) Lock() {
	sb.Locker.Lock()
}

func (sb *smartBlock) TryLock() bool {
	return sb.Locker.TryLock()
}

func (sb *smartBlock) Unlock() {
	sb.Locker.Unlock()
}

func (sb *smartBlock) metaListener(ch chan *domain.Details) {
	for {
		rec, ok := <-ch
		if !ok {
			return
		}
		sb.Lock()
		sb.onMetaChange(rec)
		sb.Unlock()
	}
}

func (sb *smartBlock) onMetaChange(details *domain.Details) {
	if details == nil {
		return
	}
	id := details.GetString(bundle.RelationKeyId)
	var msgs []*pb.EventMessage
	if v, exists := sb.lastDepDetails[id]; exists {
		diff, keysToUnset := domain.StructDiff(v, details)
		if id == sb.Id() {
			// if we've got update for ourselves, we are only interested in local-only details, because the rest details changes will be appended when applying records in the current sb
			diff = diff.CopyOnlyKeys(bundle.LocalRelationsKeys...)
		}

		msgs = append(msgs, state.StructDiffIntoEvents(sb.SpaceID(), id, diff, keysToUnset)...)
	} else {
		msgs = append(msgs, event.NewMessage(sb.SpaceID(), &pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id:      id,
				Details: details.ToProto(),
			},
		}))
	}
	sb.lastDepDetails[id] = details

	if len(msgs) == 0 {
		return
	}

	sb.sendEvent(&pb.Event{
		Messages:  msgs,
		ContextId: sb.Id(),
	})
}

// dependentSmartIds returns list of dependent objects in this order: Simple blocks(Link, mentions in Text), Relations. Both of them are returned in the order of original blocks/relations
func (sb *smartBlock) dependentSmartIds(includeRelations, includeObjTypes, includeCreatorModifier bool) (ids []string) {
	return objectlink.DependentObjectIDs(sb.Doc.(*state.State), sb.Space(), sb.spaceIndex, objectlink.Flags{
		Blocks:                   true,
		Details:                  true,
		Relations:                includeRelations,
		Types:                    includeObjTypes,
		CreatorModifierWorkspace: includeCreatorModifier,
	})
}

func (sb *smartBlock) RegisterSession(ctx session.Context) {
	sb.sessions[ctx.ID()] = ctx
}

func (sb *smartBlock) IsLocked() bool {
	var activeCount int
	for _, s := range sb.sessions {
		if sb.eventSender.IsActive(s.ID()) {
			activeCount++
		}
	}
	return activeCount > 0
}

func (sb *smartBlock) EnableLayouts() {
	sb.enableLayouts = true
}

func (sb *smartBlock) IsLayoutsEnabled() bool {
	return sb.enableLayouts
}

func (sb *smartBlock) EnabledRelationAsDependentObjects() {
	sb.includeRelationObjectsAsDependents = true
}

func (sb *smartBlock) Apply(s *state.State, flags ...ApplyFlag) (err error) {
	if sb.IsDeleted() {
		return domain.ErrObjectIsDeleted
	}
	var (
		sendEvent               = true
		addHistory              = true
		doSnapshot              = false
		checkRestrictions       = true
		hooks                   = true
		skipIfNoChanges         = false
		keepInternalFlags       = false
		ignoreNoPermissions     = false
		notPushChanges          = false
		allowApplyWithEmptyTree = false
	)
	for _, f := range flags {
		switch f {
		case NoEvent:
			sendEvent = false
		case NoHistory:
			addHistory = false
		case DoSnapshot:
			doSnapshot = true
		case NoRestrictions:
			checkRestrictions = false
		case NoHooks:
			hooks = false
		case SkipIfNoChanges:
			skipIfNoChanges = true
		case KeepInternalFlags:
			keepInternalFlags = true
		case IgnoreNoPermissions:
			ignoreNoPermissions = true
		case NotPushChanges:
			notPushChanges = true
		case AllowApplyWithEmptyTree:
			allowApplyWithEmptyTree = true
		}
	}
	if sb.ObjectTree != nil &&
		len(sb.ObjectTree.Heads()) == 1 &&
		sb.ObjectTree.Heads()[0] == sb.ObjectTree.Id() &&
		!allowApplyWithEmptyTree &&
		sb.Type() != smartblock.SmartBlockTypeChatDerivedObject &&
		sb.Type() != smartblock.SmartBlockTypeAccountObject {
		// protection for applying migrations on empty tree
		log.With("sbType", sb.Type().String(), "objectId", sb.Id()).Warnf("apply on empty tree discarded")
		return ErrApplyOnEmptyTreeDisallowed
	}

	// Inject derived details to make sure we have consistent state.
	// For example, we have to set ObjectTypeID into Type relation according to ObjectTypeKey from the state
	sb.injectDerivedDetails(s, sb.SpaceID(), sb.Type())
	sb.resolveLayout(s)

	if hooks {
		if err = sb.execHooks(HookBeforeApply, ApplyInfo{State: s}); err != nil {
			return nil
		}
	}
	if checkRestrictions && s.ParentState() != nil {
		if err = s.ParentState().CheckRestrictions(); err != nil {
			return
		}
	}

	var lastModified = time.Now()
	if s.ParentState() != nil && s.ParentState().IsTheHeaderChange() {
		// for the first change allow to set the last modified date from the state
		// this used for the object imports
		lastModifiedFromState := s.LocalDetails().GetInt64(bundle.RelationKeyLastModifiedDate)
		if lastModifiedFromState > 0 {
			lastModified = time.Unix(lastModifiedFromState, 0)
		}

		if existingCreatedDate := s.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate); existingCreatedDate == 0 || existingCreatedDate > lastModified.Unix() {
			// this can happen if we don't have creation date in the root change
			s.SetLocalDetail(bundle.RelationKeyCreatedDate, domain.Int64(lastModified.Unix()))
		}
	}

	if !keepInternalFlags {
		removeInternalFlags(s)
	}

	var (
		migrationVersionUpdated = true
		parent                  = s.ParentState()
	)

	if parent != nil {
		migrationVersionUpdated = s.MigrationVersion() != parent.MigrationVersion()
	}

	msgs, act, err := state.ApplyState(sb.SpaceID(), s, sb.enableLayouts)
	if err != nil {
		return
	}

	// we may have layout changed, so we need to update restrictions
	sb.updateRestrictions()
	sb.setRestrictionsDetail(s)

	st := sb.Doc.(*state.State)

	changes := st.GetChanges()
	var changeId string
	if skipIfNoChanges && len(changes) == 0 && !migrationVersionUpdated {
		if hasDetailsMsgs(msgs) {
			// means we have only local details changed, so lets index but skip full text
			sb.runIndexer(st, SkipFullTextIfHeadsNotChanged)
		} else {
			// we may skip indexing in case we are sure that we have previously indexed the same version of object
			sb.runIndexer(st, SkipIfHeadsNotChanged)
		}
		return nil
	}
	pushChange := func() error {
		if notPushChanges {
			return nil
		}
		if !sb.source.ReadOnly() {
			// We can set details directly in object's state, they'll be indexed correctly
			st.SetLocalDetail(bundle.RelationKeyLastModifiedBy, domain.String(sb.currentParticipantId))
			st.SetLocalDetail(bundle.RelationKeyLastModifiedDate, domain.Int64(lastModified.Unix()))
		}
		fileDetailsKeys := st.FileRelationKeys(sb.spaceIndex)
		var fileDetailsKeysFiltered []domain.RelationKey
		for _, ch := range changes {
			if ds := ch.GetDetailsSet(); ds != nil {
				if slice.FindPos(fileDetailsKeys, domain.RelationKey(ds.Key)) != -1 {
					fileDetailsKeysFiltered = append(fileDetailsKeysFiltered, domain.RelationKey(ds.Key))
				}
			}
		}
		pushChangeParams := source.PushChangeParams{
			Time:              lastModified,
			State:             st,
			Changes:           changes,
			FileChangedHashes: getChangedFileHashes(s, fileDetailsKeysFiltered, act),
			DoSnapshot:        doSnapshot,
		}
		changeId, err = sb.source.PushChange(pushChangeParams)
		// For read-only mode
		if errors.Is(err, list.ErrInsufficientPermissions) && ignoreNoPermissions {
			return nil
		}
		if err != nil {
			return err
		}

		if changeId != "" {
			sb.Doc.(*state.State).SetChangeId(changeId)
		}
		return nil
	}

	if !act.IsEmpty() {
		if len(changes) == 0 && !doSnapshot {
			log.With("sbType", sb.Type().String()).Errorf("apply 0 changes %s: %v", st.RootId(), anonymize.Events(msgsToEvents(msgs)))
		}
		err = pushChange()
		if err != nil {
			return err
		}
		if sb.undo != nil && addHistory {
			if !sb.source.ReadOnly() {
				act.Group = s.GroupId()
				sb.undo.Add(act)
			}
		}
	} else if hasChangesToPush(changes) || migrationVersionUpdated { // TODO: change to len(changes) > 0
		// log.Errorf("sb apply %s: store changes %s", sb.Id(), pbtypes.Sprint(&pb.Change{Content: changes}))
		err = pushChange()
		if err != nil {
			return err
		}
	}

	if changeId == "" && len(msgs) == 0 {
		// means we probably don't have any actual change being made
		// in case the heads are not changed, we may skip indexing
		sb.runIndexer(st, SkipIfHeadsNotChanged)
	} else {
		sb.runIndexer(st)
	}

	if sendEvent {
		events := msgsToEvents(msgs)
		if ctx := s.Context(); ctx != nil {
			ctx.SetMessages(sb.Id(), events)
		} else {
			sb.sendEvent(&pb.Event{
				Messages:  events,
				ContextId: sb.RootId(),
			})
		}
	}

	if sb.hasDepIds(&act) {
		sb.CheckSubscriptions()
	}
	if hooks {
		var parentDetails *domain.Details
		if act.Details != nil {
			parentDetails = act.Details.Before
		}
		if e := sb.execHooks(HookAfterApply, ApplyInfo{
			State:             sb.Doc.(*state.State),
			ParentDetails:     parentDetails,
			Events:            msgs,
			Changes:           changes,
			ApplyOtherObjects: true,
		}); e != nil {
			log.With("objectID", sb.Id()).Warnf("after apply execHooks error: %v", e)
		}
	}

	return
}

func (sb *smartBlock) ResetToVersion(s *state.State) (err error) {
	source.NewSubObjectsAndProfileLinksMigration(sb.Type(), sb.space, sb.currentParticipantId, sb.spaceIndex).Migrate(s)
	s.SetParent(sb.Doc.(*state.State))
	sb.storeFileKeys(s)
	sb.injectLocalDetails(s)
	if err = sb.Apply(s, NoHistory, DoSnapshot, NoRestrictions); err != nil {
		return
	}
	if sb.undo != nil {
		sb.undo.Reset()
	}
	return
}

func (sb *smartBlock) CheckSubscriptions() (changed bool) {
	depIDs := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true)
	changed = sb.setDependentIDs(depIDs)

	if sb.recordsSub == nil {
		return true
	}
	newIDs := sb.recordsSub.Subscribe(sb.depIds)

	perSpace := sb.partitionIdsBySpace(newIDs)

	for spaceId, ids := range perSpace {
		spaceIndex := sb.objectStore.SpaceIndex(spaceId)
		records, err := spaceIndex.QueryByIds(ids)
		if err != nil {
			log.Errorf("queryById error: %v", err)
		}
		for _, rec := range records {
			sb.onMetaChange(rec.Details)
		}
	}

	return true
}

func (sb *smartBlock) setDependentIDs(depIDs []string) (changed bool) {
	sort.Strings(depIDs)
	if slice.SortedEquals(sb.depIds, depIDs) {
		return false
	}
	// TODO Use algo for sorted strings
	removed, _ := slice.DifferenceRemovedAdded(sb.depIds, depIDs)
	for _, id := range removed {
		delete(sb.lastDepDetails, id)
	}
	sb.depIds = depIDs
	return true
}

func (sb *smartBlock) NewState() *state.State {
	s := sb.Doc.NewState().SetNoObjectType(sb.Type() == smartblock.SmartBlockTypeArchive)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) NewStateCtx(ctx session.Context) *state.State {
	s := sb.Doc.NewStateCtx(ctx).SetNoObjectType(sb.Type() == smartblock.SmartBlockTypeArchive)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) History() undo.History {
	return sb.undo
}

func (sb *smartBlock) SetVerticalAlign(ctx session.Context, align model.BlockVerticalAlign, ids ...string) (err error) {
	s := sb.NewStateCtx(ctx)
	for _, id := range ids {
		if b := s.Get(id); b != nil {
			b.Model().VerticalAlign = align
		}
	}
	return sb.Apply(s)
}

func (sb *smartBlock) RemoveRelations(ctx session.Context, relationIds []domain.RelationKey) (err error) {
	st := sb.NewStateCtx(ctx)
	st.RemoveRelation(relationIds...)

	return sb.Apply(st)
}

func (sb *smartBlock) StateAppend(f func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error {
	if sb.IsDeleted() {
		return domain.ErrObjectIsDeleted
	}
	s, changes, err := f(sb.Doc)
	if err != nil {
		return err
	}
	sb.updateRestrictions()
	sb.injectDerivedDetails(s, sb.SpaceID(), sb.Type())
	sb.resolveLayout(s)
	sb.execHooks(HookBeforeApply, ApplyInfo{State: s})
	msgs, act, err := state.ApplyState(sb.SpaceID(), s, sb.enableLayouts)
	if err != nil {
		return err
	}
	log.Infof("changes: stateAppend: %d events", len(msgs))

	if len(msgs) > 0 {
		sb.sendEvent(&pb.Event{
			Messages:  msgsToEvents(msgs),
			ContextId: sb.Id(),
		})
	}
	sb.storeFileKeys(s)
	if sb.hasDepIds(&act) || isBacklinksChanged(msgs) {
		sb.CheckSubscriptions()
	}
	sb.runIndexer(s)
	var parentDetails *domain.Details
	if s.ParentState() != nil {
		parentDetails = s.ParentState().Details()
	}
	if err = sb.execHooks(HookAfterApply, ApplyInfo{
		State:         s,
		ParentDetails: parentDetails,
		Events:        msgs,
		Changes:       changes,
	}); err != nil {
		log.Errorf("failed to execute smartblock hooks after apply on StateAppend: %v", err)
	}

	return nil
}

// TODO: need to test StateRebuild
func (sb *smartBlock) StateRebuild(d state.Doc) (err error) {
	if sb.IsDeleted() {
		return domain.ErrObjectIsDeleted
	}
	sb.updateRestrictions()
	sb.injectDerivedDetails(d.(*state.State), sb.SpaceID(), sb.Type())
	sb.resolveLayout(d.(*state.State))
	err = sb.injectLocalDetails(d.(*state.State))
	if err != nil {
		log.Errorf("failed to inject local details in StateRebuild: %v", err)
	}
	d.(*state.State).SetParent(sb.Doc.(*state.State))
	// todo: make store diff
	sb.execHooks(HookBeforeApply, ApplyInfo{State: d.(*state.State)})
	msgs, _, err := state.ApplyState(sb.SpaceID(), d.(*state.State), sb.enableLayouts)
	log.Infof("changes: stateRebuild: %d events", len(msgs))
	if err != nil {
		// can't make diff - reopen doc
		sb.Show()
	} else {
		if len(msgs) > 0 {
			sb.sendEvent(&pb.Event{
				Messages:  msgsToEvents(msgs),
				ContextId: sb.Id(),
			})
		}
	}
	sb.storeFileKeys(d)
	sb.CheckSubscriptions()
	sb.runIndexer(sb.Doc.(*state.State))
	applyInfo := ApplyInfo{State: sb.Doc.(*state.State), Events: msgs, Changes: d.(*state.State).GetChanges()}
	sb.execHooks(HookAfterApply, applyInfo)
	err = sb.execHooks(HookOnStateRebuild, applyInfo)
	if err != nil {
		log.With("objectId", sb.Id(), "error", err).Error("executing hook on state rebuild")
	}
	return nil
}

func (sb *smartBlock) ObjectClose(ctx session.Context) {
	sb.execHooks(HookOnBlockClose, ApplyInfo{State: sb.Doc.(*state.State)})
	delete(sb.sessions, ctx.ID())
}

func (sb *smartBlock) ObjectCloseAllSessions() {
	sb.execHooks(HookOnBlockClose, ApplyInfo{State: sb.Doc.(*state.State)})
	sb.sessions = make(map[string]session.Context)
}

func (sb *smartBlock) TryClose(objectTTL time.Duration) (res bool, err error) {
	if !sb.Locker.TryLock() {
		return false, nil
	}
	if sb.IsLocked() {
		sb.Unlock()
		return false, nil
	}
	return true, sb.closeLocked()
}

func (sb *smartBlock) Close() (err error) {
	sb.Lock()
	return sb.closeLocked()
}

func (sb *smartBlock) closeLocked() (err error) {
	sb.execHooks(HookOnClose, ApplyInfo{State: sb.Doc.(*state.State)})
	if sb.closeRecordsSub != nil {
		sb.closeRecordsSub()
		sb.closeRecordsSub = nil
	}
	sb.Unlock()

	sb.source.Close()
	log.Debugf("close smartblock %v", sb.Id())
	return
}

func (sb *smartBlock) hasDepIds(act *undo.Action) bool {
	if act == nil {
		return true
	}
	if act.ObjectTypes != nil {
		return true
	}
	if act.Details != nil {
		if act.Details.Before == nil || act.Details.After == nil {
			return true
		}

		for k, after := range act.Details.After.Iterate() {
			rel, err := sb.spaceIndex.GetRelationLink(k.String())
			if err != nil || rel == nil {
				continue
			}
			if slices.Contains([]model.RelationFormat{
				model.RelationFormat_status,
				model.RelationFormat_tag,
				model.RelationFormat_object,
				model.RelationFormat_file,
			}, rel.Format) || isCoverId(k) {
				before := act.Details.Before.Get(k)
				// Check that value is actually changed
				if !before.Ok() || !before.Equal(after) {
					return true
				}
			}
		}
	}

	for _, edit := range act.Change {
		if ls, ok := edit.After.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
		if ls, ok := edit.Before.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
	}
	for _, add := range act.Add {
		if ls, ok := add.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
	}
	for _, rem := range act.Remove {
		if ls, ok := rem.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
	}
	return false
}

// We need to provide the author's name if we download an image with unsplash
// for the cover image inside an inner smartblock
// CoverId can be either a file, a gradient, an icon, or a color
func isCoverId(key domain.RelationKey) bool {
	return key == bundle.RelationKeyCoverId
}

func getChangedFileHashes(s *state.State, fileDetailKeys []domain.RelationKey, act undo.Action) (hashes []string) {
	for _, nb := range act.Add {
		if fh, ok := nb.(simple.FileHashes); ok {
			hashes = fh.FillFileHashes(hashes)
		}
	}
	for _, eb := range act.Change {
		if fh, ok := eb.After.(simple.FileHashes); ok {
			hashes = fh.FillFileHashes(hashes)
		}
	}
	if act.Details != nil {
		det := act.Details.After
		if det != nil {
			for _, detKey := range fileDetailKeys {
				if list := det.GetStringList(detKey); len(list) > 0 {
					hashes = append(hashes, list...)
				} else if s := det.GetString(detKey); s != "" {
					hashes = append(hashes, s)
				}
			}
		}
	}

	// we may have temporary links in files, we need to ignore them
	// todo: remove after fixing of how import works
	return slice.FilterCID(hashes)
}

func (sb *smartBlock) storeFileKeys(doc state.Doc) {
	if doc == nil {
		return
	}
	keys := doc.GetAndUnsetFileKeys()
	if len(keys) == 0 {
		return
	}
	fileKeys := make([]domain.FileEncryptionKeys, len(keys))
	for i, k := range keys {
		fileKeys[i] = domain.FileEncryptionKeys{
			FileId:         domain.FileId(k.Hash),
			EncryptionKeys: k.Keys,
		}
	}
	if err := sb.objectStore.AddFileKeys(fileKeys...); err != nil {
		log.Warnf("can't store file keys: %v", err)
	}
}

func (sb *smartBlock) AddHook(f HookCallback, events ...Hook) {
	for _, e := range events {
		sb.hooks[e] = append(sb.hooks[e], f)
	}
}

// AddHookOnce adds hook only if it wasn't added before via this method with the same id
// it doesn't compare the list of events or the callback function
func (sb *smartBlock) AddHookOnce(id string, f HookCallback, events ...Hook) {
	if _, ok := sb.hooksOnce[id]; !ok {
		sb.AddHook(f, events...)
		sb.hooksOnce[id] = struct{}{}
	}
}

func (sb *smartBlock) execHooks(event Hook, info ApplyInfo) (err error) {
	for _, h := range sb.hooks[event] {
		if h != nil {
			if err = h(info); err != nil {
				return
			}
		}
	}
	return
}

func (sb *smartBlock) GetDocInfo() DocInfo {
	return sb.getDocInfo(sb.NewState())
}

func (sb *smartBlock) getDocInfo(st *state.State) DocInfo {
	creator := st.Details().GetString(bundle.RelationKeyCreator)

	// we don't want any hidden or internal relations here. We want to capture the meaningful outgoing links only
	links := sb.LocalDetails().GetStringList(bundle.RelationKeyLinks)
	// so links will have this order
	// 1. Simple blocks: links, mentions in the text
	// 2. Relations(format==Object)

	for _, link := range links {
		// sync backlinks of identity and profile objects in personal space
		if strings.HasPrefix(link, domain.ParticipantPrefix) && sb.space.IsPersonal() {
			links = append(links, sb.space.DerivedIDs().Profile)
			break
		}
	}

	// todo: heads in source and the state may be inconsistent?
	heads := sb.source.Heads()
	if len(heads) == 0 {
		lastChangeId := st.LocalDetails().GetString(bundle.RelationKeyLastChangeId)
		if lastChangeId != "" {
			heads = []string{lastChangeId}
		}
	}
	return DocInfo{
		Id:             sb.Id(),
		Space:          sb.Space(),
		Links:          links,
		Heads:          heads,
		Creator:        creator,
		Details:        sb.CombinedDetails(),
		Type:           sb.ObjectTypeKey(),
		SmartblockType: sb.Type(),
	}
}

func (sb *smartBlock) runIndexer(s *state.State, opts ...IndexOption) {
	docInfo := sb.getDocInfo(s)
	if err := sb.indexer.Index(docInfo, opts...); err != nil {
		log.Errorf("index object %s error: %s", sb.Id(), err)
	}
}

func removeInternalFlags(s *state.State) {
	flags := internalflag.NewFromState(s)

	// Run empty check only if any of these flags are present
	if flags.Has(model.InternalFlag_editorDeleteEmpty) || flags.Has(model.InternalFlag_editorSelectType) || flags.Has(model.InternalFlag_editorSelectTemplate) {
		if !s.IsEmpty(true) {
			flags.Remove(model.InternalFlag_editorDeleteEmpty)
		}
		flags.Remove(model.InternalFlag_editorSelectType)
		flags.Remove(model.InternalFlag_editorSelectTemplate)
		flags.AddToState(s)
	}
}

func (sb *smartBlock) setRestrictionsDetail(s *state.State) {
	currentRestrictions := restriction.NewObjectRestrictionsFromValue(s.LocalDetails().Get(bundle.RelationKeyRestrictions))
	if currentRestrictions.Equal(sb.Restrictions().Object) {
		return
	}

	s.SetLocalDetail(bundle.RelationKeyRestrictions, sb.Restrictions().Object.ToValue())

	if sb.Restrictions().Object.Check(model.Restrictions_Details) != nil &&
		sb.Restrictions().Object.Check(model.Restrictions_Blocks) != nil {
		s.SetDetail(bundle.RelationKeyIsReadonly, domain.Bool(true))
	} else if s.LocalDetails().GetBool(bundle.RelationKeyIsReadonly) {
		s.SetDetail(bundle.RelationKeyIsReadonly, domain.Bool(false))
	}
}

func msgsToEvents(msgs []simple.EventMessage) []*pb.EventMessage {
	events := make([]*pb.EventMessage, len(msgs))
	for i := range msgs {
		events[i] = msgs[i].Msg
	}
	return events
}

func ObjectApplyTemplate(sb SmartBlock, s *state.State, templates ...template.StateTransformer) (err error) {
	if s == nil {
		s = sb.NewState()
	}
	template.InitTemplate(s, templates...)

	return sb.Apply(s, NoHistory, NoEvent, NoRestrictions, SkipIfNoChanges)
}

func hasChangesToPush(changes []*pb.ChangeContent) bool {
	for _, ch := range changes {
		if isSuitableChanges(ch) {
			return true
		}
	}
	return false
}

func isSuitableChanges(ch *pb.ChangeContent) bool {
	return ch.GetStoreKeySet() != nil ||
		ch.GetStoreKeyUnset() != nil ||
		ch.GetStoreSliceUpdate() != nil ||
		ch.GetNotificationCreate() != nil ||
		ch.GetNotificationUpdate() != nil ||
		ch.GetDeviceUpdate() != nil ||
		ch.GetDeviceAdd() != nil
}

func hasDetailsMsgs(msgs []simple.EventMessage) bool {
	for _, msg := range msgs {
		if msg.Msg.GetObjectDetailsSet() != nil ||
			msg.Msg.GetObjectDetailsUnset() != nil ||
			msg.Msg.GetObjectDetailsAmend() != nil {
			return true
		}
	}
	return false
}

type IndexOptions struct {
	SkipIfHeadsNotChanged         bool
	SkipFullTextIfHeadsNotChanged bool
}

type IndexOption func(*IndexOptions)

func SkipIfHeadsNotChanged(o *IndexOptions) {
	o.SkipIfHeadsNotChanged = true
}

func SkipFullTextIfHeadsNotChanged(o *IndexOptions) {
	o.SkipFullTextIfHeadsNotChanged = true
}

type InitFunc = func(id string) *InitContext
