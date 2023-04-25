package pbjson

import (
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var log = logging.Logger("json-converter")

func NewConverter(s state.Doc) converter.Converter {
	return &pbj{s}
}

type pbj struct {
	s state.Doc
}

func (p *pbj) Convert(sbType model.SmartBlockType) []byte {
	st := p.s.NewState()
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:         st.BlocksToSave(),
			Details:        st.CombinedDetails(),
			ExtraRelations: st.OldExtraRelations(),
			ObjectTypes:    st.ObjectTypes(),
			Collections:    st.Store(),
			RelationLinks:  st.PickRelationLinks(),
		},
	}
	for _, fk := range p.s.GetAndUnsetFileKeys() {
		snapshot.FileKeys = append(snapshot.FileKeys, &pb.ChangeFileKeys{Hash: fk.Hash, Keys: fk.Keys})
	}
	mo := &pb.SnapshotWithType{
		SbType:   sbType,
		Snapshot: snapshot,
	}
	m := jsonpb.Marshaler{Indent: " "}
	result, err := m.MarshalToString(mo)
	if err != nil {
		log.Errorf("failed to convert object to json: %s", err.Error())
	}
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
