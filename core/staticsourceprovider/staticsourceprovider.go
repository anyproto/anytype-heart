package staticsourceprovider

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/common/types"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type StaticSource struct {
	Id       string
	SbType   smartblock.SmartBlockType
	Snapshot *pb.ChangeSnapshot
}

type StaticSourceProvider interface {
	app.Component
	GetStaticSource(ctx context.Context, req *pb.RpcObjectImportRequest) ([]source.StaticSourceParams, error)
}

type Importer interface {
	ImportSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest) ([]*types.Snapshot, error)
}

type SnapshotsProvider struct {
	snapshotImporter Importer
}

func NewSnapshotsProvider() *SnapshotsProvider {
	return &SnapshotsProvider{}
}

func (s *SnapshotsProvider) Init(a *app.App) (err error) {
	s.snapshotImporter = app.MustComponent[Importer](a)
	return nil
}

func (s *SnapshotsProvider) Name() (name string) {
	return "staticSourceProvider"
}

func (s *SnapshotsProvider) GetStaticSource(ctx context.Context, req *pb.RpcObjectImportRequest) ([]source.StaticSourceParams, error) {
	var sp []source.StaticSourceParams
	snapshots, cErr := s.snapshotImporter.ImportSnapshots(ctx, req)
	if cErr != nil {
		return nil, cErr
	}
	for _, snapshot := range snapshots {
		sp = append(sp, source.StaticSourceParams{
			Id: domain.FullID{
				ObjectID: snapshot.Id,
				SpaceID:  req.SpaceId,
			},
			SbType: snapshot.SbType,
			State:  state.NewDocFromSnapshot(snapshot.Id, snapshot.Snapshot).(*state.State),
		})
	}
	return sp, nil
}
