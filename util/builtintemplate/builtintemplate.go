package builtintemplate

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/anyproto/any-sync/app"

	smartblock2 "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const CName = "builtintemplate"

//go:embed data/bundled_templates.zip
var templatesZip []byte

func New() BuiltinTemplate {
	return new(builtinTemplate)
}

type BuiltinTemplate interface {
	Hash() string
	RegisterBuiltinTemplates(space clientspace.Space) error
	app.Component
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

func (b *builtinTemplate) RegisterBuiltinTemplates(space clientspace.Space) error {
	zr, err := zip.NewReader(bytes.NewReader(templatesZip), int64(len(templatesZip)))
	if err != nil {
		return fmt.Errorf("new reader: %w", err)
	}
	for _, zf := range zr.File {
		rd, e := zf.Open()
		if e != nil {
			return e
		}
		if err = b.registerBuiltin(space, rd); err != nil {
			return fmt.Errorf("register builtin: %w", err)
		}
	}
	return nil
}

func (b *builtinTemplate) Hash() string {
	return b.generatedHash
}

func (b *builtinTemplate) registerBuiltin(space clientspace.Space, rd io.ReadCloser) (err error) {
	defer rd.Close()
	data, err := io.ReadAll(rd)
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		return
	}
	var id string
	for _, block := range snapshot.Data.Blocks {
		if block.GetSmartblock() != nil {
			id = block.Id
			break
		}
	}

	st, err := state.NewDocFromSnapshot(id, snapshot)
	if err != nil {
		return fmt.Errorf("new state from snapshot: %w", err)
	}
	st.SetRootId(id)
	st.SetLocalDetail(bundle.RelationKeyTemplateIsBundled, domain.Bool(true))
	st.RemoveDetail(bundle.RelationKeyCreator, bundle.RelationKeyLastModifiedBy)
	st.SetLocalDetail(bundle.RelationKeyCreator, domain.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy, domain.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeySpaceId, domain.String(addr.AnytypeMarketplaceWorkspace))
	st.SetDetail(bundle.RelationKeyOrigin, domain.Int64(model.ObjectOrigin_builtin))

	err = b.setObjectTypes(st, "TODO")
	if err != nil {
		return fmt.Errorf("set object types: %w", err)
	}

	// fix divergence between extra relations and simple block relations
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := b.(relation.Block); ok {
			relKey := b.Model().GetRelation().Key
			if !st.HasRelation(relKey) {
				st.AddBundledRelationLinks(domain.RelationKey(relKey))
			}
		}
		return true
	})

	if err = b.validate(st); err != nil {
		return
	}

	fullID := domain.FullID{SpaceID: space.Id(), ObjectID: id}
	params := source.StaticSourceParams{
		Id:        fullID,
		SbType:    smartblock.SmartBlockTypeBundledTemplate,
		State:     st.Copy(),
		CreatorId: addr.AnytypeProfileId,
	}
	err = b.source.RegisterStaticSource(b.source.NewStaticSource(params))
	if err != nil {
		return fmt.Errorf("register static source: %w", err)
	}
	// Index
	return space.Do(id, func(sb smartblock2.SmartBlock) error {
		return sb.Apply(sb.NewState())
	})
}

func (b *builtinTemplate) setObjectTypes(st *state.State, spaceId string) error {
	targetObjectTypeID := st.Details().GetString(bundle.RelationKeyTargetObjectType)
	var targetObjectTypeKey domain.TypeKey
	if strings.HasPrefix(targetObjectTypeID, addr.BundledObjectTypeURLPrefix) {
		// todo: remove this hack after fixing bundled templates
		targetObjectTypeKey = domain.TypeKey(strings.TrimPrefix(targetObjectTypeID, addr.BundledObjectTypeURLPrefix))
	} else {
		targetObjectType, err := b.objectStore.SpaceIndex(spaceId).GetObjectType(targetObjectTypeID)
		if err != nil {
			return fmt.Errorf("get object type %s: %w", targetObjectTypeID, err)
		}
		targetObjectTypeKey = domain.TypeKey(targetObjectType.Key)
	}
	st.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, targetObjectTypeKey})
	return nil
}

func (b *builtinTemplate) validate(st *state.State) (err error) {
	cd := st.CombinedDetails()
	if st.ObjectTypeKey() != bundle.TypeKeyTemplate {
		return fmt.Errorf("bundled template validation: %s unexpected object type: %v", st.RootId(), st.ObjectTypeKey())
	}
	if !cd.GetBool(bundle.RelationKeyTemplateIsBundled) {
		return fmt.Errorf("bundled template validation: %s not bundled", st.RootId())
	}
	targetObjectTypeID := cd.GetString(bundle.RelationKeyTargetObjectType)
	if targetObjectTypeID == "" || domain.TypeKey(targetObjectTypeID) == st.ObjectTypeKey() {
		return fmt.Errorf("bundled template validation: %s unexpected target object type: %v", st.RootId(), targetObjectTypeID)
	}
	// todo: update templates and return the validation
	return nil
}
