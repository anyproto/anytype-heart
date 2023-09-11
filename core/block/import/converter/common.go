package converter

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

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
	"github.com/anyproto/anytype-heart/util/slice"
)

var randomIcons = []string{"📓", "📕", "📗", "📘", "📙", "📖", "📔", "📒", "📝", "📄", "📑"}

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

func GetCommonDetails(sourcePath, name, emoji string, layout model.ObjectTypeLayout) *types.Struct {
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	}
	if emoji == "" {
		emoji = slice.GetRandomString(randomIcons, name)
	}
	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():           pbtypes.String(name),
		bundle.RelationKeySourceFilePath.String(): pbtypes.String(sourcePath),
		bundle.RelationKeyIconEmoji.String():      pbtypes.String(emoji),
		bundle.RelationKeyCreatedDate.String():    pbtypes.Int64(time.Now().Unix()), // this relation will be after used in the tree header
		bundle.RelationKeyLayout.String():         pbtypes.Float64(float64(layout)),
	}
	return &types.Struct{Fields: fields}
}

func UpdateLinksToObjects(st *state.State, oldIDtoNew map[string]string, filesIDs []string) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		switch block := bl.(type) {
		case link.Block:
			handleLinkBlock(oldIDtoNew, block, st, filesIDs)
		case bookmark.Block:
			handleBookmarkBlock(oldIDtoNew, block, st)
		case text.Block:
			handleMarkdownTest(oldIDtoNew, block, st, filesIDs)
		case dataview.Block:
			handleDataviewBlock(block, oldIDtoNew, st)
		}
		return true
	})
}

func handleDataviewBlock(block simple.Block, oldIDtoNew map[string]string, st *state.State) {
	dataView := block.Model().GetDataview()
	target := dataView.TargetObjectId
	if target != "" {
		newTarget := oldIDtoNew[target]
		if newTarget == "" {
			newTarget = addr.MissingObject
		}
		dataView.TargetObjectId = newTarget
		st.Set(simple.New(block.Model()))
	}

	for _, view := range dataView.GetViews() {
		for _, filter := range view.GetFilters() {
			updateObjectIDsInFilter(filter, oldIDtoNew)
		}

		if view.DefaultTemplateId != "" {
			view.DefaultTemplateId = oldIDtoNew[view.DefaultTemplateId]
		}
	}
	for _, group := range dataView.GetGroupOrders() {
		for _, vg := range group.ViewGroups {
			groups := replaceChunks(vg.GroupId, oldIDtoNew)
			sort.Strings(groups)
			vg.GroupId = strings.Join(groups, "")
		}
	}
	for _, group := range dataView.GetObjectOrders() {
		for i, id := range group.ObjectIds {
			if newId, exist := oldIDtoNew[id]; exist {
				group.ObjectIds[i] = newId
			}
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

func handleLinkBlock(oldIDtoNew map[string]string, block simple.Block, st *state.State, filesIDs []string) {
	targetBlockID := block.Model().GetLink().TargetBlockId
	if lo.Contains(filesIDs, targetBlockID) {
		return
	}
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

func isBundledObjects(targetObjectID string) bool {
	ot, err := bundle.TypeKeyFromUrl(targetObjectID)
	if err == nil && bundle.HasObjectTypeByKey(ot) {
		return true
	}
	rel, err := pbtypes.RelationIdToKey(targetObjectID)
	if err == nil && bundle.HasRelation(rel) {
		return true
	}

	if strings.HasPrefix(targetObjectID, addr.DatePrefix) {
		return true
	}
	return false
}

func handleMarkdownTest(oldIDtoNew map[string]string, block simple.Block, st *state.State, filesIDs []string) {
	marks := block.Model().GetText().GetMarks().GetMarks()
	for i, mark := range marks {
		if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
			continue
		}
		if lo.Contains(filesIDs, mark.Param) {
			return
		}
		if isBundledObjects(mark.Param) {
			return
		}
		newTarget := oldIDtoNew[mark.Param]
		if newTarget == "" {
			newTarget = addr.MissingObject
		}

		marks[i].Param = newTarget
	}
	st.Set(simple.New(block.Model()))
}

func UpdateObjectIDsInRelations(st *state.State, oldIDtoNew map[string]string, filesIDs []string) {
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
		// For example, RelationKeySetOf is handled here
		handleObjectRelation(st, oldIDtoNew, v, k, filesIDs)
	}
}

func handleObjectRelation(st *state.State, oldIDtoNew map[string]string, v *types.Value, k string, filesIDs []string) {
	if _, ok := v.GetKind().(*types.Value_StringValue); ok {
		objectsID := v.GetStringValue()
		newObjectIDs := getNewObjectsIDForRelation([]string{objectsID}, oldIDtoNew, filesIDs)
		if len(newObjectIDs) != 0 {
			st.SetDetail(k, pbtypes.String(newObjectIDs[0]))
		}
		return
	}
	objectsIDs := pbtypes.GetStringListValue(v)
	objectsIDs = getNewObjectsIDForRelation(objectsIDs, oldIDtoNew, filesIDs)
	st.SetDetail(k, pbtypes.StringList(objectsIDs))
}

func getNewObjectsIDForRelation(objectsIDs []string, oldIDtoNew map[string]string, filesIDs []string) []string {
	for i, val := range objectsIDs {
		if lo.Contains(filesIDs, val) {
			continue
		}
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

// TODO Fix this
// func UpdateObjectType(oldIDtoNew map[string]string, st *state.State) {
// 	objectType := st.ObjectTypeKey()
// 	if newType, ok := oldIDtoNew[objectType]; ok {
// 		st.SetObjectTypeKey(newType)
// 	}
// }

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

func AddRelationsToDataView(collectionState *state.State, relationLink *model.RelationLink) error {
	return collectionState.Iterate(func(block simple.Block) (isContinue bool) {
		if dataView, ok := block.(dataview.Block); ok {
			if len(block.Model().GetDataview().GetViews()) == 0 {
				return true
			}
			for _, view := range block.Model().GetDataview().GetViews() {
				err := dataView.AddViewRelation(view.GetId(), &model.BlockContentDataviewRelation{
					Key:       relationLink.Key,
					IsVisible: true,
					Width:     192,
				})
				if err != nil {
					return true
				}
			}
			err := dataView.AddRelation(&model.RelationLink{
				Key:    relationLink.Key,
				Format: relationLink.Format,
			})
			if err != nil {
				return true
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
