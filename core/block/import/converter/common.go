package converter

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var log = logging.Logger("import")

func GetSourceDetail(fileName, importPath string) string {
	var source bytes.Buffer
	source.WriteString(strings.TrimPrefix(filepath.Ext(fileName), "."))
	source.WriteString(":")
	source.WriteString(importPath)
	source.WriteRune(filepath.Separator)
	source.WriteString(fileName)
	return source.String()
}

func GetDetails(name string) *types.Struct {
	var title string

	if title == "" {
		title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}

	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():           pbtypes.String(title),
		bundle.RelationKeySourceFilePath.String(): pbtypes.String(name),
	}
	return &types.Struct{Fields: fields}
}

func UpdateLinksToObjects(st *state.State, oldIDtoNew map[string]string, pageID string) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		switch block := bl.(type) {
		case link.Block:
			handleLinkBlock(oldIDtoNew, block, st)
		case bookmark.Block:
			handleBookmarkBlock(oldIDtoNew, block, st)
		case text.Block:
			handleMarkdownTest(oldIDtoNew, block, st)
		case dataview.Block:
			handleDataviewBlock(block, oldIDtoNew, st)
		}
		return true
	})
}

func handleDataviewBlock(block simple.Block, oldIDtoNew map[string]string, st *state.State) {
	dataview := block.Model().GetDataview()
	target := dataview.TargetObjectId
	if target != "" {
		newTarget := oldIDtoNew[target]
		if newTarget == "" {
			newTarget = addr.MissingObject
		}
		dataview.TargetObjectId = newTarget
		st.Set(simple.New(block.Model()))
	}

	for _, view := range dataview.GetViews() {
		for _, filter := range view.GetFilters() {
			updateObjectIDsInFilter(filter, oldIDtoNew)
		}
	}
}

func updateObjectIDsInFilter(filter *model.BlockContentDataviewFilter, oldIDtoNew map[string]string) {
	if filter.Format != model.RelationFormat_object &&
		filter.Format != model.RelationFormat_tag &&
		filter.Format != model.RelationFormat_status {
		return
	}
	if objectIDs := pbtypes.GetStringListValue(filter.Value); objectIDs != nil {
		var newIDs []string
		for _, objectID := range objectIDs {
			if newID := oldIDtoNew[objectID]; newID != "" {
				newIDs = append(newIDs, newID)
			}
		}
		if len(newIDs) != 0 {
			filter.Value = pbtypes.StringList(newIDs)
		}
		return
	}
	if objectID := filter.Value.GetStringValue(); objectID != "" {
		if newID := oldIDtoNew[objectID]; newID != "" {
			filter.Value = pbtypes.String(newID)
		}
	}
}

func handleBookmarkBlock(oldIDtoNew map[string]string, block simple.Block, st *state.State) {
	newTarget := oldIDtoNew[block.Model().GetBookmark().TargetObjectId]
	if newTarget == "" {
		log.Errorf("failed to find bookmark object")
		return
	}

	block.Model().GetBookmark().TargetObjectId = newTarget
	st.Set(simple.New(block.Model()))
}

func handleLinkBlock(oldIDtoNew map[string]string, block simple.Block, st *state.State) {
	targetBlockID := block.Model().GetLink().TargetBlockId
	newTarget := oldIDtoNew[targetBlockID]
	if newTarget == "" {
		if isBundledObjects(targetBlockID) {
			return
		}
		newTarget = addr.MissingObject
	}

	block.Model().GetLink().TargetBlockId = newTarget
	st.Set(simple.New(block.Model()))
}

func isBundledObjects(targetBlockID string) bool {
	ot, err := bundle.TypeKeyFromUrl(targetBlockID)
	if err == nil && bundle.HasObjectType(ot.String()) {
		return true
	}
	rel, err := pbtypes.RelationIdToKey(targetBlockID)
	if err == nil && bundle.HasRelation(rel) {
		return true
	}
	return false
}

func handleMarkdownTest(oldIDtoNew map[string]string, block simple.Block, st *state.State) {
	marks := block.Model().GetText().GetMarks().GetMarks()
	for i, mark := range marks {
		if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
			continue
		}
		newTarget := oldIDtoNew[mark.Param]
		if newTarget == "" {
			newTarget = addr.MissingObject
		}

		marks[i].Param = newTarget
	}
	st.Set(simple.New(block.Model()))
}

func UpdateRelationsIDs(st *state.State, oldIDtoNew map[string]string) {
	rels := st.GetRelationLinks()
	for k, v := range st.Details().GetFields() {
		relLink := rels.Get(k)
		if relLink == nil {
			continue
		}
		if relLink.Format != model.RelationFormat_object &&
			relLink.Format != model.RelationFormat_tag &&
			relLink.Format != model.RelationFormat_status {
			continue
		}
		if relLink.Key == bundle.RelationKeyFeaturedRelations.String() {
			// special cases
			// featured relations have incorrect IDs
			continue
		}
		handleObjectRelation(st, oldIDtoNew, v, k)
	}
}

func handleObjectRelation(st *state.State, oldIDtoNew map[string]string, v *types.Value, k string) {
	if _, ok := v.GetKind().(*types.Value_StringValue); ok {
		objectsID := v.GetStringValue()
		newObjectIDs := getNewObjectsIDForRelation([]string{objectsID}, oldIDtoNew)
		if len(newObjectIDs) != 0 {
			st.SetDetail(k, pbtypes.String(newObjectIDs[0]))
		}
		return
	}
	objectsIDs := pbtypes.GetStringListValue(v)
	objectsIDs = getNewObjectsIDForRelation(objectsIDs, oldIDtoNew)
	st.SetDetail(k, pbtypes.StringList(objectsIDs))
}

func getNewObjectsIDForRelation(objectsIDs []string, oldIDtoNew map[string]string) []string {
	for i, val := range objectsIDs {
		newTarget := oldIDtoNew[val]
		if newTarget == "" {
			// preserve links to bundled objects
			if isBundledObjects(val) {
				continue
			}
			newTarget = addr.MissingObject
		}
		objectsIDs[i] = newTarget
	}
	return objectsIDs
}

func UpdateObjectType(oldIDtoNew map[string]string, st *state.State) {
	objectType := st.ObjectType()
	if newType, ok := oldIDtoNew[objectType]; ok {
		st.SetObjectType(newType)
	}
}
