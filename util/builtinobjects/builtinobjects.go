package builtinobjects

import (
	"archive/zip"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/miolini/datacounter"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/gallery"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/uri"
)

const (
	CName            = "builtinobjects"
	injectionTimeout = 30 * time.Second

	migrationUseCase       = -1
	migrationDashboardName = "bafyreiha2hjbrzmwo7rpiiechv45vv37d6g5aezyr5wihj3agwawu6zi3u"

	contentLengthHeader        = "Content-Length"
	archiveDownloadingPercents = 30
	archiveCopyingPercents     = 10
)

type widgetParameters struct {
	layout            model.BlockContentWidgetLayout
	objectID, viewID  string
	isObjectIDChanged bool
}

//go:embed data/start_guide.zip
var startGuideZip []byte

//go:embed data/get_started.zip
var getStartedZip []byte

//go:embed data/migration_dashboard.zip
var migrationDashboardZip []byte

//go:embed data/empty.zip
var emptyZip []byte

var (
	log = logging.Logger("anytype-mw-builtinobjects")

	archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED: getStartedZip,
		pb.RpcObjectImportUseCaseRequest_GUIDE_ONLY:  startGuideZip,
		pb.RpcObjectImportUseCaseRequest_EMPTY:       emptyZip,
	}
)

type BuiltinObjects interface {
	app.Component

	CreateObjectsForUseCase(ctx session.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (dashboardId string, code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
	CreateObjectsForExperience(ctx context.Context, spaceID, url, title string, newSpace bool) (err error)
	InjectMigrationDashboard(spaceID string) error
}

type builtinObjects struct {
	objectGetter   cache.ObjectGetter
	detailsService detailservice.Service
	importer       importer.Importer
	store          objectstore.ObjectStore
	tempDirService core.TempDirProvider
	spaceService   space.Service
	progress       process.Service
	notifications  notifications.Notifications
}

func New() BuiltinObjects {
	return &builtinObjects{}
}

func (b *builtinObjects) Init(a *app.App) (err error) {
	b.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	b.detailsService = app.MustComponent[detailservice.Service](a)
	b.importer = a.MustComponent(importer.CName).(importer.Importer)
	b.store = app.MustComponent[objectstore.ObjectStore](a)
	b.tempDirService = app.MustComponent[core.TempDirProvider](a)
	b.spaceService = app.MustComponent[space.Service](a)
	b.progress = a.MustComponent(process.CName).(process.Service)
	b.notifications = app.MustComponent[notifications.Notifications](a)
	return
}

func (b *builtinObjects) Name() (name string) {
	return CName
}

func (b *builtinObjects) CreateObjectsForUseCase(
	ctx session.Context,
	spaceID string,
	useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (dashboardId string, code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	if useCase == pb.RpcObjectImportUseCaseRequest_NONE {
		return "", pb.RpcObjectImportUseCaseResponseError_NULL, nil
	}

	start := time.Now()

	archive, found := archives[useCase]
	if !found {
		return "", pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import builtinObjects: invalid Use Case value: %v", useCase)
	}

	if dashboardId, err = b.inject(ctx, spaceID, useCase, archive); err != nil {
		return "", pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR,
			fmt.Errorf("failed to import builtinObjects for Use Case %s: %w",
				pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}

	spent := time.Now().Sub(start)
	if spent > injectionTimeout {
		log.Debugf("built-in objects injection time exceeded timeout of %s and is %s", injectionTimeout.String(), spent.String())
	}

	return dashboardId, pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (b *builtinObjects) CreateObjectsForExperience(ctx context.Context, spaceID, url, title string, isNewSpace bool) (err error) {
	progress, err := b.setupProgress()
	if err != nil {
		return err
	}

	var (
		path       string
		removeFunc = func() {}
	)

	if _, err = os.Stat(url); err == nil {
		path = url
	} else {
		if path, err = b.downloadZipToFile(url, progress); err != nil {
			if pErr := progress.Cancel(); pErr != nil {
				log.Errorf("failed to cancel progress %s: %v", progress.Id(), pErr)
			}
			if notificationProgress, ok := progress.(process.Notificationable); ok {
				notificationProgress.FinishWithNotification(b.provideNotification(spaceID, progress, err, title), err)
			}
			if errors.Is(err, uri.ErrFilepathNotSupported) {
				return fmt.Errorf("invalid path to file: '%s'", url)
			}
			return err
		}
		removeFunc = func() {
			if rmErr := os.Remove(path); rmErr != nil {
				log.Errorf("failed to remove temporary file: %v", anyerror.CleanupError(rmErr))
			}
		}
	}

	importErr := b.importArchive(ctx, spaceID, path, title, pb.RpcObjectImportRequestPbParams_EXPERIENCE, progress, isNewSpace)
	if notificationProgress, ok := progress.(process.Notificationable); ok {
		notificationProgress.FinishWithNotification(b.provideNotification(spaceID, progress, importErr, title), importErr)
	}

	if importErr != nil {
		log.Errorf("failed to send notification: %v", importErr)
	}

	if isNewSpace {
		// TODO: GO-2627 Home page handling should be moved to importer
		b.handleHomePage(path, spaceID, removeFunc, false)
	} else {
		removeFunc()
	}

	return importErr
}

func (b *builtinObjects) provideNotification(spaceID string, progress process.Progress, err error, title string) *model.Notification {
	spaceName := b.store.GetSpaceName(spaceID)
	return &model.Notification{
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   spaceID,
		Payload: &model.NotificationPayloadOfGalleryImport{GalleryImport: &model.NotificationGalleryImport{
			ProcessId: progress.Id(),
			ErrorCode: common.GetImportNotificationErrorCode(err),
			SpaceId:   spaceID,
			Name:      title,
			SpaceName: spaceName,
		}},
	}
}

func (b *builtinObjects) InjectMigrationDashboard(spaceID string) error {
	_, err := b.inject(nil, spaceID, migrationUseCase, migrationDashboardZip)
	return err
}

func (b *builtinObjects) inject(ctx session.Context, spaceID string, useCase pb.RpcObjectImportUseCaseRequestUseCase, archive []byte) (dashboardId string, err error) {
	path := filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0644); err != nil {
		return "", fmt.Errorf("failed to save use case archive to temporary file: %w", err)
	}

	if err = b.importArchive(context.Background(), spaceID, path, "", pb.RpcObjectImportRequestPbParams_SPACE, nil, false); err != nil {
		return "", err
	}

	// TODO: GO-2627 Home page handling should be moved to importer
	dashboardId = b.handleHomePage(path, spaceID, func() {
		if rmErr := os.Remove(path); rmErr != nil {
			log.Errorf("failed to remove temporary file: %v", anyerror.CleanupError(rmErr))
		}
	}, useCase == migrationUseCase)

	// TODO: GO-2627 Widgets creation should be moved to importer
	b.createWidgets(ctx, spaceID, useCase)
	return
}

func (b *builtinObjects) importArchive(
	ctx context.Context,
	spaceID, path, title string,
	importType pb.RpcObjectImportRequestPbParamsType,
	progress process.Progress,
	isNewSpace bool,
) (err error) {
	origin := objectorigin.Usecase()
	importRequest := &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			SpaceId:               spaceID,
			UpdateExistingObjects: true,
			Type:                  model.Import_Pb,
			Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
			NoProgress:            progress == nil,
			IsMigration:           false,
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{
				PbParams: &pb.RpcObjectImportRequestPbParams{
					Path:            []string{path},
					NoCollection:    true,
					CollectionTitle: title,
					ImportType:      importType,
				}},
			IsNewSpace: isNewSpace,
		},
		Origin:   origin,
		Progress: progress,
		IsSync:   true,
	}
	res := b.importer.Import(ctx, importRequest)

	return res.Err
}

func (b *builtinObjects) handleHomePage(path, spaceId string, removeFunc func(), isMigration bool) (dashboardId string) {
	defer removeFunc()
	oldID := migrationDashboardName
	if !isMigration {
		r, err := zip.OpenReader(path)
		if err != nil {
			log.Errorf("cannot open zip file %s: %w", path, err)
			return
		}
		defer r.Close()

		oldID, err = b.getOldHomePageId(&r.Reader)
		if err != nil {
			log.Errorf("failed to get old id of home page object: %s", err)
			return
		}
	}

	newID, err := b.getNewObjectID(spaceId, oldID)
	if err != nil {
		log.Errorf("failed to get new id of home page object: %s", err)
		return
	}

	spc, err := b.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		log.Errorf("failed to get space: %w", err)
		return
	}
	dashboardId = newID
	b.setHomePageIdToWorkspace(spc, newID)
	return
}

func (b *builtinObjects) getOldHomePageId(zipReader *zip.Reader) (id string, err error) {
	var (
		rd           io.ReadCloser
		profileFound bool
	)
	for _, zf := range zipReader.File {
		if zf.Name == constant.ProfileFile {
			profileFound = true
			rd, err = zf.Open()
			if err != nil {
				return "", err
			}
			break
		}
	}

	if !profileFound {
		return "", fmt.Errorf("no profile file included in archive")
	}

	defer rd.Close()
	data, err := io.ReadAll(rd)

	profile := &pb.Profile{}
	if err = profile.Unmarshal(data); err != nil {
		return "", err
	}
	return profile.SpaceDashboardId, nil
}

func (b *builtinObjects) setHomePageIdToWorkspace(spc clientspace.Space, id string) {
	if err := b.detailsService.SetDetails(nil,
		spc.DerivedIDs().Workspace,
		[]domain.Detail{
			{
				Key:   bundle.RelationKeySpaceDashboardId,
				Value: domain.StringList([]string{id}),
			},
		},
	); err != nil {
		log.Errorf("Failed to set SpaceDashboardId relation to Account object: %s", err)
	}
}

func (b *builtinObjects) typeHasObjects(spaceId, typeId string) (bool, error) {
	records, err := b.store.SpaceIndex(spaceId).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyType,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.String(typeId),
		},
	}}, 1, 0)
	if err != nil {
		return false, err
	}

	return len(records) > 0, nil
}

func (b *builtinObjects) createWidgets(ctx session.Context, spaceId string, useCase pb.RpcObjectImportUseCaseRequestUseCase) {
	spc, err := b.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		log.Errorf("failed to get space: %w", err)
		return
	}

	widgetObjectID := spc.DerivedIDs().Widgets
	var widgetTargetsToCreate []string
	pageTypeId, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyPage)
	if err != nil {
		log.Errorf("failed to get type id: %w", err)
		return
	}
	taskTypeId, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyTask)
	if err != nil {
		log.Errorf("failed to get type id: %w", err)
		return
	}
	for _, typeId := range []string{pageTypeId, taskTypeId} {
		if has, err := b.typeHasObjects(spaceId, typeId); err != nil {
			log.Warnf("failed to check if type '%s' has objects: %v", pageTypeId, err)
		} else if has {
			widgetTargetsToCreate = append(widgetTargetsToCreate, typeId)
		}
	}

	if len(widgetTargetsToCreate) == 0 {
		return
	}
	if err = cache.DoStateCtx(b.objectGetter, ctx, widgetObjectID, func(s *state.State, w widget.Widget) error {
		for _, targetId := range widgetTargetsToCreate {
			if err := w.AddAutoWidget(s, targetId, "", addr.ObjectTypeAllViewId, model.BlockContentWidget_View, ""); err != nil {
				log.Errorf("failed to create widget block for type '%s': %v", targetId, err)
			}
		}
		return nil
	}); err != nil {
		log.Errorf("failed to create widget blocks for useCase '%s': %v",
			pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}
}

func (b *builtinObjects) getNewObjectID(spaceID string, oldID string) (id string, err error) {
	var ids []string
	if ids, _, err = b.store.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID,
				Value:       domain.String(oldID),
			},
		},
	}); err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no object with oldAnytypeId = '%s' in space '%s' found", oldID, spaceID)
	}
	return ids[0], nil
}

func (b *builtinObjects) downloadZipToFile(url string, progress process.Progress) (path string, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return "", fmt.Errorf("provided URL is not valid: %w", err)
	}
	if !gallery.IsInWhitelist(url) {
		return "", fmt.Errorf("provided URL is not in whitelist")
	}

	var (
		countReader *datacounter.ReaderCounter
		size        int64
	)

	ctx, cancel := context.WithCancel(context.Background())
	readerMutex := sync.Mutex{}
	defer cancel()
	go func() {
		counter := int64(0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-progress.Canceled():
				cancel()
			case <-time.After(time.Second):
				readerMutex.Lock()
				if countReader != nil && size != 0 {
					progress.SetDone(archiveDownloadingPercents + int64(archiveCopyingPercents*countReader.Count())/size)
				} else if counter < archiveDownloadingPercents {
					counter++
					progress.SetDone(counter)
				}
				readerMutex.Unlock()
			}
		}
	}()

	var reader io.ReadCloser
	reader, size, err = getArchiveReaderAndSize(url)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	readerMutex.Lock()
	countReader = datacounter.NewReaderCounter(reader)
	readerMutex.Unlock()

	path = filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	var out *os.File
	out, err = os.Create(path)
	if err != nil {
		return "", anyerror.CleanupError(err)
	}
	defer out.Close()

	if _, err = io.Copy(out, countReader); err != nil {
		return "", err
	}

	progress.SetDone(archiveDownloadingPercents + archiveCopyingPercents)
	return path, nil
}

func (b *builtinObjects) setupProgress() (process.Progress, error) {
	progress := process.NewNotificationProcess(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}, b.notifications)
	if err := b.progress.Add(progress); err != nil {
		return nil, fmt.Errorf("failed to add progress bar: %w", err)
	}
	progress.SetProgressMessage("downloading archive")
	progress.SetTotal(100)
	return progress, nil
}

func getArchiveReaderAndSize(url string) (reader io.ReadCloser, size int64, err error) {
	client := http.Client{Timeout: 15 * time.Second}
	// nolint: gosec
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("failed to fetch zip file: not OK status code: %s", resp.Status)
	}

	contentLengthStr := resp.Header.Get(contentLengthHeader)
	if size, err = strconv.ParseInt(contentLengthStr, 10, 64); err != nil {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("failed to get zip size from Content-Length: %w", err)
	}

	return resp.Body, size, nil
}
