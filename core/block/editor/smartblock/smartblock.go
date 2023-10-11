package smartblock

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	// nolint:misspell
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/core/system_object/relationutils"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type ApplyFlag int

var (
	ErrSimpleBlockNotFound                         = errors.New("simple block not found")
	ErrCantInitExistingSmartblockWithNonEmptyState = errors.New("can't init existing smartblock with non-empty state")
	ErrIsDeleted                                   = errors.New("smartblock is deleted")
)

const (
	NoHistory ApplyFlag = iota
	NoEvent
	NoRestrictions
	NoHooks
	DoSnapshot
	SkipIfNoChanges
	KeepInternalFlags
)

type Hook int

type ApplyInfo struct {
	State   *state.State
	Events  []simple.EventMessage
	Changes []*pb.ChangeContent
}

type HookCallback func(info ApplyInfo) (err error)

const (
	HookOnNewState  Hook = iota
	HookBeforeApply      // runs before user changes will be applied, provides the state that can be changed
	HookAfterApply       // runs after changes applied from the user or externally via changeListener
	HookOnClose
	HookOnBlockClose
)

type key int

const CallerKey key = 0

var log = logging.Logger("anytype-mw-smartblock")

func New(
	coreService core.Service,
	fileService files.Service,
	restrictionService restriction.Service,
	objectStore objectstore.ObjectStore,
	systemObjectService system_object.Service,
	indexer Indexer,
	eventSender event.Sender,
) SmartBlock {
	s := &smartBlock{
		hooks:     map[Hook][]HookCallback{},
		hooksOnce: map[string]struct{}{},
		Locker:    &sync.Mutex{},
		sessions:  map[string]session.Context{},

		coreService:         coreService,
		fileService:         fileService,
		restrictionService:  restrictionService,
		objectStore:         objectStore,
		systemObjectService: systemObjectService,
		indexer:             indexer,
		eventSender:         eventSender,
	}
	return s
}

type SmartBlock interface {
	Tree() objecttree.ObjectTree
	Init(ctx *InitContext) (err error)
	Id() string
	SpaceID() string
	Type() smartblock.SmartBlockType
	Show() (obj *model.ObjectView, err error)
	RegisterSession(session.Context)
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	Relations(s *state.State) relationutils.Relations
	HasRelation(s *state.State, relationKey string) bool
	AddRelationLinks(ctx session.Context, relationIds ...string) (err error)
	AddRelationLinksToState(s *state.State, relationIds ...string) (err error)
	RemoveExtraRelations(ctx session.Context, relationKeys []string) (err error)
	SetVerticalAlign(ctx session.Context, align model.BlockVerticalAlign, ids ...string) error
	SetIsDeleted()
	IsDeleted() bool
	IsLocked() bool

	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	DisableLayouts()
	EnabledRelationAsDependentObjects()
	AddHook(f HookCallback, events ...Hook)
	AddHookOnce(id string, f HookCallback, events ...Hook)
	CheckSubscriptions() (changed bool)
	GetDocInfo() DocInfo
	Restrictions() restriction.Restrictions
	SetRestrictions(r restriction.Restrictions)
	ObjectClose(ctx session.Context)
	ObjectCloseAllSessions()
	FileRelationKeys(s *state.State) []string

	ocache.Object
	state.Doc
	sync.Locker
	SetLocker(locker Locker)
}

type DocInfo struct {
	Id         string
	SpaceID    string
	Links      []string
	FileHashes []string
	Heads      []string
	Creator    string
	Type       domain.TypeKey
	Details    *types.Struct
}

// TODO Maybe create constructor? Don't want to forget required fields
type InitContext struct {
	IsNewObject    bool
	Source         source.Source
	ObjectTypeKeys []domain.TypeKey
	RelationKeys   []string
	State          *state.State
	Relations      []*model.Relation
	Restriction    restriction.Service
	ObjectStore    objectstore.ObjectStore
	SpaceID        string
	BuildOpts      source.BuildOptions
	Ctx            context.Context
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
	Index(ctx context.Context, info DocInfo, options ...IndexOption) error
	app.ComponentRunnable
}

type smartBlock struct {
	state.Doc
	objecttree.ObjectTree
	Locker
	depIds         []string // slice must be sorted
	sessions       map[string]session.Context
	undo           undo.History
	source         source.Source
	lastDepDetails map[string]*pb.EventObjectDetailsSet
	restrictions   restriction.Restrictions
	isDeleted      bool
	disableLayouts bool

	includeRelationObjectsAsDependents bool // used by some clients

	hooks     map[Hook][]HookCallback
	hooksOnce map[string]struct{}

	recordsSub      database.Subscription
	closeRecordsSub func()

	// Deps
	coreService         core.Service
	fileService         files.Service
	restrictionService  restriction.Service
	objectStore         objectstore.ObjectStore
	systemObjectService system_object.Service
	indexer             Indexer
	eventSender         event.Sender
}

func (sb *smartBlock) SetLocker(locker Locker) {
	sb.Locker = locker
}

func (sb *smartBlock) Tree() objecttree.ObjectTree {
	return sb.ObjectTree
}

func (sb *smartBlock) FileRelationKeys(s *state.State) (fileKeys []string) {
	return s.FileRelationKeys()
}

func (sb *smartBlock) HasRelation(s *state.State, key string) bool {
	for _, rel := range s.GetRelationLinks() {
		if rel.Key == key {
			return true
		}
	}
	return false
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) SpaceID() string {
	return sb.source.SpaceID()
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

func (sb *smartBlock) ObjectStore() objectstore.ObjectStore {
	return sb.objectStore
}

func (sb *smartBlock) Type() smartblock.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) ObjectTypeID() string {
	return pbtypes.GetString(sb.Doc.Details(), bundle.RelationKeyType.String())
}

func (sb *smartBlock) Init(ctx *InitContext) (err error) {
	if sb.Doc, err = ctx.Source.ReadDoc(ctx.Ctx, sb, ctx.State != nil); err != nil {
		return fmt.Errorf("reading document: %w", err)
	}

	sb.source = ctx.Source
	if provider, ok := sb.source.(source.ObjectTreeProvider); ok {
		sb.ObjectTree = provider.Tree()
	}
	sb.undo = undo.NewHistory(0)
	sb.restrictions = sb.restrictionService.GetRestrictions(sb)
	sb.lastDepDetails = map[string]*pb.EventObjectDetailsSet{}
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

	if err = sb.AddRelationLinksToState(ctx.State, ctx.RelationKeys...); err != nil {
		return
	}

	// Add bundled relations
	var relKeys []domain.RelationKey
	for k := range ctx.State.Details().GetFields() {
		if _, err := bundle.GetRelation(domain.RelationKey(k)); err == nil {
			relKeys = append(relKeys, domain.RelationKey(k))
		}
	}
	ctx.State.AddBundledRelations(relKeys...)

	if err = sb.injectLocalDetails(ctx.State); err != nil {
		return
	}
	sb.injectDerivedDetails(ctx.State, sb.SpaceID(), sb.Type())
	return
}

// updateRestrictions refetch restrictions from restriction service and update them in the smartblock
func (sb *smartBlock) updateRestrictions() {
	restrictions := sb.restrictionService.GetRestrictions(sb)
	sb.SetRestrictions(restrictions)
}

func (sb *smartBlock) SetRestrictions(r restriction.Restrictions) {
	if sb.restrictions.Equal(r) {
		return
	}
	sb.restrictions = r
	sb.SendEvent([]*pb.EventMessage{{Value: &pb.EventMessageValueOfObjectRestrictionsSet{ObjectRestrictionsSet: &pb.EventObjectRestrictionsSet{Id: sb.Id(), Restrictions: r.Proto()}}}})
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
	sb.updateBackLinks(sb.LocalDetails())

	details, err := sb.fetchMeta()
	if err != nil {
		return nil, err
	}

	undo, redo := sb.History().Counters()

	// todo: sb.Relations() makes extra query to read objectType which we already have here
	// the problem is that we can have an extra object type of the set in the objectTypes so we can't reuse it
	return &model.ObjectView{
		RootId:        sb.RootId(),
		Type:          sb.Type().ToProto(),
		Blocks:        sb.Blocks(),
		Details:       details,
		RelationLinks: sb.GetRelationLinks(),
		Restrictions:  sb.restrictions.Proto(),
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
	recordsCh := make(chan *types.Struct, 10)
	sb.recordsSub = database.NewSubscription(nil, recordsCh)

	depIDs := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true, true)
	sb.setDependentIDs(depIDs)

	var records []database.Record
	records, sb.closeRecordsSub, err = sb.objectStore.QueryByIDAndSubscribeForChanges(sb.depIds, sb.recordsSub)
	if err != nil {
		// datastore unavailable, cancel the subscription
		sb.recordsSub.Close()
		sb.closeRecordsSub = nil
		return
	}

	details = make([]*model.ObjectViewDetailsSet, 0, len(records)+1)

	// add self details
	details = append(details, &model.ObjectViewDetailsSet{
		Id:      sb.Id(),
		Details: sb.CombinedDetails(),
	})

	for _, rec := range records {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()),
			Details: rec.Details,
		})
	}
	go sb.metaListener(recordsCh)
	return
}

func (sb *smartBlock) Lock() {
	sb.Locker.Lock()
}

func (sb *smartBlock) Unlock() {
	sb.Locker.Unlock()
}

func (sb *smartBlock) metaListener(ch chan *types.Struct) {
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

func (sb *smartBlock) onMetaChange(details *types.Struct) {
	if details == nil {
		return
	}
	id := pbtypes.GetString(details, bundle.RelationKeyId.String())
	msgs := []*pb.EventMessage{}
	if v, exists := sb.lastDepDetails[id]; exists {
		diff := pbtypes.StructDiff(v.Details, details)
		if id == sb.Id() {
			// if we've got update for ourselves, we are only interested in local-only details, because the rest details changes will be appended when applying records in the current sb
			diff = pbtypes.StructFilterKeys(diff, bundle.LocalRelationsKeys)
		}

		msgs = append(msgs, state.StructDiffIntoEvents(id, diff)...)
	} else {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfObjectDetailsSet{
				ObjectDetailsSet: &pb.EventObjectDetailsSet{
					Id:      id,
					Details: details,
				},
			},
		})
	}
	sb.lastDepDetails[id] = &pb.EventObjectDetailsSet{
		Id:      id,
		Details: details,
	}

	if len(msgs) == 0 {
		return
	}

	sb.sendEvent(&pb.Event{
		Messages:  msgs,
		ContextId: sb.Id(),
	})
}

// dependentSmartIds returns list of dependent objects in this order: Simple blocks(Link, mentions in Text), Relations. Both of them are returned in the order of original blocks/relations
func (sb *smartBlock) dependentSmartIds(includeRelations, includeObjTypes, includeCreatorModifier, _ bool) (ids []string) {
	return objectlink.DependentObjectIDs(sb.Doc.(*state.State), sb.systemObjectService, true, true, includeRelations, includeObjTypes, includeCreatorModifier)
}

func (sb *smartBlock) navigationalLinks(s *state.State) []string {
	includeDetails := true
	includeRelations := sb.includeRelationObjectsAsDependents

	var ids []string

	if !internalflag.NewFromState(s).Has(model.InternalFlag_collectionDontIndexLinks) {
		// flag used when importing a large set of objects
		ids = append(ids, s.GetStoreSlice(template.CollectionStoreKey)...)
	}

	err := s.Iterate(func(b simple.Block) (isContinue bool) {
		if f := b.Model().GetFile(); f != nil {
			if f.Hash != "" && f.Type != model.BlockContentFile_Image {
				ids = append(ids, f.Hash)
			}
			return true
		}
		// Include only link to target object
		if dv := b.Model().GetDataview(); dv != nil {
			if dv.TargetObjectId != "" {
				ids = append(ids, dv.TargetObjectId)
			}

			return true
		}

		if ls, ok := b.(linkSource); ok {
			ids = ls.FillSmartIds(ids)
		}
		return true
	})
	if err != nil {
		log.With("objectID", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
	}

	var det *types.Struct
	if includeDetails {
		det = s.CombinedDetails()
	}

	for _, rel := range s.GetRelationLinks() {
		if includeRelations {
			relId, err := sb.systemObjectService.GetRelationIdByKey(context.TODO(), sb.SpaceID(), domain.RelationKey(rel.Key))
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to derive object id for relation: %s", err)
				continue
			}
			ids = append(ids, relId)
		}
		if !includeDetails {
			continue
		}

		if rel.Format != model.RelationFormat_object {
			continue
		}

		if bundle.IsSystemRelation(domain.RelationKey(rel.Key)) {
			continue
		}

		// Do not include hidden relations. Only bundled relations can be hidden, so we don't need
		// to request relations from object store.
		if r, err := bundle.GetRelation(domain.RelationKey(rel.Key)); err == nil && r.Hidden {
			continue
		}

		// Add all object relation values as dependents
		for _, targetID := range pbtypes.GetStringList(det, rel.Key) {
			if targetID != "" {
				ids = append(ids, targetID)
			}
		}
	}

	return lo.Uniq(ids)
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

func (sb *smartBlock) DisableLayouts() {
	sb.disableLayouts = true
}

func (sb *smartBlock) EnabledRelationAsDependentObjects() {
	sb.includeRelationObjectsAsDependents = true
}

func (sb *smartBlock) Apply(s *state.State, flags ...ApplyFlag) (err error) {
	startTime := time.Now()
	if sb.IsDeleted() {
		return ErrIsDeleted
	}
	var (
		sendEvent         = true
		addHistory        = true
		doSnapshot        = false
		checkRestrictions = true
		hooks             = true
		skipIfNoChanges   = false
		keepInternalFlags = false
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
		}
	}

	// Inject derived details to make sure we have consistent state.
	// For example, we have to set ObjectTypeID into Type relation according to ObjectTypeKey from the state
	sb.injectDerivedDetails(s, sb.SpaceID(), sb.Type())

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
		// in case it is the first change, allow to explicitly set the last modified time
		// this case is used when we import existing data from other sources and want to preserve the original dates
		if err != nil {
			log.Errorf("failed to get creation info: %s", err)
		} else {
			lastModified = time.Unix(pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyLastModifiedDate.String()), 0)
		}
	}
	sb.beforeStateApply(s)

	if !keepInternalFlags {
		removeInternalFlags(s)
	}

	// this one will be reverted in case we don't have any actual change being made
	s.SetLastModified(lastModified.Unix(), sb.coreService.PredefinedObjects(sb.SpaceID()).Profile)

	beforeApplyStateTime := time.Now()

	migrationVersionUpdated := true
	if parent := s.ParentState(); parent != nil {
		migrationVersionUpdated = s.MigrationVersion() != parent.MigrationVersion()
	}

	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return
	}

	// we may have layout changed, so we need to update restrictions
	sb.updateRestrictions()
	sb.setRestrictionsDetail(s)

	afterApplyStateTime := time.Now()
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
		fileDetailsKeys := sb.FileRelationKeys(st)
		var fileDetailsKeysFiltered []string
		for _, ch := range changes {
			if ds := ch.GetDetailsSet(); ds != nil {
				if slice.FindPos(fileDetailsKeys, ds.Key) != -1 {
					fileDetailsKeysFiltered = append(fileDetailsKeysFiltered, ds.Key)
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
			log.Errorf("apply 0 changes %s: %v", st.RootId(), msgs)
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
	} else if hasStoreChanges(changes) || migrationVersionUpdated { // TODO: change to len(changes) > 0
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

	afterPushChangeTime := time.Now()
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

	if hasDepIds(sb.GetRelationLinks(), &act) {
		sb.CheckSubscriptions()
	}
	afterReportChangeTime := time.Now()
	if hooks {
		if e := sb.execHooks(HookAfterApply, ApplyInfo{State: sb.Doc.(*state.State), Events: msgs, Changes: changes}); e != nil {
			log.With("objectID", sb.Id()).Warnf("after apply execHooks error: %v", e)
		}
	}
	afterApplyHookTime := time.Now()

	metrics.SharedClient.RecordEvent(metrics.StateApply{
		BeforeApplyMs:  beforeApplyStateTime.Sub(startTime).Milliseconds(),
		StateApplyMs:   afterApplyStateTime.Sub(beforeApplyStateTime).Milliseconds(),
		PushChangeMs:   afterPushChangeTime.Sub(afterApplyStateTime).Milliseconds(),
		ReportChangeMs: afterReportChangeTime.Sub(afterPushChangeTime).Milliseconds(),
		ApplyHookMs:    afterApplyHookTime.Sub(afterReportChangeTime).Milliseconds(),
		ObjectId:       sb.Id(),
	})

	return
}

func (sb *smartBlock) ResetToVersion(s *state.State) (err error) {
	s.SetParent(sb.Doc.(*state.State))
	sb.storeFileKeys(s)
	sb.injectLocalDetails(s)
	sb.injectDerivedDetails(s, sb.SpaceID(), sb.Type())
	if err = sb.Apply(s, NoHistory, DoSnapshot, NoRestrictions); err != nil {
		return
	}
	if sb.undo != nil {
		sb.undo.Reset()
	}
	return
}

func (sb *smartBlock) CheckSubscriptions() (changed bool) {
	depIDs := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true, true)
	changed = sb.setDependentIDs(depIDs)

	if sb.recordsSub == nil {
		return true
	}
	newIDs := sb.recordsSub.Subscribe(sb.depIds)
	records, err := sb.objectStore.QueryByID(newIDs)
	if err != nil {
		log.Errorf("queryById error: %v", err)
	}
	for _, rec := range records {
		sb.onMetaChange(rec.Details)
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

func (sb *smartBlock) Anytype() core.Service {
	return sb.coreService
}

func (sb *smartBlock) AddRelationLinks(ctx session.Context, relationKeys ...string) (err error) {
	s := sb.NewStateCtx(ctx)
	if err = sb.AddRelationLinksToState(s, relationKeys...); err != nil {
		return
	}
	return sb.Apply(s)
}

func (sb *smartBlock) AddRelationLinksToState(s *state.State, relationKeys ...string) (err error) {
	if len(relationKeys) == 0 {
		return
	}
	relations, err := sb.systemObjectService.FetchRelationByKeys(sb.SpaceID(), relationKeys...)
	if err != nil {
		return
	}
	links := make([]*model.RelationLink, 0, len(relations))
	for _, r := range relations {
		links = append(links, r.RelationLink())
	}
	s.AddRelationLinks(links...)
	return
}

func (sb *smartBlock) injectLinksDetails(s *state.State) {
	links := sb.navigationalLinks(s)
	links = slice.Remove(links, sb.Id())
	// todo: we need to move it to the injectDerivedDetails, but we don't call it now on apply
	s.SetLocalDetail(bundle.RelationKeyLinks.String(), pbtypes.StringList(links))
}

func (sb *smartBlock) injectLocalDetails(s *state.State) error {
	details, err := sb.getDetailsFromStore()
	if err != nil {
		return err
	}

	details, hasPendingLocalDetails := sb.appendPendingDetails(details)

	// inject also derived keys, because it may be a good idea to have created date and creator cached,
	// so we don't need to traverse changes every time
	keys := slices.Clone(bundle.LocalRelationsKeys) // Use Clone to avoid side effects on the bundle.LocalRelationsKeys slice
	keys = append(keys, bundle.DerivedRelationsKeys...)

	localDetailsFromStore := pbtypes.StructFilterKeys(details, keys)
	sb.updateBackLinks(localDetailsFromStore)

	localDetailsFromState := pbtypes.StructFilterKeys(s.LocalDetails(), keys)
	if pbtypes.StructEqualIgnore(localDetailsFromState, localDetailsFromStore, nil) {
		return nil
	}

	s.InjectLocalDetails(localDetailsFromStore)
	if p := s.ParentState(); p != nil && !hasPendingLocalDetails {
		// inject for both current and parent state
		p.InjectLocalDetails(localDetailsFromStore)
	}

	return sb.injectCreationInfo(s)
}

func (sb *smartBlock) getDetailsFromStore() (*types.Struct, error) {
	storedDetails, err := sb.objectStore.GetDetails(sb.Id())
	if err != nil || storedDetails == nil {
		return nil, err
	}
	return pbtypes.CopyStruct(storedDetails.GetDetails()), nil
}

func (sb *smartBlock) updateBackLinks(details *types.Struct) {
	backLinks, err := sb.objectStore.GetInboundLinksByID(sb.Id())
	if err != nil {
		log.With("objectID", sb.Id()).Errorf("failed to get inbound links from object store: %s", err.Error())
		return
	}
	details.Fields[bundle.RelationKeyBacklinks.String()] = pbtypes.StringList(backLinks)
}

func (sb *smartBlock) appendPendingDetails(details *types.Struct) (resultDetails *types.Struct, hasPendingLocalDetails bool) {
	// Consume pending details
	err := sb.objectStore.UpdatePendingLocalDetails(sb.Id(), func(pending *types.Struct) (*types.Struct, error) {
		if len(pending.GetFields()) > 0 {
			hasPendingLocalDetails = true
		}
		details = pbtypes.StructMerge(details, pending, false)
		return nil, nil
	})
	if err != nil {
		log.With("objectID", sb.Id()).
			With("sbType", sb.Type()).Errorf("failed to update pending details: %v", err)
	}
	return details, hasPendingLocalDetails
}

func (sb *smartBlock) injectCreationInfo(s *state.State) error {
	if pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyCreator.String()) != "" && pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyCreatedDate.String()) != 0 {
		return nil
	}
	provider, conforms := sb.source.(source.CreationInfoProvider)
	if !conforms {
		return nil
	}

	creator, createdDate, err := provider.GetCreationInfo()
	if err != nil {
		return err
	}

	if creator != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(creator))
	}

	if createdDate != 0 {
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Float64(float64(createdDate)))
	}

	return nil
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

func (sb *smartBlock) RemoveExtraRelations(ctx session.Context, relationIds []string) (err error) {
	st := sb.NewStateCtx(ctx)
	st.RemoveRelation(relationIds...)

	return sb.Apply(st)
}

func (sb *smartBlock) StateAppend(f func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error {
	if sb.IsDeleted() {
		return ErrIsDeleted
	}
	s, changes, err := f(sb.Doc)
	if err != nil {
		return err
	}
	sb.updateRestrictions()
	sb.injectDerivedDetails(s, sb.SpaceID(), sb.Type())
	sb.execHooks(HookBeforeApply, ApplyInfo{State: s})
	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
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
	if hasDepIds(sb.GetRelationLinks(), &act) {
		sb.CheckSubscriptions()
	}
	sb.runIndexer(s)
	sb.execHooks(HookAfterApply, ApplyInfo{State: s, Events: msgs, Changes: changes})

	return nil
}

// TODO: need to test StateRebuild
func (sb *smartBlock) StateRebuild(d state.Doc) (err error) {
	if sb.IsDeleted() {
		return ErrIsDeleted
	}
	sb.updateRestrictions()
	sb.injectDerivedDetails(d.(*state.State), sb.SpaceID(), sb.Type())
	err = sb.injectLocalDetails(d.(*state.State))
	if err != nil {
		log.Errorf("failed to inject local details in StateRebuild: %v", err)
	}
	d.(*state.State).SetParent(sb.Doc.(*state.State))
	// todo: make store diff
	sb.execHooks(HookBeforeApply, ApplyInfo{State: d.(*state.State)})
	msgs, _, err := state.ApplyState(d.(*state.State), !sb.disableLayouts)
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
	sb.execHooks(HookAfterApply, ApplyInfo{State: sb.Doc.(*state.State), Events: msgs, Changes: d.(*state.State).GetChanges()})
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

func hasDepIds(relations pbtypes.RelationLinks, act *undo.Action) bool {
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
		for k, after := range act.Details.After.Fields {
			rel := relations.Get(k)
			if rel != nil && (rel.Format == model.RelationFormat_status ||
				rel.Format == model.RelationFormat_tag ||
				rel.Format == model.RelationFormat_object ||
				rel.Format == model.RelationFormat_file ||
				isCoverId(rel)) {

				before := act.Details.Before.Fields[k]
				// Check that value is actually changed
				if before == nil || !before.Equal(after) {
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
func isCoverId(rel *model.RelationLink) bool {
	return rel.Key == bundle.RelationKeyCoverId.String()
}

func getChangedFileHashes(s *state.State, fileDetailKeys []string, act undo.Action) (hashes []string) {
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
		if det != nil && det.Fields != nil {
			for _, field := range fileDetailKeys {
				if list := pbtypes.GetStringList(det, field); list != nil {
					hashes = append(hashes, list...)
				} else if s := pbtypes.GetString(det, field); s != "" {
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
	fileKeys := make([]files.FileKeys, len(keys))
	for i, k := range keys {
		fileKeys[i] = files.FileKeys{
			Hash: k.Hash,
			Keys: k.Keys,
		}
	}
	if err := sb.fileService.StoreFileKeys(fileKeys...); err != nil {
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

func (sb *smartBlock) baseRelations() []*model.Relation {
	rels := []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyId), bundle.MustGetRelation(bundle.RelationKeyLayout), bundle.MustGetRelation(bundle.RelationKeyIconEmoji), bundle.MustGetRelation(bundle.RelationKeyName)}
	for _, rel := range rels {
		rel.Scope = model.Relation_object
	}
	return rels
}

// deprecated, use RelationLinks instead
func (sb *smartBlock) Relations(s *state.State) relationutils.Relations {
	var links []*model.RelationLink
	if s == nil {
		links = sb.Doc.GetRelationLinks()
	} else {
		links = s.GetRelationLinks()
	}
	rels, _ := sb.systemObjectService.FetchRelationByLinks(sb.SpaceID(), links)
	return rels
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
	fileHashes := st.GetAllFileHashes(sb.FileRelationKeys(st))
	creator := pbtypes.GetString(st.Details(), bundle.RelationKeyCreator.String())
	if creator == "" {
		creator = sb.coreService.ProfileID(sb.SpaceID())
	}

	// we don't want any hidden or internal relations here. We want to capture the meaningful outgoing links only
	links := pbtypes.GetStringList(sb.LocalDetails(), bundle.RelationKeyLinks.String())
	// so links will have this order
	// 1. Simple blocks: links, mentions in the text
	// 2. Relations(format==Object)
	// todo: heads in source and the state may be inconsistent?
	heads := sb.source.Heads()
	if len(heads) == 0 {
		lastChangeId := pbtypes.GetString(st.LocalDetails(), bundle.RelationKeyLastChangeId.String())
		if lastChangeId != "" {
			heads = []string{lastChangeId}
		}
	}
	return DocInfo{
		Id:         sb.Id(),
		SpaceID:    sb.SpaceID(),
		Links:      links,
		Heads:      heads,
		FileHashes: fileHashes,
		Creator:    creator,
		Details:    sb.CombinedDetails(),
		Type:       sb.ObjectTypeKey(),
	}
}

func (sb *smartBlock) runIndexer(s *state.State, opts ...IndexOption) {
	docInfo := sb.getDocInfo(s)
	if err := sb.indexer.Index(context.Background(), docInfo, opts...); err != nil {
		log.Errorf("index object %s error: %s", sb.Id(), err)
	}
}

func (sb *smartBlock) beforeStateApply(s *state.State) {
	sb.setRestrictionsDetail(s)
	sb.injectLinksDetails(s)
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
	rawRestrictions := make([]int, len(sb.Restrictions().Object))
	for i, r := range sb.Restrictions().Object {
		rawRestrictions[i] = int(r)
	}
	s.SetLocalDetail(bundle.RelationKeyRestrictions.String(), pbtypes.IntList(rawRestrictions...))

	// todo: verify this logic with clients
	if sb.Restrictions().Object.Check(model.Restrictions_Details) != nil &&
		sb.Restrictions().Object.Check(model.Restrictions_Blocks) != nil {

		s.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, pbtypes.Bool(true))
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

func hasStoreChanges(changes []*pb.ChangeContent) bool {
	for _, ch := range changes {
		if ch.GetStoreKeySet() != nil ||
			ch.GetStoreKeyUnset() != nil ||
			ch.GetStoreSliceUpdate() != nil {
			return true
		}
	}
	return false
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

// injectDerivedDetails injects the local deta
func (sb *smartBlock) injectDerivedDetails(s *state.State, spaceID string, sbt smartblock.SmartBlockType) {
	id := s.RootId()
	if id != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(id))
	}

	if spaceID != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(spaceID))
	} else {
		log.Errorf("InjectDerivedDetails: failed to set space id for %s: no space id provided, but in details: %s", id, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeySpaceId.String()))
	}
	if ot := s.ObjectTypeKey(); ot != "" {
		typeID, err := sb.systemObjectService.GetTypeIdByKey(context.Background(), sb.SpaceID(), ot)
		if err != nil {
			log.Errorf("failed to get type id for %s: %v", ot, err)
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(typeID))
	}

	if uki := s.UniqueKeyInternal(); uki != "" {
		// todo: remove this hack after spaceService refactored to include marketplace virtual space
		if sbt == smartblock.SmartBlockTypeBundledObjectType {
			sbt = smartblock.SmartBlockTypeObjectType
		} else if sbt == smartblock.SmartBlockTypeBundledRelation {
			sbt = smartblock.SmartBlockTypeRelation
		}

		uk, err := domain.NewUniqueKey(sbt, uki)
		if err != nil {
			log.Errorf("failed to get unique key for %s: %v", uki, err)
		} else {
			s.SetDetailAndBundledRelation(bundle.RelationKeyUniqueKey, pbtypes.String(uk.Marshal()))
		}
	}

	sb.setRestrictionsDetail(s)

	snippet := s.Snippet()
	if snippet != "" || s.LocalDetails() != nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeySnippet, pbtypes.String(snippet))
	}

	// Set isDeleted relation only if isUninstalled is present in details
	if isUninstalled := s.Details().GetFields()[bundle.RelationKeyIsUninstalled.String()]; isUninstalled != nil {
		var isDeleted bool
		if isUninstalled.GetBoolValue() {
			isDeleted = true
		}
		s.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, pbtypes.Bool(isDeleted))
	}
}

type InitFunc = func(id string) *InitContext
