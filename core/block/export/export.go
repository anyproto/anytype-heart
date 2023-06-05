package export

import (
	"bytes"
	"context"
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

	"github.com/anyproto/anytype-heart/core/block"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/converter/dot"
	"github.com/anyproto/anytype-heart/core/converter/graphjson"
	"github.com/anyproto/anytype-heart/core/converter/md"
	"github.com/anyproto/anytype-heart/core/converter/pbc"
	"github.com/anyproto/anytype-heart/core/converter/pbjson"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

const CName = "export"

const tempFileName = "temp_anytype_backup"

var log = logging.Logger("anytype-mw-export")

type Export interface {
	Export(req pb.RpcObjectListExportRequest) (path string, succeed int, err error)
	app.Component
}

type export struct {
	bs          *block.Service
	objectStore objectstore.ObjectStore
	a           core.Service
	sbtProvider typeprovider.SmartBlockTypeProvider
	fileService files.Service
}

func New(sbtProvider typeprovider.SmartBlockTypeProvider) Export {
	return &export{
		sbtProvider: sbtProvider,
	}
}

func (e *export) Init(a *app.App) (err error) {
	e.bs = a.MustComponent(block.CName).(*block.Service)
	e.a = a.MustComponent(core.CName).(core.Service)
	e.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	e.fileService = app.MustComponent[files.Service](a)
	return
}

func (e *export) Name() (name string) {
	return CName
}

func (e *export) Export(req pb.RpcObjectListExportRequest) (path string, succeed int, err error) {
	queue := e.bs.Process().NewQueue(pb.ModelProcess{
		Id:    bson.NewObjectId().Hex(),
		Type:  pb.ModelProcess_Export,
		State: 0,
	}, 4)
	queue.SetMessage("prepare")

	if err = queue.Start(); err != nil {
		return
	}
	defer queue.Stop(err)

	docs, err := e.docsForExport(req.ObjectIds, req.IncludeNested, req.IncludeArchived)
	if err != nil {
		return
	}

	var wr writer
	if req.Zip {
		if wr, err = newZipWriter(req.Path, tempFileName); err != nil {
			return
		}
	} else {
		if wr, err = newDirWriter(req.Path, req.IncludeFiles); err != nil {
			return
		}
	}

	queue.SetMessage("export docs")
	if req.Format == pb.RpcObjectListExport_DOT || req.Format == pb.RpcObjectListExport_SVG {
		var format = dot.ExportFormatDOT
		if req.Format == pb.RpcObjectListExport_SVG {
			format = dot.ExportFormatSVG
		}
		mc := dot.NewMultiConverter(format, e.sbtProvider)
		mc.SetKnownDocs(docs)
		var werr error
		if succeed, werr = e.writeMultiDoc(mc, wr, docs, queue); werr != nil {
			log.Warnf("can't export docs: %v", werr)
		}
	} else if req.Format == pb.RpcObjectListExport_GRAPH_JSON {
		mc := graphjson.NewMultiConverter(e.sbtProvider)
		mc.SetKnownDocs(docs)
		var werr error
		if succeed, werr = e.writeMultiDoc(mc, wr, docs, queue); werr != nil {
			log.Warnf("can't export docs: %v", werr)
		}
	} else {
		if req.Format == pb.RpcObjectListExport_Protobuf {
			if len(req.ObjectIds) == 0 {
				if err = e.createProfileFile(wr); err != nil {
					log.Errorf("failed to create profile file: %s", err.Error())
				}
			}
		}
		for docId := range docs {
			did := docId
			if err = queue.Wait(func() {
				log.With("objectID", did).Debugf("write doc")
				if werr := e.writeDoc(req.Format, wr, docs, queue, did, req.IncludeFiles, req.IsJson); werr != nil {
					log.With("objectID", did).Warnf("can't export doc: %v", werr)
				} else {
					succeed++
				}
			}); err != nil {
				e.cleanupFile(wr)
				succeed = 0
				return
			}
		}
	}
	queue.SetMessage("export files")
	if err = queue.Finalize(); err != nil {
		e.cleanupFile(wr)
		succeed = 0
		return
	}
	wr.Close()
	zipName := getZipName(req.Path)
	err = os.Rename(wr.Path(), zipName)
	if err != nil {
		os.Remove(wr.Path())
		return
	}
	return zipName, succeed, nil
}

func (e *export) docsForExport(reqIds []string, includeNested bool, includeArchived bool) (docs map[string]*types.Struct, err error) {
	if len(reqIds) == 0 {
		return e.getExistedObjects(includeArchived)
	}

	if len(reqIds) > 0 {
		return e.getObjectsByIDs(reqIds, includeNested)
	}
	return
}

func (e *export) getObjectsByIDs(reqIds []string, includeNested bool) (map[string]*types.Struct, error) {
	var res []*model.ObjectInfo
	docs := make(map[string]*types.Struct)
	res, _, err := e.objectStore.QueryObjectInfo(database.Query{
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
	}, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res))
	for _, r := range res {
		docs[r.Id] = r.Details
		ids = append(ids, r.Id)
	}
	if includeNested {
		for _, id := range ids {
			e.getNested(id, docs)
		}
	}
	return docs, err
}

func (e *export) getNested(id string, docs map[string]*types.Struct) {
	links, err := e.objectStore.GetOutboundLinksById(id)
	if err != nil {
		log.Errorf("export failed to get outbound links for id: %s", err.Error())
		return
	}
	for _, link := range links {
		if _, exists := docs[link]; !exists {
			sbt, sbtErr := e.sbtProvider.Type(link)
			if sbtErr != nil {
				log.Errorf("failed to get smartblocktype of id %s", link)
				continue
			}
			if !validType(sbt) {
				continue
			}
			rec, qErr := e.objectStore.QueryById(links)
			if qErr != nil {
				log.Errorf("failed to query id %s, err: %s", qErr, err.Error())
				continue
			}
			if len(rec) > 0 {
				docs[link] = rec[0].Details
				e.getNested(link, docs)
			}
		}
	}
}

func (e *export) getExistedObjects(includeArchived bool) (map[string]*types.Struct, error) {
	res, err := e.objectStore.List()
	if err != nil {
		return nil, err
	}
	objectDetails := make(map[string]*types.Struct, len(res))
	for _, info := range res {
		if !e.objectValid(info.Id, info, includeArchived) {
			continue
		}
		objectDetails[info.Id] = info.Details

	}
	if err != nil {
		return nil, err
	}
	return objectDetails, nil
}

func (e *export) writeMultiDoc(mw converter.MultiConverter, wr writer, docs map[string]*types.Struct, queue process.Queue) (succeed int, err error) {
	for did := range docs {
		if err = queue.Wait(func() {
			log.With("objectID", did).Debugf("write doc")
			werr := e.bs.Do(did, func(b sb.SmartBlock) error {
				return mw.Add(b.NewState().Copy())
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

	if err = wr.WriteFile("export"+mw.Ext(), bytes.NewReader(mw.Convert(0))); err != nil {
		return 0, err
	}

	for _, fh := range mw.FileHashes() {
		fileHash := fh
		if err = queue.Add(func() {
			if werr := e.saveFile(wr, fileHash); werr != nil {
				log.With("hash", fileHash).Warnf("can't save file: %v", werr)
			}
		}); err != nil {
			return
		}
	}
	for _, fh := range mw.ImageHashes() {
		fileHash := fh
		if err = queue.Add(func() {
			if werr := e.saveImage(wr, fileHash); werr != nil {
				log.With("hash", fileHash).Warnf("can't save image: %v", werr)
			}
		}); err != nil {
			return
		}
	}

	err = nil
	return
}

func (e *export) writeDoc(format pb.RpcObjectListExportFormat, wr writer, docInfo map[string]*types.Struct, queue process.Queue, docID string, exportFiles, isJSON bool) (err error) {
	return e.bs.Do(docID, func(b sb.SmartBlock) error {
		if pbtypes.GetBool(b.CombinedDetails(), bundle.RelationKeyIsDeleted.String()) {
			return nil
		}
		var conv converter.Converter
		switch format {
		case pb.RpcObjectListExport_Markdown:
			conv = md.NewMDConverter(e.a, b.NewState(), wr.Namer())
		case pb.RpcObjectListExport_Protobuf:
			conv = pbc.NewConverter(b, isJSON)
		case pb.RpcObjectListExport_JSON:
			conv = pbjson.NewConverter(b)
		}
		conv.SetKnownDocs(docInfo)
		result := conv.Convert(b.Type())
		filename := docID + conv.Ext()
		if format == pb.RpcObjectListExport_Markdown {
			s := b.NewState()
			name := pbtypes.GetString(s.Details(), bundle.RelationKeyName.String())
			if name == "" {
				name = s.Snippet()
			}
			filename = wr.Namer().Get("", docID, name, conv.Ext())
		}
		if docID == e.a.PredefinedBlocks().Home {
			filename = "index" + conv.Ext()
		}
		if err = wr.WriteFile(filename, bytes.NewReader(result)); err != nil {
			return err
		}
		if !exportFiles {
			return nil
		}
		for _, fh := range conv.FileHashes() {
			fileHash := fh
			if err = queue.Add(func() {
				if werr := e.saveFile(wr, fileHash); werr != nil {
					log.With("hash", fileHash).Warnf("can't save file: %v", werr)
				}
			}); err != nil {
				return err
			}
		}
		for _, fh := range conv.ImageHashes() {
			fileHash := fh
			if err = queue.Add(func() {
				if werr := e.saveImage(wr, fileHash); werr != nil {
					log.With("hash", fileHash).Warnf("can't save image: %v", werr)
				}
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (e *export) saveFile(wr writer, hash string) (err error) {
	file, err := e.fileService.FileByHash(context.TODO(), hash)
	if err != nil {
		return
	}
	origName := file.Meta().Name
	filename := wr.Namer().Get("files", hash, filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader(context.TODO())
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd)
}

func (e *export) saveImage(wr writer, hash string) (err error) {
	file, err := e.fileService.ImageByHash(context.TODO(), hash)
	if err != nil {
		return
	}
	orig, err := file.GetOriginalFile(context.TODO())
	if err != nil {
		return
	}
	origName := orig.Meta().Name
	filename := wr.Namer().Get("files", hash, filepath.Base(origName), filepath.Ext(origName))
	rd, err := orig.Reader(context.TODO())
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd)
}

func (e *export) createProfileFile(wr writer) error {
	var spaceDashBoardID string
	pr, err := e.a.LocalProfile()
	if err != nil {
		return err
	}
	err = e.bs.Do(e.a.PredefinedBlocks().Account, func(b sb.SmartBlock) error {
		spaceDashBoardID = pbtypes.GetString(b.CombinedDetails(), bundle.RelationKeySpaceDashboardId.String())
		return nil
	})
	if err != nil {
		return err
	}
	profileID := e.a.ProfileID()
	profile := &pb.Profile{
		SpaceDashboardId: spaceDashBoardID,
		Address:          pr.AccountAddr,
		Name:             pr.Name,
		Avatar:           pr.IconImage,
		ProfileId:        profileID,
	}
	data, err := profile.Marshal()
	if err != nil {
		return err
	}
	err = wr.WriteFile(constant.ProfileFile, bytes.NewReader(data))
	if err != nil {
		return err
	}
	return nil
}

func (e *export) objectValid(id string, r *model.ObjectInfo, includeArchived bool) bool {
	if r.Id == addr.AnytypeProfileId {
		return false
	}
	if !validType(smartblock.SmartBlockType(r.ObjectType)) {
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
		sbType == smartblock.SmartBlockTypeWidget
}

func (e *export) cleanupFile(wr writer) {
	wr.Close()
	os.Remove(wr.Path())
}
