package export

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/cache"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/converter/dot"
	"github.com/anyproto/anytype-heart/core/converter/graphjson"
	"github.com/anyproto/anytype-heart/core/converter/md"
	"github.com/anyproto/anytype-heart/core/converter/pbc"
	"github.com/anyproto/anytype-heart/core/converter/pbjson"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

const CName = "export"

const (
	tempFileName              = "temp_anytype_backup"
	spaceDirectory            = "spaces"
	typesDirectory            = "types"
	objectsDirectory          = "objects"
	relationsDirectory        = "relations"
	relationsOptionsDirectory = "relationsOptions"
	templatesDirectory        = "templates"
	filesObjects              = "filesObjects"
)

var log = logging.Logger("anytype-mw-export")

type Export interface {
	Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error)
	app.Component
}

type export struct {
	blockService        *block.Service
	picker              cache.ObjectGetter
	objectStore         objectstore.ObjectStore
	sbtProvider         typeprovider.SmartBlockTypeProvider
	fileService         files.Service
	spaceService        space.Service
	accountService      account.Service
	notificationService notifications.Notifications
}

func New() Export {
	return &export{}
}

func (e *export) Init(a *app.App) (err error) {
	e.blockService = a.MustComponent(block.CName).(*block.Service)
	e.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	e.fileService = app.MustComponent[files.Service](a)
	e.picker = app.MustComponent[cache.ObjectGetter](a)
	e.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	e.spaceService = app.MustComponent[space.Service](a)
	e.accountService = app.MustComponent[account.Service](a)
	e.notificationService = app.MustComponent[notifications.Notifications](a)
	return
}

func (e *export) Name() (name string) {
	return CName
}

func (e *export) Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error) {
	queue := e.blockService.Process().NewQueue(pb.ModelProcess{
		Id:    bson.NewObjectId().Hex(),
		Type:  pb.ModelProcess_Export,
		State: 0,
	}, 4)
	queue.SetMessage("prepare")

	if err = queue.Start(); err != nil {
		return
	}
	defer func() {
		queue.Stop(err)
		e.sendNotification(err, req)
	}()

	docs, err := e.docsForExport(req.SpaceId, req)
	if err != nil {
		return
	}

	var wr writer
	if req.Zip {
		if wr, err = newZipWriter(req.Path, tempFileName); err != nil {
			err = anyerror.CleanupError(err)
			return
		}
	} else {
		if wr, err = newDirWriter(req.Path, req.IncludeFiles); err != nil {
			err = anyerror.CleanupError(err)
			return
		}
	}

	queue.SetMessage("export docs")
	if req.Format == model.Export_Protobuf && len(req.ObjectIds) == 0 {
		if err = e.createProfileFile(req.SpaceId, wr); err != nil {
			log.Errorf("failed to create profile file: %s", err)
		}
	}
	if req.Format == model.Export_DOT || req.Format == model.Export_SVG {
		succeed = e.exportDotAndSVG(ctx, req, docs, succeed, wr, queue)
	} else if req.Format == model.Export_GRAPH_JSON {
		succeed = e.exportGraphJson(ctx, req, docs, succeed, wr, queue)
	} else {
		tasks := make([]process.Task, 0, len(docs))
		var succeedAsync int64
		tasks = e.exportDocs(ctx, req, docs, wr, queue, &succeedAsync, tasks)
		err := queue.Wait(tasks...)
		if err != nil {
			e.cleanupFile(wr)
			return "", 0, err
		}
		succeed += int(succeedAsync)
	}
	if err = queue.Finalize(); err != nil {
		e.cleanupFile(wr)
		return "", 0, nil
	}
	wr.Close()
	if req.Zip {
		return e.renameZipArchive(req, wr, succeed)
	}
	return wr.Path(), succeed, nil
}

func (e *export) exportDocs(ctx context.Context,
	req pb.RpcObjectListExportRequest,
	docs map[string]*types.Struct,
	wr writer, queue process.Queue,
	succeed *int64,
	tasks []process.Task,
) []process.Task {
	for docId := range docs {
		did := docId
		task := func() {
			if werr := e.writeDoc(ctx, &req, wr, docs, queue, did); werr != nil {
				log.With("objectID", did).Warnf("can't export doc: %v", werr)
			} else {
				atomic.AddInt64(succeed, 1)
			}
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func (e *export) exportGraphJson(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*types.Struct, succeed int, wr writer, queue process.Queue) int {
	mc := graphjson.NewMultiConverter(e.sbtProvider)
	mc.SetKnownDocs(docs)
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, mc, wr, docs, queue, req.IncludeFiles); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
}

func (e *export) exportDotAndSVG(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*types.Struct, succeed int, wr writer, queue process.Queue) int {
	var format = dot.ExportFormatDOT
	if req.Format == model.Export_SVG {
		format = dot.ExportFormatSVG
	}
	mc := dot.NewMultiConverter(format, e.sbtProvider)
	mc.SetKnownDocs(docs)
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, mc, wr, docs, queue, req.IncludeFiles); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
}

func (e *export) sendNotification(err error, req pb.RpcObjectListExportRequest) {
	errCode := model.NotificationExport_NULL
	if err != nil {
		errCode = model.NotificationExport_UNKNOWN_ERROR
	}
	notificationSendErr := e.notificationService.CreateAndSend(&model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Payload: &model.NotificationPayloadOfExport{Export: &model.NotificationExport{
			ErrorCode:  errCode,
			ExportType: req.Format,
		}},
		Space: req.SpaceId,
	})
	if notificationSendErr != nil {
		log.Errorf("failed to send notification: %v", notificationSendErr)
	}
}

func (e *export) renameZipArchive(req pb.RpcObjectListExportRequest, wr writer, succeed int) (string, int, error) {
	zipName := getZipName(req.Path)
	err := os.Rename(wr.Path(), zipName)
	if err != nil {
		os.Remove(wr.Path())
		return "", 0, nil
	}
	return zipName, succeed, nil
}

func isAnyblockExport(format model.ExportFormat) bool {
	return format == model.Export_Protobuf || format == model.Export_JSON
}

func (e *export) docsForExport(spaceID string, req pb.RpcObjectListExportRequest) (docs map[string]*types.Struct, err error) {
	isProtobuf := isAnyblockExport(req.Format)
	if len(req.ObjectIds) == 0 {
		return e.getExistedObjects(spaceID, req.IncludeArchived, isProtobuf)
	}

	if len(req.ObjectIds) > 0 {
		return e.getObjectsByIDs(spaceID, req.ObjectIds, req.IncludeNested, req.IncludeFiles, isProtobuf)
	}
	return
}

func (e *export) getObjectsByIDs(spaceId string, reqIds []string, includeNested bool, includeFiles bool, isProtobuf bool) (map[string]*types.Struct, error) {
	docs := make(map[string]*types.Struct)
	res, err := e.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(reqIds),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res))
	for _, r := range res {
		id := pbtypes.GetString(r.Details, bundle.RelationKeyId.String())
		docs[id] = r.Details
		ids = append(ids, id)
	}
	var nestedDocsIds []string
	if includeNested {
		for _, id := range ids {
			nestedDocsIds = e.getNested(spaceId, id, docs)
		}
	}
	ids = append(ids, nestedDocsIds...)
	if includeFiles {
		spc, err := e.spaceService.Get(context.Background(), spaceId)
		if err != nil {
			return nil, fmt.Errorf("get space: %w", err)
		}
		for _, id := range ids {
			err = e.fillLinkedFiles(spc, id, docs)
			if err != nil {
				return nil, err
			}
		}
	}

	if !isProtobuf {
		return docs, nil
	}

	err = e.addDerivedObjects(spaceId, docs, includeNested)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func (e *export) addDerivedObjects(spaceId string, docs map[string]*types.Struct, includeNested bool) error {
	derivedObjects, err := e.getRelatedDerivedObjects(docs)
	if err != nil {
		return err
	}
	derivedObjectsMap := make(map[string]*types.Struct)
	for _, object := range derivedObjects {
		id := pbtypes.GetString(object.Details, bundle.RelationKeyId.String())
		derivedObjectsMap[id] = object.Details
	}
	if includeNested {
		for _, object := range derivedObjects {
			id := pbtypes.GetString(object.Details, bundle.RelationKeyId.String())
			e.getNested(spaceId, id, derivedObjectsMap)
		}
	}
	for id, details := range derivedObjectsMap {
		docs[id] = details
	}
	return nil
}

func (e *export) getNested(spaceID string, id string, docs map[string]*types.Struct) []string {
	links, err := e.objectStore.GetOutboundLinksByID(id)
	if err != nil {
		log.Errorf("export failed to get outbound links for id: %s", err)
		return nil
	}
	var nestedDocsIds []string
	for _, link := range links {
		if _, exists := docs[link]; !exists {
			sbt, sbtErr := e.sbtProvider.Type(spaceID, link)
			if sbtErr != nil {
				log.Errorf("failed to get smartblocktype of id %s", link)
				continue
			}
			if !validType(sbt) {
				continue
			}
			rec, qErr := e.objectStore.QueryByID([]string{link})
			if qErr != nil {
				log.Errorf("failed to query id %s, err: %s", qErr, err)
				continue
			}
			if isLinkedObjectExist(rec) {
				docs[link] = rec[0].Details
				nestedDocsIds = append(nestedDocsIds, link)
				nestedDocsIds = append(nestedDocsIds, e.getNested(spaceID, link, docs)...)
			}
		}
	}
	return nestedDocsIds
}

func (e *export) fillLinkedFiles(space clientspace.Space, id string, docs map[string]*types.Struct) error {
	return space.Do(id, func(b sb.SmartBlock) error {
		b.NewState().IterateLinkedFiles(func(fileObjectId string) {
			details, err := e.objectStore.GetDetails(fileObjectId)
			if err != nil {
				log.Errorf("failed to get details for file object id %s: %v", fileObjectId, err)
				return
			}
			docs[fileObjectId] = details.GetDetails()

		})
		return nil
	})
}

func (e *export) getExistedObjects(spaceID string, includeArchived bool, isProtobuf bool) (map[string]*types.Struct, error) {
	res, err := e.objectStore.List(spaceID, false)
	if err != nil {
		return nil, err
	}
	if includeArchived {
		archivedObjects, err := e.objectStore.List(spaceID, true)
		if err != nil {
			return nil, err
		}
		res = append(res, archivedObjects...)
	}
	objectDetails := make(map[string]*types.Struct, len(res))
	for _, info := range res {
		objectSpaceID := spaceID
		if spaceID == "" {
			objectSpaceID = pbtypes.GetString(info.Details, bundle.RelationKeySpaceId.String())
		}
		sbType, err := e.sbtProvider.Type(objectSpaceID, info.Id)
		if err != nil {
			log.With("objectId", info.Id).Errorf("failed to get smartblock type: %v", err)
			continue
		}
		if !e.objectValid(sbType, info, includeArchived, isProtobuf) {
			continue
		}
		objectDetails[info.Id] = info.Details

	}
	if err != nil {
		return nil, err
	}
	return objectDetails, nil
}

func (e *export) writeMultiDoc(ctx context.Context,
	mw converter.MultiConverter,
	wr writer,
	docs map[string]*types.Struct,
	queue process.Queue,
	includeFiles bool,
) (succeed int, err error) {
	for did := range docs {
		if err = queue.Wait(func() {
			log.With("objectID", did).Debugf("write doc")
			werr := cache.Do(e.picker, did, func(b sb.SmartBlock) error {
				st := b.NewState().Copy()
				if includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
					fileName, err := e.saveFile(ctx, wr, b, false)
					if err != nil {
						return fmt.Errorf("save file: %w", err)
					}
					st.SetDetailAndBundledRelation(bundle.RelationKeySource, pbtypes.String(fileName))
				}
				if err = mw.Add(b.Space(), st); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				log.With("objectID", did).Warnf("can't export doc: %v", werr)
			} else {
				succeed++
			}

		}); err != nil {
			return
		}
	}

	if err = wr.WriteFile("export"+mw.Ext(), bytes.NewReader(mw.Convert(0)), 0); err != nil {
		return 0, err
	}
	err = nil
	return
}

func (e *export) writeDoc(ctx context.Context, req *pb.RpcObjectListExportRequest, wr writer, docInfo map[string]*types.Struct, queue process.Queue, docId string) (err error) {
	return cache.Do(e.picker, docId, func(b sb.SmartBlock) error {
		st := b.NewState()
		if pbtypes.GetBool(st.CombinedDetails(), bundle.RelationKeyIsDeleted.String()) {
			return nil
		}

		if req.IncludeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
			fileName, err := e.saveFile(ctx, wr, b, req.SpaceId == "")
			if err != nil {
				return fmt.Errorf("save file: %w", err)
			}
			st.SetDetailAndBundledRelation(bundle.RelationKeySource, pbtypes.String(fileName))
			// Don't save file objects in markdown
			if req.Format == model.Export_Markdown {
				return nil
			}
		}

		var conv converter.Converter
		switch req.Format {
		case model.Export_Markdown:
			conv = md.NewMDConverter(st, wr.Namer())
		case model.Export_Protobuf:
			conv = pbc.NewConverter(st, req.IsJson)
		case model.Export_JSON:
			conv = pbjson.NewConverter(st)
		}
		conv.SetKnownDocs(docInfo)
		result := conv.Convert(b.Type().ToProto())
		var filename string
		if req.Format == model.Export_Markdown {
			filename = e.makeMarkdownName(st, wr, docId, conv, req.SpaceId)
		} else if docId == b.Space().DerivedIDs().Home {
			filename = "index" + conv.Ext()
		} else {
			filename = e.makeFileName(docId, req.SpaceId, conv, st, b.Type())
		}
		lastModifiedDate := pbtypes.GetInt64(st.LocalDetails(), bundle.RelationKeyLastModifiedDate.String())
		if err = wr.WriteFile(filename, bytes.NewReader(result), lastModifiedDate); err != nil {
			return err
		}
		return nil
	})
}

func (e *export) makeMarkdownName(s *state.State, wr writer, docID string, conv converter.Converter, spaceId string) string {
	name := pbtypes.GetString(s.Details(), bundle.RelationKeyName.String())
	if name == "" {
		name = s.Snippet()
	}
	path := ""
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := pbtypes.GetString(s.LocalDetails(), bundle.RelationKeySpaceId.String())
		path = filepath.Join(spaceDirectory, spaceId)
	}
	return wr.Namer().Get(path, docID, name, conv.Ext())
}

func (e *export) makeFileName(docId, spaceId string, conv converter.Converter, st *state.State, blockType smartblock.SmartBlockType) string {
	dir := e.provideFileDirectory(blockType)
	filename := filepath.Join(dir, docId+conv.Ext())
	// space can be empty in case user want to export all spaces
	if spaceId == "" {
		spaceId := pbtypes.GetString(st.LocalDetails(), bundle.RelationKeySpaceId.String())
		filename = filepath.Join(spaceDirectory, spaceId, filename)
	}
	return filename
}

func (e *export) provideFileDirectory(blockType smartblock.SmartBlockType) string {
	switch blockType {
	case smartblock.SmartBlockTypeRelation:
		return relationsDirectory
	case smartblock.SmartBlockTypeRelationOption:
		return relationsOptionsDirectory
	case smartblock.SmartBlockTypeObjectType:
		return typesDirectory
	case smartblock.SmartBlockTypeTemplate:
		return templatesDirectory
	case smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypeFileObject:
		return filesObjects
	default:
		return objectsDirectory
	}
}

func (e *export) saveFile(ctx context.Context, wr writer, fileObject sb.SmartBlock, exportAllSpaces bool) (fileName string, err error) {
	fullId := domain.FullFileId{
		SpaceId: fileObject.Space().Id(),
		FileId:  domain.FileId(pbtypes.GetString(fileObject.Details(), bundle.RelationKeyFileId.String())),
	}

	file, err := e.fileService.FileByHash(ctx, fullId)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(file.Info().Media, "image") {
		image, err := e.fileService.ImageByHash(context.TODO(), fullId)
		if err != nil {
			return "", err
		}
		file, err = image.GetOriginalFile()
		if err != nil {
			return "", err
		}
	}
	origName := file.Meta().Name
	rootPath := "files"
	if exportAllSpaces {
		rootPath = filepath.Join(spaceDirectory, fileObject.Space().Id(), rootPath)
	}
	fileName = wr.Namer().Get(rootPath, fileObject.Id(), filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader(context.Background())
	if err != nil {
		return "", err
	}
	return fileName, wr.WriteFile(fileName, rd, file.Info().LastModifiedDate)
}

func (e *export) createProfileFile(spaceID string, wr writer) error {
	spc, err := e.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return err
	}
	var spaceDashBoardID string

	pr, err := e.accountService.ProfileInfo()
	if err != nil {
		return err
	}
	err = cache.Do(e.picker, spc.DerivedIDs().Workspace, func(b sb.SmartBlock) error {
		spaceDashBoardID = pbtypes.GetString(b.CombinedDetails(), bundle.RelationKeySpaceDashboardId.String())
		return nil
	})
	if err != nil {
		return err
	}
	profile := &pb.Profile{
		SpaceDashboardId: spaceDashBoardID,
		Address:          pr.AccountId,
		Name:             pr.Name,
		Avatar:           pr.IconImage,
		ProfileId:        pr.Id,
	}
	data, err := profile.Marshal()
	if err != nil {
		return err
	}
	err = wr.WriteFile(constant.ProfileFile, bytes.NewReader(data), 0)
	if err != nil {
		return err
	}
	return nil
}

func (e *export) objectValid(sbType smartblock.SmartBlockType, info *model.ObjectInfo, includeArchived bool, isProtobuf bool) bool {
	if info.Id == addr.AnytypeProfileId {
		return false
	}
	if !isProtobuf && !validTypeForNonProtobuf(sbType) && !validLayoutForNonProtobuf(info.Details) {
		return false
	}
	if isProtobuf && !validType(sbType) {
		return false
	}
	if strings.HasPrefix(info.Id, addr.BundledObjectTypeURLPrefix) || strings.HasPrefix(info.Id, addr.BundledRelationURLPrefix) {
		return false
	}
	if pbtypes.GetBool(info.Details, bundle.RelationKeyIsArchived.String()) && !includeArchived {
		return false
	}
	return true
}

func newNamer() *namer {
	return &namer{
		names: make(map[string]string),
	}
}

type namer struct {
	// id -> name and name -> id
	names map[string]string
	mu    sync.Mutex
}

func (fn *namer) Get(path, hash, title, ext string) (name string) {
	const fileLenLimit = 48
	fn.mu.Lock()
	defer fn.mu.Unlock()
	var ok bool
	if name, ok = fn.names[hash]; ok {
		return name
	}
	title = slug.Make(strings.TrimSuffix(title, ext))
	name = text.Truncate(title, fileLenLimit)
	name = strings.TrimSuffix(name, text.TruncateEllipsis)
	var (
		i = 0
		b = 36
	)
	gname := filepath.Join(path, name+ext)
	for {
		if _, ok = fn.names[gname]; !ok {
			fn.names[hash] = gname
			fn.names[gname] = hash
			return gname
		}
		i++
		n := int64(i * b)
		gname = filepath.Join(path, name+"_"+strconv.FormatInt(rand.Int63n(n), b)+ext)
	}
}

func validType(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeHome ||
		sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeSubObject ||
		sbType == smartblock.SmartBlockTypeTemplate ||
		sbType == smartblock.SmartBlockTypeWorkspace ||
		sbType == smartblock.SmartBlockTypeWidget ||
		sbType == smartblock.SmartBlockTypeObjectType ||
		sbType == smartblock.SmartBlockTypeRelation ||
		sbType == smartblock.SmartBlockTypeRelationOption ||
		sbType == smartblock.SmartBlockTypeFileObject ||
		sbType == smartblock.SmartBlockTypeParticipant
}

func validTypeForNonProtobuf(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage ||
		sbType == smartblock.SmartBlockTypeFileObject
}

func validLayoutForNonProtobuf(details *types.Struct) bool {
	return pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) != float64(model.ObjectType_collection) &&
		pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) != float64(model.ObjectType_set)
}

func (e *export) cleanupFile(wr writer) {
	wr.Close()
	os.Remove(wr.Path())
}

func (e *export) getRelatedDerivedObjects(objects map[string]*types.Struct) ([]database.Record, error) {
	derivedObjects, typesAndTemplates, err := e.iterateObjects(objects)
	if err != nil {
		return nil, err
	}
	// get derived objects only from types and templates,
	// because relations currently have only system relations and object type
	if len(typesAndTemplates) > 0 {
		derivedObjectsMap := make(map[string]*types.Struct, 0)
		for _, object := range typesAndTemplates {
			id := pbtypes.GetString(object.Details, bundle.RelationKeyId.String())
			derivedObjectsMap[id] = object.Details
		}
		iteratedObjects, typesAndTemplates, err := e.iterateObjects(derivedObjectsMap)
		if err != nil {
			return nil, err
		}
		derivedObjects = append(derivedObjects, iteratedObjects...)
		derivedObjects = append(derivedObjects, typesAndTemplates...)
	}
	return derivedObjects, nil
}

func (e *export) iterateObjects(objects map[string]*types.Struct,
) (allObjects []database.Record, typesAndTemplates []database.Record, err error) {
	var relations []string
	for id, object := range objects {
		err = cache.Do(e.picker, id, func(b sb.SmartBlock) error {
			state := b.NewState()
			relations = e.getObjectRelations(state, relations)
			details := state.Details()
			if e.isObjectWithDataview(details) {
				dataviewRelations, err := e.getDataviewRelations(state)
				if err != nil {
					return err
				}
				relations = lo.Union(relations, dataviewRelations)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		allObjects, typesAndTemplates, err = e.processObject(object, allObjects, typesAndTemplates, relations)
		if err != nil {
			return nil, nil, err
		}
	}
	return allObjects, typesAndTemplates, nil
}

func (e *export) getDataviewRelations(state *state.State) ([]string, error) {
	var relations []string
	err := state.Iterate(func(b simple.Block) (isContinue bool) {
		if dataview := b.Model().GetDataview(); dataview != nil {
			for _, view := range dataview.Views {
				for _, relation := range view.Relations {
					relations = append(relations, relation.Key)
				}
			}
		}
		return true
	})
	return relations, err
}

func (e *export) getObjectRelations(state *state.State, relations []string) []string {
	relationLinks := state.GetRelationLinks()
	for _, link := range relationLinks {
		relations = append(relations, link.Key)
	}
	return relations
}

func (e *export) isObjectWithDataview(details *types.Struct) bool {
	return pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) == float64(model.ObjectType_collection) ||
		pbtypes.GetFloat64(details, bundle.RelationKeyLayout.String()) == float64(model.ObjectType_set)
}

func (e *export) processObject(object *types.Struct,
	derivedObjects []database.Record,
	typesAndTemplates []database.Record,
	relations []string,
) ([]database.Record, []database.Record, error) {
	for _, relation := range relations {
		storeRelation, err := e.getRelation(relation)
		if err != nil {
			return nil, nil, err
		}
		if storeRelation != nil {
			derivedObjects, err = e.addRelationAndOptions(storeRelation, derivedObjects, relation)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	objectTypeId := pbtypes.GetString(object, bundle.RelationKeyType.String())

	var err error
	derivedObjects, typesAndTemplates, err = e.addObjectType(objectTypeId, derivedObjects, typesAndTemplates)
	if err != nil {
		return nil, nil, err
	}

	derivedObjects, typesAndTemplates, err = e.addTemplates(objectTypeId, derivedObjects, typesAndTemplates)
	if err != nil {
		return nil, nil, err
	}
	derivedObjects, err = e.handleSetOfRelation(object, derivedObjects)
	if err != nil {
		return nil, nil, err
	}
	return derivedObjects, typesAndTemplates, nil
}

func (e *export) addObjectType(objectTypeId string, derivedObjects []database.Record, typesAndTemplates []database.Record) ([]database.Record, []database.Record, error) {
	objectTypeDetails, err := e.objectStore.GetDetails(objectTypeId)
	if err != nil {
		return nil, nil, err
	}
	if objectTypeDetails == nil || objectTypeDetails.Details == nil || len(objectTypeDetails.Details.Fields) == 0 {
		return derivedObjects, typesAndTemplates, nil
	}
	uniqueKey := pbtypes.GetString(objectTypeDetails.Details, bundle.RelationKeyUniqueKey.String())
	key, err := domain.GetTypeKeyFromRawUniqueKey(uniqueKey)
	if err != nil {
		return nil, nil, err
	}
	if bundle.IsInternalType(key) {
		return derivedObjects, typesAndTemplates, nil
	}
	recommendedRelations := pbtypes.GetStringList(objectTypeDetails.Details, bundle.RelationKeyRecommendedRelations.String())
	for _, relation := range recommendedRelations {
		if relation == addr.MissingObject {
			continue
		}
		details, err := e.objectStore.GetDetails(relation)
		if err != nil {
			return nil, nil, err
		}
		relationKey := pbtypes.GetString(details.Details, bundle.RelationKeyUniqueKey.String())
		uniqueKey, err := domain.UnmarshalUniqueKey(relationKey)
		if err != nil {
			return nil, nil, err
		}
		if bundle.IsSystemRelation(domain.RelationKey(uniqueKey.InternalKey())) {
			continue
		}
		derivedObjects = append(derivedObjects, database.Record{Details: details.Details})
	}
	derivedObjects = append(derivedObjects, database.Record{Details: objectTypeDetails.Details})
	typesAndTemplates = append(typesAndTemplates, database.Record{Details: objectTypeDetails.Details})
	return derivedObjects, typesAndTemplates, nil
}

func (e *export) getRelation(key string) (*database.Record, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		return nil, err
	}
	relation, err := e.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(uniqueKey.Marshal()),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(relation) == 0 {
		return nil, nil
	}
	return &relation[0], nil
}

func (e *export) addRelationAndOptions(relation *database.Record, derivedObjects []database.Record, relationKey string) ([]database.Record, error) {
	derivedObjects = e.addRelation(*relation, derivedObjects)
	format := pbtypes.GetInt64(relation.Details, bundle.RelationKeyRelationFormat.String())
	if format == int64(model.RelationFormat_tag) || format == int64(model.RelationFormat_status) {
		relationOptions, err := e.getRelationOptions(relationKey)
		if err != nil {
			return nil, err
		}
		derivedObjects = append(derivedObjects, relationOptions...)
	}

	return derivedObjects, nil
}

func (e *export) addRelation(relation database.Record, derivedObjects []database.Record) []database.Record {
	if relationKey := pbtypes.GetString(relation.Details, bundle.RelationKeyRelationKey.String()); relationKey != "" {
		if !bundle.HasRelation(relationKey) {
			derivedObjects = append(derivedObjects, relation)
		}
	}
	return derivedObjects
}

func (e *export) getRelationOptions(relationKey string) ([]database.Record, error) {
	relationOptionsDetails, err := e.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(relationKey),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return relationOptionsDetails, nil
}

func (e *export) addTemplates(id string, derivedObjects []database.Record, typesAndTemplates []database.Record) ([]database.Record, []database.Record, error) {
	templates, err := e.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(id),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	derivedObjects = append(derivedObjects, templates...)
	typesAndTemplates = append(typesAndTemplates, templates...)
	return derivedObjects, typesAndTemplates, nil
}

func (e *export) handleSetOfRelation(object *types.Struct, derivedObjects []database.Record) ([]database.Record, error) {
	setOfList := pbtypes.GetStringList(object, bundle.RelationKeySetOf.String())
	if len(setOfList) > 0 {
		types, err := e.objectStore.Query(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.StringList(setOfList),
				},
				{
					RelationKey: bundle.RelationKeyIsArchived.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Bool(false),
				},
				{
					RelationKey: bundle.RelationKeyIsDeleted.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Bool(false),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		derivedObjects = append(derivedObjects, types...)
	}
	return derivedObjects, nil
}

func isLinkedObjectExist(rec []database.Record) bool {
	return len(rec) > 0 && !pbtypes.GetBool(rec[0].Details, bundle.RelationKeyIsDeleted.String())
}
