package source

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/google/uuid"
)

func NewVirtual(a anytype.Service, t pb.SmartBlockType) (s Source) {
	return &virtual{
		id:     uuid.New().String(),
		a:      a,
		sbType: t,
	}
}

type virtual struct {
	id     string
	a      anytype.Service
	sbType pb.SmartBlockType
}

func (v *virtual) Id() string {
	return v.id
}

func (v *virtual) Anytype() anytype.Service {
	return v.a
}

func (v *virtual) Type() pb.SmartBlockType {
	return v.sbType
}

func (v *virtual) Virtual() bool {
	return true
}

func (v *virtual) ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	return state.NewDoc(v.id, nil), nil
}

func (v *virtual) ReadMeta(_ ChangeReceiver) (doc state.Doc, err error) {
	return state.NewDoc(v.id, nil), nil
}

func (v *virtual) PushChange(st *state.State, changes []*pb.ChangeContent, fileChangedHashes []string, doSnapshot bool) (id string, err error) {
	return "", nil
}

func (v *virtual) Close() (err error) {
	return
}
