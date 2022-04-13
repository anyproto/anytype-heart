package pbjson

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
)

func NewConverter(s state.Doc) converter.Converter {
	return &pbj{s}
}

type pbj struct {
	s state.Doc
}

func (p *pbj) Convert() []byte {
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
	m := jsonpb.Marshaler{Indent: " "}
	result, _ := m.MarshalToString(snapshot)
	return []byte(result)
}

func (p *pbj) Ext() string {
	return ".pb.json"
}

func (p *pbj) SetKnownDocs(map[string]*types.Struct) converter.Converter {
	return p
}

func (p *pbj) FileHashes() []string {
	return nil
}

func (p *pbj) ImageHashes() []string {
	return nil
}
