package pbjson

import (
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("json-converter")

func NewConverter() converter.Converter {
	return &pbj{}
}

type pbj struct{}

func (p *pbj) Convert(st *state.State, sbType model.SmartBlockType, filename string) []byte {
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
	m := jsonpb.Marshaler{Indent: " "}
	result, err := m.MarshalToString(mo)
	if err != nil {
		log.Errorf("failed to convert object to json: %s", err)
	}
	return []byte(result)
}

func (p *pbj) Ext(model.ObjectTypeLayout) string {
	return ".pb.json"
}

func (p *pbj) FileHashes() []string {
	return nil
}

func (p *pbj) ImageHashes() []string {
	return nil
}
