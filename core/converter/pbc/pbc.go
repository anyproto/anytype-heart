package pbc

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewConverter(s *state.State) converter.Converter {
	return &pbc{s}
}

type pbc struct {
	s *state.State
}

func (p *pbc) Convert() (result []byte) {
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:         p.s.BlocksToSave(),
			Details:        p.s.Details(),
			ExtraRelations: p.s.ExtraRelations(),
			ObjectTypes:    p.s.ObjectTypes(),
			Collections:    p.s.Collections(),
		},
	}
	for _, fk := range p.s.GetFileKeys() {
		snapshot.FileKeys = append(snapshot.FileKeys, &fk)
	}
	result, _ = snapshot.Marshal()
	return
}

func (p *pbc) Ext() string {
	return ".pb"
}

func (p *pbc) SetKnownLinks(ids []string) converter.Converter {
	return p
}

func (p *pbc) FileHashes() []string {
	return nil
}

func (p *pbc) ImageHashes() []string {
	return nil
}
