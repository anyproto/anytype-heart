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
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/miolini/datacounter"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
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

	migrationUseCase   = -1
	defaultDashboardId = "lastOpened"

	contentLengthHeader        = "Content-Length"
	archiveDownloadingPercents = 30
	archiveCopyingPercents     = 10
)

//go:embed data/start_guide.zip
var startGuideZip []byte

//go:embed data/get_started.zip
var getStartedZip []byte

//go:embed data/migration_dashboard.zip
var migrationDashboardZip []byte

//go:embed data/empty.zip
var emptyZip []byte

//go:embed data/get_started_mobile.zip
var getStartedMobileZip []byte

//go:embed data/empty_mobile.zip
var emptyMobileZip []byte

var (
	log = logging.Logger("anytype-mw-builtinobjects")

	archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
		pb.RpcObjectImportUseCaseRequest_GET_STARTED:        getStartedZip,
		pb.RpcObjectImportUseCaseRequest_GUIDE_ONLY:         startGuideZip,
		pb.RpcObjectImportUseCaseRequest_EMPTY:              emptyZip,
		pb.RpcObjectImportUseCaseRequest_GET_STARTED_MOBILE: getStartedMobileZip,
		pb.RpcObjectImportUseCaseRequest_EMPTY_MOBILE:       emptyMobileZip,
	}
)

type BuiltinObjects interface {
	app.Component

	CreateObjectsForUseCase(ctx session.Context, spaceID string, req pb.RpcObjectImportUseCaseRequestUseCase) (dashboardId string, code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
	CreateObjectsForExperience(ctx context.Context, spaceID, url, title string, newSpace, isAi bool) (err error)
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

type manifest struct {
	DashboardPagePath string `json:"dashboardPage"`
}

func readAiManifest(path string) (m manifest, err error) {
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		return
	}
	defer zipReader.Close()
	f, err := zipReader.Open("manifest.json")
	if err != nil {
		return
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	err = dec.Decode(&m)
	return m, err
}

func (b *builtinObjects) CreateObjectsForExperience(ctx context.Context, spaceID, url, title string, isNewSpace, isAi bool) (err error) {
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
			if !isAi {
				if notificationProgress, ok := progress.(process.Notificationable); ok {
					notificationProgress.FinishWithNotification(b.provideNotification(spaceID, progress, err, title), err)
				}
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

	importFormat := model.Import_Pb
	if isAi {
		importFormat = model.Import_Markdown
		progress = nil
	}
	importErr := b.importArchive(ctx, spaceID, path, title, pb.RpcObjectImportRequestPbParams_SPACE, importFormat, progress)
	if notificationProgress, ok := progress.(process.Notificationable); !isAi && ok {
		notificationProgress.FinishWithNotification(b.provideNotification(spaceID, progress, importErr, title), importErr)
	}

	if importErr != nil {
		log.Errorf("failed to send notification: %v", importErr)
	}

	if isNewSpace && importFormat == model.Import_Pb {
		profile, err := b.getProfile(path)
		if err != nil {
			log.Warnf("failed to profile object: %s", err)
		}
		// TODO: GO-2627 Home page handling should be moved to importer
		b.handleHomePage(profile, spaceID, false)
		removeFunc()
	} else if importFormat == model.Import_Markdown {
		// try to read manifest.json from archive
		manifestData, err := readAiManifest(path)
		if err != nil {
			log.Warnf("failed to read manifest file: %s", err)
		} else {
			sourcePath := common.GetSourceFileHash(manifestData.DashboardPagePath)
			records, err := b.store.SpaceIndex(spaceID).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
				database.FilterLike{
					Key:   bundle.RelationKeySourceFilePath,
					Value: sourcePath,
				},
			}}, 1, 0)
			if err != nil {
				log.Errorf("failed to query object by source path '%s': %v", sourcePath, err)
			}
			if len(records) > 0 {
				id := records[0].Details.GetString(bundle.RelationKeyId)
				spc, err := b.spaceService.Get(context.Background(), spaceID)
				if err != nil {
					log.Errorf("failed to get space: %w", err)
					return err
				}
				b.setHomePageIdToWorkspace(spc, id)
				widgets := []*pb.WidgetBlock{{
					Layout:         model.BlockContentWidget_Link,
					TargetObjectId: id,
				}}
				b.createWidgets(nil, spaceID, pb.RpcObjectImportUseCaseRequest_GET_STARTED, widgets)
			}
		}

		err = b.addTypesView(ctx, spaceID)
		if err != nil {
			log.Errorf("failed to add types view: %v", err)
		}
		removeFunc()
	}

	return importErr
}

func (b *builtinObjects) getTypeViewProperties(spaceID, typeID string) ([]*model.RelationLink, error) {
	spaceIndex := b.store.SpaceIndex(spaceID)
	details, err := spaceIndex.GetDetails(typeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get type details: %w", err)
	}
	// Build relation links from recommended and featured relations
	relationLinks := []*model.RelationLink{
		{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_longtext,
		},
	}

	// Add featured relations
	featuredRelations := details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
	for _, relId := range featuredRelations {
		// Get relation format from space index
		if rel, err := spaceIndex.GetRelationById(relId); err == nil && rel != nil {
			relationLinks = append(relationLinks, &model.RelationLink{
				Key:    rel.Key,
				Format: rel.Format,
			})
		}
	}

	// Add recommended relations
	recommendedRelations := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	for _, relId := range recommendedRelations {
		// Get relation format from space index
		if rel, err := spaceIndex.GetRelationById(relId); err == nil && rel != nil {
			relationLinks = append(relationLinks, &model.RelationLink{
				Key:    rel.Key,
				Format: rel.Format,
			})
		}
	}

	relationLinks = slices.DeleteFunc(relationLinks, func(rel *model.RelationLink) bool {
		return rel.Key == bundle.RelationKeyType.String()
	})
	return relationLinks, nil
}

func (b *builtinObjects) addTypesView(ctx context.Context, spaceID string) error {
	systemTypesUniqueKeys := make([]string, 0, len(bundle.SystemTypes))
	for _, t := range bundle.SystemTypes {
		systemTypesUniqueKeys = append(systemTypesUniqueKeys, t.URL())
	}

	records, err := b.store.SpaceIndex(spaceID).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Value: domain.Int64(model.ObjectType_objectType),
		},
		database.FilterNot{
			database.FilterIn{
				Key:   bundle.RelationKeyUniqueKey,
				Value: domain.StringList(systemTypesUniqueKeys).WrapToList(),
			},
		},
		// todo: later filter-out types in case they were not part of this import
	}}, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to query types: %w", err)
	}
	if len(records) == 0 {
		return nil
	}
	spc, err := b.spaceService.Get(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
	}

	for _, rec := range records {
		uk, err := domain.UnmarshalUniqueKey(rec.Details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			log.Warnf("failed to unmarshal unique key '%s': %v", rec.Details.GetString(bundle.RelationKeyUniqueKey), err)
			continue
		}

		typeId, err := spc.GetTypeIdByKey(ctx, domain.TypeKey(uk.InternalKey()))
		if err != nil {
			log.Warnf("failed to get type id by key '%s': %v", uk.InternalKey(), err)
			continue
		}
		relationLinks, err := b.getTypeViewProperties(spaceID, typeId)
		if err != nil {
			log.Warnf("failed to get type view properties for type '%s': %v", typeId, err)
			continue
		}

		err = cache.DoStateCtx(b.objectGetter, nil, typeId, func(s *state.State, sb dataview.Dataview) error {
			dvBlock, err := sb.GetDataviewBlock(s, template.DataviewBlockId)
			if err != nil {
				return fmt.Errorf("failed to get dataview block: %w", err)
			}

			allView, err := dvBlock.GetView(addr.ObjectTypeAllViewId)
			if err == nil {
				allView.Name = "List"
			}
			view := template.MakeDataviewView(false, relationLinks, model.BlockContentDataviewView_Table, addr.ObjectTypeAllTableViewId, "Grid")
			dvBlock.AddView(view)
			return nil
		})
		if err != nil {
			log.Warnf("failed to add view to type '%s': %v", typeId, err)
			continue
		}
	}
	return nil
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

func (b *builtinObjects) inject(ctx session.Context, spaceID string, useCase pb.RpcObjectImportUseCaseRequestUseCase, archive []byte) (startingPageId string, err error) {
	path := filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0644); err != nil {
		return "", fmt.Errorf("failed to save use case archive to temporary file: %w", err)
	}
	defer func() {
		if rmErr := os.Remove(path); rmErr != nil {
			log.Errorf("failed to remove temporary file: %v", anyerror.CleanupError(rmErr))
		}
	}()

	if err = b.importArchive(context.Background(), spaceID, path, "", pb.RpcObjectImportRequestPbParams_SPACE, model.Import_Pb, nil); err != nil {
		return "", err
	}

	profile, err := b.getProfile(path)
	if err != nil {
		log.Warnf("failed to get profile object: %s", err)
	}
	widgets := b.getWidgets(profile, spaceID)
	if len(widgets) > 0 {
		startingPageId = widgets[0].TargetObjectId
	}

	// TODO: GO-2627 Home page handling should be moved to importer
	_ = b.handleHomePage(profile, spaceID, useCase == migrationUseCase)

	// TODO: GO-2627 Widgets creation should be moved to importer
	b.createWidgets(ctx, spaceID, useCase, widgets)

	return
}

func (b *builtinObjects) importArchive(
	ctx context.Context,
	spaceID, path, title string,
	importType pb.RpcObjectImportRequestPbParamsType,
	importFormat model.ImportType,
	progress process.Progress,
) (err error) {
	origin := objectorigin.Usecase()

	var params pb.IsRpcObjectImportRequestParams
	if importFormat == model.Import_Pb {
		params = &pb.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb.RpcObjectImportRequestPbParams{
				Path:            []string{path},
				NoCollection:    true,
				CollectionTitle: title,
				ImportType:      importType,
			}}
	} else if importFormat == model.Import_Markdown {
		params = &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
				Path:         []string{path},
				NoCollection: true,
			}}
	} else {
		return fmt.Errorf("unsupported import format: %s", importFormat)
	}
	importRequest := &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			SpaceId:               spaceID,
			UpdateExistingObjects: true,
			Type:                  importFormat,
			Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
			NoProgress:            progress == nil,
			IsMigration:           true,
			Params:                params,
			IsNewSpace:            true,
		},
		Origin:   origin,
		Progress: progress,
		IsSync:   true,
	}
	res := b.importer.Import(ctx, importRequest)

	return res.Err
}

func (b *builtinObjects) getWidgets(profile *pb.Profile, spaceId string) []*pb.WidgetBlock {
	if profile == nil {
		return nil
	}

	if len(profile.Widgets) == 0 {
		if profile.StartingPage == "" {
			return nil
		}

		newID, err := b.getNewObjectID(spaceId, profile.StartingPage)
		if err != nil {
			log.Errorf("failed to get new id of home page object: %v", err)
			return nil
		}

		return []*pb.WidgetBlock{{
			Layout:         model.BlockContentWidget_Tree,
			TargetObjectId: newID,
		}}
	}

	for _, w := range profile.Widgets {
		newID, err := b.getNewObjectID(spaceId, w.TargetObjectId)
		if err != nil {
			log.Errorf("failed to get new id of home page object: %v", err)
			return nil
		}
		w.TargetObjectId = newID
	}

	return profile.Widgets
}

func (b *builtinObjects) handleHomePage(profile *pb.Profile, spaceId string, isMigration bool) (dashboardId string) {
	var oldID string
	if !isMigration {
		oldID = profile.SpaceDashboardId
		if oldID == "" {
			oldID = defaultDashboardId
		}
	} else if profile != nil {
		oldID = profile.SpaceDashboardId
	}

	if oldID == "" {
		oldID = defaultDashboardId
	}

	var newID string
	if oldID != defaultDashboardId {
		var err error
		newID, err = b.getNewObjectID(spaceId, oldID)
		if err != nil {
			log.Errorf("failed to get new id of home page object: %s", err)
		} else {
			newID = defaultDashboardId
		}
	} else {
		newID = defaultDashboardId
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

func (b *builtinObjects) getProfile(path string) (profile *pb.Profile, err error) {
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		log.Errorf("cannot open zip file %s: %w", path, err)
		return
	}
	defer zipReader.Close()

	var (
		rd           io.ReadCloser
		profileFound bool
	)
	for _, zf := range zipReader.File {
		if zf.Name == constant.ProfileFile {
			profileFound = true
			rd, err = zf.Open()
			if err != nil {
				return nil, err
			}
			break
		}
	}

	if !profileFound {
		return nil, fmt.Errorf("no profile file included in archive")
	}

	defer rd.Close()
	data, err := io.ReadAll(rd)

	profile = &pb.Profile{}
	if err = profile.Unmarshal(data); err != nil {
		return nil, err
	}
	return profile, nil
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

func (b *builtinObjects) createWidgets(
	ctx session.Context,
	spaceId string,
	useCase pb.RpcObjectImportUseCaseRequestUseCase,
	widgets []*pb.WidgetBlock,
) {
	if useCase != pb.RpcObjectImportUseCaseRequest_GET_STARTED || len(widgets) == 0 {
		return
	}

	spc, err := b.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		log.Errorf("failed to get space: %w", err)
		return
	}

	widgetObjectID := spc.DerivedIDs().Widgets
	requests := make([]*pb.RpcBlockCreateWidgetRequest, 0, len(widgets))
	for _, w := range widgets {
		requests = append(requests, &pb.RpcBlockCreateWidgetRequest{
			ContextId:    widgetObjectID,
			WidgetLayout: w.Layout,
			Position:     model.Block_Inner,
			TargetId:     widgetObjectID,
			ObjectLimit:  w.ObjectLimit,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: w.TargetObjectId,
					},
				},
			},
		})
	}

	if err = cache.DoStateCtx(b.objectGetter, ctx, widgetObjectID, func(s *state.State, w widget.Widget) error {
		for _, req := range requests {
			if _, err := w.CreateBlock(s, req); err != nil {
				log.Errorf("failed to create widget for home page: %v", err)
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
