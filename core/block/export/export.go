package export

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/converter/md"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/globalsign/mgo/bson"
)

var log = logging.Logger("anytype-mw-export")

func NewExport(a anytype.Service, bs block.Service) Export {
	return &export{
		bs: bs,
		a:  a,
	}
}

type Export interface {
	Export(req pb.RpcExportRequest) (path string, err error)
}

type export struct {
	bs block.Service
	a  anytype.Service
}

func (e *export) Export(req pb.RpcExportRequest) (path string, err error) {
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

	docIds, err := e.idsForExport(req.DocIds)
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
	for _, docId := range docIds {
		did := docId
		if err = queue.Wait(func() {
			log.With("threadId", did).Debugf("write doc")
			if werr := e.writeDoc(wr, queue, did); werr != nil {
				log.With("threadId", did).Warnf("can't export doc: %v", werr)
			}
		}); err != nil {
			return
		}
	}

	queue.SetMessage("export files")
	if err = queue.Finalize(); err != nil {
		return
	}
	return wr.Path(), nil
}

func (e *export) idsForExport(reqIds []string) (ids []string, err error) {
	if len(reqIds) > 0 {
		return reqIds, nil
	}
	res, _, err := e.a.ObjectStore().QueryObjectInfo(database.Query{}, []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeHome,
		smartblock.SmartBlockTypePage,
	})
	if err != nil {
		return
	}
	for _, r := range res {
		ids = append(ids, r.Id)
	}
	return
}

func (e *export) writeDoc(wr writer, queue process.Queue, docId string) (err error) {
	return e.bs.Do(docId, func(b sb.SmartBlock) error {
		conv := md.NewMDConverter(e.a, b.NewState())
		result := conv.Convert()
		filename := docId + ".md"
		if docId == e.a.PredefinedBlocks().Home {
			filename = "index.md"
		}
		if err = wr.WriteFile(filename, strings.NewReader(result)); err != nil {
			return err
		}
		for _, fh := range conv.FileHashes() {
			fileHash := fh
			queue.Add(func() {
				if werr := e.saveFile(wr, fileHash); werr != nil {
					log.With("hash", fileHash).Warnf("can't save file: %v", werr)
				}
			})
		}
		for _, fh := range conv.ImageHashes() {
			fileHash := fh
			queue.Add(func() {
				if werr := e.saveImage(wr, fileHash); werr != nil {
					log.With("hash", fileHash).Warnf("can't save image: %v", werr)
				}
			})
		}
		return nil
	})
}

func (e *export) saveFile(wr writer, hash string) (err error) {
	file, err := e.a.FileByHash(context.TODO(), hash)
	if err != nil {
		return
	}
	filename := filepath.Join("files", hash+"_"+file.Meta().Name)
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
	orig, err := file.GetFileForLargestWidth(context.TODO())
	if err != nil {
		return
	}
	filename := filepath.Join("files", hash+"_"+orig.Meta().Name)
	rd, err := orig.Reader()
	if err != nil {
		return
	}
	return wr.WriteFile(filename, rd)
}
