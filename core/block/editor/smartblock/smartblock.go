package smartblock

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

type ApplyFlag int

var (
	ErrSimpleBlockNotFound                         = errors.New("simple block not found")
	ErrCantInitExistingSmartblockWithNonEmptyState = errors.New("can't init existing smartblock with non-empty state")
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

const (
	HookOnNewState Hook = iota
	HookOnClose
	HookOnBlockClose
)

var log = logging.Logger("anytype-mw-smartblock")

// DepSmartblockEventsTimeout sets the timeout after which we will stop to synchronously wait dependent smart blocks and will send them as a separate events in the background
var DepSmartblockSyncEventsTimeout = time.Second * 1

func New(ms meta.Service) SmartBlock {
	return &smartBlock{meta: ms}
}

type SmartblockOpenListner interface {
	// should not do any Do operations inside
	SmartblockOpened(*state.Context)
}

type SmartBlock interface {
	Init(ctx *InitContext) (err error)
	Id() string
	Type() model.SmartBlockType
	Meta() *core.SmartBlockMeta
	Show(*state.Context) (err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	Anytype() core.Service
	SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail, showEvent bool) (err error)
	Relations() []*model.Relation
	RelationsState(s *state.State, aggregateFromDS bool) []*model.Relation
	HasRelation(relationKey string) bool
	AddExtraRelations(ctx *state.Context, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error)
	UpdateExtraRelations(ctx *state.Context, relations []*model.Relation, createIfMissing bool) (err error)
	RemoveExtraRelations(ctx *state.Context, relationKeys []string) (err error)
	AddExtraRelationOption(ctx *state.Context, relationKey string, option model.RelationOption, showEvent bool) (*model.RelationOption, error)
	UpdateExtraRelationOption(ctx *state.Context, relationKey string, option model.RelationOption, showEvent bool) error
	DeleteExtraRelationOption(ctx *state.Context, relationKey string, optionId string, showEvent bool) error

	SetObjectTypes(ctx *state.Context, objectTypes []string) (err error)

	FileRelationKeys() []string

	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	DisableLayouts()
	AddHook(f func(), events ...Hook)
	CheckSubscriptions() (changed bool)
	GetSearchInfo() (indexer.SearchInfo, error)
	MetaService() meta.Service
	Restrictions() restriction.Restrictions
	BlockClose()

	Close() (err error)
	state.Doc
	sync.Locker
}

type InitContext struct {
	Source         source.Source
	ObjectTypeUrls []string
	State          *state.State
	Relations      []*model.Relation
	App            *app.App
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

type smartBlock struct {
	state.Doc
	sync.Mutex
	depIds            []string
	sendEvent         func(e *pb.Event)
	undo              undo.History
	source            source.Source
	meta              meta.Service
	metaSub           meta.Subscriber
	metaData          *core.SmartBlockMeta
	lastDepDetails    map[string]*pb.EventObjectDetailsSet
	restrictions      restriction.Restrictions
	disableLayouts    bool
	onNewStateHooks   []func()
	onCloseHooks      []func()
	onBlockCloseHooks []func()
}

func (sb *smartBlock) HasRelation(key string) bool {
	for _, rel := range sb.Relations() {
		if rel.Key == key {
			return true
		}
	}
	return false
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) Meta() *core.SmartBlockMeta {
	return &core.SmartBlockMeta{
		ObjectTypes: sb.ObjectTypes(),
		Details:     sb.Details(),
		Relations:   sb.ExtraRelations(),
	}
}

func (sb *smartBlock) Type() model.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) Init(ctx *InitContext) (err error) {
	if sb.Doc, err = ctx.Source.ReadDoc(sb, ctx.State != nil); err != nil {
		return fmt.Errorf("reading document: %w", err)
	}

	sb.source = ctx.Source
	sb.undo = undo.NewHistory(0)
	sb.storeFileKeys()
	sb.Doc.BlocksInit()

	if ctx.State == nil {
		ctx.State = sb.NewState()
	} else {
		if !sb.Doc.(*state.State).IsEmpty() {
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

	if len(ctx.Relations) > 0 {
		if _, err = sb.addExtraRelations(ctx.State, ctx.Relations); err != nil {
			return
		}
	}

	if err = sb.normalizeRelations(ctx.State); err != nil {
		return
	}
	sb.restrictions = ctx.App.MustComponent(restriction.CName).(restriction.Service).RestrictionsByObj(sb)
	return
}

func (sb *smartBlock) normalizeRelations(s *state.State) error {
	if sb.Type() == model.SmartBlockType_Archive || sb.source.Virtual() {
		return nil
	}

	relations := sb.RelationsState(s, true)
	details := s.Details()
	if details == nil || details.Fields == nil {
		return nil
	}
	for k := range details.Fields {
		rel := pbtypes.GetRelation(relations, k)
		if rel == nil {
			if bundleRel, _ := bundle.GetRelation(bundle.RelationKey(k)); bundleRel != nil {
				s.AddRelation(bundleRel)
				log.Warnf("NormalizeRelations bundle relation is missing, have been added: %s", k)
			} else {
				log.Errorf("NormalizeRelations relation is missing: %s", k)
			}

			continue
		}

		if rel.Scope != model.Relation_object {
			log.Warnf("NormalizeRelations change scope for relation %s", rel.Key)
			s.SetExtraRelation(rel)
		}
	}
	return nil
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

func (sb *smartBlock) Show(ctx *state.Context) error {
	if ctx != nil {
		details, objectTypes, err := sb.fetchMeta()
		if err != nil {
			return err
		}

		// omit relations
		// todo: switch to other pb type
		for _, ot := range objectTypes {
			ot.Relations = nil
		}

		// todo: sb.Relations() makes extra query to read objectType which we already have here
		// the problem is that we can have an extra object type of the set in the objectTypes so we can't reuse it
		ctx.AddMessages(sb.Id(), []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfObjectShow{ObjectShow: &pb.EventObjectShow{
					RootId:       sb.RootId(),
					Type:         sb.Type(),
					Blocks:       sb.Blocks(),
					Details:      details,
					Relations:    sb.Relations(),
					ObjectTypes:  objectTypes,
					Restrictions: sb.restrictions.Proto(),
				}},
			},
		})
	}
	return nil
}

func (sb *smartBlock) fetchMeta() (details []*pb.EventObjectDetailsSet, objectTypes []*model.ObjectType, err error) {
	if sb.metaSub != nil {
		sb.metaSub.Close()
	}
	sb.metaSub = sb.meta.PubSub().NewSubscriber()
	sb.depIds = sb.dependentSmartIds(true, true)
	var ch = make(chan meta.Meta)
	subscriber := sb.metaSub.Callback(func(d meta.Meta) {
		ch <- d
	}).Subscribe(sb.depIds...)
	sb.meta.ReportChange(meta.Meta{
		BlockId:        sb.Id(),
		SmartBlockMeta: *sb.Meta(),
	})

	var uniqueObjTypes []string

	if sb.Type() == model.SmartBlockType_Set {
		// add the object type from the dataview source
		if b := sb.Doc.Pick("dataview"); b != nil {
			if dv := b.Model().GetDataview(); dv != nil {
				if dv.Source == "" {
					panic("empty dv source")
				}
				uniqueObjTypes = append(uniqueObjTypes, dv.Source)
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

	// todo: should we use badger here?
	timeout := time.After(DepSmartblockSyncEventsTimeout)

	sb.lastDepDetails = make(map[string]*pb.EventObjectDetailsSet, len(sb.depIds))
loop:
	for len(sb.lastDepDetails) < len(sb.depIds) {
		select {
		case <-timeout:
			var missingDeps []string
			for _, dep := range sb.depIds {
				if _, exists := sb.lastDepDetails[dep]; !exists {
					missingDeps = append(missingDeps, dep)
				}
			}
			log.Warnf("got %d out of %d dep objects after timeout: missing %v", len(sb.lastDepDetails), len(sb.depIds), missingDeps)
			break loop
		case d := <-ch:
			if d.Details != nil {
				sb.lastDepDetails[d.BlockId] = &pb.EventObjectDetailsSet{
					Id:      d.BlockId,
					Details: d.Details,
				}
			}
			if d.ObjectTypes != nil {
				if len(d.SmartBlockMeta.ObjectTypes) > 0 {
					if len(d.SmartBlockMeta.ObjectTypes) > 1 {
						log.Error("object has more than 1 object type which is not supported on clients. types are truncated")
					}
					ot := d.SmartBlockMeta.ObjectTypes[0]
					if len(ot) == 0 {
						log.Errorf("sb %s has empty objectType", sb.Id())
					} else {
						if slice.FindPos(uniqueObjTypes, ot) == -1 {
							uniqueObjTypes = append(uniqueObjTypes, ot)
						}
					}
				}
			}
		}
	}

	objectTypes = sb.meta.FetchObjectTypes(uniqueObjTypes)
	if len(objectTypes) != len(uniqueObjTypes) {
		var m = map[string]struct{}{}
		for _, ot := range objectTypes {
			m[ot.Url] = struct{}{}
		}

		for _, ot := range uniqueObjTypes {
			if _, exists := m[ot]; !exists {
				log.Errorf("failed to load object type '%s' for sb %s", ot, sb.Id())
			}
		}
	}

	for _, det := range sb.lastDepDetails {
		details = append(details, det)
	}

	defer func() {
		go func() {
			for d := range ch {
				sb.onMetaChange(d)
			}
		}()
		subscriber.Callback(sb.onMetaChange)
		close(ch)
	}()
	return
}

func (sb *smartBlock) onMetaChange(d meta.Meta) {
	sb.Lock()
	defer sb.Unlock()
	if sb.sendEvent != nil && d.BlockId != sb.Id() {
		msgs := []*pb.EventMessage{}
		if d.Details != nil {
			if v, exists := sb.lastDepDetails[d.BlockId]; exists {
				diff := pbtypes.StructDiff(v.Details, d.Details)
				msgs = append(msgs, state.StructDiffIntoEvents(d.BlockId, diff)...)
			} else {
				msgs = append(msgs, &pb.EventMessage{
					Value: &pb.EventMessageValueOfObjectDetailsSet{
						ObjectDetailsSet: &pb.EventObjectDetailsSet{
							Id:      d.BlockId,
							Details: d.Details,
						},
					},
				})
			}
			sb.lastDepDetails[d.BlockId] = &pb.EventObjectDetailsSet{
				Id:      d.BlockId,
				Details: d.Details,
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

func (sb *smartBlock) dependentSmartIds(includeObjTypes bool, includeCreator bool) (ids []string) {
	ids = sb.Doc.(*state.State).DepSmartIds()
	if sb.Type() != model.SmartBlockType_Breadcrumbs && sb.Type() != model.SmartBlockType_Home {
		ids = append(ids, sb.Id())

		if includeObjTypes {
			for _, ot := range sb.ObjectTypes() {
				ids = append(ids, ot)
			}
		}

		details := sb.Details()

		for _, rel := range sb.RelationsState(sb.Doc.(*state.State), false) {
			if rel.Format != model.RelationFormat_object && rel.Format != model.RelationFormat_file {
				continue
			}

			if rel.Key == bundle.RelationKeyId.String() ||
				rel.Key == bundle.RelationKeyType.String() ||
				rel.Key == bundle.RelationKeyRecommendedRelations.String() ||
				rel.Key == bundle.RelationKeyFeaturedRelations.String() ||
				!includeCreator && rel.Key == bundle.RelationKeyCreator.String() {
				continue
			}

			// add all object relation values as dependents
			for _, targetId := range pbtypes.GetStringList(details, rel.Key) {
				if targetId != "" {
					ids = append(ids, targetId)
				}
			}
		}
	}
	ids = util.UniqueStrings(ids)
	sort.Strings(ids)

	return
}

func (sb *smartBlock) SetEventFunc(f func(e *pb.Event)) {
	sb.sendEvent = f
}

func (sb *smartBlock) DisableLayouts() {
	sb.disableLayouts = true
}

func (sb *smartBlock) Apply(s *state.State, flags ...ApplyFlag) (err error) {
	var sendEvent, addHistory, doSnapshot, checkRestrictions = true, true, false, true
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
		}
	}
	if sb.source.ReadOnly() && addHistory {
		// workaround to detect user-generated action
		return fmt.Errorf("object is readonly")
	}
	if checkRestrictions {
		if err = s.CheckRestrictions(); err != nil {
			return
		}
	}
	err = source.InjectCreationInfo(sb.source, s)
	if err != nil {
		log.With("thread", sb.Id()).Errorf("injectCreationInfo failed: %s", err.Error())
	}
	// inject lastModifiedDate
	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return
	}
	if act.IsEmpty() {
		return nil
	}
	if sb.Anytype() != nil {
		s.SetLastModified(time.Now().Unix(), sb.Anytype().Account())
	}
	st := sb.Doc.(*state.State)
	if act.Details != nil && act.Details.After != nil {
		var hasDetailsChange bool
		for _, ch := range s.GetChanges() {
			if ch.GetDetailsSet() != nil {
				hasDetailsChange = true
				break
			}
		}
		// we don't need to do this in case we have other details changes inside...
		if !hasDetailsChange {
			// todo: REFACTOR ME: we need to rework indexer to include virtual changes so the localstore update will be triggered from the same place
			// here is we handling a case for indexing local-only details
			before := pbtypes.StructFilterKeys(act.Details.Before, append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...))
			after := pbtypes.StructFilterKeys(act.Details.After, append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...))
			if !pbtypes.StructEqualIgnore(before, after, nil) {
				err = sb.Anytype().ObjectStore().UpdateObjectDetails(sb.Id(), s.Details(), &model.Relations{Relations: s.ExtraRelations()}, false)
				if err != nil {
					log.Errorf("failed to update object details: %s", err.Error())
				}
			}
		}
	}

	fileDetailsKeys := sb.FileRelationKeys()
	pushChangeParams := source.PushChangeParams{
		State:             st,
		Changes:           st.GetChanges(),
		FileChangedHashes: getChangedFileHashes(s, fileDetailsKeys, act),
		DoSnapshot:        doSnapshot,
		GetAllFileHashes: func() []string {
			return st.GetAllFileHashes(fileDetailsKeys)
		},
	}
	id, err := sb.source.PushChange(pushChangeParams)
	if err != nil {
		return
	}

	sb.Doc.(*state.State).SetChangeId(id)
	if sb.undo != nil && addHistory {
		act.Group = s.GroupId()
		sb.undo.Add(act)
	}
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

	if act.Details != nil || act.ObjectTypes != nil || act.Relations != nil {
		sb.meta.ReportChange(meta.Meta{
			BlockId:        sb.Id(),
			SmartBlockMeta: *sb.Meta(),
		})
	}
	if hasDepIds(&act) {
		sb.CheckSubscriptions()
	}
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
	depIds := sb.dependentSmartIds(true, true)
	if !slice.SortedEquals(sb.depIds, depIds) {
		sb.depIds = depIds
		if sb.metaSub != nil {
			sb.metaSub.ReSubscribe(depIds...)
		}
		return true
	}
	return false
}

func (sb *smartBlock) NewState() *state.State {
	sb.execHooks(HookOnNewState)
	return sb.Doc.NewState().SetNoObjectType(sb.Type() == model.SmartBlockType_Archive || sb.Type() == model.SmartBlockType_Breadcrumbs)
}

func (sb *smartBlock) NewStateCtx(ctx *state.Context) *state.State {
	sb.execHooks(HookOnNewState)
	return sb.Doc.NewStateCtx(ctx).SetNoObjectType(sb.Type() == model.SmartBlockType_Archive || sb.Type() == model.SmartBlockType_Breadcrumbs)
}

func (sb *smartBlock) History() undo.History {
	return sb.undo
}

func (sb *smartBlock) Anytype() core.Service {
	return sb.source.Anytype()
}

func (sb *smartBlock) SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail, showEvent bool) (err error) {
	s := sb.NewStateCtx(ctx)
	detCopy := pbtypes.CopyStruct(s.Details())
	if detCopy == nil || detCopy.Fields == nil {
		detCopy = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}

	relations := sb.Relations()
	for _, detail := range details {
		if detail.Value != nil {
			rel := pbtypes.GetRelation(relations, detail.Key)
			if rel == nil {
				log.Errorf("SetDetails: missing relation for detail %s", detail.Key)
				return fmt.Errorf("relation not found: you should add the missing relation first")
			}

			if rel.Scope != model.Relation_object {
				s.SetExtraRelation(rel)
			}
			if rel.Format == model.RelationFormat_status || rel.Format == model.RelationFormat_tag {
				newOptsIds := slice.Difference(pbtypes.GetStringListValue(detail.Value), pbtypes.GetStringListValue(detCopy.Fields[detail.Key]))
				var missingOptsIds []string
				for _, newOptId := range newOptsIds {
					if !pbtypes.HasOption(rel.SelectDict, newOptId) {
						missingOptsIds = append(missingOptsIds, newOptId)
					}
				}

				if len(missingOptsIds) > 0 {
					opts, err := sb.Anytype().ObjectStore().GetAggregatedOptions(rel.Key, rel.Format, s.ObjectType())
					if err != nil {
						return err
					}

					for _, missingOptsId := range missingOptsIds {
						opt := pbtypes.GetOption(opts, missingOptsId)
						if opt == nil {
							log.Errorf("relation %s is missing option: %s", rel.Key, missingOptsId)
							continue
						}
						optCopy := *opt
						// reset scope
						optCopy.Scope = model.RelationOption_local
						_, err := s.AddExtraRelationOption(*rel, optCopy)
						if err != nil {
							return err
						}
					}
				}
			}

			err = s.ValidateNewDetail(detail.Key, detail.Value)
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

func (sb *smartBlock) AddExtraRelations(ctx *state.Context, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error) {
	s := sb.NewStateCtx(ctx)

	if relationsWithKeys, err = sb.addExtraRelations(s, relations); err != nil {
		return
	}

	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	if ctx != nil {
		// todo: send an atomic event for each changed relation
		ctx.AddMessages(sb.Id(), []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfObjectRelationsSet{
				ObjectRelationsSet: &pb.EventObjectRelationsSet{
					Id:        s.RootId(),
					Relations: sb.Relations(),
				},
			},
		}})
	}

	return
}

func (sb *smartBlock) addExtraRelations(s *state.State, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error) {
	copy := pbtypes.CopyRelations(sb.RelationsState(s, false))

	var existsMap = map[string]*model.Relation{}
	for _, rel := range copy {
		existsMap[rel.Key] = rel
	}
	for _, rel := range relations {
		if rel.Key == "" {
			rel.Key = bson.NewObjectId().Hex()
		}
		if relEx, exists := existsMap[rel.Key]; !exists {
			// we return the pointers slice here just for clarity
			relationsWithKeys = append(relationsWithKeys, rel)
			copy = append(copy, pbtypes.CopyRelation(rel))
		} else if pbtypes.RelationEqualOmitDictionary(relEx, rel) {
			relationsWithKeys = append(relationsWithKeys, relEx)
		} else {
			log.Errorf("failed to AddExtraRelations: provided relation %s not equal to existing aggregated one", rel.Key)
			return nil, fmt.Errorf("provided relation not equal to existing aggregated with the same key")
		}
		if !pbtypes.HasField(s.Details(), rel.Key) {
			s.SetDetail(rel.Key, pbtypes.Null())
		}
	}

	s = s.SetExtraRelations(copy)
	return
}

func (sb *smartBlock) SetObjectTypes(ctx *state.Context, objectTypes []string) (err error) {
	s := sb.NewState()

	if err = sb.setObjectTypes(s, objectTypes); err != nil {
		return
	}

	// send event here to send updated details to client
	if err = sb.Apply(s); err != nil {
		return
	}

	if ctx != nil {
		// todo: send an atomic event for each changed relation
		ctx.AddMessages(sb.Id(), []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfObjectRelationsSet{
				ObjectRelationsSet: &pb.EventObjectRelationsSet{
					Id:        s.RootId(),
					Relations: sb.Relations(),
				},
			},
		}})
	}
	return
}

func (sb *smartBlock) setObjectTypes(s *state.State, objectTypes []string) (err error) {
	if len(objectTypes) == 0 {
		return fmt.Errorf("you must provide at least 1 object type")
	}

	otypes := sb.meta.FetchObjectTypes(objectTypes)
	if len(otypes) == 0 {
		return fmt.Errorf("object types not found")
	}

	ot := otypes[len(otypes)-1]
	s.SetObjectTypes(objectTypes)
	s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Float64(float64(ot.Layout)))
	return
}

// UpdateExtraRelations sets the extra relations, it skips the
func (sb *smartBlock) UpdateExtraRelations(ctx *state.Context, relations []*model.Relation, createIfMissing bool) (err error) {
	objectTypeRelations := pbtypes.CopyRelations(sb.ObjectTypeRelations())
	extraRelations := pbtypes.CopyRelations(sb.ExtraRelations())
	relationsToSet := pbtypes.CopyRelations(relations)

	var somethingChanged bool
	var newRelations []*model.Relation
mainLoop:
	for i := range relationsToSet {
		for j := range objectTypeRelations {
			if objectTypeRelations[j].Key == relationsToSet[i].Key {
				if pbtypes.RelationEqual(objectTypeRelations[j], relationsToSet[i]) {
					continue mainLoop
				} else if !pbtypes.RelationCompatible(objectTypeRelations[j], relationsToSet[i]) {
					return fmt.Errorf("can't set extraRelation incompatible with the same-key relation in the objectType")
				}
			}
		}
		for j := range extraRelations {
			if extraRelations[j].Key == relationsToSet[i].Key {
				if !pbtypes.RelationEqual(extraRelations[j], relationsToSet[i]) {
					if !pbtypes.RelationCompatible(extraRelations[j], relationsToSet[i]) {
						return fmt.Errorf("can't update extraRelation: provided format is incompatible")
					}

					extraRelations[j] = relationsToSet[i]
					somethingChanged = true
				}
				continue mainLoop
			}
		}

		if createIfMissing {
			somethingChanged = true
			newRelations = append(newRelations, relations[i])
		}
	}

	if !somethingChanged {
		return
	}

	s := sb.NewStateCtx(ctx).SetExtraRelations(append(extraRelations, newRelations...))
	if err = sb.Apply(s); err != nil {
		return
	}

	if ctx != nil {
		// todo: send an atomic event for each changed relation
		ctx.AddMessages(sb.Id(), []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfObjectRelationsSet{
				ObjectRelationsSet: &pb.EventObjectRelationsSet{
					Id:        s.RootId(),
					Relations: sb.Relations(),
				},
			},
		}})
	}
	return
}

func (sb *smartBlock) RemoveExtraRelations(ctx *state.Context, relationKeys []string) (err error) {
	copy := pbtypes.CopyRelations(sb.ExtraRelations())
	filtered := []*model.Relation{}
	st := sb.NewStateCtx(ctx)

	for _, rel := range copy {
		var toBeRemoved bool
		for _, relationKey := range relationKeys {
			if rel.Key == relationKey {
				toBeRemoved = true
				break
			}
		}
		if !toBeRemoved {
			det := st.Details()
			if pbtypes.HasField(det, rel.Key) {
				delete(det.Fields, rel.Key)
			}
			filtered = append(filtered, rel)
		}
	}

	if err = sb.Apply(st, NoEvent); err != nil {
		return
	}
	if ctx != nil {
		// todo: send an atomic event for each changed relation
		ctx.AddMessages(sb.Id(), []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfObjectRelationsSet{
				ObjectRelationsSet: &pb.EventObjectRelationsSet{
					Id:        st.RootId(),
					Relations: sb.Relations(),
				},
			},
		}})
	}

	return
}

// AddRelationOption adds a new option to the select dict. It returns existing option for the relation key in case there is a one with the same text
func (sb *smartBlock) AddExtraRelationOption(ctx *state.Context, relationKey string, option model.RelationOption, showEvent bool) (*model.RelationOption, error) {
	s := sb.NewStateCtx(ctx)
	rel := pbtypes.GetRelation(sb.Relations(), relationKey)
	if rel == nil {
		var err error
		rel, err = sb.Anytype().ObjectStore().GetRelation(relationKey)
		if err != nil {
			return nil, fmt.Errorf("relation not found: %s", err.Error())
		}
	}

	if rel.Format != model.RelationFormat_status && rel.Format != model.RelationFormat_tag {
		return nil, fmt.Errorf("incorrect relation format")
	}

	newOption, err := s.AddExtraRelationOption(*rel, option)
	if err != nil {
		return nil, err
	}

	if showEvent {
		return newOption, sb.Apply(s)
	}
	return newOption, sb.Apply(s, NoEvent)
}

func (sb *smartBlock) UpdateExtraRelationOption(ctx *state.Context, relationKey string, option model.RelationOption, showEvent bool) error {
	s := sb.NewStateCtx(ctx)
	for _, rel := range sb.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != model.RelationFormat_status && rel.Format != model.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == option.Id {
				copy := pbtypes.CopyRelation(rel)
				copy.SelectDict[i] = &option
				s.SetExtraRelation(copy)

				if showEvent {
					return sb.Apply(s)
				}
				return sb.Apply(s, NoEvent)
			}
		}

		return fmt.Errorf("relation option not found")
	}

	return fmt.Errorf("relation not found")
}

func (sb *smartBlock) DeleteExtraRelationOption(ctx *state.Context, relationKey string, optionId string, showEvent bool) error {
	s := sb.NewStateCtx(ctx)
	for _, rel := range sb.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != model.RelationFormat_status && rel.Format != model.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == optionId {
				copy := pbtypes.CopyRelation(rel)
				copy.SelectDict = append(rel.SelectDict[:i], rel.SelectDict[i+1:]...)
				s.SetExtraRelation(copy)
				if showEvent {
					return sb.Apply(s)
				}
				return sb.Apply(s, NoEvent)
			}
		}
		// todo: should we remove option and value from all objects within type?

		return fmt.Errorf("relation option not found")
	}

	return fmt.Errorf("relation not found")
}

func (sb *smartBlock) StateAppend(f func(d state.Doc) (s *state.State, err error)) error {
	s, err := f(sb.Doc)
	if err != nil {
		return err
	}
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
	sb.storeFileKeys()
	if hasDepIds(&act) {
		sb.CheckSubscriptions()
	}
	return nil
}

func (sb *smartBlock) StateRebuild(d state.Doc) (err error) {
	msgs, e := sb.Doc.(*state.State).Diff(d.(*state.State))
	sb.Doc = d
	log.Infof("changes: stateRebuild: %d events", len(msgs))
	if e != nil {
		// can't make diff - reopen doc
		sb.Show(state.NewContext(sb.sendEvent))
	} else {
		if len(msgs) > 0 && sb.sendEvent != nil {
			sb.sendEvent(&pb.Event{
				Messages:  msgsToEvents(msgs),
				ContextId: sb.Id(),
			})
		}
	}
	sb.storeFileKeys()
	sb.CheckSubscriptions()
	return nil
}

func (sb *smartBlock) MetaService() meta.Service {
	return sb.meta
}

func (sb *smartBlock) BlockClose() {
	sb.execHooks(HookOnBlockClose)
	sb.SetEventFunc(nil)
}

func (sb *smartBlock) Close() (err error) {
	sb.Lock()
	sb.execHooks(HookOnClose)
	sb.Unlock()
	if sb.metaSub != nil {
		sb.metaSub.Close()
	}
	sb.source.Close()
	log.Debugf("close smartblock %v", sb.Id())
	return
}

func hasDepIds(act *undo.Action) bool {
	if act == nil {
		return true
	}
	// todo: check details for exact object-relations changes
	if act.Relations != nil || act.ObjectTypes != nil || act.Details != nil {
		return true
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

func (sb *smartBlock) storeFileKeys() {
	keys := sb.Doc.GetFileKeys()
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

func (sb *smartBlock) AddHook(f func(), events ...Hook) {
	for _, e := range events {
		switch e {
		case HookOnClose:
			sb.onCloseHooks = append(sb.onCloseHooks, f)
		case HookOnNewState:
			sb.onNewStateHooks = append(sb.onNewStateHooks, f)
		case HookOnBlockClose:
			sb.onBlockCloseHooks = append(sb.onBlockCloseHooks, f)
		}
	}
}

func mergeAndSortRelations(objTypeRelations []*model.Relation, extraRelations []*model.Relation, aggregatedRelations []*model.Relation, details *types.Struct) []*model.Relation {
	var m = make(map[string]struct{}, len(extraRelations))
	var rels = make([]*model.Relation, 0, len(objTypeRelations)+len(extraRelations))

	for _, rel := range extraRelations {
		m[rel.Key] = struct{}{}
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	for _, rel := range objTypeRelations {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
		m[rel.Key] = struct{}{}
	}

	for _, rel := range aggregatedRelations {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		m[rel.Key] = struct{}{}
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	if details == nil || details.Fields == nil {
		return rels
	}

	sort.Slice(rels, func(i, j int) bool {
		_, iExists := details.Fields[rels[i].Key]
		_, jExists := details.Fields[rels[j].Key]

		if iExists && !jExists {
			return true
		}

		return false
	})

	return rels
}

func (sb *smartBlock) baseRelations() []*model.Relation {
	rels := []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyId), bundle.MustGetRelation(bundle.RelationKeyLayout), bundle.MustGetRelation(bundle.RelationKeyIconEmoji), bundle.MustGetRelation(bundle.RelationKeyName)}
	for _, rel := range rels {
		rel.Scope = model.Relation_object
	}
	return rels
}

func (sb *smartBlock) Relations() []*model.Relation {
	return sb.RelationsState(sb.Doc.(*state.State), true)
}

func (sb *smartBlock) RelationsState(s *state.State, aggregateFromDS bool) []*model.Relation {
	if sb.Type() == model.SmartBlockType_Archive || sb.source.Virtual() {
		return sb.baseRelations()
	}

	objType := s.ObjectType()

	var err error
	var aggregatedRelation []*model.Relation
	if objType != "" && aggregateFromDS {
		aggregatedRelation, err = sb.Anytype().ObjectStore().AggregateRelationsFromSetsOfType(objType)
		if err != nil {
			log.Errorf("failed to get aggregated relations for type: %s", err.Error())
		}
	}

	rels := mergeAndSortRelations(sb.objectTypeRelations(s), s.ExtraRelations(), aggregatedRelation, s.Details())
	sb.fillAggregatedRelations(rels)
	return rels
}

func (sb *smartBlock) fillAggregatedRelations(rels []*model.Relation) {
	for i, rel := range rels {
		if rel.Format != model.RelationFormat_status && rel.Format != model.RelationFormat_tag {
			continue
		}

		options, err := sb.Anytype().ObjectStore().GetAggregatedOptions(rel.Key, rel.Format, sb.ObjectType())
		if err != nil {
			log.Errorf("failed to GetAggregatedOptions %s", err.Error())
			continue
		}

		rels[i].SelectDict = pbtypes.MergeOptionsPreserveScope(rel.SelectDict, options)
	}
}

func (sb *smartBlock) ObjectTypeRelations() []*model.Relation {
	return sb.objectTypeRelations(sb.Doc.(*state.State))
}

func (sb *smartBlock) objectTypeRelations(s *state.State) []*model.Relation {
	var relations []*model.Relation
	if sb.meta != nil {
		objectTypes := sb.meta.FetchObjectTypes(s.ObjectTypes())
		//if !(len(objectTypes) == 1 && objectTypes[0].Url == bundle.TypeKeyObjectType.URL()) {
		// do not fetch objectTypes for object type type to avoid universe collapse
		for _, objType := range objectTypes {
			relations = append(relations, objType.Relations...)
		}
		//}
	}
	return relations
}

func (sb *smartBlock) execHooks(event Hook) {
	var hooks []func()
	switch event {
	case HookOnNewState:
		hooks = sb.onNewStateHooks
	case HookOnClose:
		hooks = sb.onCloseHooks
	case HookOnBlockClose:
		hooks = sb.onBlockCloseHooks
	}
	for _, h := range hooks {
		if h != nil {
			h()
		}
	}
}

func (sb *smartBlock) FileRelationKeys() (fileKeys []string) {
	for _, rel := range sb.RelationsState(sb.Doc.(*state.State), false) {
		if rel.Format == model.RelationFormat_file {
			if slice.FindPos(fileKeys, rel.Key) == -1 {
				fileKeys = append(fileKeys, rel.Key)
			}
		}
	}
	return
}

func (sb *smartBlock) GetSearchInfo() (indexer.SearchInfo, error) {
	depIds := slice.Remove(sb.dependentSmartIds(false, false), sb.Id())

	return indexer.SearchInfo{
		Id:      sb.Id(),
		Title:   pbtypes.GetString(sb.Details(), bundle.RelationKeyName.String()),
		Snippet: sb.Snippet(),
		Text:    sb.Doc.SearchText(),
		Links:   depIds,
	}, nil
}

func msgsToEvents(msgs []simple.EventMessage) []*pb.EventMessage {
	events := make([]*pb.EventMessage, len(msgs))
	for i := range msgs {
		events[i] = msgs[i].Msg
	}
	return events
}
