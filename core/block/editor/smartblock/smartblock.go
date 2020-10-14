package smartblock

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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
	DoSnapshot
)

var log = logging.Logger("anytype-mw-smartblock")

func New(ms meta.Service) SmartBlock {
	return &smartBlock{meta: ms}
}

type SmartblockOpenListner interface {
	SmartblockOpened(*state.Context)
}

type SmartBlock interface {
	Init(s source.Source, allowEmpty bool) (err error)
	Id() string
	Type() pb.SmartBlockType
	Meta() *core.SmartBlockMeta
	Show(*state.Context) (err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() undo.History
	Anytype() anytype.Service
	SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail) (err error)
	Reindex() error
	SendEvent(msgs []*pb.EventMessage)
	ResetToVersion(s *state.State) (err error)
	DisableLayouts()
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
	depIds         []string
	sendEvent      func(e *pb.Event)
	undo           undo.History
	source         source.Source
	meta           meta.Service
	metaSub        meta.Subscriber
	metaData       *core.SmartBlockMeta
	disableLayouts bool
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) Meta() *core.SmartBlockMeta {
	return &core.SmartBlockMeta{
		Details: sb.Details(),
	}
}

func (sb *smartBlock) Type() pb.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) Init(s source.Source, allowEmpty bool) (err error) {
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
		details, err := sb.fetchDetails()
		if err != nil {
			return err
		}
		ctx.AddMessages(sb.Id(), []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfBlockShow{BlockShow: &pb.EventBlockShow{
					RootId:  sb.RootId(),
					Blocks:  sb.Blocks(),
					Details: details,
					Type:    sb.Type(),
				}},
			},
		})
	}
	return nil
}

func (sb *smartBlock) fetchDetails() (details []*pb.EventBlockSetDetails, err error) {
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

	defer func() {
		go func() {
			for d := range ch {
				sb.onMetaChange(d)
			}
		}()
		subscriber.Callback(sb.onMetaChange)
		close(ch)
	}()

	timeout := time.After(time.Second)
	for i := 0; i < len(sb.depIds); i++ {
		select {
		case <-timeout:
			return
		case d := <-ch:
			details = append(details, &pb.EventBlockSetDetails{
				Id:      d.BlockId,
				Details: d.SmartBlockMeta.Details,
			})
		}
	}
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
	var beforeSnippet = sb.Doc.Snippet()
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
		details *types.Struct
		snippet string
		links   []string
	}

	if act == nil || act.Details != nil {
		storeInfo.details = pbtypes.CopyStruct(sb.Details())
	}

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
			return at.PageStore().UpdatePage(sb.Id(), storeInfo.details, storeInfo.links, storeInfo.snippet)
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
		log.Warnf("can't ctore file keys: %v", err)
	}
}

func msgsToEvents(msgs []simple.EventMessage) []*pb.EventMessage {
	events := make([]*pb.EventMessage, len(msgs))
	for i := range msgs {
		events[i] = msgs[i].Msg
	}
	return events
}
