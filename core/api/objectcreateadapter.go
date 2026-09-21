package api

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// objectCreateAdapter implements apicore.ObjectCreator over the standard
// object-creation service: snapshot → state.NewDocFromSnapshot →
// CreateSmartBlockFromState — the API v2 create path (APIV2.md §2 Phase 2).
// The whole document lands as the object's initial state, so composite
// creates (a set with its dataview) are one change set (§8/R10), and the
// creation routes through the editor machinery (restrictions, derived
// details, undo) exactly like user-initiated creates.
type objectCreateAdapter struct {
	creator   objectcreator.Service
	spaces    space.Service
	templates template.Service
	store     objectstore.ObjectStore
}

func newObjectCreateAdapter(creator objectcreator.Service, spaces space.Service, templates template.Service, store objectstore.ObjectStore) apicore.ObjectCreator {
	return &objectCreateAdapter{creator: creator, spaces: spaces, templates: templates, store: store}
}

func (a *objectCreateAdapter) CreateObjectFromSnapshot(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase, templateId string) (string, error) {
	rootId := snapshotRootId(snapshot)
	if rootId == "" {
		return "", fmt.Errorf("snapshot has no root block")
	}
	createState, err := state.NewDocFromSnapshot(rootId, &pb.ChangeSnapshot{Data: snapshot})
	if err != nil {
		return "", fmt.Errorf("state from snapshot: %w", err)
	}

	typeKeys := createState.ObjectTypeKeys()
	if len(typeKeys) == 0 {
		typeKeys = []domain.TypeKey{bundle.TypeKeyPage}
	}

	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space %s: %w", spaceId, err)
	}
	// A document may reference bundled types/relations not yet present in the
	// space (a fresh space has only a few installed); install them so the
	// created object's type and relations resolve (mirrors the import path).
	if ids := bundledIdsToInstall(createState.AllRelationKeys(), typeKeys); len(ids) > 0 {
		if _, _, err := a.creator.InstallBundledObjects(ctx, spc, ids); err != nil {
			return "", fmt.Errorf("install bundled objects: %w", err)
		}
	}

	if templateId != "" {
		if createState, err = a.applyTemplate(ctx, spc, typeKeys, createState, templateId); err != nil {
			return "", err
		}
	}
	// after the template merge, so the detail lands on whichever state is
	// created — and so it is never offered to the template service as one of
	// the document's own details (a template state strips origin on purpose)
	createState.SetDetail(bundle.RelationKeyOrigin, domain.Int64(int64(model.ObjectOrigin_api)))

	id, _, err := a.creator.CreateSmartBlockFromStateInSpace(ctx, spc, typeKeys, createState)
	if err != nil {
		return "", fmt.Errorf("create object from state: %w", err)
	}
	return id, nil
}

// applyTemplate rebases the document on the state the template produces.
//
// The base is built by the SAME call a client-initiated create makes
// (objectcreator.createCommonObject), so a template reaches the API exactly
// as it reaches the app: its blocks, its details merged with the caller's
// under the template's own precedence rules, its placeholders resolved and
// its layout converted. What the API adds on top is the document the caller
// sent — its blocks after the template's, its relation links and its
// collection items — because unlike a client create, this one carries
// content of its own.
//
// Validation is off: the id was checked against this type's live templates
// before anything was written (v2service.resolveCreateTemplate), and
// WithTemplateValidation would re-resolve it to the BLANK template on any
// miss — turning a vanished template into a silent empty object after the
// response has already told the caller which template was applied.
func (a *objectCreateAdapter) applyTemplate(
	ctx context.Context, spc clientspace.Space, typeKeys []domain.TypeKey, doc *state.State, templateId string,
) (*state.State, error) {
	typeId, err := spc.GetTypeIdByKey(ctx, typeKeys[0])
	if err != nil {
		return nil, fmt.Errorf("derive type id for %s: %w", typeKeys[0], err)
	}
	layout := model.ObjectType_basic
	if objectType, err := a.store.SpaceIndex(spc.Id()).GetObjectType(typeId); err == nil {
		layout = objectType.Layout
	} else {
		log.With("typeId", typeId).Warnf("template create: read recommended layout: %v", err)
	}
	base, err := a.templates.CreateTemplateStateWithDetails(template.CreateTemplateRequest{
		SpaceId:    spc.Id(),
		TemplateId: templateId,
		TypeId:     typeId,
		Layout:     layout,
		Details:    doc.Details(),
	})
	if err != nil {
		return nil, fmt.Errorf("build state from template %s: %w", templateId, err)
	}
	base.SetObjectTypeKeys(typeKeys)
	if key := doc.UniqueKeyInternal(); key != "" {
		base.SetUniqueKeyInternal(key)
	}
	mergeDocumentIntoTemplate(base, doc)
	return base, nil
}

// mergeDocumentIntoTemplate copies what the caller's document holds and the
// template state does not: its blocks, appended after the template's own, its
// relation links (a value whose property has no link is written as a relation
// REMOVAL), and its collection items. The document's details are not copied —
// they were handed to the template service, which owns the precedence between
// the two (a template's cover and tags win, its name fills an unnamed object).
//
// A document block whose id the template state already holds is REMINTED,
// references included: authored ids are the caller's to choose (`title` and
// `header` are ordinary words), and a collision would otherwise overwrite a
// template block — the one way this merge could silently lose template
// content.
func mergeDocumentIntoTemplate(base, doc *state.State) {
	root := doc.Pick(doc.RootId())
	if root == nil {
		return
	}
	var (
		ordered []simple.Block
		renamed = map[string]string{}
	)
	// the walk is from the root, so a block the document never parented is
	// dropped the same way the create path drops it without a template
	doc.Iterate(func(b simple.Block) bool {
		if b.Model().Id == doc.RootId() {
			return true
		}
		ordered = append(ordered, b.Copy())
		if base.Exists(b.Model().Id) {
			renamed[b.Model().Id] = bson.NewObjectId().Hex()
		}
		return true
	})
	rename := func(id string) string {
		if minted, ok := renamed[id]; ok {
			return minted
		}
		return id
	}
	for _, b := range ordered {
		block := b.Model()
		block.Id = rename(block.Id)
		for i, child := range block.ChildrenIds {
			block.ChildrenIds[i] = rename(child)
		}
		base.Add(b)
	}
	baseRoot := base.Get(base.RootId())
	if baseRoot == nil {
		return
	}
	for _, child := range root.Model().ChildrenIds {
		baseRoot.Model().ChildrenIds = append(baseRoot.Model().ChildrenIds, rename(child))
	}
	base.AddRelationLinks(doc.PickRelationLinks()...)
	if store := doc.Store(); store != nil {
		for key, value := range store.Fields {
			base.SetInStore([]string{key}, value)
		}
	}
}

func (a *objectCreateAdapter) TypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (string, error) {
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space %s: %w", spaceId, err)
	}
	id, err := spc.GetTypeIdByKey(ctx, key)
	if err != nil {
		return "", fmt.Errorf("derive type id for %s: %w", key, err)
	}
	return id, nil
}

func (a *objectCreateAdapter) RelationIdByKey(ctx context.Context, spaceId string, key domain.RelationKey) (string, error) {
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space %s: %w", spaceId, err)
	}
	id, err := spc.GetRelationIdByKey(ctx, key)
	if err != nil {
		return "", fmt.Errorf("derive relation id for %s: %w", key, err)
	}
	return id, nil
}

// snapshotRootId finds the snapshot's root block: the block carrying the
// smartblock content (anyblockjson.Unmarshal always emits exactly one).
func snapshotRootId(snapshot *model.SmartBlockSnapshotBase) string {
	if snapshot == nil {
		return ""
	}
	for _, b := range snapshot.Blocks {
		if b.GetSmartblock() != nil {
			return b.Id
		}
	}
	return ""
}

// bundledIdsToInstall lists the bundled-object source ids for every bundled
// relation key and type key referenced by the state (the same set the import
// path installs; InstallBundledObjects skips already-installed ids).
func bundledIdsToInstall(relationKeys []domain.RelationKey, typeKeys []domain.TypeKey) []string {
	ids := make([]string, 0, len(relationKeys)+len(typeKeys))
	for _, key := range relationKeys {
		if bundle.HasRelation(key) {
			ids = append(ids, key.BundledURL())
		}
	}
	for _, key := range typeKeys {
		if bundle.HasObjectTypeByKey(key) {
			ids = append(ids, addr.BundledObjectTypeURLPrefix+string(key))
		}
	}
	return ids
}
