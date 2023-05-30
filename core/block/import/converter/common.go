package converter

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	dv := block.Model().GetDataview()
	target := dv.TargetObjectId
	if target != "" {
		newTarget := oldIDtoNew[target]
		if newTarget == "" {
			newTarget = addr.MissingObject
		}
		dv.TargetObjectId = newTarget
		st.Set(simple.New(block.Model()))
	}

	for _, view := range dv.GetViews() {
		for _, filter := range view.GetFilters() {
			updateObjectIDsInFilter(filter, oldIDtoNew)
		}
		for _, relation := range view.Relations {
			if newID, ok := oldIDtoNew[addr.RelationKeyToIdPrefix+relation.Key]; ok && newID != addr.RelationKeyToIdPrefix+relation.Key {
				updateRelationID(block, relation, view, newID)
			}
		}
	}
	for _, group := range dv.GetGroupOrders() {
		for _, vg := range group.ViewGroups {
			groups := replaceChunks(vg.GroupId, oldIDtoNew)
			sort.Strings(groups)
			vg.GroupId = strings.Join(groups, "")
		}
	}
	for _, group := range dv.GetObjectOrders() {
		for i, id := range group.ObjectIds {
			if newId, exist := oldIDtoNew[id]; exist {
				group.ObjectIds[i] = newId
			}
		}
	}
}

func updateRelationID(block simple.Block, relation *model.BlockContentDataviewRelation, view *model.BlockContentDataviewView, newID string) {
	oldKey := relation.Key
	db := block.(dataview.Block)
	err := db.RemoveViewRelations(view.Id, []string{oldKey})
	if err != nil {
		log.Error("failed to remove relation from view, %s", err.Error())
		return
	}
	relation.Key = strings.TrimPrefix(newID, addr.RelationKeyToIdPrefix)
	err = db.AddViewRelation(view.Id, relation)
	if err != nil {
		log.Error("failed to add new relations from view, %s", err.Error())
		return
	}
	for _, relationLink := range db.Model().GetDataview().GetRelationLinks() {
		if relationLink.Key == oldKey {
			relationLink.Key = strings.TrimPrefix(newID, addr.RelationKeyToIdPrefix)
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
		if widget.IsPredefinedWidgetTargetId(targetBlockID) {
			return
		}
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

func UpdateObjectIDsInRelations(st *state.State, oldIDtoNew map[string]string) {
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

func replaceChunks(s string, oldToNew map[string]string) []string {
	var result []string
	i := 0

	var buf strings.Builder
	for i < len(s) {
		// Assume no match found
		foundMatch := false

		// Iterate through the oldToNew map keys to find the first match
		for o, n := range oldToNew {
			if strings.HasPrefix(s[i:], o) {
				// Write the new substring to the result
				if buf.Len() != 0 {
					// dump the buffer to the result
					result = append(result, buf.String())
					buf.Reset()
				}

				result = append(result, n)

				// Move the index forward by the length of the matched old substring
				i += len(o)
				foundMatch = true
				break
			}
		}

		// If no match found, append the current character to the result
		if !foundMatch {
			buf.WriteByte(s[i])
			i++
		}
	}

	return result
}

func AddRelationsToDataView(st *state.State, rel *model.RelationLink) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		if dv, ok := bl.(dataview.Block); ok {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return false
			}
			for _, view := range bl.Model().GetDataview().GetViews() {
				err := dv.AddViewRelation(view.GetId(), &model.BlockContentDataviewRelation{
					Key:       rel.Key,
					IsVisible: true,
					Width:     192,
				})
				if err != nil {
					return false
				}
			}
			err := dv.AddRelation(&model.RelationLink{
				Key:    rel.Key,
				Format: rel.Format,
			})
			if err != nil {
				return false
			}
		}
		return true
	})
}

func ConvertStringToTime(t string) int64 {
	parsedTime, err := time.Parse(time.RFC3339, t)
	if err != nil {
		log.Errorf("failed to convert time %s", t)
		return 0
	}
	return parsedTime.Unix()
}
