package smartblock

import (
	"errors"
	"fmt"
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

func New() SmartBlock {
	return &smartBlock{}
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
	depIds    []string
	sendEvent func(e *pb.Event)
	hist      history.History
	source    source.Source
	metaSub   meta.Subscriber
	metaData  *core.SmartBlockMeta
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) Meta() *core.SmartBlockMeta {
	return sb.metaData
}

func (sb *smartBlock) Type() pb.SmartBlockType {
	return sb.source.Type()
}

func (sb *smartBlock) init(s source.Source, ver *core.SmartBlockVersion) error {
	var blocks = make(map[string]simple.Block)
	if ver != nil {
		models, e := ver.Snapshot.Blocks()
		if e != nil {
			return e
		}
		for _, m := range models {
			blocks[m.Id] = simple.New(m)
		}
		sb.metaData, e = ver.Snapshot.Meta()
		if e != nil {
			return fmt.Errorf("can't get meta from snapshot: %v", e)
		}
	}
	sb.Doc = state.NewDoc(s.Id(), blocks)
	sb.source = s
	sb.hist = history.NewHistory(0)
	if sb.metaData == nil {
		sb.metaData = &core.SmartBlockMeta{
			Details: &types.Struct{
				Fields: make(map[string]*types.Value),
			},
		}
	}
	return sb.checkRootBlock()
}

func (sb *smartBlock) Init(s source.Source, allowEmpty bool) error {
	ver, err := s.ReadVersion()
	if err != nil && (err != core.ErrBlockSnapshotNotFound || !allowEmpty) {
		return err
	}

	return sb.init(s, ver)
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
	sb.metaSub = sb.source.Meta().PubSub().NewSubscriber()
	sb.depIds = sb.dependentSmartIds()
	var ch = make(chan meta.Meta)
	subscriber := sb.metaSub.Callback(func(d meta.Meta) {
		ch <- d
	}).Subscribe(sb.depIds...)
	sb.source.Meta().ReportChange(meta.Meta{
		BlockId:        sb.Id(),
		SmartBlockMeta: *sb.metaData,
	})

	defer func() {
		subscriber.Callback(sb.onMetaChange)
		go func() {
			for d := range ch {
				sb.onMetaChange(d)
			}
		}()
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
	var sendEvent, addHistory = true, true
	msgs, act, err := state.ApplyState(s)
	if err != nil {
		return
	}
	for _, f := range flags {
		switch f {
		case NoEvent:
			sendEvent = false
		case NoHistory:
			addHistory = false
		}
	}

	if err = sb.source.WriteVersion(source.Version{
		Meta:   sb.metaData,
		Blocks: sb.Blocks(),
	}); err != nil {
		return
	}

	if len(msgs) == 0 {
		return
	}

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
	for _, edit := range act.Change {
		if ls, ok := edit.After.(linkSource); ok && ls.HasSmartIds() {
			sb.checkSubscriptions()
			return
		}
		if ls, ok := edit.Before.(linkSource); ok && ls.HasSmartIds() {
			sb.checkSubscriptions()
			return
		}
	}
	for _, add := range act.Add {
		if ls, ok := add.(linkSource); ok && ls.HasSmartIds() {
			sb.checkSubscriptions()
			return
		}
	}
	for _, rem := range act.Remove {
		if ls, ok := rem.(linkSource); ok && ls.HasSmartIds() {
			sb.checkSubscriptions()
			return
		}
	}
	return
}

func (sb *smartBlock) checkSubscriptions() {
	if sb.metaSub != nil {
		depIds := sb.dependentSmartIds()
		if !slice.SortedEquals(sb.depIds, depIds) {
			sb.depIds = depIds
			sb.metaSub.ReSubscribe(depIds...)
		}
	}
}

func (sb *smartBlock) History() history.History {
	return sb.hist
}

func (sb *smartBlock) Anytype() anytype.Service {
	return sb.source.Anytype()
}

func (sb *smartBlock) SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error) {
	if sb.metaData == nil {
		sb.metaData = &core.SmartBlockMeta{}
	}
	if sb.metaData.Details == nil || sb.metaData.Details.Fields == nil {
		sb.metaData.Details = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	var copy = pbtypes.CopyStruct(sb.metaData.Details)
	if copy.Fields == nil {
		copy.Fields = make(map[string]*types.Value)
	}
	for _, detail := range details {
		copy.Fields[detail.Key] = detail.Value
	}
	if copy.Equal(sb.metaData) {
		return
	}
	sb.metaData.Details = copy
	if err = sb.Apply(sb.NewState(), NoHistory, NoEvent); err != nil {
		return
	}
	sb.source.Meta().ReportChange(meta.Meta{
		BlockId:        sb.Id(),
		SmartBlockMeta: *sb.metaData,
	})
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
