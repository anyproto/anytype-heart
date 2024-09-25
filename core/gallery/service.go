package gallery

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
	"path"
	"path/filepath"
	"strconv"
	"strings"
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
	CName                          = "gallery-service"
	injectionTimeout               = 30 * time.Second
	downloadManifestTimeoutSeconds = 1

	contentLengthHeader        = "Content-Length"
	archiveDownloadingPercents = 30
	archiveCopyingPercents     = 10

	indexName = "app-index.json"
)

type builtInUseCaseInfo struct {
	Title, DownloadLink string
}

var (
	log = logger.NewNamed(CName)

	// TODO: GO-4131 Fill in download links when built-in usecases will be downloaded to gallery
	ucCodeToInfo = map[pb.RpcObjectImportUseCaseRequestUseCase]builtInUseCaseInfo{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED:       {"Get Started", "https://storage.gallery.any.coop/get_started/get_started.zip"},
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: {"Personal Projects", ""},
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    {"Knowledge Base", ""},
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       {"Notes and Diary", ""},
		pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: {"Strategic Writing", ""},
		pb.RpcObjectImportUseCaseRequest_EMPTY:             {"Empty", ""},
	}

	errOutdatedArchive = fmt.Errorf("archive is outdated")
)

type Service interface {
	app.Component

	ImportExperience(ctx context.Context, spaceID, url, title, cachePath string, newSpace bool) (err error)
	ImportBuiltInUseCase(
		ctx context.Context, spaceID, cachePath string, req pb.RpcObjectImportUseCaseRequestUseCase,
	) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)

	GetGalleryIndex(clientCachePath string) (*pb.RpcGalleryDownloadIndexResponse, error)
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

func (s *service) ImportBuiltInUseCase(
	ctx context.Context,
	spaceID, cachePath string,
	useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	if useCase == pb.RpcObjectImportUseCaseRequest_NONE {
		return pb.RpcObjectImportUseCaseResponseError_NULL, nil
	}

	start := time.Now()

	info, found := ucCodeToInfo[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import built-in usecase: invalid Use Case value: %v", useCase)
	}

	if cachePath == "" {
		// TODO: GO-4131 Remove this call when clients support cache
		return s.importUseCase(ctx, spaceID, info.Title, useCase)
		// return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
		// 	fmt.Errorf("failed to import built-in usecase: no path to client cache provided")
	}

	if err = s.ImportExperience(ctx, spaceID, info.DownloadLink, info.Title, cachePath, true); err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR,
			fmt.Errorf("failed to import built-in usecase %s: %w",
				pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}

	spent := time.Since(start)
	if spent > injectionTimeout {
		log.Debug("built-in objects injection time exceeded timeout", zap.String("timeout", injectionTimeout.String()), zap.String("spent", spent.String()))
	}

	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (s *service) ImportExperience(ctx context.Context, spaceID, url, title, cachePath string, isNewSpace bool) (err error) {
	var (
		progress      process.Notificationable
		pathToArchive string
		remove        = func(string) {}
	)

	defer func() {
		if remove != nil && pathToArchive != "" {
			remove(pathToArchive)
		}
		progress.FinishWithNotification(s.provideNotification(spaceID, progress, err, title), err)
	}()

	progress, err = s.setupProgress()
	if err != nil {
		return err
	}

	pathToArchive, remove, err = s.getPathAndRemoveFunc(url, cachePath, progress)
	if err != nil {
		return err
	}

	err = s.importArchive(ctx, spaceID, pathToArchive, title, progress, isNewSpace)
	return err
}

func (s *service) getPathAndRemoveFunc(
	url, cachePath string, progress process.Notificationable,
) (path string, removeFunc func(string), err error) {
	if _, err = os.Stat(url); err == nil {
		return url, func(string) {}, nil
	}

	cachedArchive, err := s.getArchiveFromCache(url, cachePath)
	if err == nil {
		return s.saveArchiveToTempFile(cachedArchive)
	}

	if errors.Is(err, errOutdatedArchive) {
		log.Debug("archive in client cache is outdated. Trying to download it from remote")
	}

	path, err = s.downloadZipToFile(url, progress)
	if err == nil {
		return path, removeTempFile, nil
	}

	if cachedArchive != nil {
		log.Warn("failed to download archive from remote. Importing cached archive", zap.Error(err))
		return s.saveArchiveToTempFile(cachedArchive)
	}
	return "", nil, err
}

func (s *service) getArchiveFromCache(downloadLink, cachePath string) (archive []byte, err error) {
	archives, index, err := readClientCache(cachePath, false)
	if err != nil {
		return nil, err
	}

	archive = archives[downloadLink]
	if archive == nil {
		return nil, fmt.Errorf("archive is not cached")
	}

	cachedManifest, err := getManifestByDownloadLink(index, downloadLink)
	if err != nil {
		return nil, err
	}

	downloadLink = cachedManifest.DownloadLink

	latestManifest, err := s.indexCache.GetManifest(downloadLink, downloadManifestTimeoutSeconds)
	if err == nil && latestManifest.Hash != cachedManifest.Hash {
		// if hashes are different, then we can get fresher version of archive from remote
		return archive, errOutdatedArchive
	}
	if err != nil {
		log.Error("failed to get latest manifest", zap.String("downloadLink", downloadLink), zap.Error(err))
	}
	return archive, nil
}

func (s *service) saveArchiveToTempFile(archive []byte) (path string, removeFunc func(string), err error) {
	path = filepath.Join(s.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0600); err != nil {
		return "", nil, fmt.Errorf("failed to save archive to temporary file: %w", err)
	}
	return path, removeTempFile, nil
}

func removeTempFile(path string) {
	if rmErr := os.Remove(path); rmErr != nil {
		log.Error("failed to remove temporary file: %v", zap.Error(anyerror.CleanupError(rmErr)))
	}
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

// readClientCache returns Gallery Index and map of archives aggregated by DownloadLink
func readClientCache(cachePath string, indexOnly bool) (archives map[string][]byte, index *pb.RpcGalleryDownloadIndexResponse, err error) {
	if cachePath == "" {
		return nil, nil, fmt.Errorf("no cache path specified")
	}
	r, err := zip.OpenReader(cachePath)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != indexName {
			continue
		}
		indexData, err := readData(f)
		if err != nil {
			return nil, nil, err
		}
		err = json.Unmarshal(indexData, &index)
		if err != nil {
			return nil, nil, err
		}
		if indexOnly {
			return nil, index, nil
		}
		break
	}

	if index == nil {
		return nil, nil, fmt.Errorf("no index file found")
	}
	downloadLinks := generateMapOfDownloadLinksByNames(index)

	archives = make(map[string][]byte, len(index.Experiences))
	for _, f := range r.File {
		if f.Name == indexName {
			continue
		}

		ext := path.Ext(f.Name)
		if ext != ".zip" {
			return nil, nil, fmt.Errorf("zip archive is expected, got: '%s'", f.Name)
		}

		var data []byte
		data, err = readData(f)
		if err != nil {
			return nil, nil, err
		}

		name := strings.TrimSuffix(f.Name, ext)
		link, found := downloadLinks[name]
		if !found {
			return nil, nil, fmt.Errorf("archive '%s' is presented in cache, but not in the index", name)
		}

		archives[link] = data
	}
	return
}

func (s *service) provideNotification(spaceID string, progress process.Progress, err error, title string) *model.Notification {
	spaceName := s.spaceNameGetter.GetSpaceName(spaceID)
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
					count := countReader.Count()
					if count > uint64(^int64(0)+1) {
						count = uint64(^int64(0) + 1)
					}
					// nolint:gosec
					progress.SetDone(archiveDownloadingPercents + archiveCopyingPercents*int64(count)/size)
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

func (s *service) setupProgress() (process.Notificationable, error) {
	progress := process.NewNotificationProcess(pb.ModelProcess_Import, s.notifications)
	if err := s.progress.Add(progress); err != nil {
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

func getManifestByDownloadLink(index *pb.RpcGalleryDownloadIndexResponse, link string) (*model.ManifestInfo, error) {
	for _, manifest := range index.Experiences {
		if manifest.DownloadLink == link {
			return manifest, nil
		}
	}
	return nil, fmt.Errorf("failed to find manifest for url: %s", link)
}

func generateMapOfDownloadLinksByNames(index *pb.RpcGalleryDownloadIndexResponse) map[string]string {
	m := make(map[string]string, len(index.Experiences))
	for _, manifest := range index.Experiences {
		m[manifest.Name] = manifest.DownloadLink
	}
	return m
}
