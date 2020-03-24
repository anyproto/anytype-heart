package smartblock

import (
	"errors"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
	Close() (err error)
	state.Doc
	sync.Locker
}

type smartBlock struct {
	state.Doc
	sync.Mutex
	sendEvent func(e *pb.Event)
	hist      history.History
	source    source.Source
}

func (sb *smartBlock) Id() string {
	return sb.source.Id()
}

func (sb *smartBlock) Init(s source.Source) (err error) {
	ver, err := s.ReadVersion()
	if err != nil {
		return
	}
	models, err := ver.Snapshot.Blocks()
	if err != nil {
		return
	}
	var blocks = make(map[string]simple.Block)
	for _, m := range models {
		blocks[m.Id] = simple.New(m)
	}
	sb.Doc = state.NewDoc(s.Id(), blocks)
	sb.source = s
	sb.hist = history.NewHistory(0)
	return
}

func (sb *smartBlock) Show() (err error) {
	if sb.sendEvent != nil {
		sb.sendEvent(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfBlockShow{BlockShow: &pb.EventBlockShow{
						RootId: sb.RootId(),
						Blocks: sb.Blocks(),
					}}},
			},
			ContextId: sb.RootId(),
		})
	}
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
	return
}

func (sb *smartBlock) History() history.History {
	return sb.hist
}

func (sb *smartBlock) Anytype() anytype.Service {
	return sb.source.Anytype()
}

func (sb *smartBlock) Close() (err error) {
	return
}
