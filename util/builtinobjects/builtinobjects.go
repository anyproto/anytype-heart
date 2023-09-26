package builtinobjects

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName            = "builtinobjects"
	injectionTimeout = 30 * time.Second

	migrationUseCase       = -1
	migrationDashboardName = "bafyreiha2hjbrzmwo7rpiiechv45vv37d6g5aezyr5wihj3agwawu6zi3u"
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

var (
	log = logging.Logger("anytype-mw-builtinobjects")

	archives = map[pb.RpcObjectImportUseCaseRequestUseCase][]byte{
		pb.RpcObjectImportUseCaseRequest_SKIP:              skipZip,
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: personalProjectsZip,
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE:    knowledgeBaseZip,
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY:       notesDiaryZip,
		pb.RpcObjectImportUseCaseRequest_STRATEGIC_WRITING: strategicWritingZip,
	}

	// TODO: GO-2009 Now we need to create widgets by hands, widget import is not implemented yet
	widgetParams = map[pb.RpcObjectImportUseCaseRequestUseCase][]widgetParameters{
		pb.RpcObjectImportUseCaseRequest_SKIP: {
			{model.BlockContentWidget_Link, "bafyreiag57kbhehecmhe4xks5nv7p5x5flr3xoc6gm7y4i7uznp2f2spum", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_PERSONAL_PROJECTS: {
			{model.BlockContentWidget_Link, "bafyreier6tne4keezldgkfj5qmix4a64gehznuu4vbpqq3edl53qjoswk4", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, "bafyreicigdupziu7vg56l7chnd6shof3e53tkcqqya33ub2v67fmclbjki", "", true}, // Task tracker
			{model.BlockContentWidget_CompactList, "bafyreigtcovw3g3kaowacqzty7t6wcnp2u2365zjzytvgezb7rqjzokbwe", "", true}, // My Notes
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_KNOWLEDGE_BASE: {
			{model.BlockContentWidget_Link, "bafyreiaszkibjyfls2og3ztgxfllqlom422y5ic64z7w3k3oio6f3pc2ia", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, "bafyreiatdyctn5noworljilcvmhhanzyiszrtch6njw3ekz4eichw6g4eu", "", true}, // Task tracker
			{model.BlockContentWidget_CompactList, "bafyreidjcztbyyee3qcxbkk3gp6nbkwjc5zftguubhznwvjej6q5jflp5q", "", true}, // My Notes
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetSet, "", false},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetRecent, "", false},
		},
		pb.RpcObjectImportUseCaseRequest_NOTES_DIARY: {
			{model.BlockContentWidget_Link, "bafyreiexkrata5ofvswxyisuumukmkyerdwv3xa34qkxpgx6jtl7waah34", "", true},
			{model.BlockContentWidget_CompactList, widget.DefaultWidgetFavorite, "", false},
			{model.BlockContentWidget_CompactList, "bafyreighzavahzdk3ewvlcftwfid6uebirl4asymohgikhpanwbzmfdaq4", "", true}, // Task tracker
			{model.BlockContentWidget_CompactList, "bafyreignt4iidebdxh5ydjohhp75yrffebcqhgo4wjonzc3thobitdari4", "", true}, // My Notes
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

	CreateObjectsForUseCase(*session.Context, pb.RpcObjectImportUseCaseRequestUseCase) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error)
	InjectMigrationDashboard() error
}

type builtinObjects struct {
	service        *block.Service
	coreService    core.Service
	importer       importer.Importer
	store          objectstore.ObjectStore
	tempDirService core.TempDirProvider
}

func New() BuiltinObjects {
	return &builtinObjects{}
}

func (b *builtinObjects) Init(a *app.App) (err error) {
	b.service = a.MustComponent(block.CName).(*block.Service)
	b.coreService = a.MustComponent(core.CName).(core.Service)
	b.importer = a.MustComponent(importer.CName).(importer.Importer)
	b.store = app.MustComponent[objectstore.ObjectStore](a)
	b.tempDirService = app.MustComponent[core.TempDirProvider](a)
	return
}

func (b *builtinObjects) Name() (name string) {
	return CName
}

func (b *builtinObjects) CreateObjectsForUseCase(
	ctx *session.Context, useCase pb.RpcObjectImportUseCaseRequestUseCase,
) (code pb.RpcObjectImportUseCaseResponseErrorCode, err error) {
	start := time.Now()

	archive, found := archives[useCase]
	if !found {
		return pb.RpcObjectImportUseCaseResponseError_BAD_INPUT,
			fmt.Errorf("failed to import builtinObjects: invalid Use Case value: %v", useCase)
	}

	if err = b.inject(ctx, useCase, archive); err != nil {
		return pb.RpcObjectImportUseCaseResponseError_UNKNOWN_ERROR,
			fmt.Errorf("failed to import builtinObjects for Use Case %s: %s",
				pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err.Error())
	}

	spent := time.Now().Sub(start)
	if spent > injectionTimeout {
		log.Debugf("built-in objects injection time exceeded timeout of %s and is %s", injectionTimeout.String(), spent.String())
	}

	return pb.RpcObjectImportUseCaseResponseError_NULL, nil
}

func (b *builtinObjects) InjectMigrationDashboard() error {
	return b.inject(nil, migrationUseCase, migrationDashboardZip)
}

func (b *builtinObjects) inject(ctx *session.Context, useCase pb.RpcObjectImportUseCaseRequestUseCase, archive []byte) (err error) {
	path := filepath.Join(b.tempDirService.TempDir(), time.Now().Format("tmp.20060102.150405.99")+".zip")
	if err = os.WriteFile(path, archive, 0644); err != nil {
		return fmt.Errorf("failed to save use case archive to temporary file: %s", err.Error())
	}

	if err = b.importArchive(ctx, path); err != nil {
		return err
	}

	// TODO: GO-1387 Need to use profile.pb to handle dashboard injection during migration
	oldID := migrationDashboardName
	if useCase != migrationUseCase {
		oldID, err = b.getOldSpaceDashboardId(archive)
		if err != nil {
			log.Errorf("Failed to get old id of space dashboard object: %s", err.Error())
			return nil
		}
	}

	newID, err := b.getNewObjectID(oldID)
	if err != nil {
		log.Errorf("Failed to get new id of space dashboard object: %s", err.Error())
		return nil
	}

	b.handleSpaceDashboard(newID)
	b.createWidgets(ctx, useCase)
	return
}

func (b *builtinObjects) importArchive(ctx *session.Context, path string) (err error) {
	if _, err = b.importer.Import(ctx, &pb.RpcObjectImportRequest{
		UpdateExistingObjects: false,
		Type:                  pb.RpcObjectImportRequest_Pb,
		Mode:                  pb.RpcObjectImportRequest_ALL_OR_NOTHING,
		NoProgress:            true,
		IsMigration:           false,
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb.RpcObjectImportRequestPbParams{
				Path:         []string{path},
				NoCollection: true,
			}},
	}); err != nil {
		return err
	}

	if err = os.Remove(path); err != nil {
		log.Errorf("failed to remove temporary file: %s", err.Error())
	}

	return nil
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

func (b *builtinObjects) getNewObjectID(oldID string) (id string, err error) {
	ids, _, err := b.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldID),
			},
		},
	}, nil)
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}
	return "", err
}

func (b *builtinObjects) handleSpaceDashboard(id string) {
	if err := b.service.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: b.coreService.PredefinedBlocks().Account,
		Details: []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeySpaceDashboardId.String(),
				Value: pbtypes.String(id),
			},
		},
	}); err != nil {
		log.Errorf("Failed to set SpaceDashboardId relation to Account object: %s", err.Error())
	}
}

func (b *builtinObjects) createWidgets(ctx *session.Context, useCase pb.RpcObjectImportUseCaseRequestUseCase) {
	var err error
	widgetObjectID := b.coreService.PredefinedBlocks().Widgets

	if err = block.DoStateCtx(b.service, ctx, widgetObjectID, func(s *state.State, w widget.Widget) error {
		for _, param := range widgetParams[useCase] {
			objectID := param.objectID
			if param.isObjectIDChanged {
				objectID, err = b.getNewObjectID(objectID)
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
				return err
			}
		}
		return nil
	}); err != nil {
		log.Errorf("failed to create widget blocks for useCase '%s': %v",
			pb.RpcObjectImportUseCaseRequestUseCase_name[int32(useCase)], err)
	}
}
