package export

import (
	"bytes"
	"context"
	"github.com/anytypeio/go-anytype-middleware/core/converter/dot"
	"github.com/anytypeio/go-anytype-middleware/core/converter/graphjson"
	"math/rand"
	"path/filepath"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
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
	"github.com/globalsign/mgo/bson"
)

const CName = "export"

var log = logging.Logger("anytype-mw-export")

func New() Export {
	return new(export)
}

type Export interface {
	Export(req pb.RpcExportRequest) (path string, succeed int, err error)
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

func (e *export) Export(req pb.RpcExportRequest) (path string, succeed int, err error) {
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

	docIds, err := e.idsForExport(req.DocIds, req.IncludeNested)
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
	if req.Format == pb.RpcExport_DOT || req.Format == pb.RpcExport_SVG {
		var format = dot.ExportFormatDOT
		if req.Format == pb.RpcExport_SVG {
			format = dot.ExportFormatSVG
		}
		mc := dot.NewMultiConverter(format)
		mc.SetKnownLinks(docIds)
		var werr error
		if succeed, werr = e.writeMultiDoc(mc, wr, docIds, queue); werr != nil {
			log.Warnf("can't export docs: %v", werr)
		}
	} else if req.Format == pb.RpcExport_GRAPH_JSON {
		mc := graphjson.NewMultiConverter()
		mc.SetKnownLinks(docIds)
		var werr error
		if succeed, werr = e.writeMultiDoc(mc, wr, docIds, queue); werr != nil {
			log.Warnf("can't export docs: %v", werr)
		}
	} else {
		for _, docId := range docIds {
			did := docId
			if err = queue.Wait(func() {
				log.With("threadId", did).Debugf("write doc")
				if werr := e.writeDoc(req.Format, wr, docIds, queue, did); werr != nil {
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

func (e *export) idsForExport(reqIds []string, includeNested bool) (ids []string, err error) {
	if len(reqIds) == 0 {
		var res []*model.ObjectInfo
		res, _, err = e.a.ObjectStore().QueryObjectInfo(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIsArchived.String(),
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
			ids = append(ids, r.Id)
		}
		return ids, nil
	}

	var m map[string]struct{}
	if includeNested {
		m = make(map[string]struct{}, len(reqIds)*10)
	} else {
		m = make(map[string]struct{}, len(reqIds))
	}
	var getNested func(id string)
	getNested = func(id string) {
		links, err := e.a.ObjectStore().GetOutboundLinksById(id)
		if err != nil {
			log.Errorf("export failed to get outbound links for id: %s", err.Error())
			return
		}
		for _, link := range links {
			if _, exists := m[link]; !exists {
				sbt, err2 := smartblock.SmartBlockTypeFromID(link)
				if err2 != nil {
					log.Errorf("failed to get smartblocktype of id %s", link)
					continue
				}
				if sbt != smartblock.SmartBlockTypePage && sbt != smartblock.SmartBlockTypeSet {
					continue
				}
				ids = append(ids, link)
				m[link] = struct{}{}
				getNested(link)
			}
		}
	}

	for _, id := range reqIds {
		if _, exists := m[id]; !exists {
			ids = append(ids, id)
			m[id] = struct{}{}
			if includeNested {
				getNested(id)
			}
		}
	}

	return
}

func (e *export) writeMultiDoc(mw converter.MultiConverter, wr writer, docIds []string, queue process.Queue) (succeed int, err error) {
	for _, did := range docIds {
		if err = queue.Wait(func() {
			log.With("threadId", did).Debugf("write doc")
			werr := e.bs.Do(did, func(b sb.SmartBlock) error {
				return mw.Add(b.NewState())
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

func (e *export) writeDoc(format pb.RpcExportFormat, wr writer, docIds []string, queue process.Queue, docId string) (err error) {

	return e.bs.Do(docId, func(b sb.SmartBlock) error {
		var conv converter.Converter
		switch format {
		case pb.RpcExport_Markdown:
			conv = md.NewMDConverter(e.a, b.NewState(), wr.Namer())
		case pb.RpcExport_Protobuf:
			conv = pbc.NewConverter(b.NewState())
		case pb.RpcExport_JSON:
			conv = pbjson.NewConverter(b.NewState())
		}
		conv.SetKnownLinks(docIds)
		result := conv.Convert()
		filename := docId + conv.Ext()
		if docId == e.a.PredefinedBlocks().Home {
			filename = "index" + conv.Ext()
		}
		if err = wr.WriteFile(filename, bytes.NewReader(result)); err != nil {
			return err
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
	filename := filepath.Join("files", wr.Namer().Get(hash, file.Meta().Name))
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
	filename := filepath.Join("files", wr.Namer().Get(hash, orig.Meta().Name))
	rd, err := orig.Reader()
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd)
}

func newNamer() *fileNamer {
	return &fileNamer{
		names: make(map[string]string),
	}
}

type fileNamer struct {
	// id -> name and name -> id
	names map[string]string
	mu    sync.Mutex
}

func (fn *fileNamer) Get(hash, title string) (name string) {
	const fileLenLimit = 30
	fn.mu.Lock()
	defer fn.mu.Unlock()
	var ok bool
	if name, ok = fn.names[hash]; ok {
		return name
	}
	if l := utf8.RuneCountInString(title); l > fileLenLimit {
		buf := bytes.NewBuffer(nil)
		for i := l - fileLenLimit; i < l; i++ {
			buf.WriteRune([]rune(title)[i])
		}
		name = buf.String()
	} else {
		name = title
	}
	var (
		i = 0
		b = 36
	)
	gname := name
	for {
		if _, ok = fn.names[gname]; !ok {
			fn.names[hash] = gname
			fn.names[gname] = hash
			return gname
		}
		i++
		n := int64(i * b)
		gname = strconv.FormatInt(rand.Int63n(n), b) + "_" + name
	}
}
