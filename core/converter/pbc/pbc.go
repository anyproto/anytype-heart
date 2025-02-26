package pbc

import (
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("pb-converter")

func NewConverter(isJSON bool) converter.Converter {
	return &pbc{
		isJSON: isJSON,
	}
}

type pbc struct {
	isJSON bool
}

func (p *pbc) Convert(st *state.State, sbType model.SmartBlockType, filename string) []byte {
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.BlocksToSave(),
			Details:       st.CombinedDetails().ToProto(),
			ObjectTypes:   domain.MarshalTypeKeys(st.ObjectTypeKeys()),
			Collections:   st.Store(),
			RelationLinks: st.PickRelationLinks(),
			Key:           st.UniqueKeyInternal(),
			FileInfo:      st.GetFileInfo().ToModel(),
		},
	}
	mo := &pb.SnapshotWithType{
		SbType:   sbType,
		Snapshot: snapshot,
	}
	if p.isJSON {
		m := jsonpb.Marshaler{Indent: " ", EmitDefaults: true}
		result, err := m.MarshalToString(mo)
		if err != nil {
			log.Errorf("failed to convert object to json: %s", err)
		}
		return []byte(result)
	}
	result, err := mo.Marshal()
	if err != nil {
		log.Errorf("failed to marshal object: %s", err)
	}
	return result
}

func (p *pbc) Ext(model.ObjectTypeLayout) string {
	if p.isJSON {
		return ".pb.json"
	}
	return ".pb"
}

func (p *pbc) SetKnownDocs(map[string]*domain.Details) {}

func (p *pbc) FileHashes() []string {
	return nil
}

func (p *pbc) ImageHashes() []string {
	return nil
}
