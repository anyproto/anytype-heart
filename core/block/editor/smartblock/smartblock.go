package smartblock

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
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
)

var log = logging.Logger("anytype-mw-smartblock")

func New(ms meta.Service, defaultObjectTypeUrl string) SmartBlock {
	return &smartBlock{meta: ms, defaultObjectTypeUrl: defaultObjectTypeUrl}
}

type SmartblockOpenListner interface {
	SmartblockOpened(*state.Context)
}

type SmartBlock interface {
	Init(s source.Source, allowEmpty bool, objectTypeUrls []string) (err error)
	Id() string
	DefaultObjectTypeUrl() string
	Type() pb.SmartBlockType
	Meta() *core.SmartBlockMeta
	Show(*state.Context) (err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	Anytype() anytype.Service
	SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail) (err error)
	AddRelations(relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	UpdateRelations(relations []*pbrelation.Relation) (err error)
	RemoveRelations(relationKeys []string) (err error)
	AddObjectTypes(objectTypes []string) (err error)
	RemoveObjectTypes(objectTypes []string) (err error)

	Reindex() error
	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	DisableLayouts()
	AddHook(f func(), events ...Hook)
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
	depIds               []string
	sendEvent            func(e *pb.Event)
	undo                 undo.History
	source               source.Source
	meta                 meta.Service
	metaSub              meta.Subscriber
	metaData             *core.SmartBlockMeta
	disableLayouts       bool
	defaultObjectTypeUrl string
	onNewStateHooks      []func()
	onCloseHooks         []func()
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) Meta() *core.SmartBlockMeta {
	return &core.SmartBlockMeta{
		ObjectTypes: sb.ObjectTypes(),
		Details:     sb.Details(),
		Relations:   sb.Relations(),
	}
}

func (sb *smartBlock) DefaultObjectTypeUrl() string {
	return sb.defaultObjectTypeUrl
}

func (sb *smartBlock) Type() pb.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	if sb.Doc, err = s.ReadDoc(sb, allowEmpty); err != nil {
		return err
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
		details, _, objectTypesUrlByObject, objectTypes, err := sb.fetchMeta()
		if err != nil {
			return err
		}

		var layout pbrelation.ObjectTypeLayout
		for _, objectTypesUrlForObject := range objectTypesUrlByObject {
			if objectTypesUrlForObject.ObjectId != sb.Id() {
				continue
			}
			for _, otUrl := range objectTypesUrlForObject.ObjectTypes {
				for _, ot := range objectTypes {
					if ot.Url == otUrl {
						layout = ot.Layout
					}
				}
			}

		}

		ctx.AddMessages(sb.Id(), []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfBlockShow{BlockShow: &pb.EventBlockShow{
					RootId:               sb.RootId(),
					Type:                 sb.Type(),
					Blocks:               sb.Blocks(),
					Details:              details,
					ObjectTypesPerObject: objectTypesUrlByObject,
					ObjectTypes:          objectTypes,
					Layout:               layout,
				}},
			},
		})
	}
	return nil
}

func (sb *smartBlock) fetchMeta() (details []*pb.EventBlockSetDetails, relationsByObject []*pb.EventBlockShowRelationWithValuePerObject, objectTypesUrlByObject []*pb.EventBlockShowObjectTypesPerObject, objectTypes []*pbrelation.ObjectType, err error) {
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

	var objectTypesMap = map[string]struct{}{}
	for _, ot := range sb.ObjectTypes() {
		objectTypesMap[ot] = struct{}{}
	}

	if sb.Type() == pb.SmartBlockType_Set {
		// add the object type from the dataview source
		if b := sb.Doc.Pick("dataview"); b != nil {
			if dv := b.Model().GetDataview(); dv != nil {
				objectTypesMap[dv.Source] = struct{}{}
			}
		}
	}

	timeout := time.After(time.Second)
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
				for _, ot := range d.ObjectTypes {
					objectTypesMap[ot] = struct{}{}
				}

				objectTypesUrlByObject = append(objectTypesUrlByObject, &pb.EventBlockShowObjectTypesPerObject{
					ObjectId:    d.BlockId,
					ObjectTypes: d.SmartBlockMeta.ObjectTypes,
				})
			}
		}
	}

	var objectTypesUrls []string
	for ot := range objectTypesMap {
		objectTypesUrls = append(objectTypesUrls, ot)
	}

	objectTypes = sb.meta.FetchObjectTypes(objectTypesUrls)
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
		sb.sendEvent(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockSetDetails{
						BlockSetDetails: &pb.EventBlockSetDetails{
							Id:      d.BlockId,
							Details: d.Details,
						},
					},
				},
			},
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
	}
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

	var beforeSnippet = sb.Doc.Snippet()
	msgs, act, err := state.ApplyState(s, !sb.disableLayouts)
	if err != nil {
		return
	}
	if act.IsEmpty() {
		return nil
	}
	changes := sb.Doc.(*state.State).GetChanges()
	fileHashes := getChangedFileHashes(act)
	id, err := sb.source.PushChange(sb.Doc.(*state.State), changes, fileHashes, doSnapshot)
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

	if act.Details != nil {
		sb.meta.ReportChange(meta.Meta{
			BlockId:        sb.Id(),
			SmartBlockMeta: *sb.Meta(),
		})
	}
	sb.updatePageStoreNoErr(beforeSnippet, &act)
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
	sb.updatePageStoreNoErr("", nil)
	return
}

func (sb *smartBlock) updatePageStoreNoErr(beforeSnippet string, act *undo.Action) {
	if e := sb.updatePageStore(beforeSnippet, act); e != nil {
		log.Warnf("can't update pageStore info: %v", e)
	}
}

func (sb *smartBlock) updatePageStore(beforeSnippet string, act *undo.Action) (err error) {
	if sb.Type() == pb.SmartBlockType_Archive {
		return
	}

	var storeInfo struct {
		details   *types.Struct
		relations *pbrelation.Relations
		snippet   string
		links     []string
	}

	if act == nil || act.Details != nil {
		storeInfo.details = pbtypes.CopyStruct(sb.Details())
	}

	if act == nil || act.Relations != nil {
		storeInfo.relations = &pbrelation.Relations{Relations: pbtypes.CopyRelations(sb.Relations())}
	}

	if storeInfo.details == nil || storeInfo.details.Fields == nil {
		storeInfo.details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	storeInfo.details.Fields["type"] = pbtypes.StringList(sb.ObjectTypes())

	if hasDepIds(act) {
		if sb.checkSubscriptions() {
			storeInfo.links = make([]string, len(sb.depIds))
			copy(storeInfo.links, sb.depIds)
			storeInfo.links = slice.Remove(storeInfo.links, sb.Id())
		}
	}

	if afterSnippet := sb.Doc.Snippet(); beforeSnippet != afterSnippet {
		storeInfo.snippet = afterSnippet
	}

	if at := sb.Anytype(); at != nil && sb.Type() != pb.SmartBlockType_Breadcrumbs {
		if storeInfo.links != nil || storeInfo.details != nil || len(storeInfo.snippet) > 0 {
			return at.ObjectStore().UpdateObject(sb.Id(), storeInfo.details, storeInfo.relations, storeInfo.links, storeInfo.snippet)
		}
	}

	return nil
}

func (sb *smartBlock) checkSubscriptions() (changed bool) {
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
		copy.Fields[detail.Key] = detail.Value
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

func (sb *smartBlock) AddRelations(relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error) {
	copy := pbtypes.CopyRelations(sb.Relations())

	var existsMap = map[string]struct{}{}
	for _, rel := range copy {
		existsMap[rel.Key] = struct{}{}
	}
	for _, rel := range relations {
		if _, exists := existsMap[rel.Key]; !exists {
			if rel.Key == "" {
				rel.Key = bson.NewObjectId().Hex()
			}
			// we return the pointers slice here just for clarity
			relationsWithKeys = append(relationsWithKeys, rel)
			copy = append(copy, rel)
		} else {
			return nil, fmt.Errorf("relation with the same key already exists")
		}
	}

	s := sb.NewState().SetRelations(copy)

	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) AddObjectTypes(objectTypes []string) (err error) {
	c := make([]string, len(sb.ObjectTypes()))
	copy(c, sb.ObjectTypes())

	c = append(c, objectTypes...)
	s := sb.NewState().SetObjectTypes(c)

	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) RemoveObjectTypes(objectTypes []string) (err error) {
	filtered := []string{}

	for _, ot := range sb.ObjectTypes() {
		var toBeRemoved bool
		for _, OTToRemove := range objectTypes {
			if ot == OTToRemove {
				toBeRemoved = true
				break
			}
		}
		if !toBeRemoved {
			filtered = append(filtered, ot)
		}
	}

	s := sb.NewState().SetObjectTypes(filtered)

	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) UpdateRelations(relations []*pbrelation.Relation) (err error) {
	copy := pbtypes.CopyRelations(sb.Relations())

	for i, rel := range copy {
		for _, relation := range relations {
			if rel.Key == relation.Key {
				copy[i] = relation
			}
		}
	}

	s := sb.NewState().SetRelations(copy)
	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) RemoveRelations(relationKeys []string) (err error) {
	copy := pbtypes.CopyRelations(sb.Relations())
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

	s := sb.NewState().SetRelations(filtered)
	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) StateAppend(f func(d state.Doc) (s *state.State, err error)) error {
	beforeSnippet := sb.Doc.Snippet()
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
	sb.updatePageStoreNoErr(beforeSnippet, &act)
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
	sb.updatePageStoreNoErr("", nil)
	return nil
}

func (sb *smartBlock) Reindex() (err error) {
	return sb.updatePageStore("", nil)
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

func getChangedFileHashes(act undo.Action) (hashes []string) {
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
			for _, field := range state.DetailsFileFields {
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
	fileKeys := make([]core.FileKeys, len(keys))
	for i, k := range keys {
		fileKeys[i] = core.FileKeys{
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
		}
	}
}

func (sb *smartBlock) execHooks(event Hook) {
	var hooks []func()
	switch event {
	case HookOnNewState:
		hooks = sb.onNewStateHooks
	case HookOnClose:
		hooks = sb.onCloseHooks
	}
	for _, h := range hooks {
		if h != nil {
			h()
		}
	}
}

func msgsToEvents(msgs []simple.EventMessage) []*pb.EventMessage {
	events := make([]*pb.EventMessage, len(msgs))
	for i := range msgs {
		events[i] = msgs[i].Msg
	}
	return events
}
