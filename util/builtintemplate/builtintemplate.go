package builtintemplate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	_ "embed"
)

const CName = "builtintemplate"

//go:embed data/bundled_templates.zip
var templatesZip []byte

func New() BuiltinTemplate {
	return new(builtinTemplate)
}

type BuiltinTemplate interface {
	Hash() string
	app.ComponentRunnable
}

type builtinTemplate struct {
	source        source.Service
	sbtProvider   typeprovider.SmartBlockTypeProvider
	generatedHash string
}

func (b *builtinTemplate) Init(a *app.App) (err error) {
	b.source = a.MustComponent(source.CName).(source.Service)
	b.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	b.makeGenHash(4)
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

func (b *builtinTemplate) Run(context.Context) (err error) {
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
		snapshotWithType := &pb.SnapshotWithType{}
		if err = snapshotWithType.Unmarshal(data); err != nil {
			return
		}
		snapshot = snapshotWithType.Snapshot
	}

	st := state.NewDocFromSnapshot("", snapshot, state.DoNotMigrateTypes).(*state.State)
	st = st.NewState()
	id := st.RootId()
	st = st.Copy()
	st.SetLocalDetail(bundle.RelationKeyTemplateIsBundled.String(), pbtypes.Bool(true))
	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String())
	st.SetLocalDetail(bundle.RelationKeyCreator.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyWorkspaceId.String(), pbtypes.String(addr.AnytypeMarketplaceWorkspace))
	st.SetLocalDetail(bundle.RelationKeySpaceId.String(), pbtypes.String(addr.AnytypeMarketplaceWorkspace))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.BundledURL(), pbtypes.Get(st.Details(), bundle.RelationKeyTargetObjectType.String()).GetStringValue()})

	st.InjectDerivedDetails()

	// fix divergence between extra relations and simple block relations
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := b.(relation.Block); ok {
			relKey := b.Model().GetRelation().Key
			if !st.HasRelation(relKey) {
				st.AddBundledRelations(bundle.RelationKey(relKey))
			}
		}
		return true
	})

	if err = b.validate(st.Copy()); err != nil {
		return
	}

	b.source.RegisterStaticSource(id, b.source.NewStaticSource(id, model.SmartBlockType_BundledTemplate, st.Copy(), nil))
	b.sbtProvider.RegisterStaticType(id, smartblock.SmartBlockTypeBundledTemplate)
	return
}

func (b *builtinTemplate) validate(st *state.State) (err error) {
	cd := st.CombinedDetails()
	if st.ObjectType() != bundle.TypeKeyTemplate.BundledURL() {
		return fmt.Errorf("bundled template validation: %s unexpected object type: %v", st.RootId(), st.ObjectType())
	}
	if !pbtypes.GetBool(cd, bundle.RelationKeyTemplateIsBundled.String()) {
		return fmt.Errorf("bundled template validation: %s not bundled", st.RootId())
	}
	if tt := pbtypes.GetString(cd, bundle.RelationKeyTargetObjectType.String()); tt == "" || tt == st.ObjectType() {
		return fmt.Errorf("bundled template validation: %s unexpected target object type: %v", st.RootId(), tt)
	}
	// todo: update templates and return the validation
	return nil
	var relKeys []string
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if rb, ok := b.(relation.Block); ok {
			relKeys = append(relKeys, rb.Model().GetRelation().Key)
		}
		return true
	})
	for _, rk := range relKeys {
		if !st.HasRelation(rk) {
			return fmt.Errorf("bundled template validation: relation '%v' exists in block but not in extra relations", rk)
		}
	}
	return nil
}

func (b *builtinTemplate) Close(ctx context.Context) (err error) {
	return
}
