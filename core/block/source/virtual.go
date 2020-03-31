package source

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/google/uuid"
)

func NewVirtual(a anytype.Service, m meta.Service) (s Source) {
	return &virtual{
		id:   uuid.New().String(),
		a:    a,
		meta: m,
	}
}

type virtual struct {
	id   string
	a    anytype.Service
	meta meta.Service
}

func (v *virtual) Id() string {
	return v.id
}

func (v *virtual) Anytype() anytype.Service {
	return v.a
}

func (v *virtual) Meta() meta.Service {
	return v.meta
}

func (v *virtual) Type() core.SmartBlockType {
	return 0
}

func (v *virtual) ReadVersion() (*core.SmartBlockVersion, error) {
	return nil, core.ErrBlockSnapshotNotFound
}

func (v *virtual) WriteVersion(_ Version) (err error) {
	return
}

func (v *virtual) Close() (err error) {
	return
}
