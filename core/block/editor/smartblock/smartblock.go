package smartblock

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/mutex"
	"github.com/anytypeio/go-anytype-middleware/util/ocache"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type ApplyFlag int

var (
	ErrSimpleBlockNotFound                         = errors.New("simple block not found")
	ErrCantInitExistingSmartblockWithNonEmptyState = errors.New("can't init existing smartblock with non-empty state")
	ErrRelationOptionNotFound                      = errors.New("relation option not found")
	ErrRelationNotFound                            = errors.New("relation not found")
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
	s := &smartBlock{hooks: map[Hook][]HookCallback{}, Locker: mutex.NewLocker()}
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
	SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error)
	Relations(s *state.State) relationutils.Relations
	HasRelation(s *state.State, relationKey string) bool
	AddRelationLinks(ctx *session.Context, relationIds ...string) (err error)
	RemoveExtraRelations(ctx *session.Context, relationKeys []string) (err error)
	TemplateCreateFromObjectState() (*state.State, error)
	SetObjectTypes(ctx *session.Context, objectTypes []string) (err error)
	SetAlign(ctx *session.Context, align model.BlockAlign, ids ...string) error
	SetVerticalAlign(ctx *session.Context, align model.BlockVerticalAlign, ids ...string) error
	SetLayout(ctx *session.Context, layout model.ObjectTypeLayout) error
	SetIsDeleted()
	IsDeleted() bool
	IsLocked() bool

	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	DisableLayouts()
	EnabledRelationAsDependentObjects()
	AddHook(f HookCallback, events ...Hook)
	CheckSubscriptions() (changed bool)
	GetDocInfo() (doc.DocInfo, error)
	Restrictions() restriction.Restrictions
	SetRestrictions(r restriction.Restrictions)
	ObjectClose()
	FileRelationKeys(s *state.State) []string

	ocache.ObjectLocker
	state.Doc
	sync.Locker
}

type InitContext struct {
	Source         source.Source
	ObjectTypeUrls []string
	RelationIds    []string
	State          *state.State
	Relations      []*model.Relation
	Restriction    restriction.Service
	Doc            doc.Service
	ObjectStore    objectstore.ObjectStore
	Ctx            context.Context
	App            *app.App
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

type smartBlock struct {
	state.Doc
	sync.Locker
	depIds              []string // slice must be sorted
	sendEvent           func(e *pb.Event)
	undo                undo.History
	source              source.Source
	doc                 doc.Service
	metaData            *core.SmartBlockMeta
	lastDepDetails      map[string]*pb.EventObjectDetailsSet
	restrictions        restriction.Restrictions
	restrictionsChanged bool
	objectStore         objectstore.ObjectStore
	relationService     relation2.Service
	isDeleted           bool
	disableLayouts      bool

	includeRelationObjectsAsDependents bool // used by some clients

	hooks map[Hook][]HookCallback

	recordsSub      database.Subscription
	closeRecordsSub func()
}

func (sb *smartBlock) FileRelationKeys(s *state.State) (fileKeys []string) {
	for _, rel := range s.GetRelationLinks() {
		// coverId can contains both hash or predefined cover id
		if rel.Format == model.RelationFormat_file || rel.Key == bundle.RelationKeyCoverId.String() {
			if slice.FindPos(fileKeys, rel.Key) == -1 {
				fileKeys = append(fileKeys, rel.Key)
			}
		}
	}
	return
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
	sb.undo = undo.NewHistory(0)
	sb.restrictions = ctx.App.MustComponent(restriction.CName).(restriction.Service).RestrictionsByObj(sb)
	sb.relationService = ctx.App.MustComponent(relation2.CName).(relation2.Service)
	sb.doc = ctx.App.MustComponent(doc.CName).(doc.Service)
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

	if len(ctx.ObjectTypeUrls) > 0 && len(sb.ObjectTypes()) == 0 {
		err = sb.setObjectTypes(ctx.State, ctx.ObjectTypeUrls)
		if err != nil {
			return err
		}
	}
	if err = sb.addRelations(ctx.State, ctx.RelationIds...); err != nil {
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

func (sb *smartBlock) SetRestrictions(r restriction.Restrictions) {
	if sb.restrictions.Equal(r) {
		return
	}
	sb.restrictions = r
	sb.restrictionsChanged = true
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
	sb.depIds = sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true, true)
	sort.Strings(sb.depIds)
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

	if sb.Type() == model.SmartBlockType_Set {
		// add the object type from the dataview source
		if b := sb.Doc.Pick("dataview"); b != nil {
			if dv := b.Model().GetDataview(); dv != nil {
				if len(dv.Source) == 0 || dv.Source[0] == "" {
					panic("empty dv source")
				}
				uniqueObjTypes = append(uniqueObjTypes, dv.Source...)
				for _, rel := range dv.Relations {
					if rel.Format == model.RelationFormat_file || rel.Format == model.RelationFormat_object {
						if rel.Key == bundle.RelationKeyId.String() || rel.Key == bundle.RelationKeyType.String() {
							continue
						}
						for _, ot := range rel.ObjectTypes {
							if slice.FindPos(uniqueObjTypes, ot) == -1 {
								if ot == "" {
									log.Errorf("dv relation %s(%s) has empty obj types", rel.Key, rel.Name)
								} else {
									if strings.HasPrefix(ot, "http") {
										log.Errorf("dv rels has http source")
									}
									uniqueObjTypes = append(uniqueObjTypes, ot)
								}
							}
						}
					}
				}
			}
		}
	}

	objectTypes, _ = objectstore.GetObjectTypes(sb.objectStore, uniqueObjTypes)
	go sb.metaListener(recordsCh)
	return
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
	if sb.sendEvent != nil {
		id := pbtypes.GetString(details, bundle.RelationKeyId.String())
		msgs := []*pb.EventMessage{}
		if details != nil {
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
		}

		if len(msgs) == 0 {
			return
		}

		sb.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: sb.Id(),
		})
	}
}

// dependentSmartIds returns list of dependent objects in this order: Simple blocks(Link, mentions in Text), Relations. Both of them are returned in the order of original blocks/relations
func (sb *smartBlock) dependentSmartIds(includeRelations, includeObjTypes, includeCreatorModifier, _ bool) (ids []string) {
	if sb.Type() == model.SmartBlockType_Breadcrumbs {
		// little optimisation for breadcrumbs: we don't need any dependencies except simple blocks
		return sb.Doc.(*state.State).DepSmartIds(true, false, false, false, false)
	}

	return sb.Doc.(*state.State).DepSmartIds(true, true, includeRelations, includeObjTypes, includeCreatorModifier)
}

func (sb *smartBlock) SetEventFunc(f func(e *pb.Event)) {
	sb.sendEvent = f
}

func (sb *smartBlock) Locked() bool {
	sb.Lock()
	defer sb.Unlock()
	return sb.IsLocked()
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
	var sendEvent, addHistory, doSnapshot, checkRestrictions, hooks = true, true, false, true, true
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
		}
	}
	if sb.source.ReadOnly() && addHistory {
		// workaround to detect user-generated action
		return fmt.Errorf("object is readonly")
	}
	if hooks {
		if err = sb.execHooks(HookBeforeApply, ApplyInfo{State: s}); err != nil {
			return nil
		}
	}
	if checkRestrictions {
		if err = s.CheckRestrictions(); err != nil {
			return
		}
	}
	if err = sb.onApply(s); err != nil {
		return
	}
	if sb.Anytype() != nil {
		// this one will be reverted in case we don't have any actual change being made
		s.SetLastModified(time.Now().Unix(), sb.Anytype().Account())
	}
	beforeApplyStateTime := time.Now()
	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return
	}
	afterApplyStateTime := time.Now()
	st := sb.Doc.(*state.State)

	changes := st.GetChanges()
	pushChange := func() {
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
		var id string
		id, err = sb.source.PushChange(pushChangeParams)
		if err != nil {
			return
		}
		sb.Doc.(*state.State).SetChangeId(id)
	}
	if !act.IsEmpty() {
		if len(changes) == 0 && !doSnapshot {
			log.Errorf("apply 0 changes %s: %v", st.RootId(), msgs)
		}
		pushChange()
		if sb.undo != nil && addHistory {
			act.Group = s.GroupId()
			sb.undo.Add(act)
		}
	} else if hasStoreChanges(changes) { // TODO: change to len(changes) > 0
		//log.Errorf("sb apply %s: store changes %s", sb.Id(), pbtypes.Sprint(&pb.Change{Content: changes}))
		pushChange()
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

	sb.reportChange(st)

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

	return
}

func (sb *smartBlock) ResetToVersion(s *state.State) (err error) {
	s.SetParent(sb.Doc.(*state.State))
	if err = sb.Apply(s, NoHistory, DoSnapshot, NoRestrictions); err != nil {
		return
	}
	if sb.undo != nil {
		sb.undo.Reset()
	}
	return
}

func (sb *smartBlock) CheckSubscriptions() (changed bool) {
	depIds := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true, true)
	sort.Strings(depIds)
	if !slice.SortedEquals(sb.depIds, depIds) {
		sb.depIds = depIds
		if sb.recordsSub != nil {
			newIds := sb.recordsSub.Subscribe(sb.depIds)
			records, err := sb.objectStore.QueryById(newIds)
			if err != nil {
				log.Errorf("queryById error: %v", err)
			}
			for _, rec := range records {
				sb.onMetaChange(rec.Details)
			}
		}
		return true
	}
	return false
}

func (sb *smartBlock) NewState() *state.State {
	s := sb.Doc.NewState().SetNoObjectType(sb.Type() == model.SmartBlockType_Archive || sb.Type() == model.SmartBlockType_Breadcrumbs)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) NewStateCtx(ctx *session.Context) *state.State {
	s := sb.Doc.NewStateCtx(ctx).SetNoObjectType(sb.Type() == model.SmartBlockType_Archive || sb.Type() == model.SmartBlockType_Breadcrumbs)
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

func (sb *smartBlock) SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	s := sb.NewStateCtx(ctx)
	detCopy := pbtypes.CopyStruct(s.CombinedDetails())
	if detCopy == nil || detCopy.Fields == nil {
		detCopy = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}

	for _, detail := range details {
		if detail.Value != nil {
			if detail.Key == bundle.RelationKeyType.String() {
				// special case when client sets the type's detail directly instead of using setObjectType command
				err = sb.SetObjectTypes(ctx, pbtypes.GetStringListValue(detail.Value))
				if err != nil {
					log.Errorf("failed to set object's type via detail: %s", err.Error())
				} else {
					continue
				}
			}
			if detail.Key == bundle.RelationKeyLayout.String() {
				// special case when client sets the layout detail directly instead of using setLayout command
				err = sb.SetLayout(ctx, model.ObjectTypeLayout(detail.Value.GetNumberValue()))
				if err != nil {
					log.Errorf("failed to set object's layout via detail: %s", err.Error())
				}
				continue
			}

			// TODO: add relation2.WithWorkspaceId(workspaceId) filter
			rel, err := sb.RelationService().FetchKey(detail.Key)
			if err != nil {
				return fmt.Errorf("fetch relation by key %s: %w", detail.Key, err)
			}
			if rel == nil {
				return fmt.Errorf("relation %s is not found", detail.Key)
			}
			s.AddRelationLinks(&model.RelationLink{
				Format: rel.Format,
				Key:    rel.Key,
			})

			err = sb.RelationService().ValidateFormat(detail.Key, detail.Value)
			if err != nil {
				return fmt.Errorf("relation %s validation failed: %s", detail.Key, err.Error())
			}

			// special case for type relation that we are storing in a separate object's field
			if detail.Key == bundle.RelationKeyType.String() {
				ot := pbtypes.GetStringListValue(detail.Value)
				if len(ot) > 0 {
					s.SetObjectType(ot[0])
				}
			}
			detCopy.Fields[detail.Key] = detail.Value
		} else {
			delete(detCopy.Fields, detail.Key)
		}
	}
	if detCopy.Equal(sb.Details()) {
		return
	}

	s.SetDetails(detCopy)
	if err = sb.Apply(s); err != nil {
		return
	}

	// filter-out setDetails event
	if !showEvent && ctx != nil {
		var filtered []*pb.EventMessage
		msgs := ctx.GetMessages()
		var isFiltered bool
		for i, msg := range msgs {
			if sd := msg.GetObjectDetailsSet(); sd == nil || sd.Id != sb.Id() {
				filtered = append(filtered, msgs[i])
			} else {
				isFiltered = true
			}
		}
		if isFiltered {
			ctx.SetMessages(sb.Id(), filtered)
		}

	}
	return nil
}

func (sb *smartBlock) AddRelationLinks(ctx *session.Context, relationKeys ...string) (err error) {
	s := sb.NewStateCtx(ctx)
	if err = sb.addRelations(s, relationKeys...); err != nil {
		return
	}
	return sb.Apply(s)
}

func (sb *smartBlock) addRelations(s *state.State, relationKeys ...string) (err error) {
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

	pendingDetails, err := sb.objectStore.GetPendingLocalDetails(sb.Id())
	if err == nil {
		storedDetails.Details = pbtypes.StructMerge(storedDetails.GetDetails(), pendingDetails.GetDetails(), false)
		err = sb.objectStore.UpdatePendingLocalDetails(sb.Id(), nil)
		if err != nil {
			log.With("thread", sb.Id()).
				With("sbType", sb.Type()).
				Errorf("failed to update pending details: %v", err)
		}
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

func (sb *smartBlock) SetObjectTypes(ctx *session.Context, objectTypes []string) (err error) {
	s := sb.NewStateCtx(ctx)

	if len(objectTypes) > 0 {
		ot, err := objectstore.GetObjectType(sb.objectStore, objectTypes[0])
		if err != nil {
			return err
		}

		if ot.Layout == model.ObjectType_note {
			if name, ok := s.Details().Fields[bundle.RelationKeyName.String()]; ok && name.GetStringValue() != "" {
				newBlock := simple.New(&model.Block{
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: name.GetStringValue()},
					},
				})
				s.Add(newBlock)

				if err := s.InsertTo(template.HeaderLayoutId, model.Block_Bottom, newBlock.Model().Id); err != nil {
					return err
				}

				s.RemoveDetail(bundle.RelationKeyName.String())
			}
		}
	}

	if layout, ok := s.Layout(); ok && layout == model.ObjectType_note {
		if name, ok := s.Details().Fields[bundle.RelationKeyName.String()]; !ok || name.GetStringValue() == "" {
			textBlock, err := s.GetFirstTextBlock()
			if err != nil {
				return err
			}
			if textBlock != nil {
				s.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(textBlock.Text.Text))
				if err := s.Iterate(func(b simple.Block) (isContinue bool) {
					if b.Model().Content == textBlock {
						s.Unlink(b.Model().Id)
						return false
					}
					return true
				}); err != nil {
					return err
				}
			}
		}
	}

	if err = sb.setObjectTypes(s, objectTypes); err != nil {
		return
	}

	flags := internalflag.NewFromState(s)
	flags.Remove(model.InternalFlag_editorSelectType)
	flags.AddToState(s)

	// send event here to send updated details to client
	if err = sb.Apply(s, NoRestrictions); err != nil {
		return
	}
	return
}

func (sb *smartBlock) SetAlign(ctx *session.Context, align model.BlockAlign, ids ...string) (err error) {
	s := sb.NewStateCtx(ctx)
	if err = sb.setAlign(s, align, ids...); err != nil {
		return
	}
	return sb.Apply(s)
}

func (sb *smartBlock) setAlign(s *state.State, align model.BlockAlign, ids ...string) (err error) {
	if len(ids) == 0 {
		s.SetDetail(bundle.RelationKeyLayoutAlign.String(), pbtypes.Int64(int64(align)))
		ids = []string{template.TitleBlockId, template.DescriptionBlockId, template.FeaturedRelationsId}
	}
	for _, id := range ids {
		if b := s.Get(id); b != nil {
			b.Model().Align = align
		}
	}
	return
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

func (sb *smartBlock) SetLayout(ctx *session.Context, layout model.ObjectTypeLayout) (err error) {
	if err = sb.Restrictions().Object.Check(model.Restrictions_LayoutChange); err != nil {
		return
	}

	s := sb.NewStateCtx(ctx)
	if err = sb.setLayout(s, layout); err != nil {
		return
	}
	return sb.Apply(s, NoRestrictions)
}

func (sb *smartBlock) setLayout(s *state.State, layout model.ObjectTypeLayout) (err error) {
	s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(layout)))
	// reset align when layout todo
	if layout == model.ObjectType_todo {
		if err = sb.setAlign(s, model.Block_AlignLeft); err != nil {
			return
		}
	}
	return template.InitTemplate(s, template.ByLayout(layout)...)
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

func (sb *smartBlock) setObjectTypes(s *state.State, objectTypes []string) (err error) {
	if len(objectTypes) == 0 {
		return fmt.Errorf("you must provide at least 1 object type")
	}

	otypes, err := objectstore.GetObjectTypes(sb.objectStore, objectTypes)
	if err != nil {
		return
	}
	if len(otypes) == 0 {
		return fmt.Errorf("object types not found")
	}

	ot := otypes[len(otypes)-1]

	prevType, _ := objectstore.GetObjectType(sb.objectStore, s.ObjectType())

	s.SetObjectTypes(objectTypes)
	if v := pbtypes.Get(s.Details(), bundle.RelationKeyLayout.String()); v == nil || // if layout is not set yet
		prevType == nil || // if we have no type set for some reason or it is missing
		float64(prevType.Layout) == v.GetNumberValue() { // or we have a objecttype recommended layout set for this object
		if err = sb.setLayout(s, ot.Layout); err != nil {
			return
		}
	}
	return
}

func (sb *smartBlock) RemoveExtraRelations(ctx *session.Context, relationIds []string) (err error) {
	st := sb.NewStateCtx(ctx)
	st.RemoveRelation(relationIds...)

	return sb.Apply(st)
}

func (sb *smartBlock) StateAppend(f func(d state.Doc) (s *state.State, err error), changes []*pb.ChangeContent) error {
	if sb.IsDeleted() {
		return ErrIsDeleted
	}
	s, err := f(sb.Doc)
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
	sb.reportChange(s)
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
	sb.reportChange(sb.Doc.(*state.State))
	sb.execHooks(HookAfterApply, ApplyInfo{State: sb.Doc.(*state.State), Events: msgs, Changes: d.(*state.State).GetChanges()})
	return nil
}

func (sb *smartBlock) DocService() doc.Service {
	return sb.doc
}

func (sb *smartBlock) ObjectClose() {
	sb.execHooks(HookOnBlockClose, ApplyInfo{State: sb.Doc.(*state.State)})
	sb.SetEventFunc(nil)
}

func (sb *smartBlock) Close() (err error) {
	sb.Lock()
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
			if rel != nil && (rel.Format == model.RelationFormat_status || rel.Format == model.RelationFormat_tag || rel.Format == model.RelationFormat_object || rel.Format == model.RelationFormat_file) {
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

func mergeAndSortRelations(objTypeRelations []*model.Relation, extraRelations []*model.Relation, aggregatedRelations []*model.Relation, details *types.Struct) []*model.Relation {
	var m = make(map[string]int, len(extraRelations))
	var rels = make([]*model.Relation, 0, len(objTypeRelations)+len(extraRelations))

	for i, rel := range extraRelations {
		m[rel.Key] = i
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	for _, rel := range objTypeRelations {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
		m[rel.Key] = len(rels) - 1
	}

	for _, rel := range aggregatedRelations {
		if i, exists := m[rel.Key]; exists {
			// overwrite name that we've got from DS
			if rels[i].Name != rel.Name {
				rels[i].Name = rel.Name
			}
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
		m[rel.Key] = len(rels) - 1
	}

	if details == nil || details.Fields == nil {
		return rels
	}
	return rels
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

func (sb *smartBlock) GetDocInfo() (doc.DocInfo, error) {
	return sb.getDocInfo(sb.NewState()), nil
}

func (sb *smartBlock) getDocInfo(st *state.State) doc.DocInfo {
	fileHashes := st.GetAllFileHashes(sb.FileRelationKeys(st))
	creator := pbtypes.GetString(st.Details(), bundle.RelationKeyCreator.String())
	if creator == "" {
		creator = sb.Anytype().ProfileID()
	}

	// we don't want any hidden or internal relations here. We want to capture the meaningful outgoing links only
	links := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, false, false, false)

	links = slice.Remove(links, sb.Id())
	// so links will have this order
	// 1. Simple blocks: links, mentions in the text
	// 2. Relations(format==Object)
	return doc.DocInfo{
		Id:         sb.Id(),
		Links:      links,
		LogHeads:   sb.source.LogHeads(),
		FileHashes: fileHashes,
		Creator:    creator,
		State:      st.Copy(),
	}
}

func (sb *smartBlock) reportChange(s *state.State) {
	if sb.doc == nil {
		return
	}
	docInfo := sb.getDocInfo(s)
	sb.doc.ReportChange(context.TODO(), docInfo)
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
	if err = template.InitTemplate(s, templates...); err != nil {
		return
	}
	return sb.Apply(s, NoHistory, NoEvent, NoRestrictions, SkipIfNoChanges)
}

func hasStoreChanges(changes []*pb.ChangeContent) bool {
	for _, ch := range changes {
		if ch.GetStoreKeySet() != nil || ch.GetStoreKeyUnset() != nil {
			return true
		}
	}
	return false
}
