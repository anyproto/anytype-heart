package builtintemplate

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"io"
	"io/ioutil"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const CName = "builtintemplate"

//go:embed data/bundled_templates.zip
var templatesZip []byte

var log = logging.Logger("anytype-mw-builtintemplate")

func New() BuiltinTemplate {
	return new(builtinTemplate)
}

type BuiltinTemplate interface {
	Hash() string
	app.ComponentRunnable
}

type builtinTemplate struct {
	core          core.Service
	blockService  block.Service
	source        source.Service
	generatedHash string
}

func (b *builtinTemplate) Init(a *app.App) (err error) {
	b.blockService = a.MustComponent(block.CName).(block.Service)
	b.core = a.MustComponent(core.CName).(core.Service)
	b.source = a.MustComponent(source.CName).(source.Service)
	b.makeGenHash(0)
	return
}

func (b *builtinTemplate) makeGenHash(version uint32) {
	h := md5.New()
	h.Write(templatesZip)
	binary.Write(h, binary.LittleEndian, version)
	b.generatedHash = hex.EncodeToString(h.Sum(nil))
}

func (b *builtinTemplate) Name() (name string) {
	return CName
}

func (b *builtinTemplate) Run() (err error) {
	zr, err := zip.NewReader(bytes.NewReader(templatesZip), int64(len(templatesZip)))
	if err != nil {
		return
	}
	for _, zf := range zr.File {
		rd, e := zf.Open()
		if e != nil {
			return e
		}
		if err = b.registerBuiltin(rd); err != nil {
			return
		}
	}
	return
}

func (b *builtinTemplate) Hash() string {
	return b.generatedHash
}

func (b *builtinTemplate) registerBuiltin(rd io.ReadCloser) (err error) {
	defer rd.Close()
	data, err := ioutil.ReadAll(rd)
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		return
	}
	st := state.NewDocFromSnapshot("", snapshot).(*state.State)
	id, err := threads.PatchSmartBlockType(st.RootId(), smartblock.SmartBlockTypeBundledTemplate)
	if err != nil {
		return
	}
	st.SetRootId(id)
	st = st.Copy()
	if ot := st.ObjectType(); ot != bundle.TypeKeyTemplate.URL() {
		st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(ot))
	}
	st.SetObjectType(bundle.TypeKeyTemplate.URL())
	b.source.RegisterStaticSource(id, func() source.Source {
		return b.source.NewStaticSource(id, model.SmartBlockType_BundledTemplate, st.Copy())
	})
	return
}

func (b *builtinTemplate) Close() (err error) {
	return
}
