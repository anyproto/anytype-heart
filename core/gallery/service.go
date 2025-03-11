package gallery

import (
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

	artifactGalleryDir = "gallery"
	indexName          = "app-index.json"
)

type UseCaseInfo struct {
	Name, Title, DownloadLink string
}

var (
	log = logger.NewNamed(CName)

	// TODO: GO-4131 Fill in download links when built-in usecases will be downloaded to gallery
	ucCodeToInfo = map[pb.RpcObjectImportUseCaseRequestUseCase]UseCaseInfo{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED: {"get_started", "Get Started", "https://storage.gallery.any.coop/get_started/get_started.zip"},
		pb.RpcObjectImportUseCaseRequest_EMPTY:       {"empty", "Empty", ""},
	}

	errOutdatedArchive = fmt.Errorf("archive is outdated")
)

type Service interface {
	app.Component

	ImportExperience(ctx context.Context, spaceID, artifactPath string, info UseCaseInfo, newSpace bool) (err error)
	ImportBuiltInUseCase(
		ctx context.Context, spaceID, artifactPath string, req pb.RpcObjectImportUseCaseRequestUseCase,
	) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)

	GetGalleryIndex(artifactPath string) (*pb.RpcGalleryDownloadIndexResponse, error)
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
	spaceID, artifactPath string,
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

	if artifactPath == "" {
		// TODO: GO-4131 Remove this call when clients support cache
		return s.importUseCase(ctx, spaceID, info.Title, useCase)
		// return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
		// 	fmt.Errorf("failed to import built-in usecase: no path to client cache provided")
	}

	if err = s.ImportExperience(ctx, spaceID, artifactPath, info, true); err != nil {
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

func (s *service) ImportExperience(ctx context.Context, spaceID, artifactPath string, info UseCaseInfo, isNewSpace bool) (err error) {
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

	pathToArchive, remove, err = s.getPathAndRemoveFunc(info, artifactPath, progress)
	if err != nil {
		return err
	}

	err = s.importArchive(ctx, spaceID, pathToArchive, info.Title, progress, isNewSpace)
	return err
}

func (s *service) getPathAndRemoveFunc(
	info UseCaseInfo, artifactPath string, progress process.Progress,
) (path string, removeFunc func(string), err error) {
	if _, err = os.Stat(info.DownloadLink); err == nil {
		return info.DownloadLink, func(string) {}, nil
	}

	pathToArchiveInArtifact, err := s.getPathFromArtifact(info.Name, artifactPath)
	if err == nil {
		return pathToArchiveInArtifact, func(string) {}, nil
	}

	if errors.Is(err, errOutdatedArchive) {
		log.Debug("archive in the artifact is outdated. Trying to download it from remote")
	}

	path, err = s.downloadZipToFile(info.DownloadLink, progress)
	if err == nil {
		return path, removeTempFile, nil
	}

	if pathToArchiveInArtifact != "" {
		log.Warn("failed to download archive from remote. Importing archive from artifact", zap.Error(err))
		return pathToArchiveInArtifact, func(string) {}, nil
	}
	return "", nil, err
}

func (s *service) getPathFromArtifact(name, artifactPath string) (archivePath string, err error) {
	archivesPaths, index, err := readArtifact(artifactPath, false)
	if err != nil {
		return "", err
	}

	archivePath = archivesPaths[name]
	if archivePath == "" {
		return "", fmt.Errorf("artifact does not contain archive '%s'", name)
	}

	manifestFromArtifact, err := getManifestByName(index, name)
	if err != nil {
		return "", err
	}

	latestManifest, err := s.indexCache.GetManifest(name, downloadManifestTimeoutSeconds)
	if err == nil && latestManifest.Hash != manifestFromArtifact.Hash {
		// if hashes are different, then we can get fresher version of archive from remote
		return archivePath, errOutdatedArchive
	}
	if err != nil {
		log.Error("failed to get latest manifest", zap.String("name", name), zap.Error(err))
	}
	return archivePath, nil
}

func removeTempFile(path string) {
	if rmErr := os.Remove(path); rmErr != nil {
		log.Error("failed to remove temporary file: %v", zap.Error(anyerror.CleanupError(rmErr)))
	}
}

// readArtifact returns Gallery Index and map of paths to archives aggregated by names
func readArtifact(artifactPath string, indexOnly bool) (archivesPaths map[string]string, index *pb.RpcGalleryDownloadIndexResponse, err error) {
	if artifactPath == "" {
		return nil, nil, fmt.Errorf("no artifact path specified")
	}
	galleryPath := path.Join(artifactPath, artifactGalleryDir)
	entries, err := os.ReadDir(galleryPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read gallery path %s: %w", galleryPath, err)
	}

	archivesPaths = make(map[string]string)
	for _, entry := range entries {
		if entry.Name() == indexName {
			indexData, err := os.ReadFile(path.Join(galleryPath, entry.Name()))
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
			continue
		}

		if indexOnly {
			continue
		}

		if !entry.IsDir() {
			return nil, nil, fmt.Errorf("found non-dir file besides of index: %s", entry.Name())
		}

		innerEntries, err := os.ReadDir(path.Join(galleryPath, entry.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read gallery dir '%s': %w", entry.Name(), err)
		}

		for _, innerEntry := range innerEntries {
			if path.Ext(innerEntry.Name()) != ".zip" {
				continue
			}

			if strings.TrimSuffix(innerEntry.Name(), ".zip") != entry.Name() {
				return nil, nil, fmt.Errorf("zip archive should have the same name as containing folder. Folder: '%s'. Archive: %s",
					entry.Name(), innerEntry.Name())
			}

			archivesPaths[entry.Name()] = path.Join(galleryPath, entry.Name(), innerEntry.Name())
		}
	}

	if index == nil {
		return nil, nil, fmt.Errorf("no index file found")
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
