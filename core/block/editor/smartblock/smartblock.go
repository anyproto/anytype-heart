package smartblock

import (
	"errors"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
)

var log = logging.Logger("anytype-mw-smartblock")

func New(ms meta.Service) SmartBlock {
	return &smartBlock{meta: ms}
}

type SmartBlock interface {
	Init(s source.Source, allowEmpty bool) (err error)
	Id() string
	Type() pb.SmartBlockType
	Meta() *core.SmartBlockMeta
	Show(*state.Context) (err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() history.History
	Anytype() anytype.Service
	SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error)
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
	depIds           []string
	sendEvent        func(e *pb.Event)
	hist             history.History
	source           source.Source
	meta             meta.Service
	metaSub          meta.Subscriber
	metaFetchResults chan meta.Meta
	metaFetchMu      sync.Mutex
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
	if sb.Doc, err = s.ReadDoc(); err != nil {
		return err
	}
	sb.source = s
	sb.hist = history.NewHistory(0)
	return sb.checkRootBlock()
}

func (sb *smartBlock) checkRootBlock() (err error) {
	s := sb.NewState()
	if root := s.Get(sb.RootId()); root != nil {
		return
	}
	s.Add(simple.New(&model.Block{
		Id: sb.RootId(),
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
	}))
	return sb.Apply(s, NoEvent, NoHistory)
}

func (sb *smartBlock) Show(ctx *state.Context) error {
	if ctx != nil {
		details, err := sb.fetchDetails()
		if err != nil {
			return err
		}
		ctx.SetMessages(sb.Id(), []*pb.EventMessage{
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
	sb.metaFetchMu.Lock()
	sb.metaFetchResults = make(chan meta.Meta)
	if sb.metaSub != nil {
		sb.metaSub.Close()
	}
	sb.metaSub = sb.meta.PubSub().NewSubscriber()
	sb.depIds = sb.dependentSmartIds()
	sb.metaSub.Callback(sb.onMetaChange).Subscribe(sb.depIds...)
	sb.metaFetchMu.Unlock()
	defer func() {
		sb.metaFetchMu.Lock()
		ch := sb.metaFetchResults
		sb.metaFetchResults = nil
		sb.metaFetchMu.Unlock()
		timeout := time.After(time.Millisecond * 10)
		for {
			select {
			case d := <-ch:
				sb.onMetaChange(d)
			case <-timeout:
				return
			}
		}
	}()
	sb.meta.ReportChange(meta.Meta{
		BlockId:        sb.Id(),
		SmartBlockMeta: *sb.Meta(),
	})
	timeout := time.After(time.Second)
	for i := 0; i < len(sb.depIds); i++ {
		select {
		case <-timeout:
			return
		case d := <-sb.metaFetchResults:
			details = append(details, &pb.EventBlockSetDetails{
				Id:      d.BlockId,
				Details: d.SmartBlockMeta.Details,
			})
		}
	}
	return
}

func (sb *smartBlock) onMetaChange(d meta.Meta) {
	sb.metaFetchMu.Lock()
	if sb.metaFetchResults != nil {
		sb.metaFetchResults <- d
		sb.metaFetchMu.Unlock()
		return
	}
	sb.metaFetchMu.Unlock()
	sb.Lock()
	defer sb.Unlock()
	if sb.sendEvent != nil {
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
	ids = make([]string, 0, 30)
	sb.Doc.(*state.State).Iterate(func(b simple.Block) (isContinue bool) {
		if ls, ok := b.(linkSource); ok {
			ids = ls.FillSmartIds(ids)
		}
		return true
	})
	if sb.Type() != pb.SmartBlockType_Breadcrumbs && sb.Type() != pb.SmartBlockType_Home {
		ids = append(ids, sb.Id())
	}
	sort.Strings(ids)
	return
}

func (sb *smartBlock) SetEventFunc(f func(e *pb.Event)) {
	sb.sendEvent = f
}

func (sb *smartBlock) Apply(s *state.State, flags ...ApplyFlag) (err error) {
	var beforeSnippet = sb.Doc.Snippet()
	var sendEvent, addHistory = true, true
	msgs, act, err := state.ApplyState(s)
	if err != nil {
		return
	}
	if act.IsEmpty() {
		return nil
	}
	for _, f := range flags {
		switch f {
		case NoEvent:
			sendEvent = false
		case NoHistory:
			addHistory = false
		}
	}
	changes := sb.Doc.(*state.State).GetChanges()
	if len(changes) == 0 {
		log.Infof("empty changes, but not empty history: %+v", act)
		debug.PrintStack()
	}
	id, err := sb.source.PushChange(sb.Doc.(*state.State), changes...)
	if err != nil {
		return
	}
	sb.Doc.(*state.State).SetChangeId(id)
	if sb.hist != nil && addHistory {
		sb.hist.Add(act)
	}
	if sendEvent {
		if ctx := s.Context(); ctx != nil {
			ctx.SetMessages(sb.Id(), msgs)
		} else if sb.sendEvent != nil {
			sb.sendEvent(&pb.Event{
				Messages:  msgs,
				ContextId: sb.RootId(),
			})
		}
	}

	var storeInfo struct {
		details *types.Struct
		snippet *string
		links   []string
	}

	if act.Details != nil {
		sb.meta.ReportChange(meta.Meta{
			BlockId:        sb.Id(),
			SmartBlockMeta: *sb.Meta(),
		})
		storeInfo.details = pbtypes.CopyStruct(sb.Details())
	}

	if hasDepIds(act) {
		if sb.checkSubscriptions() {
			storeInfo.links = make([]string, len(sb.depIds))
			copy(storeInfo.links, sb.depIds)
			storeInfo.links = slice.Remove(storeInfo.links, sb.Id())
		}
	}

	afterSnippet := sb.Doc.Snippet()
	if beforeSnippet != afterSnippet {
		storeInfo.snippet = &afterSnippet
	}

	if at := sb.Anytype(); at != nil && sb.Type() != pb.SmartBlockType_Breadcrumbs {
		if storeInfo.links != nil || storeInfo.details != nil || storeInfo.snippet != nil {
			if e := at.PageStore().Update(sb.Id(), storeInfo.details, storeInfo.links, storeInfo.snippet); e != nil {
				log.Warnf("can't update pageStore info: %v", e)
			}
			log.Infof("pageStore: %s: %+v", sb.Id(), storeInfo)
		}
	}
	return
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

func (sb *smartBlock) History() history.History {
	return sb.hist
}

func (sb *smartBlock) Anytype() anytype.Service {
	return sb.source.Anytype()
}

func (sb *smartBlock) SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error) {
	copy := pbtypes.CopyStruct(sb.Details())
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
	s := sb.NewState().SetDetails(copy)
	if err = sb.Apply(s, NoEvent); err != nil {
		return
	}
	return
}

func (sb *smartBlock) Close() (err error) {
	if sb.metaSub != nil {
		sb.metaSub.Close()
	}
	sb.source.Close()
	log.Debugf("close smartblock %v", sb.Id())
	return
}

func hasDepIds(act history.Action) bool {
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
