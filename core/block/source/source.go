package source

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

type Version struct {
	Meta   *core.SmartBlockMeta
	Blocks []*model.Block
}

type Source interface {
	Id() string
	Anytype() anytype.Service
	Type() core.SmartBlockType
	ReadVersion() (*core.SmartBlockVersion, error)
	WriteVersion(v Version) (err error)
	Close() (err error)
}

func NewSource(a anytype.Service, id string) (s Source, err error) {
	sb, err := a.GetBlock(id)
	if err != nil {
		return
	}
	s = &source{
		id:    id,
		a:     a,
		sb:    sb,
		state: nil,
	}
	return
}

type source struct {
	id    string
	a     anytype.Service
	sb    core.SmartBlock
	state core.SmartBlockState
}

func (s *source) Id() string {
	return s.id
}

func (s *source) Anytype() anytype.Service {
	return s.a
}

func (s *source) Type() core.SmartBlockType {
	return s.sb.Type()
}

func (s *source) ReadVersion() (*core.SmartBlockVersion, error) {
	v, err := s.sb.GetLastDownloadedVersion()
	if err != nil {
		return nil, err
	}
	s.state = v.State
	return v, nil
}

func (s *source) WriteVersion(v Version) (err error) {
	_, err = s.sb.PushSnapshot(s.state, v.Meta, v.Blocks)
	return
}

func (s *source) Close() (err error) {
	return nil
}
