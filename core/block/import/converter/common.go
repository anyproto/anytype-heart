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
		bundle.RelationKeyName.String():   pbtypes.String(title),
		bundle.RelationKeySource.String(): pbtypes.String(name),
	}
	return &types.Struct{Fields: fields}
}

func UpdateLinksToObjects(st *state.State, oldIDtoNew map[string]string, pageID string) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		switch block := bl.(type) {
		case link.Block:
			handleLinkBlock(oldIDtoNew, block, st)
		case bookmark.Block:
			handleBookmarkBlock(oldIDtoNew, block, pageID, st)
		case text.Block:
			handleMarkdownTest(oldIDtoNew, block, st, pageID)
		case dataview.Block:
			handleDataviewBlock(block, oldIDtoNew, st)
		}
		return true
	})
}

func handleDataviewBlock(block simple.Block, oldIDtoNew map[string]string, st *state.State) {
	target := block.Model().GetDataview().TargetObjectId
	if target == "" {
		return
	}
	newTarget := oldIDtoNew[target]
	if newTarget == "" {
		newTarget = addr.MissingObject
	}

	block.Model().GetDataview().TargetObjectId = newTarget
	st.Set(simple.New(block.Model()))
}

func handleBookmarkBlock(oldIDtoNew map[string]string, block simple.Block, pageID string, st *state.State) {
	newTarget := oldIDtoNew[block.Model().GetBookmark().TargetObjectId]
	if newTarget == "" {
		newTarget = addr.MissingObject
	}

	block.Model().GetBookmark().TargetObjectId = newTarget
	st.Set(simple.New(block.Model()))
}

func handleLinkBlock(oldIDtoNew map[string]string, block simple.Block, st *state.State) {
	newTarget := oldIDtoNew[block.Model().GetLink().TargetBlockId]
	if newTarget == "" {
		newTarget = addr.MissingObject
	}

	block.Model().GetLink().TargetBlockId = newTarget
	st.Set(simple.New(block.Model()))
}

func handleMarkdownTest(oldIDtoNew map[string]string, block simple.Block, st *state.State, objectID string) {
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

func UpdateRelationsIDs(st *state.State, pageID string, oldIDtoNew map[string]string) {
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

		objectsIDs := pbtypes.GetStringListValue(v)
		objectsIDs = getNewRelationsID(objectsIDs, oldIDtoNew, pageID, k)
		st.SetDetail(k, pbtypes.StringList(objectsIDs))
	}
}

func getNewRelationsID(objectsIDs []string, oldIDtoNew map[string]string, pageID string, k string) []string {
	for i, val := range objectsIDs {
		if bundle.HasRelation(val) {
			continue
		}
		newTarget := oldIDtoNew[val]
		if newTarget == "" {
			log.With("object", pageID).Errorf("cant find target id for relation %s: %s", k, val)
			continue
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
