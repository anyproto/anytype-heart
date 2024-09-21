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

// TODO: Remove all embeds when clients support cache
//
//go:embed builtin/get_started.zip
var getStartedZip []byte

//go:embed builtin/personal_projects.zip
var personalProjectsZip []byte

//go:embed builtin/knowledge_base.zip
var knowledgeBaseZip []byte

//go:embed builtin/notes_diary.zip
var notesDiaryZip []byte

//go:embed builtin/strategic_writing.zip
var strategicWritingZip []byte

//go:embed builtin/empty.zip
var emptyZip []byte

var archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
	pb.RpcObjectImportUseCaseRequest_GET_STARTED:       getStartedZip,
	pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: personalProjectsZip,
	pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    knowledgeBaseZip,
	pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       notesDiaryZip,
	pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: strategicWritingZip,
	pb.RpcObjectImportUseCaseRequest_EMPTY:             emptyZip,
}

const (
	CName                          = "gallery-service"
	injectionTimeout               = 30 * time.Second
	downloadManifestTimeoutSeconds = 1

	contentLengthHeader        = "Content-Length"
	archiveDownloadingPercents = 30
	archiveCopyingPercents     = 10

	indexName = "app-index.json"
)

var (
	log = logger.NewNamed(CName)

	ucCodeToTitle = map[pb.RpcObjectImportUseCaseRequestUseCase]string{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED:       "get_started",
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: "personal_projects",
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    "knowledge_base",
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       "notes_diary",
		pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: "strategic_writing",
		pb.RpcObjectImportUseCaseRequest_EMPTY:             "empty",
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
}

type service struct {
	importer       importer.Importer
	store          objectstore.ObjectStore
	tempDirService core.TempDirProvider
	progress       process.Service
	notifications  notifications.Notifications
	indexCache     IndexCache
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.importer = a.MustComponent(importer.CName).(importer.Importer)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.tempDirService = app.MustComponent[core.TempDirProvider](a)
	s.progress = a.MustComponent(process.CName).(process.Service)
	s.notifications = app.MustComponent[notifications.Notifications](a)
	s.indexCache = app.MustComponent[IndexCache](a)
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

	title, found := ucCodeToTitle[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import built-in usecase: invalid Use Case value: %v", useCase)
	}

	if cachePath == "" {
		if ctx == nil {
			ctx = context.Background()
		}
		// TODO: Remove this call when clients support cache
		return s.importUseCase(ctx, spaceID, title, useCase)
		// return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
		// 	fmt.Errorf("failed to import built-in usecase: no path to client cache provided")
	}

	if err = s.ImportExperience(ctx, spaceID, "", title, cachePath, true); err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR,
			fmt.Errorf("failed to import built-in usecase %s: %w",
				pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}

	spent := time.Now().Sub(start)
	if spent > injectionTimeout {
		log.Debug("built-in objects injection time exceeded timeout", zap.String("timeout", injectionTimeout.String()), zap.String("spent", spent.String()))
	}

	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (s *service) ImportExperience(ctx context.Context, spaceID, url, title, cachePath string, isNewSpace bool) (err error) {
	progress, err := s.setupProgress()
	if err != nil {
		return err
	}

	path, removeFunc, err := s.getPathAndRemoveFunc(url, title, cachePath, spaceID, progress)
	if err != nil {
		return err
	}

	importErr := s.importArchive(ctx, spaceID, path, title, pb.RpcObjectImportRequestPbParams_EXPERIENCE, progress, isNewSpace)
	progress.FinishWithNotification(s.provideNotification(spaceID, progress, err, title), err)

	if err != nil {
		log.Error("failed to send notification", zap.Error(err))
	}

	removeFunc(path)
	return importErr
}

// TODO: Remove this method when clients support cache
func (s *service) importUseCase(
	ctx context.Context, spaceID, title string, useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	archive, found := archives[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import built-in usecase: invalid Use Case value: %v", useCase)
	}

	path, remove, err := s.saveArchiveToTempFile(archive)
	if err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR, fmt.Errorf("failed to save built-in usecase in temp file: %w", err)
	}

	err = s.importArchive(ctx, spaceID, path, title, pb.RpcObjectImportRequestPbParams_EXPERIENCE, nil, true)
	remove(path)

	if err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR, err
	}
	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (s *service) getPathAndRemoveFunc(
	url, title, cachePath, spaceID string,
	progress process.Notificationable,
) (path string, removeFunc func(string), err error) {
	if _, err = os.Stat(url); err == nil {
		return url, func(_ string) {}, nil
	}

	var (
		cachedArchive []byte
		downloadLink  string
	)

	cachedArchive, downloadLink, err = s.getArchiveFromCache(title, cachePath)
	if err == nil {
		return s.saveArchiveToTempFile(cachedArchive)
	}

	if errors.Is(err, errOutdatedArchive) {
		url = downloadLink
	}

	path, err = s.downloadZipToFile(url, progress)
	if err == nil {
		return path, removeTempFile, nil
	}

	if cachedArchive != nil {
		log.Warn("failed to download archive from remote. Importing cached archive", zap.Error(err))
		return s.saveArchiveToTempFile(cachedArchive)
	}

	if pErr := progress.Cancel(); pErr != nil {
		log.Error("failed to cancel progress", zap.String("progress id", progress.Id()), zap.Error(pErr))
	}
	progress.FinishWithNotification(s.provideNotification(spaceID, progress, err, title), err)
	if errors.Is(err, uri.ErrFilepathNotSupported) {
		err = fmt.Errorf("invalid path to file: '%s'", url)
	}
	return "", nil, err
}

func (s *service) getArchiveFromCache(
	title, cachePath string,
) (archive []byte, downloadLink string, err error) {
	archives, index, err := readClientCache(cachePath, false)
	if err != nil {
		return nil, "", err
	}

	archive = archives[title]
	if archive == nil {
		return nil, "", fmt.Errorf("archive is not cached")
	}

	cachedManifest, err := getManifestByTitle(index, title)
	if err != nil {
		return nil, "", err
	}

	downloadLink = cachedManifest.DownloadLink

	latestManifest, err := s.indexCache.GetManifest(downloadLink, downloadManifestTimeoutSeconds)
	if err == nil && latestManifest.Hash != cachedManifest.Hash {
		// if hashes are different, then we can get fresher version of archive from remote
		return archive, downloadLink, errOutdatedArchive
	}
	if err != nil {
		log.Error("failed to get latest manifest", zap.String("downloadLink", downloadLink), zap.Error(err))
	}
	return archive, downloadLink, nil
}

func (s *service) saveArchiveToTempFile(archive []byte) (path string, removeFunc func(string), err error) {
	path = filepath.Join(s.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0644); err != nil {
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

func readClientCache(cachePath string, readOnlyIndex bool) (archives map[string][]byte, index *pb.RpcGalleryDownloadIndexResponse, err error) {
	if cachePath == "" {
		return nil, nil, fmt.Errorf("no cache path specified")
	}
	r, err := zip.OpenReader(cachePath)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()

	archives = make(map[string][]byte)

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
			if readOnlyIndex {
				return nil, index, nil
			}
			continue
		}

		ext := path.Ext(f.Name)
		if ext != ".zip" {
			return nil, nil, fmt.Errorf("zip archive is expected, got: '%s'", f.Name)
		}
		title := strings.TrimSuffix(f.Name, ext)

		var data []byte
		data, err = readData(f)
		if err != nil {
			return nil, nil, err
		}
		archives[title] = data
	}
	return
}

func (s *service) provideNotification(spaceID string, progress process.Progress, err error, title string) *model.Notification {
	spaceName := s.store.GetSpaceName(spaceID)
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
	res := s.importer.Import(ctx, importRequest)

	return res.Err
}

func (s *service) downloadZipToFile(url string, progress process.Progress) (path string, err error) {
	if err = uri.ValidateURI(url); err != nil {
		return "", fmt.Errorf("provided URL is not valid: %w", err)
	}
	if !IsInWhitelist(url) {
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

func getManifestByTitle(index *pb.RpcGalleryDownloadIndexResponse, title string) (*model.ManifestInfo, error) {
	if index == nil {
		return nil, fmt.Errorf("index is nil")
	}
	for _, manifest := range index.Experiences {
		if manifest.Title == title {
			return manifest, nil
		}
	}
	return nil, fmt.Errorf("failed to find manifest for title: %s", title)
}

func getManifestByDownloadLink(index *pb.RpcGalleryDownloadIndexResponse, url string) (*model.ManifestInfo, error) {
	for _, manifest := range index.Experiences {
		if manifest.DownloadLink == url {
			return manifest, nil
		}
	}
	return nil, fmt.Errorf("failed to find manifest for url: %s", url)
}
