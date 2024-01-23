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
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
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

	docs, err := e.docsForExport(req.SpaceId, req.ObjectIds, req.IncludeNested, req.IncludeArchived, isAnyblockExport(req.Format))
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
		succeed, tasks = e.exportDocs(ctx, req, docs, wr, succeed, tasks)
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

func (e *export) exportDocs(ctx context.Context, req pb.RpcObjectListExportRequest, docs map[string]*types.Struct, wr writer, succeed int, tasks []process.Task) (int, []process.Task) {
	for docId := range docs {
		did := docId
		task := func() {
			if werr := e.writeDoc(ctx, req, wr, docs, did); werr != nil {
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

func (e *export) docsForExport(spaceID string, reqIds []string, includeNested bool, includeArchived bool, isProtobuf bool) (docs map[string]*types.Struct, err error) {
	if len(reqIds) == 0 {
		return e.getExistedObjects(spaceID, includeArchived, isProtobuf)
	}

	if len(reqIds) > 0 {
		return e.getObjectsByIDs(spaceID, reqIds, includeNested, isProtobuf)
	}
	return
}

func (e *export) getObjectsByIDs(spaceID string, reqIds []string, includeNested bool, isProtobuf bool) (map[string]*types.Struct, error) {
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
			e.getNested(spaceID, id, docs)
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

	for id := range docs {
		err := e.saveFilesForObject(id, docs)
		if err != nil {
			return nil, err
		}
	}
	return docs, nil
}

func (e *export) saveFilesForObject(objectID string, docs map[string]*types.Struct) error {
	var (
		fileHashes []string
		st         *state.State
	)
	if err := getblock.Do(e.picker, objectID, func(b sb.SmartBlock) error {
		st = b.NewState()
		return nil
	}); err != nil {
		return nil
	}
	err := st.Iterate(func(bl simple.Block) (isContinue bool) {
		if fh, ok := bl.(simple.FileHashes); ok {
			fileHashes = fh.FillFileHashes(fileHashes)
		}
		return true
	})
	if err != nil {
		log.Errorf("failed to collect file hashes in state, %s", err)
	}
	fileHashes = e.getFilesFromRelations(st, fileHashes)
	filesObjects, _, err := e.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(fileHashes),
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
		return nil
	}
	for _, fo := range filesObjects {
		id := pbtypes.GetString(fo.Details, bundle.RelationKeyId.String())
		docs[id] = fo.Details
	}
	return nil
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

func (e *export) writeMultiDoc(ctx context.Context, mw converter.MultiConverter, wr writer, docs map[string]*types.Struct, queue process.Queue, includeFiles bool) (succeed int, err error) {
	for did := range docs {
		if err = queue.Wait(func() {
			log.With("objectID", did).Debugf("write doc")
			werr := getblock.Do(e.picker, did, func(b sb.SmartBlock) error {
				if err = mw.Add(b.Space(), b.NewState().Copy()); err != nil {
					return err
				}
				if !includeFiles {
					return nil
				}
				fileHashes := b.GetAndUnsetFileKeys()
				for _, fh := range fileHashes {
					if _, saveFileErr := e.saveFile(ctx, wr, fh.Hash); saveFileErr != nil {
						log.With("hash", fh.Hash).Warnf("can't save file: %v", saveFileErr)
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

	if err = wr.WriteFile("export"+mw.Ext(), bytes.NewReader(mw.Convert(nil)), 0); err != nil {
		return 0, err
	}
	err = nil
	return
}

func (e *export) writeDoc(ctx context.Context,
	req pb.RpcObjectListExportRequest,
	wr writer,
	docInfo map[string]*types.Struct,
	docID string,
) (err error) {
	return getblock.Do(e.picker, docID, func(b sb.SmartBlock) error {
		if pbtypes.GetBool(b.CombinedDetails(), bundle.RelationKeyIsDeleted.String()) {
			return nil
		}
		if !req.IncludeFiles && b.Type() == smartblock.SmartBlockTypeFile {
			return nil
		}
		var (
			keys *files.FileKeys
			s    = b.NewState()
			conv converter.Converter
		)
		if b.Type() == smartblock.SmartBlockTypeFile {
			keys, err = e.handleFileObject(ctx, keys, req.SpaceId, docID, wr, s)
			if err != nil {
				log.With("hash", docID).Warnf("can't save file: %v", err)
				return err
			}
		}
		switch req.Format {
		case model.Export_Markdown:
			conv = md.NewMDConverter(s, wr.Namer())
		case model.Export_Protobuf:
			conv = pbc.NewConverter(s, req.IsJson)
		case model.Export_JSON:
			conv = pbjson.NewConverter(s)
		}
		conv.SetKnownDocs(docInfo)
		conv.SetFileKeys(keys)

		result := conv.Convert(b)
		filename := docID + conv.Ext()
		if req.Format == model.Export_Markdown {
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
		if req.IncludeFiles && !isAnyblockExport(req.Format) {
			e.saveFiles(ctx, b, wr)
		}
		return nil
	})
}

func (e *export) handleFileObject(ctx context.Context,
	keys *files.FileKeys,
	spaceID, docID string,
	wr writer,
	s *state.State,
) (*files.FileKeys, error) {
	keys, err := e.fileService.FileGetKeys(domain.FullID{SpaceID: spaceID, ObjectID: docID})
	if err != nil {
		return nil, err
	}
	filename, err := e.saveFile(ctx, wr, docID)
	if err != nil {
		return nil, err
	}
	s.SetDetail(bundle.RelationKeySource.String(), pbtypes.String(filename))
	return keys, nil
}

func (e *export) saveFiles(ctx context.Context, b sb.SmartBlock, wr writer) {
	st := b.NewState()
	var fileHashes []string
	err := st.Iterate(func(bl simple.Block) (isContinue bool) {
		if fh, ok := bl.(simple.FileHashes); ok {
			fileHashes = fh.FillFileHashes(fileHashes)
		}
		return true
	})
	fileHashes = e.getFilesFromRelations(st, fileHashes)
	if err != nil {
		log.Errorf("failed to collect file hashes in state, %s", err)
	}
	for _, fh := range fileHashes {
		if _, werr := e.saveFile(ctx, wr, fh); werr != nil {
			log.With("hash", fh).Warnf("can't save file: %v", werr)
		}
	}
}

func (e *export) getFilesFromRelations(st *state.State, fileHashes []string) []string {
	for _, relLink := range st.GetRelationLinks() {
		if relLink.Format == model.RelationFormat_file {
			if fileHash := pbtypes.GetString(st.Details(), relLink.GetKey()); fileHash != "" {
				fileHashes = append(fileHashes, fileHash)

			}
			if relationFileHashes := pbtypes.GetStringList(st.Details(), relLink.GetKey()); len(relationFileHashes) > 0 {
				fileHashes = append(fileHashes, relationFileHashes...)

			}
		}
	}
	return fileHashes
}

func (e *export) saveFile(ctx context.Context, wr writer, hash string) (filename string, err error) {
	spaceID, err := e.resolver.ResolveSpaceID(hash)
	if err != nil {
		return "", fmt.Errorf("resolve spaceID: %w", err)
	}
	id := domain.FullID{
		SpaceID:  spaceID,
		ObjectID: hash,
	}
	file, err := e.fileService.FileByHash(ctx, id)
	if err != nil {
		return
	}
	if strings.HasPrefix(file.Info().Media, "image") {
		image, err := e.fileService.ImageByHash(context.TODO(), id)
		if err != nil {
			return "", err
		}
		file, err = image.GetOriginalFile(context.TODO())
		if err != nil {
			return "", err
		}
	}
	origName := file.Meta().Name
	filename = wr.Namer().Get("files", hash, filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader(context.Background())
	if err != nil {
		return
	}
	return filename, wr.WriteFile(filename, rd, file.Info().LastModifiedDate)
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
		sbType == smartblock.SmartBlockTypeFile
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
