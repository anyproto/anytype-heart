package smartblock

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

type ApplyFlag int

var (
	ErrSimpleBlockNotFound = errors.New("simple block not found")
)

const (
	NoHistory ApplyFlag = iota
	NoEvent
	NoRestrictions
	NoHooks
	DoSnapshot
)

type Hook int

const (
	HookOnNewState Hook = iota
	HookOnClose
	HookOnBlockClose
)

var log = logging.Logger("anytype-mw-smartblock")

func New(ms meta.Service) SmartBlock {
	return &smartBlock{meta: ms}
}

type SmartblockOpenListner interface {
	SmartblockOpened(*state.Context)
}

type SmartBlock interface {
	Init(s source.Source, allowEmpty bool, objectTypeUrls []string) (err error)
	Id() string
	Type() pb.SmartBlockType
	Meta() *core.SmartBlockMeta
	Show(*state.Context) (err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	Anytype() anytype.Service
	SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail) (err error)
	Relations() []*pbrelation.Relation
	HasRelation(relationKey string) bool
	AddExtraRelations(relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	UpdateExtraRelations(relations []*pbrelation.Relation, createIfMissing bool) (err error)
	RemoveExtraRelations(relationKeys []string) (err error)
	AddExtraRelationSelectOption(ctx *state.Context, relationKey string, option pbrelation.RelationSelectOption, showEvent bool) (*pbrelation.RelationSelectOption, error)
	UpdateExtraRelationSelectOption(ctx *state.Context, relationKey string, option pbrelation.RelationSelectOption, showEvent bool) error
	DeleteExtraRelationSelectOption(ctx *state.Context, relationKey string, optionId string, showEvent bool) error

	SetObjectTypes(objectTypes []string) (err error)

	FileRelationKeys() []string

	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	DisableLayouts()
	AddHook(f func(), events ...Hook)
	CheckSubscriptions() (changed bool)
	GetSearchInfo() (indexer.SearchInfo, error)
	MetaService() meta.Service
	BlockClose()

	Close() (err error)
	state.Doc
	sync.Locker
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

func (sb *smartBlock) Type() pb.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	if sb.Doc, err = s.ReadDoc(sb, allowEmpty); err != nil {
		return fmt.Errorf("reading document: %w", err)
	}

	sb.source = s
	sb.undo = undo.NewHistory(0)
	sb.storeFileKeys()
	sb.Doc.BlocksInit()
	return
}

func (sb *smartBlock) SendEvent(msgs []*pb.EventMessage) {
	if sb.sendEvent != nil {
		sb.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: sb.Id(),
		})
	}
}

func (sb *smartBlock) Show(ctx *state.Context) error {
	if ctx != nil {
		details, objectTypeUrlByObject, objectTypes, err := sb.fetchMeta()
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
				Value: &pb.EventMessageValueOfBlockShow{BlockShow: &pb.EventBlockShow{
					RootId:              sb.RootId(),
					Type:                sb.Type(),
					Blocks:              sb.Blocks(),
					Details:             details,
					Relations:           sb.Relations(),
					ObjectTypePerObject: objectTypeUrlByObject,
					ObjectTypes:         objectTypes,
				}},
			},
		})
	}
	return nil
}

func (sb *smartBlock) fetchMeta() (details []*pb.EventBlockSetDetails, objectTypeUrlByObject []*pb.EventBlockShowObjectTypePerObject, objectTypes []*pbrelation.ObjectType, err error) {
	if sb.metaSub != nil {
		sb.metaSub.Close()
	}
	sb.metaSub = sb.meta.PubSub().NewSubscriber()
	sb.depIds = sb.dependentSmartIds()
	var ch = make(chan meta.Meta)
	subscriber := sb.metaSub.Callback(func(d meta.Meta) {
		ch <- d
	}).Subscribe(sb.depIds...)
	sb.meta.ReportChange(meta.Meta{
		BlockId:        sb.Id(),
		SmartBlockMeta: *sb.Meta(),
	})

	var uniqueObjTypes []string

	if sb.Type() == pb.SmartBlockType_Set {
		// add the object type from the dataview source
		if b := sb.Doc.Pick("dataview"); b != nil {
			if dv := b.Model().GetDataview(); dv != nil {
				if dv.Source == "" {
					panic("empty dv source")
				}
				uniqueObjTypes = append(uniqueObjTypes, dv.Source)
				for _, rel := range dv.Relations {
					if rel.Format == pbrelation.RelationFormat_file || rel.Format == pbrelation.RelationFormat_object {
						if rel.Key == bundle.RelationKeyId.String() || rel.Key == bundle.RelationKeyType.String() {
							continue
						}
						for _, ot := range rel.ObjectTypes {
							if slice.FindPos(uniqueObjTypes, ot) == -1 {
								if ot == "" {
									log.Errorf("dv relation %s(%s) has empty obj types", rel.Key, rel.Name)
								} else {
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
	timeout := time.After(time.Second)
	var objectTypeUrlByObjectMap = map[string]string{}

	for i := 0; i < len(sb.depIds); i++ {
		select {
		case <-timeout:
			return
		case d := <-ch:
			if d.Details != nil {
				details = append(details, &pb.EventBlockSetDetails{
					Id:      d.BlockId,
					Details: d.SmartBlockMeta.Details,
				})
			}
			if d.ObjectTypes != nil {
				if len(d.SmartBlockMeta.ObjectTypes) > 0 {
					if len(d.SmartBlockMeta.ObjectTypes) > 1 {
						log.Error("object has more than 1 object type which is not supported on clients. types are truncated")
					}
					trimedType := d.SmartBlockMeta.ObjectTypes[0]
					if slice.FindPos(uniqueObjTypes, trimedType) == -1 {
						uniqueObjTypes = append(uniqueObjTypes, trimedType)
					}
					objectTypeUrlByObjectMap[d.BlockId] = trimedType
				}
			}
		}
	}

	objectTypes = sb.meta.FetchObjectTypes(uniqueObjTypes)
	for id, ot := range objectTypeUrlByObjectMap {
		objectTypeUrlByObject = append(objectTypeUrlByObject, &pb.EventBlockShowObjectTypePerObject{
			ObjectId:   id,
			ObjectType: ot,
		})
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
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockSetDetails{
					BlockSetDetails: &pb.EventBlockSetDetails{
						Id:      d.BlockId,
						Details: d.Details,
					},
				},
			})
		}

		if d.Relations != nil {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockSetRelations{
					BlockSetRelations: &pb.EventBlockSetRelations{
						Id:        d.BlockId,
						Relations: d.Relations,
					},
				},
			})
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

func (sb *smartBlock) dependentSmartIds() (ids []string) {
	ids = sb.Doc.(*state.State).DepSmartIds()
	if sb.Type() != pb.SmartBlockType_Breadcrumbs && sb.Type() != pb.SmartBlockType_Home {
		ids = append(ids, sb.Id())

		for _, ot := range sb.ObjectTypes() {
			if strings.HasSuffix(ot, objects.CustomObjectTypeURLPrefix) {
				ids = append(ids, strings.TrimPrefix(ot, objects.CustomObjectTypeURLPrefix))
			}
		}

		details := sb.Doc.(*state.State).Details()

		for _, rel := range sb.Relations() {
			if rel.Format != pbrelation.RelationFormat_object && rel.Format != pbrelation.RelationFormat_file {
				continue
			}
			// add all custom object types as dependents
			for _, ot := range rel.ObjectTypes {
				if strings.HasPrefix(ot, objects.CustomObjectTypeURLPrefix) {
					ids = append(ids, strings.TrimPrefix(ot, objects.CustomObjectTypeURLPrefix))
				}
			}

			if rel.Key == bundle.RelationKeyId.String() || rel.Key == bundle.RelationKeyType.String() {
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
	util.UniqueStrings(ids)
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
	if checkRestrictions {
		if err = s.CheckRestrictions(); err != nil {
			return
		}
	}

	if !pbtypes.Exists(sb.Details(), "createdDate") {
		if e := sb.setCreationInfo(s); e != nil {
			log.With("thread", sb.Id()).Errorf("can't set creation info: %v", err)
		}
	}

	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return
	}
	if act.IsEmpty() {
		return nil
	}
	st := sb.Doc.(*state.State)
	fileDetailsKeys := sb.FileRelationKeys()
	pushChangeParams := source.PushChangeParams{
		State:             st,
		Changes:           st.GetChanges(),
		FileChangedHashes: getChangedFileHashes(s, fileDetailsKeys, act),
		DoSnapshot:        doSnapshot,
		GetAllFileHashes: func() []string {
			return st.GetAllFileHashes(sb.FileRelationKeys())
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

func (sb *smartBlock) setCreationInfo(s *state.State) (err error) {
	if sb.Anytype() == nil {
		return nil
	}
	var (
		createdDate = time.Now().Unix()
		createdBy   = sb.Anytype().Account()
	)
	fc, err := sb.source.FindFirstChange()
	if err == change.ErrEmpty {
		err = nil
	} else if err != nil {
		return
	} else {
		createdDate = fc.Timestamp
		createdBy = fc.Account
	}
	s.SetDetail("createdDate", pbtypes.Float64(float64(createdDate)))
	if profileId, e := threads.ProfileThreadIDFromAccountAddress(createdBy); e == nil {
		s.SetDetail("createdBy", pbtypes.String(profileId.String()))
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
	depIds := sb.dependentSmartIds()
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
	return sb.Doc.NewState()
}

func (sb *smartBlock) NewStateCtx(ctx *state.Context) *state.State {
	sb.execHooks(HookOnNewState)
	return sb.Doc.NewStateCtx(ctx)
}

func (sb *smartBlock) History() undo.History {
	return sb.undo
}

func (sb *smartBlock) Anytype() anytype.Service {
	return sb.source.Anytype()
}

func (sb *smartBlock) SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail) (err error) {
	s := sb.NewStateCtx(ctx)
	copy := pbtypes.CopyStruct(s.Details())
	if copy == nil || copy.Fields == nil {
		copy = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	for _, detail := range details {
		if detail.Value != nil {
			copy.Fields[detail.Key] = detail.Value
		} else {
			delete(copy.Fields, detail.Key)
		}
	}
	if copy.Equal(sb.Details()) {
		return
	}
	s.SetDetails(copy)
	if err = sb.Apply(s); err != nil {
		return
	}
	return
}

func (sb *smartBlock) AddExtraRelations(relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error) {
	copy := pbtypes.CopyRelations(sb.Relations())

	var existsMap = map[string]struct{}{}
	for _, rel := range copy {
		existsMap[rel.Key] = struct{}{}
	}
	for _, rel := range relations {
		if rel.Key == "" {
			rel.Key = bson.NewObjectId().Hex()
		}
		if _, exists := existsMap[rel.Key]; !exists {
			// we return the pointers slice here just for clarity
			relationsWithKeys = append(relationsWithKeys, rel)
			copy = append(copy, pbtypes.CopyRelation(rel))
		} else {
			return nil, fmt.Errorf("relation with the same key already exists")
		}
	}

	s := sb.NewState().SetExtraRelations(copy)

	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) SetObjectTypes(objectTypes []string) (err error) {
	if len(objectTypes) == 0 {
		return fmt.Errorf("you must provide at least 1 object type")
	}

	otypes := sb.meta.FetchObjectTypes(objectTypes)
	if len(otypes) == 0 {
		return fmt.Errorf("object types not found")
	}

	ot := otypes[len(otypes)-1]
	s := sb.NewState().SetObjectTypes(objectTypes)
	s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Float64(float64(ot.Layout)))

	// send event here to send updated details to client
	if err = sb.Apply(s); err != nil {
		return
	}
	return
}

func (sb *smartBlock) UpdateExtraRelations(relations []*pbrelation.Relation, createIfMissing bool) (err error) {
	objectTypeRelations := pbtypes.CopyRelations(sb.ObjectTypeRelations())
	extraRelations := pbtypes.CopyRelations(sb.ExtraRelations())
	relationsToSet := pbtypes.CopyRelations(relations)

	var somethingChanged bool
	var newRelations []*pbrelation.Relation
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

	s := sb.NewState().SetExtraRelations(append(extraRelations, newRelations...))
	if err = sb.Apply(s); err != nil {
		return
	}
	return
}

func (sb *smartBlock) RemoveExtraRelations(relationKeys []string) (err error) {
	copy := pbtypes.CopyRelations(sb.ExtraRelations())
	filtered := []*pbrelation.Relation{}
	for _, rel := range copy {
		var toBeRemoved bool
		for _, relationKey := range relationKeys {
			if rel.Key == relationKey {
				toBeRemoved = true
				break
			}
		}
		if !toBeRemoved {
			filtered = append(filtered, rel)
		}
	}

	s := sb.NewState().SetExtraRelations(filtered)
	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

// AddRelationSelectOption adds a new option to the select dict. It returns existing option for the relation key in case there is a one with the same text
func (sb *smartBlock) AddExtraRelationSelectOption(ctx *state.Context, relationKey string, option pbrelation.RelationSelectOption, showEvent bool) (*pbrelation.RelationSelectOption, error) {
	s := sb.NewStateCtx(ctx)
	for _, rel := range s.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return nil, fmt.Errorf("relation has incorrect format")
		}
		for _, opt := range rel.SelectDict {
			if strings.EqualFold(opt.Text, option.Text) {
				// here we can have the option with another color. but it looks
				return opt, nil
			}
		}

		option.Id = bson.NewObjectId().Hex()
		rel.SelectDict = append(rel.SelectDict, &option)

		if showEvent {
			return &option, sb.Apply(s)
		}
		return &option, sb.Apply(s, NoEvent)
	}

	return nil, fmt.Errorf("relation not found")
}

func (sb *smartBlock) UpdateExtraRelationSelectOption(ctx *state.Context, relationKey string, option pbrelation.RelationSelectOption, showEvent bool) error {
	s := sb.NewStateCtx(ctx)

	for _, rel := range s.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == option.Id {
				rel.SelectDict[i] = &option
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

func (sb *smartBlock) DeleteExtraRelationSelectOption(ctx *state.Context, relationKey string, optionId string, showEvent bool) error {
	s := sb.NewStateCtx(ctx)

	for _, rel := range s.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == optionId {
				rel.SelectDict = append(rel.SelectDict[:i], rel.SelectDict[i+1:]...)
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
	sb.execHooks(HookOnClose)
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
				if v := det.Fields[field]; v != nil && v.GetStringValue() != "" {
					hashes = append(hashes, v.GetStringValue())
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

func mergeAndSortRelations(objTypeRelations []*pbrelation.Relation, extraRelations []*pbrelation.Relation, details *types.Struct) []*pbrelation.Relation {
	var m = make(map[string]struct{}, len(extraRelations))
	var rels = make([]*pbrelation.Relation, 0, len(objTypeRelations)+len(extraRelations))

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

func (sb *smartBlock) Relations() []*pbrelation.Relation {
	return mergeAndSortRelations(sb.ObjectTypeRelations(), sb.ExtraRelations(), sb.Details())
}

func (sb *smartBlock) ObjectTypeRelations() []*pbrelation.Relation {
	var relations []*pbrelation.Relation
	if sb.meta != nil {
		objectTypes := sb.meta.FetchObjectTypes(sb.ObjectTypes())
		if !(len(objectTypes) == 1 && objectTypes[0].Url == bundle.TypeKeyObjectType.URL()) {
			// do not fetch objectTypes for object type type to avoid universe collapse
			for _, objType := range objectTypes {
				relations = append(relations, objType.Relations...)
			}
		}
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
	for _, rel := range sb.Relations() {
		if rel.Format == pbrelation.RelationFormat_file {
			if slice.FindPos(fileKeys, rel.Key) == -1 {
				fileKeys = append(fileKeys, rel.Key)
			}
		}
	}
	return
}

func (sb *smartBlock) GetSearchInfo() (indexer.SearchInfo, error) {
	depIds := slice.Remove(sb.dependentSmartIds(), sb.Id())
	return indexer.SearchInfo{
		Id:      sb.Id(),
		Title:   pbtypes.GetString(sb.Details(), "name"),
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
