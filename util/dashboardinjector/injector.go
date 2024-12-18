package dashboardinjector

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anyproto/any-sync/app"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

const CName = "migration-dashboard-injector"

//go:embed data/migration_dashboard.zip
var migrationDashboardZip []byte

var log = logging.Logger(CName)

type DashboardInjector interface {
	app.Component
	InjectMigrationDashboard(spaceID string) error
}

type dashboardInjector struct {
	importer       importer.Importer
	tempDirService core.TempDirProvider
}

func New() DashboardInjector {
	return &dashboardInjector{}
}

func (b *dashboardInjector) Init(a *app.App) (err error) {
	b.importer = a.MustComponent(importer.CName).(importer.Importer)
	b.tempDirService = app.MustComponent[core.TempDirProvider](a)
	return
}

func (b *dashboardInjector) Name() (name string) {
	return CName
}

func (b *dashboardInjector) InjectMigrationDashboard(spaceID string) error {
	path := filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err := os.WriteFile(path, migrationDashboardZip, 0600); err != nil {
		return fmt.Errorf("failed to save use case archive to temporary file: %w", err)
	}

	defer func() {
		if rmErr := os.Remove(path); rmErr != nil {
			log.Errorf("failed to remove temporary file: %v", anyerror.CleanupError(rmErr))
		}
	}()

	importRequest := &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			SpaceId:               spaceID,
			UpdateExistingObjects: false,
			Type:                  model.Import_Pb,
			Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
			NoProgress:            true,
			IsMigration:           false,
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{
				PbParams: &pb.RpcObjectImportRequestPbParams{
					Path:         []string{path},
					NoCollection: true,
					ImportType:   pb.RpcObjectImportRequestPbParams_EXPERIENCE,
				}},
			IsNewSpace: true,
		},
		Origin: objectorigin.Usecase(),
		IsSync: true,
	}
	res := b.importer.Import(context.Background(), importRequest)
	return res.Err
}
