package smartblock

import (
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func New() SmartBlock {
	return &smartBlock{
	}
}

type SmartBlock interface {
	Init(s source.Source) (err error)
	Open() (err error)
	Show() (err error)
	SetEventFunc(f func(e *pb.Event))
	Close() (err error)
	NewState() *state.State
	sync.Locker
}

type smartBlock struct {
	state.Doc
	sync.Mutex
	sendEvent func(e *pb.Event)
}

func (sb *smartBlock) Init(s source.Source) (err error) {
	panic("implement me")
}

func (sb *smartBlock) Open() (err error) {
	panic("implement me")
}

func (sb *smartBlock) Show() (err error) {
	panic("implement me")
}

func (sb *smartBlock) SetEventFunc(f func(e *pb.Event)) {
	panic("implement me")
}

func (sb *smartBlock) Close() (err error) {
	panic("implement me")
}
