package export

import (
	"bytes"
	"context"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/core/converter/dot"
	"github.com/anytypeio/go-anytype-middleware/core/converter/graphjson"
	"github.com/anytypeio/go-anytype-middleware/core/converter/md"
	"github.com/anytypeio/go-anytype-middleware/core/converter/pbc"
	"github.com/anytypeio/go-anytype-middleware/core/converter/pbjson"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/text"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/gosimple/slug"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const CName = "export"

var log = logging.Logger("anytype-mw-export")

func New() Export {
	return new(export)
}

type Export interface {
	Export(req pb.RpcObjectListExportRequest) (path string, succeed int, err error)
	app.Component
}

type export struct {
	bs block.Service
	a  core.Service
}

func (e *export) Init(a *app.App) (err error) {
	e.bs = a.MustComponent(block.CName).(block.Service)
	e.a = a.MustComponent(core.CName).(core.Service)
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

	docs, err := e.docsForExport(req.ObjectIds, req.IncludeNested)
	if err != nil {
		return
	}

	var wr writer
	if req.Zip {
		if wr, err = newZipWriter(req.Path); err != nil {
			return
		}
	} else {
		if wr, err = newDirWriter(req.Path); err != nil {
			return
		}
	}

	defer wr.Close()

	queue.SetMessage("export docs")
	if req.Format == pb.RpcObjectListExport_DOT || req.Format == pb.RpcObjectListExport_SVG {
		var format = dot.ExportFormatDOT
		if req.Format == pb.RpcObjectListExport_SVG {
			format = dot.ExportFormatSVG
		}
		mc := dot.NewMultiConverter(format)
		mc.SetKnownDocs(docs)
		var werr error
		if succeed, werr = e.writeMultiDoc(mc, wr, docs, queue); werr != nil {
			log.Warnf("can't export docs: %v", werr)
		}
	} else if req.Format == pb.RpcObjectListExport_GRAPH_JSON {
		mc := graphjson.NewMultiConverter()
		mc.SetKnownDocs(docs)
		var werr error
		if succeed, werr = e.writeMultiDoc(mc, wr, docs, queue); werr != nil {
			log.Warnf("can't export docs: %v", werr)
		}
	} else {
		for docId := range docs {
			did := docId
			if err = queue.Wait(func() {
				log.With("threadId", did).Debugf("write doc")
				if werr := e.writeDoc(req.Format, wr, docs, queue, did, req.IncludeFiles); werr != nil {
					log.With("threadId", did).Warnf("can't export doc: %v", werr)
				} else {
					succeed++
				}
			}); err != nil {
				succeed = 0
				return
			}
		}
	}
	queue.SetMessage("export files")
	if err = queue.Finalize(); err != nil {
		succeed = 0
		return
	}
	return wr.Path(), succeed, nil
}

func (e *export) docsForExport(reqIds []string, includeNested bool) (docs map[string]*types.Struct, err error) {
	docs = make(map[string]*types.Struct)
	if len(reqIds) == 0 {
		var res []*model.ObjectInfo
		res, _, err = e.a.ObjectStore().QueryObjectInfo(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
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
		}, []smartblock.SmartBlockType{
			smartblock.SmartBlockTypeHome,
			smartblock.SmartBlockTypeProfilePage,
			smartblock.SmartBlockTypePage,
		})
		if err != nil {
			return
		}

		for _, r := range res {
			docs[r.Id] = r.Details
		}
		return docs, nil
	}

	var getNested func(id string)
	getNested = func(id string) {
		links, err := e.a.ObjectStore().GetOutboundLinksById(id)
		if err != nil {
			log.Errorf("export failed to get outbound links for id: %s", err.Error())
			return
		}
		for _, link := range links {
			if _, exists := docs[link]; !exists {
				sbt, err2 := smartblock.SmartBlockTypeFromID(link)
				if err2 != nil {
					log.Errorf("failed to get smartblocktype of id %s", link)
					continue
				}
				if sbt != smartblock.SmartBlockTypePage && sbt != smartblock.SmartBlockTypeSet {
					continue
				}
				rec, _ := e.a.ObjectStore().QueryById(links)
				if len(rec) > 0 {
					docs[link] = rec[0].Details
					getNested(link)
				}
			}
		}
	}
	if len(reqIds) > 0 {
		var res []*model.ObjectInfo
		res, _, err = e.a.ObjectStore().QueryObjectInfo(database.Query{
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
			return
		}
		var ids []string
		for _, r := range res {
			docs[r.Id] = r.Details
			ids = append(ids, r.Id)
		}
		if includeNested {
			for _, id := range ids {
				getNested(id)
			}
		}
	}
	return
}

func (e *export) writeMultiDoc(mw converter.MultiConverter, wr writer, docs map[string]*types.Struct, queue process.Queue) (succeed int, err error) {
	for did := range docs {
		if err = queue.Wait(func() {
			log.With("threadId", did).Debugf("write doc")
			werr := e.bs.Do(did, func(b sb.SmartBlock) error {
				return mw.Add(b.NewState().Copy())
			})
			if err != nil {
				log.With("threadId", did).Warnf("can't export doc: %v", werr)
			} else {
				succeed++
			}

		}); err != nil {
			return
		}
	}

	if err = wr.WriteFile("export"+mw.Ext(), bytes.NewReader(mw.Convert())); err != nil {
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

func (e *export) writeDoc(format pb.RpcObjectListExportFormat, wr writer, docInfo map[string]*types.Struct, queue process.Queue, docId string, exportFiles bool) (err error) {
	return e.bs.Do(docId, func(b sb.SmartBlock) error {
		if pbtypes.GetBool(b.CombinedDetails(), bundle.RelationKeyIsArchived.String()) {
			return nil
		}
		if pbtypes.GetBool(b.CombinedDetails(), bundle.RelationKeyIsDeleted.String()) {
			return nil
		}
		var conv converter.Converter
		switch format {
		case pb.RpcObjectListExport_Markdown:
			conv = md.NewMDConverter(e.a, b.NewState(), wr.Namer())
		case pb.RpcObjectListExport_Protobuf:
			conv = pbc.NewConverter(b)
		case pb.RpcObjectListExport_JSON:
			conv = pbjson.NewConverter(b)
		}
		conv.SetKnownDocs(docInfo)
		result := conv.Convert()
		filename := docId + conv.Ext()
		if format == pb.RpcObjectListExport_Markdown {
			s := b.NewState()
			name := pbtypes.GetString(s.Details(), bundle.RelationKeyName.String())
			if name == "" {
				name = s.Snippet()
			}
			filename = wr.Namer().Get("", docId, name, conv.Ext())
		}
		if docId == e.a.PredefinedBlocks().Home {
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
	file, err := e.a.FileByHash(context.TODO(), hash)
	if err != nil {
		return
	}
	origName := file.Meta().Name
	filename := wr.Namer().Get("files", hash, filepath.Base(origName), filepath.Ext(origName))
	rd, err := file.Reader()
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd)
}

func (e *export) saveImage(wr writer, hash string) (err error) {
	file, err := e.a.ImageByHash(context.TODO(), hash)
	if err != nil {
		return
	}
	orig, err := file.GetOriginalFile(context.TODO())
	if err != nil {
		return
	}
	origName := orig.Meta().Name
	filename := wr.Namer().Get("files", hash, filepath.Base(origName), filepath.Ext(origName))
	rd, err := orig.Reader()
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd)
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
