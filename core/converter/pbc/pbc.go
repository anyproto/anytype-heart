package pbc

import (
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("pb-converter")

func NewConverter(s state.Doc, isJSON bool) converter.Converter {
	return &pbc{
		s:      s,
		isJSON: isJSON,
	}
}

type pbc struct {
	s        state.Doc
	isJSON   bool
	fileKeys *files.FileKeys
}

func (p *pbc) Convert(sb smartblock.SmartBlock) []byte {
	st := p.s.NewState()
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:        st.BlocksToSave(),
			Details:       st.CombinedDetails(),
			ObjectTypes:   domain.MarshalTypeKeys(st.ObjectTypeKeys()),
			Collections:   st.Store(),
			RelationLinks: st.PickRelationLinks(),
			Key:           p.s.UniqueKeyInternal(),
		},
	}
	var sbType model.SmartBlockType
	if sb != nil {
		for _, fk := range sb.GetAndUnsetFileKeys() {
			snapshot.FileKeys = append(snapshot.FileKeys, &pb.ChangeFileKeys{Hash: fk.Hash, Keys: fk.Keys})
		}
		if sb.Type() == coresb.SmartBlockTypeFile && p.fileKeys != nil {
			snapshot.FileKeys = append(snapshot.FileKeys, &pb.ChangeFileKeys{Hash: p.fileKeys.Hash, Keys: p.fileKeys.Keys})
		}
		sbType = sb.Type().ToProto()
	}

	mo := &pb.SnapshotWithType{
		SbType:   sbType,
		Snapshot: snapshot,
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

func (p *pbc) SetKnownDocs(map[string]*types.Struct) converter.Converter {
	return p
}

func (p *pbc) FileHashes() []string {
	return nil
}

func (p *pbc) SetFileKeys(fileKeys *files.FileKeys) {
	p.fileKeys = fileKeys
}

func (p *pbc) ImageHashes() []string {
	return nil
}
