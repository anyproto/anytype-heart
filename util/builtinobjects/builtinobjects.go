package builtinobjects

import (
	"archive/zip"
	"context"
	_ "embed"
	"encoding/json"
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

	"github.com/anyproto/anytype-heart/core/block"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/gallery"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/uri"
)

const (
	CName            = "builtinobjects"
	injectionTimeout = 30 * time.Second

	contentLengthHeader        = "Content-Length"
	archiveDownloadingPercents = 30
	archiveCopyingPercents     = 10

	indexName = "app-index.json"
)

//go:embed data/migration_dashboard.zip
var migrationDashboardZip []byte

var (
	log = logging.Logger("anytype-mw-builtinobjects")

	ucCodeToTitle = map[pb.RpcObjectImportUseCaseRequestUseCase]string{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED:       "get_started",
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: "personal_projects",
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    "knowledge_base",
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       "notes_diary",
		pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: "strategic_writing",
		pb.RpcObjectImportUseCaseRequest_EMPTY:             "empty",
	}
)

type BuiltinObjects interface {
	app.Component

	CreateObjectsForUseCase(ctx context.Context, spaceID, cachePath string, req pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
	CreateObjectsForExperience(ctx context.Context, spaceID, url, title, cachePath string, newSpace bool) (err error)
	InjectMigrationDashboard(spaceID string) error
}

type builtinObjects struct {
	blockService   *block.Service
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
	b.blockService = a.MustComponent(block.CName).(*block.Service)
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
	ctx context.Context,
	spaceID, cachePath string,
	useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	if useCase == pb.RpcObjectImportUseCaseRequest_NONE {
		return pb.RpcObjectImportUseCaseResponseError_NULL, nil
	}

	start := time.Now()

	title, found := ucCodeToTitle[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import builtinObjects: invalid Use Case value: %v", useCase)
	}

	if err = b.CreateObjectsForExperience(ctx, spaceID, "", title, cachePath, true); err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR,
			fmt.Errorf("failed to import builtinObjects for Use Case %s: %w",
				pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}

	spent := time.Now().Sub(start)
	if spent > injectionTimeout {
		log.Debugf("built-in objects injection time exceeded timeout of %s and is %s", injectionTimeout.String(), spent.String())
	}

	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (b *builtinObjects) CreateObjectsForExperience(ctx context.Context, spaceID, url, title, cachePath string, isNewSpace bool) (err error) {
	progress, err := b.setupProgress()
	if err != nil {
		return err
	}

	var (
		path       string
		removeFunc = func() {}
	)

	content, cachedIndex, cacheErr := readCache(cachePath, title)

	// TODO: need to check usecase relevance in cache by hash check in indexes from cache and from remote
	log.Info(index)

	// TODO: if usecase is not found in cache and no url is specified, we need to retrieve URL from cached index

	if content != nil && cacheErr == nil {
		path = filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
		if err = os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to save use case archive to temporary file: %w", err)
		}

		removeFunc = func() {
			if rmErr := os.Remove(path); rmErr != nil {
				log.Errorf("failed to remove temporary file: %v", anyerror.CleanupError(rmErr))
			}
		}
	} else if _, err = os.Stat(url); err == nil {
		path = url
	} else {
		if path, err = b.downloadZipToFile(url, progress); err != nil {
			if pErr := progress.Cancel(); pErr != nil {
				log.Errorf("failed to cancel progress %s: %v", progress.Id(), pErr)
			}
			progress.FinishWithNotification(b.provideNotification(spaceID, progress, err, title), err)
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
	progress.FinishWithNotification(b.provideNotification(spaceID, progress, err, title), err)

	if err != nil {
		log.Errorf("failed to send notification: %v", err)
	}

	removeFunc()
	return importErr
}

func readData(f *zip.File) ([]byte, error) {
	rd, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot open pb file %s: %w", f.Name, err)
	}
	defer rd.Close()
	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("cannot read data from file %s: %w", f.Name, err)
	}
	return data, nil
}

func readCache(cachePath, title string) (data []byte, index *pb.RpcGalleryDownloadIndexResponse, err error) {
	if cachePath == "" {
		return nil, nil, fmt.Errorf("no cache path specified")
	}
	r, err := zip.OpenReader(cachePath)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	zipName := title + ".zip"
	for _, f := range r.File {
		if f.Name == indexName {
			indexData, err := readData(f)
			if err != nil {
				return nil, nil, err
			}
			err = json.Unmarshal(indexData, &index)
			if err != nil {
				return nil, nil, err
			}
			continue
		}

		if f.Name == zipName {
			data, err = readData(f)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	return
}

func (b *builtinObjects) provideNotification(spaceID string, progress process.Progress, err error, title string) *model.Notification {
	spaceName := b.store.GetSpaceName(spaceID)
	return &model.Notification{
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   spaceID,
		Payload: &model.NotificationPayloadOfGalleryImport{GalleryImport: &model.NotificationGalleryImport{
			ProcessId: progress.Id(),
			ErrorCode: common.GetImportErrorCode(err),
			SpaceId:   spaceID,
			Name:      title,
			SpaceName: spaceName,
		}},
	}
}

func (b *builtinObjects) InjectMigrationDashboard(spaceID string) (err error) {
	path := filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, migrationDashboardZip, 0644); err != nil {
		return fmt.Errorf("failed to save use case archive to temporary file: %w", err)
	}

	if err = b.importArchive(context.Background(), spaceID, path, "", pb.RpcObjectImportRequestPbParams_EXPERIENCE, nil, true); err != nil {
		return err
	}

	if rmErr := os.Remove(path); rmErr != nil {
		log.Errorf("failed to remove temporary file: %v", anyerror.CleanupError(rmErr))
	}
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
			UpdateExistingObjects: false,
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

func (b *builtinObjects) setupProgress() (process.Notificationable, error) {
	progress := process.NewNotificationProcess(pb.ModelProcess_Import, b.notifications)
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
