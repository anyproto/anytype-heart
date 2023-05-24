package process

import (
	"github.com/anyproto/anytype-heart/pb"
)

type noOp struct{}

func NewNoOp() Progress {
	return &noOp{}
}

// nolint:revive
func (n *noOp) Id() string {
	return ""
}

func (n *noOp) Cancel() (err error) {
	return err
}

func (n *noOp) Info() pb.ModelProcess {
	return pb.ModelProcess{}
}

func (n *noOp) Done() chan struct{} {
	return nil
}

func (n *noOp) SetTotal(total int64) {
}

func (n *noOp) SetDone(done int64) {
}

func (n *noOp) AddDone(delta int64) {
}

func (n *noOp) SetProgressMessage(msg string) {
}

func (n *noOp) Canceled() chan struct{} {
	return nil
}

func (n *noOp) Finish() {
}

func (n *noOp) TryStep(delta int64) error {
	return nil
}
