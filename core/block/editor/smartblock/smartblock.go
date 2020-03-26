package smartblock

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/mohae/deepcopy"
	"github.com/prometheus/common/log"
)

type ApplyFlag int

var (
	ErrSimpleBlockNotFound = errors.New("simple block not found")
)

const (
	NoHistory ApplyFlag = iota
	NoEvent
)

func New() SmartBlock {
	return &smartBlock{}
}

type SmartBlock interface {
	Init(s source.Source) (err error)
	Id() string
	Show() (err error)
	SetEventFunc(f func(e *pb.Event))
	Apply(s *state.State, flags ...ApplyFlag) error
	History() history.History
	Anytype() anytype.Service
	SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error)
	Close() (err error)
	state.Doc
	sync.Locker
}

type smartBlock struct {
	state.Doc
	sync.Mutex
	sendEvent        func(e *pb.Event)
	hist             history.History
	source           source.Source
	metaSub          meta.Subscriber
	metaData         *core.SmartBlockMeta
	metaFetchMode    int32
	metaFetchResults chan meta.Meta
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) Init(s source.Source) error {
	ver, err := s.ReadVersion()
	if err != nil && err != core.ErrBlockSnapshotNotFound {
		return err
	}
	var blocks = make(map[string]simple.Block)
	if err == nil {
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
	sb.metaFetchResults = make(chan meta.Meta, 10)
	if sb.metaData == nil {
		sb.metaData = &core.SmartBlockMeta{
			Details: &types.Struct{
				Fields: make(map[string]*types.Value),
			},
		}
	}
	return nil
}

func (sb *smartBlock) Show() error {
	if sb.sendEvent != nil {
		details, err := sb.fetchDetails()
		if err != nil {
			return err
		}
		sb.sendEvent(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockShow{BlockShow: &pb.EventBlockShow{
						RootId:  sb.RootId(),
						Blocks:  sb.Blocks(),
						Details: details,
					}}},
			},
			ContextId: sb.RootId(),
		})
	}
	return nil
}

func (sb *smartBlock) fetchDetails() (details []*pb.EventBlockSetDetails, err error) {
	if sb.metaSub != nil {
		sb.metaSub.Close()
	}
	sb.metaSub = sb.source.Meta().PubSub().NewSubscriber()
	dependentIds := sb.dependentSmartIds()
	dependentIds = append(dependentIds, sb.Id())
	atomic.StoreInt32(&sb.metaFetchMode, 1)
	defer atomic.StoreInt32(&sb.metaFetchMode, 0)
	sb.metaSub.Callback(sb.onMetaChange).Subscribe(dependentIds...)

	sb.source.Meta().ReportChange(meta.Meta{
		BlockId:        sb.Id(),
		SmartBlockMeta: *sb.metaData,
	})
	timeout := time.After(time.Second)
	for i := 0; i < len(dependentIds); i++ {
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
	if atomic.LoadInt32(&sb.metaFetchMode) == 1 {
		sb.metaFetchResults <- d
		log.Infof("%s: detailsSend 1: %v", sb.Id(), d)
	} else {
		sb.Lock()
		defer sb.Unlock()
		if sb.sendEvent != nil {
			log.Infof("%s: detailsSend 0: %v", sb.Id(), d)
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
}

func (sb *smartBlock) dependentSmartIds() (ids []string) {
	sb.Doc.(*state.State).Iterate(func(b simple.Block) (isContinue bool) {
		if link := b.Model().GetLink(); link != nil {
			ids = append(ids, link.TargetBlockId)
		}
		return true
	})
	return
}

func (sb *smartBlock) SetEventFunc(f func(e *pb.Event)) {
	sb.Lock()
	defer sb.Unlock()
	sb.sendEvent = f
}

func (sb *smartBlock) Apply(s *state.State, flags ...ApplyFlag) (err error) {
	var sendEvent, addHistory = true, true
	msgs, act, err := state.ApplyState(s)
	if err != nil {
		return
	}
	if len(msgs) == 0 {
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
		Meta:   nil, // TODO: fill meta
		Blocks: sb.Blocks(),
	}); err != nil {
		return
	}

	if sb.hist != nil && addHistory {
		sb.hist.Add(act)
	}
	if sb.sendEvent != nil && sendEvent {
		sb.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: sb.RootId(),
		})
	}
	for _, add := range act.Add {
		if add.Model().GetLink() != nil {
			sb.checkSubscriptions()
			return
		}
	}
	for _, rem := range act.Remove {
		if rem.Model().GetLink() != nil {
			sb.checkSubscriptions()
			return
		}
	}
	return
}

func (sb *smartBlock) checkSubscriptions() {
	if sb.metaSub != nil {
		sb.metaSub.ReSubscribe(sb.dependentSmartIds()...)
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
	var copy = deepcopy.Copy(sb.metaData.Details).(*types.Struct)
	for _, detail := range details {
		copy.Fields[detail.Key] = detail.Value
	}
	if copy.Equal(sb.metaData) {
		return
	}
	sb.metaData.Details = copy
	if err = sb.source.WriteVersion(source.Version{
		Meta: sb.metaData,
	}); err != nil {
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
	return
}
