package source

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/vclock"
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
		err = fmt.Errorf("anytype.GetBlock(%v) error: %v", id, err)
		return
	}
	s = &source{
		id:    id,
		a:     a,
		sb:    sb,
		state: vclock.New(),
	}
	return
}

type source struct {
	id    string
	a     anytype.Service
	sb    core.SmartBlock
	state vclock.VClock
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
		if err.Error() == "no block versions found" {
			err = core.ErrorNoBlockVersionsFound
		}
		if err != core.ErrorNoBlockVersionsFound {
			err = fmt.Errorf("anytype.GetLastDownloadedVersion error: %v", err)
		}
		return nil, err
	}
	s.state = v.State
	return v, nil
}

func (s *source) WriteVersion(v Version) (err error) {
	sh, err := s.sb.PushSnapshot(s.state, v.Meta, v.Blocks)
	if err != nil {
		return
	}
	s.state = sh.State()
	return
}

func (s *source) Close() (err error) {
	return nil
}
