package pbc

import (
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var log = logging.Logger("pb-converter")

func NewConverter(s state.Doc, isJSON bool) converter.Converter {
	return &pbc{
		s:      s,
		isJSON: isJSON,
	}
}

type pbc struct {
	s      state.Doc
	isJSON bool
}

func (p *pbc) Convert(sbType model.SmartBlockType, id string) []byte {
	st := p.s.NewState()
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.BlocksToSave(),
			Details:       st.CombinedDetails(),
			ObjectTypes:   st.ObjectTypes(),
			Collections:   st.Store(),
			RelationLinks: st.PickRelationLinks(),
		},
	}
	for _, fk := range p.s.GetAndUnsetFileKeys() {
		snapshot.FileKeys = append(snapshot.FileKeys, &pb.ChangeFileKeys{Hash: fk.Hash, Keys: fk.Keys})
	}

	if snapshot.Data.Details == nil || snapshot.Data.Details.Fields == nil {
		snapshot.Data.Details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	snapshot.Data.Details.Fields[bundle.RelationKeyOldAnytypeID.String()] = pbtypes.String(id)

	mo := &pb.SnapshotWithType{
		SbType:   sbType,
		Snapshot: snapshot,
	}
	if p.isJSON {
		m := jsonpb.Marshaler{Indent: " "}
		result, err := m.MarshalToString(mo)
		if err != nil {
			log.Errorf("failed to convert object to json: %s", err.Error())
		}
		return []byte(result)
	}
	result, err := mo.Marshal()
	if err != nil {
		log.Errorf("failed to marshal object: %s", err.Error())
	}
	return result
}

func (p *pbc) Ext() string {
	if p.isJSON {
		return ".pb.json"
	}
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
