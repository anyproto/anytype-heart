package pbc

import (
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("pb-converter")

func NewConverter(s state.Doc, isJSON bool, dependentDetails []database.Record) converter.Converter {
	return &pbc{
		s:                s,
		isJSON:           isJSON,
		dependentDetails: dependentDetails,
	}
}

type pbc struct {
	s                state.Doc
	isJSON           bool
	dependentDetails []database.Record
}

func (p *pbc) Convert(sbType model.SmartBlockType) []byte {
	st := p.s.NewState()
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.BlocksToSave(),
			Details:       st.CombinedDetails().ToProto(),
			ObjectTypes:   domain.MarshalTypeKeys(st.ObjectTypeKeys()),
			Collections:   st.Store(),
			RelationLinks: st.PickRelationLinks(),
			Key:           p.s.UniqueKeyInternal(),
			FileInfo:      st.GetFileInfo().ToModel(),
		},
	}
	dependentDetails := make([]*pb.DependantDetail, 0, len(p.dependentDetails))
	for _, detail := range p.dependentDetails {
		dependentDetails = append(dependentDetails, &pb.DependantDetail{
			Id:      detail.Details.GetString(bundle.RelationKeyId),
			Details: detail.Details.ToProto(),
		})
	}
	mo := &pb.SnapshotWithType{
		SbType:   sbType,
		Snapshot: snapshot,
	}
	if len(dependentDetails) > 0 {
		mo.DependantDetails = dependentDetails
	}
	if p.isJSON {
		m := jsonpb.Marshaler{Indent: " "}
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

func (p *pbc) Ext() string {
	if p.isJSON {
		return ".pb.json"
	}
	return ".pb"
}

func (p *pbc) SetKnownDocs(map[string]*domain.Details) converter.Converter {
	return p
}

func (p *pbc) FileHashes() []string {
	return nil
}

func (p *pbc) ImageHashes() []string {
	return nil
}
