package source

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	pb2 "github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type snapshot struct {
	zipPath string
	id      string
	spaceId string

	provider staticSourceProvider
	source   *service
}

func NewSnapshotSource(
	ctx context.Context,
	id, spaceId string,
	provider staticSourceProvider,
	source *service,
) (s Source) {
	zipPath := ctx.Value("zipPath").(string)
	return &snapshot{
		id:       id,
		spaceId:  spaceId,
		zipPath:  zipPath,
		provider: provider,
		source:   source,
	}
}

func (s *snapshot) ListIds() ([]string, error) {
	return []string{s.id}, nil
}

func (s *snapshot) ReadOnly() bool {
	return true
}

func (s *snapshot) Id() string {
	return s.id
}

func (s *snapshot) SpaceID() string {
	return s.spaceId
}

func (s *snapshot) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeSnapshot
}

func (s *snapshot) getDetails() (p *types.Struct) {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeySourceFilePath.String(): pbtypes.String(s.zipPath),
		bundle.RelationKeyId.String():             pbtypes.String(s.id),
		bundle.RelationKeyIsReadonly.String():     pbtypes.Bool(true),
	}}
}

func (s *snapshot) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	virtualSpaceId := addr.VirtualSpace + s.id
	staticSources, cErr := s.provider.GetStaticSource(ctx, &pb2.RpcObjectImportRequest{
		SpaceId: virtualSpaceId,
		Params: &pb2.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb2.RpcObjectImportRequestPbParams{
				Path:         []string{s.zipPath},
				NoCollection: true,
			},
		},
		Type:       model.Import_Pb,
		NoProgress: true,
	})
	if cErr != nil {
		return nil, cErr
	}
	for _, sn := range staticSources {
		err := s.source.RegisterStaticSource(s.source.NewStaticSource(sn))
		if err != nil {
			return nil, err
		}
	}
	st := state.NewDoc(addr.Snapshot, nil).(*state.State)
	st.SetDetails(s.getDetails())
	return st, nil
}

func (s *snapshot) Close() (err error) {
	return
}

func (s *snapshot) Heads() []string {
	return []string{"todo"}
}

func (s *snapshot) GetFileKeysSnapshot() []*pb2.ChangeFileKeys {
	return nil
}

func (s *snapshot) PushChange(params PushChangeParams) (id string, err error) {
	return
}

func (s *snapshot) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
