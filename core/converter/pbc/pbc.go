package pbc

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

func NewConverter(s state.Doc) converter.Converter {
	return &pbc{s}
}

type pbc struct {
	s state.Doc
}

func (p *pbc) Convert() (result []byte) {
	st := p.s.NewState()
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:         st.BlocksToSave(),
			Details:        st.CombinedDetails(),
			ExtraRelations: st.ExtraRelations(),
			ObjectTypes:    st.ObjectTypes(),
			Collections:    st.Store(),
		},
	}
	for _, fk := range p.s.GetAndUnsetFileKeys() {
		snapshot.FileKeys = append(snapshot.FileKeys, &pb.ChangeFileKeys{Hash: fk.Hash, Keys: fk.Keys})
	}
	result, _ = snapshot.Marshal()
	return
}

func (p *pbc) Ext() string {
	return ".pb"
}

func (p *pbc) SetKnownDocs(map[string]*types.Struct) converter.Converter {
	return p
}

func (p *pbc) FileHashes() []string {
	return nil
}

func (p *pbc) ImageHashes() []string {
	return nil
}
