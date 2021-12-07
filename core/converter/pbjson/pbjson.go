package pbjson

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/jsonpb"
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
	for _, fk := range p.s.GetFileKeys() {
		snapshot.FileKeys = append(snapshot.FileKeys, &fk)
	}
	m := jsonpb.Marshaler{Indent: " "}
	result, _ := m.MarshalToString(snapshot)
	return []byte(result)
}

func (p *pbj) Ext() string {
	return ".pb.json"
}

func (p *pbj) SetKnownLinks(ids []string) converter.Converter {
	return p
}

func (p *pbj) FileHashes() []string {
	return nil
}

func (p *pbj) ImageHashes() []string {
	return nil
}
