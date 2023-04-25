package process

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type NoOp struct{}

func NewNoOp() IProgress {
	return &NoOp{}
}

// nolint:revive
func (n *NoOp) Id() string {
	return ""
}

func (n *NoOp) Cancel() (err error) {
	return err
}

func (n *NoOp) Info() pb.ModelProcess {
	return pb.ModelProcess{}
}

func (n *NoOp) Done() chan struct{} {
	return nil
}

func (n *NoOp) SetTotal(total int64) {
}

func (n *NoOp) SetDone(done int64) {
}

func (n *NoOp) AddDone(delta int64) {
}

func (n *NoOp) SetProgressMessage(msg string) {
}

func (n *NoOp) Canceled() chan struct{} {
	return nil
}

func (n *NoOp) Finish() {
}

func (n *NoOp) TryStep(delta int64) error {
	return nil
}
