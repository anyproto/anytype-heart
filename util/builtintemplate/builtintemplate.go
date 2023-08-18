package builtintemplate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	objectStore   objectstore.ObjectStore
	generatedHash string
}

func (b *builtinTemplate) Init(a *app.App) (err error) {
	b.source = app.MustComponent[source.Service](a)
	b.objectStore = app.MustComponent[objectstore.ObjectStore](a)

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
	var id string
	for _, block := range snapshot.Data.Blocks {
		if block.GetSmartblock() != nil {
			id = block.Id
			break
		}
	}

	id = addr.BundledTemplatesURLPrefix + id
	st := state.NewDocFromSnapshot(id, snapshot).(*state.State)
	st.SetRootId(id)
	st = st.NewState()
	st = st.Copy()
	st.SetLocalDetail(bundle.RelationKeyTemplateIsBundled.String(), pbtypes.Bool(true))
	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String())
	st.SetLocalDetail(bundle.RelationKeyCreator.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyWorkspaceId.String(), pbtypes.String(addr.AnytypeMarketplaceWorkspace))
	st.SetLocalDetail(bundle.RelationKeySpaceId.String(), pbtypes.String(addr.AnytypeMarketplaceWorkspace))

	err = b.setObjectTypes(st)
	if err != nil {
		return fmt.Errorf("set object types: %w", err)
	}

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
	return
}

func (b *builtinTemplate) setObjectTypes(st *state.State) error {
	targetObjectTypeID := pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String())
	var targetObjectTypeKey bundle.TypeKey
	if strings.HasPrefix(targetObjectTypeID, addr.BundledObjectTypeURLPrefix) {
		// todo: remove this hack after fixing bundled templates
		targetObjectTypeKey = bundle.TypeKey(strings.TrimPrefix(targetObjectTypeID, addr.BundledObjectTypeURLPrefix))
	} else {
		targetObjectType, err := b.objectStore.GetObjectType(targetObjectTypeID)
		if err != nil {
			return fmt.Errorf("get object type %s: %w", targetObjectTypeID, err)
		}
		targetObjectTypeKey = bundle.TypeKey(targetObjectType.Key)
	}
	st.SetObjectTypeKeys([]bundle.TypeKey{bundle.TypeKeyTemplate, targetObjectTypeKey})
	return nil
}

func (b *builtinTemplate) validate(st *state.State) (err error) {
	cd := st.CombinedDetails()
	if st.ObjectTypeKey() != bundle.TypeKeyTemplate {
		return fmt.Errorf("bundled template validation: %s unexpected object type: %v", st.RootId(), st.ObjectTypeKey())
	}
	if !pbtypes.GetBool(cd, bundle.RelationKeyTemplateIsBundled.String()) {
		return fmt.Errorf("bundled template validation: %s not bundled", st.RootId())
	}
	targetObjectTypeID := pbtypes.GetString(cd, bundle.RelationKeyTargetObjectType.String())
	if targetObjectTypeID == "" || bundle.TypeKey(targetObjectTypeID) == st.ObjectTypeKey() {
		return fmt.Errorf("bundled template validation: %s unexpected target object type: %v", st.RootId(), targetObjectTypeID)
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
