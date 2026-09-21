package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	editortemplate "github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/table"
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

func (a *objectCreateAdapter) CreateObjectFromSnapshot(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase, templateId string) (apicore.CreateOutcome, error) {
	var outcome apicore.CreateOutcome
	rootId := snapshotRootId(snapshot)
	if rootId == "" {
		return outcome, fmt.Errorf("snapshot has no root block")
	}
	createState, err := state.NewDocFromSnapshot(rootId, &pb.ChangeSnapshot{Data: snapshot})
	if err != nil {
		return outcome, fmt.Errorf("state from snapshot: %w", err)
	}

	typeKeys := createState.ObjectTypeKeys()
	if len(typeKeys) == 0 {
		typeKeys = []domain.TypeKey{bundle.TypeKeyPage}
	}

	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return outcome, fmt.Errorf("get space %s: %w", spaceId, err)
	}
	// A document may reference bundled types/relations not yet present in the
	// space (a fresh space has only a few installed); install them so the
	// created object's type and relations resolve (mirrors the import path).
	if ids := bundledIdsToInstall(createState.AllRelationKeys(), typeKeys); len(ids) > 0 {
		if _, _, err := a.creator.InstallBundledObjects(ctx, spc, ids); err != nil {
			return outcome, fmt.Errorf("install bundled objects: %w", err)
		}
	}

	if templateId != "" {
		var templateBlocks int
		if createState, templateBlocks, err = a.applyTemplate(ctx, spc, typeKeys, createState, templateId); err != nil {
			return outcome, err
		}
		outcome.TemplateBlocks = templateBlocks
	}
	// after the template merge, so the detail lands on whichever state is
	// created — and so it is never offered to the template service as one of
	// the document's own details (a template state strips origin on purpose)
	createState.SetDetail(bundle.RelationKeyOrigin, domain.Int64(int64(model.ObjectOrigin_api)))

	id, _, err := a.creator.CreateSmartBlockFromStateInSpace(ctx, spc, typeKeys, createState)
	if err != nil {
		return outcome, fmt.Errorf("create object from state: %w", err)
	}
	outcome.Id = id
	return outcome, nil
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
) (*state.State, int, error) {
	typeId, err := spc.GetTypeIdByKey(ctx, typeKeys[0])
	if err != nil {
		return nil, 0, fmt.Errorf("derive type id for %s: %w", typeKeys[0], err)
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
		return nil, 0, fmt.Errorf("%w: build state from template %s: %v", apicore.ErrTemplateUnavailable, templateId, err)
	}
	// The template service degrades a template it cannot load to the BLANK
	// template and reports no error (templateimpl.createCustomTemplateState
	// does this for one that was deleted or archived between the API's index
	// check and this load). Silence there would mean an object created with
	// none of the content the response says it started from, so the degrade
	// is detected and reported: a state built from a real template is rooted
	// at that template's id, the blank one is not. The root is used rather
	// than the sourceObject detail because a document's own properties are
	// merged into these details, so the detail is reachable by the caller and
	// the root is not.
	if base.RootId() != templateId {
		return nil, 0, fmt.Errorf("%w: template %s did not load", apicore.ErrTemplateUnavailable, templateId)
	}
	base.SetObjectTypeKeys(typeKeys)
	if key := doc.UniqueKeyInternal(); key != "" {
		base.SetUniqueKeyInternal(key)
	}
	// counted BEFORE the merge, so the number is the template's contribution
	// and not the whole document
	contributed := countContentBlocks(base)
	mergeDocumentIntoTemplate(base, doc)
	return base, contributed, nil
}

// countContentBlocks counts the blocks a template state contributes to the
// object, skipping the root and the header subtree — the title, description
// and featured relations an object of this layout carries whether or not a
// template was applied. What is left is what the caller did not send and
// would otherwise have to read the object back to discover.
func countContentBlocks(st *state.State) int {
	root := st.Pick(st.RootId())
	if root == nil {
		return 0
	}
	// state.Iterate cannot skip a SUBTREE — a false return ends the whole
	// walk, and the header is the root's first child — so the walk is its
	// own. The visited set is what keeps a malformed state from looping.
	var (
		count   int
		visited = map[string]bool{st.RootId(): true}
		walk    func(ids []string)
	)
	walk = func(ids []string) {
		for _, id := range ids {
			if id == editortemplate.HeaderLayoutId || visited[id] {
				continue
			}
			visited[id] = true
			block := st.Pick(id)
			if block == nil {
				continue
			}
			count++
			walk(block.Model().ChildrenIds)
		}
	}
	walk(root.Model().ChildrenIds)
	return count
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
		id := b.Model().Id
		if id == doc.RootId() {
			return true
		}
		ordered = append(ordered, b.Copy())
		if base.Exists(id) || editorReservedBlockIds[id] {
			renamed[id] = bson.NewObjectId().Hex()
		}
		return true
	})
	carryTableCells(ordered, renamed)
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
	// the document's root block keeps its identity from the template — it IS
	// the template's root — but the attributes the caller set on theirs are
	// theirs, and dropping them would make a create with a template quietly
	// lose what the same create without one keeps
	mergeRootAttributes(baseRoot.Model(), root.Model())
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

// InstallBundledType is the create path's bundled install for ONE type, for
// the set_type op: basic.SetObjectTypesInState reads the target type from
// the space's store, so a bundled type the space never installed has to be
// installed before the edit takes the object lock.
func (a *objectCreateAdapter) InstallBundledType(ctx context.Context, spaceId string, key domain.TypeKey) error {
	ids := bundledIdsToInstall(nil, []domain.TypeKey{key})
	if len(ids) == 0 {
		return nil
	}
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space %s: %w", spaceId, err)
	}
	// the installer answers a read-only space with silent success; say so
	// instead, or the caller waits for a row that never comes
	if spc.IsReadOnly() {
		return apicore.ErrSpaceReadOnly
	}
	if _, _, err := a.creator.InstallBundledObjects(ctx, spc, ids); err != nil {
		return fmt.Errorf("install bundled type %s: %w", key, err)
	}
	return nil
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

// editorReservedBlockIds are the ids the editor owns on every object: it
// creates them, moves them and, for a note, UNLINKS the title outright
// (template.WithNoTitle). A caller's block carrying one of them is reminted
// even when the template state has no such block, because the object's own
// initialization will assert its claim afterwards and the caller's content
// would go with it.
//
// Nothing is lost by the rename: these blocks are structural, and a read of
// this API never serves them — a document a caller pastes back cannot carry
// one, so an id from this set is always authored rather than cloned.
var editorReservedBlockIds = map[string]bool{
	editortemplate.HeaderLayoutId:      true,
	editortemplate.TitleBlockId:        true,
	editortemplate.DescriptionBlockId:  true,
	editortemplate.FeaturedRelationsId: true,
}

// carryTableCells keeps a renamed table coherent. A cell's id is not free:
// it is `<rowId>-<colId>` (table.MakeCellID), and the row and column blocks
// reach their cells through that arithmetic rather than through a reference
// the rename map would rewrite. So a renamed row or column drags its cells
// with it, and a cell that collided on its own is not reminted in place —
// its ROW is renamed instead, which is the only rename a cell id can follow.
// A minted id is bson hex and carries no separator, so a derived cell id
// still parses as one.
//
// The test is the id's SHAPE, and a caller's ordinary `intro-1` matches it.
// That is harmless: every reference to an id travels through the same map, so
// renaming a block that is not a cell is invisible.
func carryTableCells(blocks []simple.Block, renamed map[string]string) {
	rowOf := map[string]string{}
	for _, b := range blocks {
		id := b.Model().Id
		if !table.IsTableCell(id) {
			continue
		}
		row, _, found := strings.Cut(id, table.TableCellSeparator)
		if !found {
			continue
		}
		rowOf[id] = row
	}
	// a cell that collided cannot take a fresh id of its own: promote the
	// collision to its row, whose rename every cell of that row then follows
	for cellId, rowId := range rowOf {
		if _, collided := renamed[cellId]; !collided {
			continue
		}
		delete(renamed, cellId)
		if _, alreadyRenamed := renamed[rowId]; !alreadyRenamed {
			renamed[rowId] = bson.NewObjectId().Hex()
		}
	}
	for cellId, rowId := range rowOf {
		row, col, _ := strings.Cut(cellId, table.TableCellSeparator)
		newRow, rowRenamed := renamed[rowId]
		newCol, colRenamed := renamed[col]
		if !rowRenamed && !colRenamed {
			continue
		}
		if !rowRenamed {
			newRow = row
		}
		if !colRenamed {
			newCol = col
		}
		renamed[cellId] = newRow + table.TableCellSeparator + newCol
	}
}

// mergeRootAttributes copies the root-block attributes the caller's document
// set onto the template's root. The caller wins where they said something:
// what the template carries on its root is a default, and what the document
// carries is a choice.
func mergeRootAttributes(base, doc *model.Block) {
	if doc.BackgroundColor != "" {
		base.BackgroundColor = doc.BackgroundColor
	}
	if doc.Align != model.Block_AlignLeft {
		base.Align = doc.Align
	}
	if doc.VerticalAlign != model.Block_VerticalAlignTop {
		base.VerticalAlign = doc.VerticalAlign
	}
	if doc.Fields == nil || len(doc.Fields.Fields) == 0 {
		return
	}
	if base.Fields == nil {
		base.Fields = &types.Struct{Fields: map[string]*types.Value{}}
	}
	if base.Fields.Fields == nil {
		base.Fields.Fields = map[string]*types.Value{}
	}
	for key, value := range doc.Fields.Fields {
		base.Fields.Fields[key] = value
	}
}
