package smartblock

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/ocache"
	"github.com/anytypeio/any-sync/commonspace"
	// nolint:misspell
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

func New() SmartBlock {
	s := &smartBlock{hooks: map[Hook][]HookCallback{}, hooksOnce: map[string]struct{}{}, Locker: &sync.Mutex{}}
	return s
}

type SmartObjectOpenListner interface {
	// should not do any Do operations inside
	SmartObjectOpened(*session.Context)
}

type SmartBlock interface {
	Init(ctx *InitContext) (err error)
	Id() string
	Type() model.SmartBlockType
	Show(*session.Context) (obj *model.ObjectView, err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	Relations(s *state.State) relationutils.Relations
	HasRelation(s *state.State, relationKey string) bool
	AddRelationLinks(ctx *session.Context, relationIds ...string) (err error)
	AddRelationLinksToState(s *state.State, relationIds ...string) (err error)
	RemoveExtraRelations(ctx *session.Context, relationKeys []string) (err error)
	TemplateCreateFromObjectState() (*state.State, error)
	SetVerticalAlign(ctx *session.Context, align model.BlockVerticalAlign, ids ...string) error
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
	ObjectClose()
	FileRelationKeys(s *state.State) []string
	Inner() SmartBlock

	ocache.Object
	state.Doc
	sync.Locker
}

type DocInfo struct {
	Id         string
	Links      []string
	FileHashes []string
	Heads      []string
	Creator    string
	State      *state.State
}

type InitContext struct {
	IsNewObject    bool
	Source         source.Source
	ObjectTypeUrls []string
	RelationKeys   []string
	State          *state.State
	Relations      []*model.Relation
	Restriction    restriction.Service
	ObjectStore    objectstore.ObjectStore
	SpaceID        string
	BuildTreeOpts  commonspace.BuildTreeOpts
	Ctx            context.Context
	App            *app.App
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
	Index(ctx context.Context, info DocInfo) error
	app.ComponentRunnable
}

type smartBlock struct {
	state.Doc
	objecttree.ObjectTree
	Locker
	depIds              []string // slice must be sorted
	sendEvent           func(e *pb.Event)
	undo                undo.History
	source              source.Source
	indexer             Indexer
	metaData            *core.SmartBlockMeta
	lastDepDetails      map[string]*pb.EventObjectDetailsSet
	restrictionsUpdater func()
	restrictions        restriction.Restrictions
	objectStore         objectstore.ObjectStore
	relationService     relation2.Service
	isDeleted           bool
	disableLayouts      bool

	includeRelationObjectsAsDependents bool // used by some clients

	hooks     map[Hook][]HookCallback
	hooksOnce map[string]struct{}

	recordsSub      database.Subscription
	closeRecordsSub func()
}

type LockerSetter interface {
	SetLocker(locker Locker)
}

func (sb *smartBlock) SetLocker(locker Locker) {
	sb.Locker = locker
}

func (sb *smartBlock) Tree() objecttree.ObjectTree {
	return sb.ObjectTree
}

func (sb *smartBlock) Inner() SmartBlock {
	return sb
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

func (s *smartBlock) GetAndUnsetFileKeys() (keys []pb.ChangeFileKeys) {
	keys2 := s.source.GetFileKeysSnapshot()
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

func (sb *smartBlock) Type() model.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) Init(ctx *InitContext) (err error) {
	cctx := ctx.Ctx
	if cctx == nil {
		cctx = context.Background()
	}
	if sb.Doc, err = ctx.Source.ReadDoc(cctx, sb, ctx.State != nil); err != nil {
		return fmt.Errorf("reading document: %w", err)
	}

	sb.source = ctx.Source
	if provider, ok := sb.source.(source.ObjectTreeProvider); ok {
		sb.ObjectTree = provider.Tree()
	}
	sb.undo = undo.NewHistory(0)
	sb.restrictionsUpdater = func() {
		restrictions := ctx.App.MustComponent(restriction.CName).(restriction.Service).RestrictionsByObj(sb)
		sb.SetRestrictions(restrictions)
	}
	sb.restrictions = ctx.App.MustComponent(restriction.CName).(restriction.Service).RestrictionsByObj(sb)
	sb.relationService = ctx.App.MustComponent(relation2.CName).(relation2.Service)
	sb.indexer = app.MustComponent[Indexer](ctx.App)
	sb.objectStore = ctx.App.MustComponent(objectstore.CName).(objectstore.ObjectStore)
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
	var relKeys []bundle.RelationKey
	for k := range ctx.State.Details().GetFields() {
		if _, err := bundle.GetRelation(bundle.RelationKey(k)); err == nil {
			relKeys = append(relKeys, bundle.RelationKey(k))
		}
	}
	ctx.State.AddBundledRelations(relKeys...)

	if err = sb.injectLocalDetails(ctx.State); err != nil {
		return
	}
	return
}

// updateRestrictions refetch restrictions from restriction service and update them in the smartblock
func (sb *smartBlock) updateRestrictions() {
	if sb.restrictionsUpdater != nil {
		sb.restrictionsUpdater()
	}
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

func (sb *smartBlock) SendEvent(msgs []*pb.EventMessage) {
	if sb.sendEvent != nil {
		sb.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: sb.Id(),
		})
	}
}

func (sb *smartBlock) Restrictions() restriction.Restrictions {
	return sb.restrictions
}

func (sb *smartBlock) Show(ctx *session.Context) (*model.ObjectView, error) {
	if ctx == nil {
		return nil, nil
	}

	details, objectTypes, err := sb.fetchMeta()
	if err != nil {
		return nil, err
	}
	// omit relations
	// todo: switch to other pb type
	for _, ot := range objectTypes {
		ot.RelationLinks = nil
	}

	undo, redo := sb.History().Counters()

	// todo: sb.Relations() makes extra query to read objectType which we already have here
	// the problem is that we can have an extra object type of the set in the objectTypes so we can't reuse it
	return &model.ObjectView{
		RootId:        sb.RootId(),
		Type:          sb.Type(),
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

func (sb *smartBlock) fetchMeta() (details []*model.ObjectViewDetailsSet, objectTypes []*model.ObjectType, err error) {
	if sb.closeRecordsSub != nil {
		sb.closeRecordsSub()
		sb.closeRecordsSub = nil
	}
	recordsCh := make(chan *types.Struct, 10)
	sb.recordsSub = database.NewSubscription(nil, recordsCh)

	depIDs := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true, true)
	sb.setDependentIDs(depIDs)

	var records []database.Record
	if records, sb.closeRecordsSub, err = sb.objectStore.QueryByIdAndSubscribeForChanges(sb.depIds, sb.recordsSub); err != nil {
		// datastore unavailable, cancel the subscription
		sb.recordsSub.Close()
		sb.closeRecordsSub = nil
		return
	}

	var uniqueObjTypes []string

	var addObjectTypesByDetails = func(det *types.Struct) {
		for _, key := range []string{bundle.RelationKeyType.String(), bundle.RelationKeyTargetObjectType.String()} {
			ot := pbtypes.GetString(det, key)
			if ot != "" && slice.FindPos(uniqueObjTypes, ot) == -1 {
				uniqueObjTypes = append(uniqueObjTypes, ot)
			}
		}
	}

	details = make([]*model.ObjectViewDetailsSet, 0, len(records)+1)

	// add self details
	details = append(details, &model.ObjectViewDetailsSet{
		Id:      sb.Id(),
		Details: sb.CombinedDetails(),
	})
	addObjectTypesByDetails(sb.CombinedDetails())

	for _, rec := range records {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()),
			Details: rec.Details,
		})
		addObjectTypesByDetails(rec.Details)
	}

	objectTypes, _ = objectstore.GetObjectTypes(sb.objectStore, uniqueObjTypes)
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
	if sb.sendEvent == nil {
		return
	}
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
			if len(diff.GetFields()) > 0 {
				log.With("thread", sb.Id()).Debugf("onMetaChange current object: %s", pbtypes.Sprint(diff))
			}
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
	return sb.Doc.(*state.State).DepSmartIds(true, true, includeRelations, includeObjTypes, includeCreatorModifier)
}

func (sb *smartBlock) navigationalLinks() []string {
	includeDetails := true
	includeRelations := sb.includeRelationObjectsAsDependents

	s := sb.Doc.(*state.State)

	var ids []string

	if !internalflag.NewFromState(s).Has(model.InternalFlag_collectionDontIndex) {
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
		log.With("thread", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
	}

	var det *types.Struct
	if includeDetails {
		det = s.CombinedDetails()
	}

	for _, rel := range s.GetRelationLinks() {
		if includeRelations {
			ids = append(ids, addr.RelationKeyToIdPrefix+rel.Key)
		}
		if !includeDetails {
			continue
		}

		if rel.Format != model.RelationFormat_object {
			continue
		}

		if bundle.RelationKey(rel.Key).IsSystem() {
			continue
		}

		// Do not include hidden relations. Only bundled relations can be hidden, so we don't need
		// to request relations from object store.
		if r, err := bundle.GetRelation(bundle.RelationKey(rel.Key)); err == nil && r.Hidden {
			continue
		}

		// Add all object relation values as dependents
		for _, targetID := range pbtypes.GetStringList(det, rel.Key) {
			if targetID != "" {
				ids = append(ids, targetID)
			}
		}
	}

	return util.UniqueStrings(ids)
}

func (sb *smartBlock) SetEventFunc(f func(e *pb.Event)) {
	sb.sendEvent = f
}

func (sb *smartBlock) IsLocked() bool {
	return sb.sendEvent != nil
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
		}
	}

	if hooks {
		if err = sb.execHooks(HookBeforeApply, ApplyInfo{State: s}); err != nil {
			return nil
		}
	}
	if checkRestrictions {
		if err = s.ParentState().CheckRestrictions(); err != nil {
			return
		}
	}
	if err = sb.onApply(s); err != nil {
		return
	}
	if sb.Anytype() != nil {
		// this one will be reverted in case we don't have any actual change being made
		s.SetLastModified(time.Now().Unix(), sb.Anytype().PredefinedBlocks().Profile)
	}
	beforeApplyStateTime := time.Now()

	migrationVersionUpdated := true
	if parent := s.ParentState(); parent != nil {
		migrationVersionUpdated = s.MigrationVersion() != parent.MigrationVersion()
	}

	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return
	}
	afterApplyStateTime := time.Now()
	st := sb.Doc.(*state.State)

	changes := st.GetChanges()
	var changeId string
	if skipIfNoChanges && len(changes) == 0 && !migrationVersionUpdated {
		if hasDetailsMsgs(msgs) {
			sb.runIndexer(st)
		}
		return nil
	}
	pushChange := func() error {
		fileDetailsKeys := sb.FileRelationKeys(st)
		fileDetailsKeysFiltered := fileDetailsKeys[:0]
		for _, ch := range changes {
			if ds := ch.GetDetailsSet(); ds != nil {
				if slice.FindPos(fileDetailsKeys, ds.Key) != -1 {
					fileDetailsKeysFiltered = append(fileDetailsKeysFiltered, ds.Key)
				}
			}
		}
		pushChangeParams := source.PushChangeParams{
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
	} else if hasStoreChanges(changes) { // TODO: change to len(changes) > 0
		// log.Errorf("sb apply %s: store changes %s", sb.Id(), pbtypes.Sprint(&pb.Change{Content: changes}))
		err = pushChange()
		if err != nil {
			return err
		}
	}

	if changeId != "" || hasDetailsMsgs(msgs) {
		// if changeId is empty, it means that we didn't push any changes to the source
		// but we can also have some local details changes, so check the events
		sb.runIndexer(st)
	}
	afterPushChangeTime := time.Now()
	if sendEvent {
		events := msgsToEvents(msgs)
		if ctx := s.Context(); ctx != nil {
			ctx.SetMessages(sb.Id(), events)
		} else if sb.sendEvent != nil {
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
			log.With("thread", sb.Id()).Warnf("after apply execHooks error: %v", e)
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

	// we may have layout changed, so we need to update restrictions
	sb.updateRestrictions()
	return
}

func (sb *smartBlock) ResetToVersion(s *state.State) (err error) {
	s.SetParent(sb.Doc.(*state.State))
	sb.storeFileKeys(s)
	sb.injectLocalDetails(s)
	s.InjectDerivedDetails()
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
	records, err := sb.objectStore.QueryById(newIDs)
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
	s := sb.Doc.NewState().SetNoObjectType(sb.Type() == model.SmartBlockType_Archive)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) NewStateCtx(ctx *session.Context) *state.State {
	s := sb.Doc.NewStateCtx(ctx).SetNoObjectType(sb.Type() == model.SmartBlockType_Archive)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) History() undo.History {
	return sb.undo
}

func (sb *smartBlock) Anytype() core.Service {
	return sb.source.Anytype()
}

func (sb *smartBlock) RelationService() relation2.Service {
	return sb.relationService
}

func (sb *smartBlock) AddRelationLinks(ctx *session.Context, relationKeys ...string) (err error) {
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
	relations, err := sb.relationService.FetchKeys(relationKeys...)
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

func (sb *smartBlock) injectLocalDetails(s *state.State) error {
	if sb.objectStore == nil {
		return nil
	}
	storedDetails, err := sb.objectStore.GetDetails(sb.Id())
	if err != nil {
		return err
	}

	// Consume pending details
	err = sb.objectStore.UpdatePendingLocalDetails(sb.Id(), func(pending *types.Struct) (*types.Struct, error) {
		storedDetails.Details = pbtypes.StructMerge(storedDetails.GetDetails(), pending, false)
		return nil, nil
	})
	if err != nil {
		log.With("thread", sb.Id()).
			With("sbType", sb.Type()).
			Errorf("failed to update pending details: %v", err)
	}

	// inject also derived keys, because it may be a good idea to have created date and creator cached,
	// so we don't need to traverse changes every time
	keys := append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...)
	storedLocalScopeDetails := pbtypes.StructFilterKeys(storedDetails.GetDetails(), keys)
	sbLocalScopeDetails := pbtypes.StructFilterKeys(s.LocalDetails(), keys)
	if pbtypes.StructEqualIgnore(sbLocalScopeDetails, storedLocalScopeDetails, nil) {
		return nil
	}

	s.InjectLocalDetails(storedLocalScopeDetails)
	if pbtypes.HasField(s.LocalDetails(), bundle.RelationKeyCreator.String()) {
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

	s.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Float64(float64(createdDate)))
	wsId, _ := sb.Anytype().GetWorkspaceIdForObject(sb.Id())
	if wsId != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(wsId))
	}
	return nil
}

func (sb *smartBlock) SetVerticalAlign(ctx *session.Context, align model.BlockVerticalAlign, ids ...string) (err error) {
	s := sb.NewStateCtx(ctx)
	for _, id := range ids {
		if b := s.Get(id); b != nil {
			b.Model().VerticalAlign = align
		}
	}
	return sb.Apply(s)
}

func (sb *smartBlock) TemplateCreateFromObjectState() (*state.State, error) {
	st := sb.NewState().Copy()
	st.SetLocalDetails(nil)
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(st.ObjectType()))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), st.ObjectType()})
	for _, rel := range sb.Relations(st) {
		if rel.DataSource == model.Relation_details && !rel.Hidden {
			st.RemoveDetail(rel.Key)
		}
	}
	return st, nil
}

func (sb *smartBlock) RemoveExtraRelations(ctx *session.Context, relationIds []string) (err error) {
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
	s.InjectDerivedDetails()
	sb.execHooks(HookBeforeApply, ApplyInfo{State: s})
	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return err
	}
	log.Infof("changes: stateAppend: %d events", len(msgs))
	if len(msgs) > 0 && sb.sendEvent != nil {
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
	d.(*state.State).InjectDerivedDetails()
	d.(*state.State).SetParent(sb.Doc.(*state.State))
	// todo: make store diff
	sb.execHooks(HookBeforeApply, ApplyInfo{State: d.(*state.State)})
	msgs, _, err := state.ApplyState(d.(*state.State), !sb.disableLayouts)
	log.Infof("changes: stateRebuild: %d events", len(msgs))
	if err != nil {
		// can't make diff - reopen doc
		sb.Show(session.NewContext(session.WithSendEvent(sb.sendEvent)))
	} else {
		if len(msgs) > 0 && sb.sendEvent != nil {
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

func (sb *smartBlock) ObjectClose() {
	sb.execHooks(HookOnBlockClose, ApplyInfo{State: sb.Doc.(*state.State)})
	sb.SetEventFunc(nil)
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
	return
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
	if err := sb.Anytype().FileStoreKeys(fileKeys...); err != nil {
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
	rels, _ := sb.RelationService().FetchLinks(links)
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
		creator = sb.Anytype().ProfileID()
	}

	// we don't want any hidden or internal relations here. We want to capture the meaningful outgoing links only
	links := sb.navigationalLinks()
	links = slice.Remove(links, sb.Id())
	// so links will have this order
	// 1. Simple blocks: links, mentions in the text
	// 2. Relations(format==Object)
	return DocInfo{
		Id:         sb.Id(),
		Links:      links,
		Heads:      sb.source.Heads(),
		FileHashes: fileHashes,
		Creator:    creator,
		State:      st.Copy(),
	}
}

func (sb *smartBlock) runIndexer(s *state.State) {
	docInfo := sb.getDocInfo(s)
	if err := sb.indexer.Index(context.TODO(), docInfo); err != nil {
		log.Errorf("index object %s error: %s", sb.Id(), err)
	}
}

func (sb *smartBlock) onApply(s *state.State) (err error) {
	flags := internalflag.NewFromState(s)

	// Run empty check only if any of these flags are present
	if flags.Has(model.InternalFlag_editorDeleteEmpty) || flags.Has(model.InternalFlag_editorSelectType) {
		if !s.IsEmpty(true) {
			flags.Remove(model.InternalFlag_editorDeleteEmpty)
		}
		if !s.IsEmpty(false) {
			flags.Remove(model.InternalFlag_editorSelectType)
		}

		flags.AddToState(s)
	}

	sb.setRestrictionsDetail(s)
	return
}

func (sb *smartBlock) setRestrictionsDetail(s *state.State) {
	var ints = make([]int, len(sb.Restrictions().Object))
	for i, v := range sb.Restrictions().Object {
		ints[i] = int(v)
	}
	s.SetLocalDetail(bundle.RelationKeyRestrictions.String(), pbtypes.IntList(ints...))
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
