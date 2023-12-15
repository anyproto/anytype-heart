package builtinobjects

import (
	"archive/zip"
	"bytes"
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
	"github.com/miolini/datacounter"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/gallery"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/constant"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

//go:embed data/skip.zip
var skipZip []byte

//go:embed data/personal_projects.zip
var personalProjectsZip []byte

//go:embed data/knowledge_base.zip
var knowledgeBaseZip []byte

//go:embed data/notes_diary.zip
var notesDiaryZip []byte

//go:embed data/migration_dashboard.zip
var migrationDashboardZip []byte

//go:embed data/strategic_writing.zip
var strategicWritingZip []byte

//go:embed data/empty.zip
var emptyZip []byte

var (
	log = logging.Logger("anytype-mw-builtinobjects")

	archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
		pb.RpcObjectImportUseCaseRequest_EMPTY:             emptyZip,
		pb.RpcObjectImportUseCaseRequest_SKIP:              skipZip,
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: personalProjectsZip,
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    knowledgeBaseZip,
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       notesDiaryZip,
		pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: strategicWritingZip,
	}

	// TODO: GO-2009 Now we need to create widgets by hands, widget import is not implemented yet
	widgetParams = map[pb.RpcObjectImportUseCaseRequestUseCase][]widgetParameters{
		pb.RpcObjectImportUseCaseRequest_EMPTY: {
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_SKIP: {
			{model.BlockContentWidget_Link, "bafyreiembqdejpkqhupwhukcyqtsjhi43bnqkbp6zfszu26r4c5o6zkeyu", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: {
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, "bafyreibdfkwnnj6xndyzazkm2gersm5fk3yg2274d5hqr6drurncxiyeoi", "", true}, // Tasks
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE: {
			{model.BlockContentWidget_Link, "bafyreiaszkibjyfls2og3ztgxfllqlom422y5ic64z7w3k3oio6f3pc2ia", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY: {
			{model.BlockContentWidget_Link, "bafyreiexkrata5ofvswxyisuumukmkyerdwv3xa34qkxpgx6jtl7waah34", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: {
			{model.BlockContentWidget_List, "bafyreido5lhh4vntmlxh2hwn4b3xfmz53yw5rrfmcl22cdb4phywhjlcdu", "f984ddde-eb13-497e-809a-2b9a96fd3503", true}, // Task tracker
			{model.BlockContentWidget_List, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_Tree, "bafyreicblsgojhhlfduz7ek4g4jh6ejy24fle2q5xjbue5kkcd7ifbc4ki", "", true}, // My Home
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
			{model.BlockContentWidget_Link, "bafyreiaoeaxv4dkw4xgdcgubetieyuqlf24q2kg5pdysz4prun6qg5v2ru", "", true}, // About Anytype
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
		},
	}
)

type BuiltinObjects interface {
	app.Component

	CreateObjectsForUseCase(ctx session.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
	CreateObjectsForExperience(ctx context.Context, spaceID, url, title string, newSpace bool) (err error)
	InjectMigrationDashboard(spaceID string) error
}

type builtinObjects struct {
	service        *block.Service
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
	b.service = a.MustComponent(block.CName).(*block.Service)
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
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	start := time.Now()

	archive, found := archives[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import builtinObjects: invalid Use Case value: %v", useCase)
	}

	if err = b.inject(ctx, spaceID, useCase, archive); err != nil {
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

func (b *builtinObjects) CreateObjectsForExperience(ctx context.Context, spaceID, url, title string, isNewSpace bool) (err error) {
	var (
		path     string
		progress process.Progress
	)
	if _, err = os.Stat(url); err == nil {
		path = url
	} else {
		if progress, err = b.setupProgress(); err != nil {
			return err
		}
		if path, err = b.downloadZipToFile(url, progress); err != nil {
			return err
		}
		defer func() {
			if rmErr := os.Remove(path); rmErr != nil {
				log.Errorf("failed to remove temporary file: %v", oserror.TransformError(rmErr))
			}
		}()
	}

	err = b.importArchive(ctx, spaceID, path, title, pb.RpcObjectImportRequestPbParams_EXPERIENCE, progress, isNewSpace)

	notifErr := b.notifications.CreateAndSendLocal(&model.Notification{
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   spaceID,
		Payload: &model.NotificationPayloadOfGalleryImport{GalleryImport: &model.NotificationGalleryImport{
			ProcessId: progress.Id(),
			ErrorCode: common.GetImportErrorCode(err),
			SpaceId:   spaceID,
			Name:      title,
		}},
	})
	if notifErr != nil {
		log.Errorf("failed to send notification: %v", notifErr)
	}

	return err
}

func (b *builtinObjects) InjectMigrationDashboard(spaceID string) error {
	return b.inject(nil, spaceID, migrationUseCase, migrationDashboardZip)
}

func (b *builtinObjects) inject(ctx session.Context, spaceID string, useCase pb.RpcObjectImportUseCaseRequestUseCase, archive []byte) (err error) {
	path := filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0644); err != nil {
		return fmt.Errorf("failed to save use case archive to temporary file: %w", err)
	}

	defer func() {
		if rmErr := os.Remove(path); rmErr != nil {
			log.Errorf("failed to remove temporary file: %v", oserror.TransformError(rmErr))
		}
	}()

	if err = b.importArchive(context.Background(), spaceID, path, "", pb.RpcObjectImportRequestPbParams_SPACE, nil, false); err != nil {
		return err
	}

	// TODO: GO-1387 Need to use profile.pb to handle dashboard injection during migration
	oldID := migrationDashboardName
	if useCase != migrationUseCase {
		oldID, err = b.getOldSpaceDashboardId(archive)
		if err != nil {
			log.Errorf("Failed to get old id of space dashboard object: %s", err)
			return nil
		}
	}

	newID, err := b.getNewSpaceDashboardId(spaceID, oldID)
	if err != nil {
		log.Errorf("Failed to get new id of space dashboard object: %s", err)
		return nil
	}

	spc, err := b.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	b.handleSpaceDashboard(spc, newID)
	b.createWidgets(ctx, spc, useCase)
	return
}

func (b *builtinObjects) importArchive(ctx context.Context, spaceID, path, title string, importType pb.RpcObjectImportRequestPbParamsType, progress process.Progress, isNewSpace bool) (err error) {
	_, _, err = b.importer.Import(ctx, &pb.RpcObjectImportRequest{
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
	}, model.ObjectOrigin_usecase, progress)

	return err
}

func (b *builtinObjects) getOldSpaceDashboardId(archive []byte) (id string, err error) {
	var (
		rd      io.ReadCloser
		openErr error
	)
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", err
	}
	profileFound := false
	for _, zf := range zr.File {
		if zf.Name == constant.ProfileFile {
			profileFound = true
			rd, openErr = zf.Open()
			if openErr != nil {
				return "", openErr
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

func (b *builtinObjects) getNewSpaceDashboardId(spaceID string, oldID string) (id string, err error) {
	ids, _, err := b.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldID),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}
	return "", err
}

func (b *builtinObjects) handleSpaceDashboard(spc space.Space, id string) {
	if err := b.service.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: spc.DerivedIDs().Workspace,
		Details: []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeySpaceDashboardId.String(),
				Value: pbtypes.String(id),
			},
		},
	}); err != nil {
		log.Errorf("Failed to set SpaceDashboardId relation to Account object: %s", err)
	}
}

func (b *builtinObjects) createWidgets(ctx session.Context, spc space.Space, useCase pb.RpcObjectImportUseCaseRequestUseCase) {
	var err error
	widgetObjectID := spc.DerivedIDs().Widgets

	if err = block.DoStateCtx(b.service, ctx, widgetObjectID, func(s *state.State, w widget.Widget) error {
		for _, param := range widgetParams[useCase] {
			objectID := param.objectID
			if param.isObjectIDChanged {
				objectID, err = b.getNewObjectID(spc.Id(), objectID)
				if err != nil {
					log.Errorf("Skipping creation of widget block as failed to get new object id using old one '%s': %v", objectID, err)
					continue
				}
			}
			request := &pb.RpcBlockCreateWidgetRequest{
				ContextId:    widgetObjectID,
				Position:     model.Block_Bottom,
				WidgetLayout: param.layout,
				Block: &model.Block{
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: objectID,
							Style:         model.BlockContentLink_Page,
							IconSize:      model.BlockContentLink_SizeNone,
							CardStyle:     model.BlockContentLink_Inline,
							Description:   model.BlockContentLink_None,
						},
					},
				},
			}
			if param.viewID != "" {
				request.ViewId = param.viewID
			}
			if _, err = w.CreateBlock(s, request); err != nil {
				log.Errorf("Failed to make Widget blocks: %v", err)
			}
		}
		return nil
	}); err != nil {
		log.Errorf("failed to create widget blocks for useCase '%s': %v",
			pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}
}

func (b *builtinObjects) getNewObjectID(spaceID string, oldID string) (id string, err error) {
	ids, _, err := b.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldID),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}
	return "", err
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
		return "", oserror.TransformError(err)
	}
	defer out.Close()

	if _, err = io.Copy(out, countReader); err != nil {
		return "", err
	}

	progress.SetDone(archiveDownloadingPercents + archiveCopyingPercents)
	return path, nil
}

func (b *builtinObjects) setupProgress() (process.Progress, error) {
	progress := process.NewProgress(pb.ModelProcess_Import)
	if err := b.progress.Add(progress); err != nil {
		return nil, fmt.Errorf("failed to add progress bar: %w", err)
	}
	progress.SetProgressMessage("downloading archive")
	progress.SetTotal(100)
	return progress, nil
}

func getArchiveReaderAndSize(url string) (reader io.ReadCloser, size int64, err error) {
	// nolint: gosec
	resp, err := http.Get(url)
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
