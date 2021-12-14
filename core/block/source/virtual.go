package source

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/google/uuid"
)

func NewVirtual(a core.Service, t model.SmartBlockType) (s Source) {
	return &virtual{
		id:     addr.VirtualPrefix + t.String() + "_" + uuid.New().String(),
		a:      a,
		sbType: t,
	}
}

type virtual struct {
	id     string
	a      core.Service
	sbType model.SmartBlockType
}

func (v *virtual) ReadOnly() bool {
	return false
}

func (v *virtual) Id() string {
	return v.id
}

func (v *virtual) Anytype() core.Service {
	return v.a
}

func (v *virtual) Type() model.SmartBlockType {
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

func (v *virtual) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *virtual) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *virtual) ListIds() ([]string, error) {
	// not supported
	return nil, nil
}

func (v *virtual) Close() (err error) {
	return
}

func (v *virtual) LogHeads() map[string]string {
	return nil
}

func (s *virtual) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
