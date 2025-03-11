package gallery

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/miolini/datacounter"
	"go.uber.org/zap"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/uri"
)

const (
	CName            = "gallery-service"
	injectionTimeout = 30 * time.Second

	contentLengthHeader        = "Content-Length"
	archiveDownloadingPercents = 30
	archiveCopyingPercents     = 10
)

type UseCaseInfo struct {
	Name, Title, DownloadLink string
}

var (
	log = logger.NewNamed(CName)

	ucCodeToInfo = map[pb.RpcObjectImportUseCaseRequestUseCase]UseCaseInfo{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED: {"get_started", "Get Started", "https://storage.gallery.any.coop/get_started/get_started.zip"},
		pb.RpcObjectImportUseCaseRequest_EMPTY:       {"empty", "Empty", ""},
	}
)

type Service interface {
	app.Component

	ImportExperience(ctx context.Context, spaceID string, info UseCaseInfo, newSpace bool) (err error)
	ImportBuiltInUseCase(ctx context.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)

	GetGalleryIndex() (*pb.RpcGalleryDownloadIndexResponse, error)
	GetManifest(url string, checkWhitelist bool) (info *model.ManifestInfo, err error)
}

type service struct {
	importer        importer.Importer
	spaceNameGetter objectstore.SpaceNameGetter
	tempDirService  core.TempDirProvider
	progress        process.Service
	notifications   notifications.Notifications
	indexCache      IndexCache

	withUrlValidation bool
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.importer = a.MustComponent(importer.CName).(importer.Importer)
	s.spaceNameGetter = app.MustComponent[objectstore.SpaceNameGetter](a)
	s.tempDirService = app.MustComponent[core.TempDirProvider](a)
	s.progress = a.MustComponent(process.CName).(process.Service)
	s.notifications = app.MustComponent[notifications.Notifications](a)
	s.indexCache = app.MustComponent[IndexCache](a)

	s.withUrlValidation = true
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) ImportExperience(ctx context.Context, spaceID string, info UseCaseInfo, isNewSpace bool) (err error) {
	var (
		progress      process.Progress
		pathToArchive string
		remove        = func(string) {}
	)

	defer func() {
		if remove != nil && pathToArchive != "" {
			remove(pathToArchive)
		}
		if np, ok := progress.(process.Notificationable); ok {
			np.FinishWithNotification(s.provideNotification(spaceID, progress, err, info.Title), err)
		} else {
			log.Error("progress does not implement Notificationable interface")
		}
	}()

	progress, err = s.setupProgress()
	if err != nil {
		return err
	}

	pathToArchive, remove, err = s.getPathAndRemoveFunc(info, progress)
	if err != nil {
		return err
	}

	err = s.importArchive(ctx, spaceID, pathToArchive, info.Title, progress, isNewSpace)
	return err
}

func (s *service) getPathAndRemoveFunc(info UseCaseInfo, progress process.Progress) (path string, removeFunc func(string), err error) {
	if _, err = os.Stat(info.DownloadLink); err == nil {
		return info.DownloadLink, func(string) {}, nil
	}

	path, err = s.downloadZipToFile(info.DownloadLink, progress)
	if err != nil {
		return "", nil, err
	}
	return path, removeTempFile, nil
}

func removeTempFile(path string) {
	if rmErr := os.Remove(path); rmErr != nil {
		log.Error("failed to remove temporary file: %v", zap.Error(anyerror.CleanupError(rmErr)))
	}
}

func (s *service) provideNotification(spaceID string, progress process.Progress, err error, title string) *model.Notification {
	spaceName := s.spaceNameGetter.GetSpaceName(spaceID)
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

func (s *service) importArchive(
	ctx context.Context,
	spaceID, path, title string,
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
					ImportType:      pb.RpcObjectImportRequestPbParams_EXPERIENCE,
				}},
			IsNewSpace: isNewSpace,
		},
		Origin:   origin,
		Progress: progress,
		IsSync:   true,
	}
	res := s.importer.Import(ctx, importRequest)

	return res.Err
}

func (s *service) downloadZipToFile(url string, progress process.Progress) (path string, err error) {
	if s.withUrlValidation {
		if err = uri.ValidateURI(url); err != nil {
			return "", fmt.Errorf("provided URL is not valid: %w. Or invalid path to file", err)
		}
		if !isInWhitelist(url) {
			return "", fmt.Errorf("provided URL is not in whitelist")
		}
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
					// nolint:gosec
					progress.SetDone(archiveDownloadingPercents + archiveCopyingPercents*int64(countReader.Count())/size)
				} else if counter < archiveDownloadingPercents {
					counter++
					progress.SetDone(counter)
				}
				readerMutex.Unlock()
			}
		}
	}()

	var reader io.ReadCloser
	reader, size, err = getArchiveReaderAndSize(ctx, url)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	readerMutex.Lock()
	countReader = datacounter.NewReaderCounter(reader)
	readerMutex.Unlock()

	path = filepath.Join(s.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
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

func (s *service) setupProgress() (process.Progress, error) {
	progress := process.NewNotificationProcess(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}, s.notifications)
	if err := s.progress.Add(progress); err != nil {
		return nil, fmt.Errorf("failed to add progress bar: %w", err)
	}
	progress.SetProgressMessage("downloading archive")
	progress.SetTotal(100)
	return progress, nil
}

func getArchiveReaderAndSize(ctx context.Context, url string) (reader io.ReadCloser, size int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := http.DefaultClient.Do(req)
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

func getManifestByName(index *pb.RpcGalleryDownloadIndexResponse, name string) (*model.ManifestInfo, error) {
	for _, manifest := range index.Experiences {
		if manifest.Name == name {
			return manifest, nil
		}
	}
	return nil, fmt.Errorf("failed to find manifest with name: %s", name)
}
