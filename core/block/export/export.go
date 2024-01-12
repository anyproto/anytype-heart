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

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/gosimple/slug"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/process"
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
	"github.com/anyproto/anytype-heart/util/constant"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

const CName = "export"

const tempFileName = "temp_anytype_backup"

var log = logging.Logger("anytype-mw-export")

type Export interface {
	Export(ctx context.Context, req pb.RpcObjectListExportRequest) (path string, succeed int, err error)
	app.Component
}

type export struct {
	blockService        *block.Service
	picker              getblock.ObjectGetter
	objectStore         objectstore.ObjectStore
	sbtProvider         typeprovider.SmartBlockTypeProvider
	fileService         files.Service
	resolver            idresolver.Resolver
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
	e.picker = app.MustComponent[getblock.ObjectGetter](a)
	e.resolver = a.MustComponent(idresolver.CName).(idresolver.Resolver)
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
			err = oserror.TransformError(err)
			return
		}
	} else {
		if wr, err = newDirWriter(req.Path, req.IncludeFiles); err != nil {
			err = oserror.TransformError(err)
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
		succeed, tasks = e.exportDocs(ctx, req, docs, wr, queue, succeed, tasks)
		err := queue.Wait(tasks...)
		if err != nil {
			e.cleanupFile(wr)
			return "", 0, err
		}
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

func (e *export) exportDocs(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*types.Struct, wr writer, queue process.Queue, succeed int, tasks []process.Task) (int, []process.Task) {
	for docId := range docs {
		did := docId
		task := func() {
			if werr := e.writeDoc(ctx, req.Format, wr, docs, queue, did, req.IncludeFiles, req.IsJson); werr != nil {
				log.With("objectID", did).Warnf("can't export doc: %v", werr)
			} else {
				succeed++
			}
		}
		tasks = append(tasks, task)
	}
	return succeed, tasks
}

func (e *export) exportGraphJson(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*types.Struct, succeed int, wr writer, queue process.Queue) int {
	mc := graphjson.NewMultiConverter(e.sbtProvider)
	mc.SetKnownDocs(docs)
	var werr error
	if succeed, werr = e.writeMultiDoc(ctx, req.SpaceId, mc, wr, docs, queue, req.IncludeFiles); werr != nil {
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
	if succeed, werr = e.writeMultiDoc(ctx, req.SpaceId, mc, wr, docs, queue, req.IncludeFiles); werr != nil {
		log.Warnf("can't export docs: %v", werr)
	}
	return succeed
}

func (e *export) sendNotification(err error, req pb.RpcObjectListExportRequest) {
	errCode := model.NotificationExport_NULL
	if err != nil {
		errCode = model.NotificationExport_UNKNOWN_ERROR
	}
	notificationSendErr := e.notificationService.CreateAndSendLocal(&model.Notification{
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
	spc, err := e.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	docs := make(map[string]*types.Struct)
	res, _, err := e.objectStore.Query(database.Query{
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
	if includeNested {
		for _, id := range ids {
			e.getNested(spaceId, id, docs)
		}
	}
	if includeFiles {
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

	derivedObjects, err := e.getRelatedDerivedObjects(docs)
	if err != nil {
		return nil, err
	}

	for _, do := range derivedObjects {
		id := pbtypes.GetString(do.Details, bundle.RelationKeyId.String())
		docs[id] = do.Details
	}
	return docs, nil
}

func (e *export) getNested(spaceID string, id string, docs map[string]*types.Struct) {
	links, err := e.objectStore.GetOutboundLinksByID(id)
	if err != nil {
		log.Errorf("export failed to get outbound links for id: %s", err)
		return
	}
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
			if len(rec) > 0 {
				docs[link] = rec[0].Details
				e.getNested(spaceID, link, docs)
			}
		}
	}
}

func (e *export) fillLinkedFiles(space clientspace.Space, id string, docs map[string]*types.Struct) error {
	return space.Do(id, func(b sb.SmartBlock) error {
		b.NewState().IterateLinkedFiles(func(fileObjectId string) bool {
			details, err := e.objectStore.GetDetails(fileObjectId)
			if err != nil {
				log.Errorf("failed to get details for file object id %s: %v", fileObjectId, err)
				return true
			}
			docs[fileObjectId] = details.GetDetails()
			return true
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
		sbType, err := e.sbtProvider.Type(spaceID, info.Id)
		if err != nil {
			return nil, fmt.Errorf("get smartblock type: %w", err)
		}
		if !e.objectValid(sbType, info.Id, info, includeArchived, isProtobuf) {
			continue
		}
		objectDetails[info.Id] = info.Details

	}
	if err != nil {
		return nil, err
	}
	return objectDetails, nil
}

func (e *export) writeMultiDoc(ctx context.Context, spaceId string, mw converter.MultiConverter, wr writer, docs map[string]*types.Struct, queue process.Queue, includeFiles bool) (succeed int, err error) {
	for did := range docs {
		if err = queue.Wait(func() {
			log.With("objectID", did).Debugf("write doc")
			werr := getblock.Do(e.picker, did, func(b sb.SmartBlock) error {
				if err = mw.Add(b.Space(), b.NewState().Copy()); err != nil {
					return err
				}
				if includeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
					err = e.saveFile(ctx, wr, b)
					if err != nil {
						return fmt.Errorf("save file: %w", err)
					}
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

func (e *export) writeDoc(ctx context.Context, format model.ExportFormat, wr writer, docInfo map[string]*types.Struct, queue process.Queue, docID string, exportFiles, isJSON bool) (err error) {
	return getblock.Do(e.picker, docID, func(b sb.SmartBlock) error {
		if pbtypes.GetBool(b.CombinedDetails(), bundle.RelationKeyIsDeleted.String()) {
			return nil
		}
		var conv converter.Converter
		switch format {
		case model.Export_Markdown:
			conv = md.NewMDConverter(b.NewState(), wr.Namer())
		case model.Export_Protobuf:
			conv = pbc.NewConverter(b, isJSON)
		case model.Export_JSON:
			conv = pbjson.NewConverter(b)
		}

		if exportFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
			err = e.saveFile(ctx, wr, b)
			if err != nil {
				return fmt.Errorf("save file: %w", err)
			}
			// Don't save file objects in markdown
			if format == model.Export_Markdown {
				return nil
			}
		}

		conv.SetKnownDocs(docInfo)
		result := conv.Convert(b.Type().ToProto())
		filename := docID + conv.Ext()
		s := b.NewState()
		if format == model.Export_Markdown {
			name := pbtypes.GetString(s.Details(), bundle.RelationKeyName.String())
			if name == "" {
				name = s.Snippet()
			}
			filename = wr.Namer().Get("", docID, name, conv.Ext())
		}
		if docID == b.Space().DerivedIDs().Home {
			filename = "index" + conv.Ext()
		}
		lastModifiedDate := pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyLastModifiedDate.String())
		if err = wr.WriteFile(filename, bytes.NewReader(result), lastModifiedDate); err != nil {
			return err
		}
		return nil
	})
}

func (e *export) saveFile(ctx context.Context, wr writer, fileObject sb.SmartBlock) (err error) {
	fullId := domain.FullFileId{
		SpaceId: fileObject.Space().Id(),
		FileId:  domain.FileId(pbtypes.GetString(fileObject.Details(), bundle.RelationKeyFileId.String())),
	}

	file, err := e.fileService.FileByHash(ctx, fullId)
	if err != nil {
		return
	}
	if strings.HasPrefix(file.Info().Media, "image") {
		image, err := e.fileService.ImageByHash(context.TODO(), fullId)
		if err != nil {
			return err
		}
		file, err = image.GetOriginalFile(context.TODO())
		if err != nil {
			return err
		}
	}
	origName := file.Meta().Name
	filename := wr.Namer().Get("files", fileObject.Id(), filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader(context.Background())
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd, file.Info().LastModifiedDate)
}

func (e *export) createProfileFile(spaceID string, wr writer) error {
	spc, err := e.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return err
	}
	var spaceDashBoardID string

	pr, err := e.accountService.LocalProfile()
	if err != nil {
		return err
	}
	err = getblock.Do(e.picker, spc.DerivedIDs().Workspace, func(b sb.SmartBlock) error {
		spaceDashBoardID = pbtypes.GetString(b.CombinedDetails(), bundle.RelationKeySpaceDashboardId.String())
		return nil
	})
	if err != nil {
		return err
	}
	profile := &pb.Profile{
		SpaceDashboardId: spaceDashBoardID,
		Address:          pr.AccountAddr,
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

func (e *export) objectValid(sbType smartblock.SmartBlockType, id string, r *model.ObjectInfo, includeArchived bool, isProtobuf bool) bool {
	if r.Id == addr.AnytypeProfileId {
		return false
	}
	if !isProtobuf && !validTypeForNonProtobuf(sbType) {
		return false
	}
	if isProtobuf && !validType(sbType) {
		return false
	}
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) || strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return false
	}
	if pbtypes.GetBool(r.Details, bundle.RelationKeyIsArchived.String()) && !includeArchived {
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
		sbType == smartblock.SmartBlockTypeDate ||
		sbType == smartblock.SmartBlockTypeWorkspace ||
		sbType == smartblock.SmartBlockTypeWidget ||
		sbType == smartblock.SmartBlockTypeObjectType ||
		sbType == smartblock.SmartBlockTypeRelation ||
		sbType == smartblock.SmartBlockTypeRelationOption ||
		sbType == smartblock.SmartBlockTypeFileObject
}

func validTypeForNonProtobuf(sbType smartblock.SmartBlockType) bool {
	return sbType == smartblock.SmartBlockTypeProfilePage ||
		sbType == smartblock.SmartBlockTypePage
}

func (e *export) cleanupFile(wr writer) {
	wr.Close()
	os.Remove(wr.Path())
}

func (e *export) getRelatedDerivedObjects(objects map[string]*types.Struct) ([]database.Record, error) {
	var (
		derivedObjects []database.Record
		err            error
		relationLinks  pbtypes.RelationLinks
	)

	for id, object := range objects {
		err = getblock.Do(e.picker, id, func(b sb.SmartBlock) error {
			state := b.NewState()
			relationLinks = state.GetRelationLinks()
			return nil
		})
		if err != nil {
			return nil, err
		}
		derivedObjects, err = e.processObject(object, derivedObjects, relationLinks)
		if err != nil {
			return nil, err
		}
	}
	return derivedObjects, nil
}

func (e *export) processObject(object *types.Struct, derivedObjects []database.Record, relationLinks pbtypes.RelationLinks) ([]database.Record, error) {
	for _, relation := range relationLinks {
		storeRelation, err := e.getRelation(relation.Key)
		if err != nil {
			return nil, err
		}
		if storeRelation != nil {
			derivedObjects, err = e.addRelationAndOptions(storeRelation, object, derivedObjects, relation.Key)
			if err != nil {
				return nil, err
			}
		}
	}

	objectTypeDetails, err := e.objectStore.GetDetails(pbtypes.GetString(object, bundle.RelationKeyType.String()))
	if err != nil {
		return nil, err
	}
	derivedObjects = append(derivedObjects, database.Record{Details: objectTypeDetails.Details})

	templates, err := e.getTemplates(pbtypes.GetString(objectTypeDetails.Details, bundle.RelationKeyId.String()))
	if err != nil {
		return nil, err
	}
	derivedObjects = append(derivedObjects, templates...)
	return derivedObjects, nil
}

func (e *export) getRelation(key string) (*database.Record, error) {
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		return nil, err
	}
	relation, _, err := e.objectStore.Query(database.Query{
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

func (e *export) addRelationAndOptions(relation *database.Record, object *types.Struct, derivedObjects []database.Record, relationKey string) ([]database.Record, error) {
	derivedObjects = e.addRelation(relation, derivedObjects)
	format := pbtypes.GetInt64(relation.Details, bundle.RelationKeyRelationFormat.String())
	if format == int64(model.RelationFormat_tag) || format == int64(model.RelationFormat_status) {
		if value := pbtypes.Get(object, relationKey); value != nil {
			relationOptions, err := e.getRelationOptions(value)
			if err != nil {
				return nil, err
			}
			derivedObjects = append(derivedObjects, relationOptions...)
		}
	}

	return derivedObjects, nil
}

func (e *export) addRelation(relation *database.Record, derivedObjects []database.Record) []database.Record {
	if relationKey := relation.Get(bundle.RelationKeyRelationKey.String()); relationKey != nil {
		if !bundle.HasRelation(relationKey.GetStringValue()) {
			derivedObjects = append(derivedObjects, *relation)
		}
	}
	return derivedObjects
}

func (e *export) getRelationOptions(relationOptions *types.Value) ([]database.Record, error) {
	var filter *model.BlockContentDataviewFilter
	if relationOptions.GetStringValue() != "" {
		filter = e.getFilterForStringOption(relationOptions, filter)
	}
	if relationOptions.GetListValue() != nil && len(relationOptions.GetListValue().Values) != 0 {
		filter = e.getFilterForOptionsList(relationOptions, filter)
	}
	if filter == nil {
		return nil, nil
	}
	relationOptionsDetails, _, err := e.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			filter,
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

func (e *export) getFilterForOptionsList(relationOptions *types.Value, filter *model.BlockContentDataviewFilter) *model.BlockContentDataviewFilter {
	ids := make([]string, 0, len(relationOptions.GetListValue().Values))
	for _, id := range relationOptions.GetListValue().Values {
		ids = append(ids, id.GetStringValue())
	}
	filter = &model.BlockContentDataviewFilter{
		RelationKey: bundle.RelationKeyId.String(),
		Condition:   model.BlockContentDataviewFilter_In,
		Value:       pbtypes.StringList(ids),
	}
	return filter
}

func (e *export) getFilterForStringOption(value *types.Value, filter *model.BlockContentDataviewFilter) *model.BlockContentDataviewFilter {
	id := value.GetStringValue()
	filter = &model.BlockContentDataviewFilter{
		RelationKey: bundle.RelationKeyId.String(),
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       pbtypes.String(id),
	}
	return filter
}

func (e *export) getTemplates(id string) ([]database.Record, error) {
	templates, _, err := e.objectStore.Query(database.Query{
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
		return nil, err
	}
	return templates, nil
}
