package block

import "github.com/anytypeio/go-anytype-middleware/core/anytype"

func newDashboard(s *service, block anytype.Block) (smartBlock, error) {
	d := &dashboard{s: s, id: block.GetId()}
	return d, nil
}

type dashboard struct {
	id string
	s  *service
}

func (d *dashboard) Open(b anytype.Block) error {
	panic("implement me")
}

func (d *dashboard) GetId() string {
	return d.id
}

func (d *dashboard) Type() smartBlockType {
	return smartBlockTypeDashboard
}

func (d *dashboard) Close() error {
	panic("implement me")
}
