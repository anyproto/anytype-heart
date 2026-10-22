package enginetest

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Dump is the golden-comparable projection of a run's materialized objects:
// semantic block trees (no volatile ids), details without timestamps, store
// membership and uploaded files.
type Dump struct {
	Objects []ObjectDump `json:"objects"`
	Uploads []string     `json:"uploads,omitempty"`
}

type ObjectDump struct {
	Id          string         `json:"id"`
	ObjectTypes []string       `json:"objectTypes,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	Members     []string       `json:"members,omitempty"`
	Blocks      []BlockDump    `json:"blocks,omitempty"`
}

type BlockDump struct {
	Kind     string      `json:"kind"`
	Style    string      `json:"style,omitempty"`
	Text     string      `json:"text,omitempty"`
	Marks    []MarkDump  `json:"marks,omitempty"`
	Target   string      `json:"target,omitempty"`
	Url      string      `json:"url,omitempty"`
	Children []BlockDump `json:"children,omitempty"`
}

type MarkDump struct {
	Type  string `json:"type"`
	Param string `json:"param,omitempty"`
}

// volatileDetails vary run-to-run (fs timestamps) and are excluded.
var volatileDetails = map[string]struct{}{
	bundle.RelationKeyCreatedDate.String():      {},
	bundle.RelationKeyLastModifiedDate.String(): {},
}

func (fx *Fixture) Dump() Dump {
	fx.Space.mu.Lock()
	defer fx.Space.mu.Unlock()

	dump := Dump{}
	ids := make([]string, 0, len(fx.Space.Created))
	for id := range fx.Space.Created {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		st := fx.Space.Created[id]
		if st == nil {
			continue
		}
		typeKeys := make([]string, 0, len(st.ObjectTypeKeys()))
		for _, key := range st.ObjectTypeKeys() {
			typeKeys = append(typeKeys, key.String())
		}
		object := ObjectDump{
			Id:          id,
			ObjectTypes: typeKeys,
			Details:     detailsToMap(st.CombinedDetails().ToProto()),
			Members:     st.GetStoreSlice(template.CollectionStoreKey),
			Blocks:      blockChildren(st.Blocks(), id),
		}
		dump.Objects = append(dump.Objects, object)
	}

	fx.Uploader.mu.Lock()
	uploads := make([]string, 0, len(fx.Uploader.Uploads))
	for _, localPath := range fx.Uploader.Uploads {
		uploads = append(uploads, filepath.Base(localPath))
	}
	fx.Uploader.mu.Unlock()
	sort.Strings(uploads)
	dump.Uploads = uploads
	return dump
}

func detailsToMap(details *types.Struct) map[string]any {
	result := map[string]any{}
	for key, value := range details.GetFields() {
		if _, volatile := volatileDetails[key]; volatile {
			continue
		}
		result[key] = valueToAny(value)
	}
	return result
}

func valueToAny(value *types.Value) any {
	switch kind := value.GetKind().(type) {
	case *types.Value_StringValue:
		return kind.StringValue
	case *types.Value_NumberValue:
		return kind.NumberValue
	case *types.Value_BoolValue:
		return kind.BoolValue
	case *types.Value_ListValue:
		list := make([]any, 0, len(kind.ListValue.Values))
		for _, item := range kind.ListValue.Values {
			list = append(list, valueToAny(item))
		}
		return list
	default:
		return value.String()
	}
}

func blockChildren(blocks []*model.Block, rootId string) []BlockDump {
	byId := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		byId[b.Id] = b
	}
	root, ok := byId[rootId]
	if !ok {
		return nil
	}
	return dumpChildren(root, byId)
}

func dumpChildren(parent *model.Block, byId map[string]*model.Block) []BlockDump {
	children := make([]BlockDump, 0, len(parent.ChildrenIds))
	for _, childId := range parent.ChildrenIds {
		child, ok := byId[childId]
		if !ok {
			continue
		}
		children = append(children, dumpBlock(child, byId))
	}
	return children
}

func dumpBlock(b *model.Block, byId map[string]*model.Block) BlockDump {
	dump := BlockDump{Children: dumpChildren(b, byId)}
	switch content := b.Content.(type) {
	case *model.BlockContentOfText:
		dump.Kind = "text"
		dump.Style = content.Text.Style.String()
		dump.Text = content.Text.Text
		for _, mark := range content.Text.GetMarks().GetMarks() {
			dump.Marks = append(dump.Marks, MarkDump{Type: mark.Type.String(), Param: mark.Param})
		}
	case *model.BlockContentOfLink:
		dump.Kind = "link"
		dump.Target = content.Link.TargetBlockId
	case *model.BlockContentOfFile:
		dump.Kind = "file"
		dump.Style = content.File.Type.String()
		dump.Target = content.File.TargetObjectId
	case *model.BlockContentOfBookmark:
		dump.Kind = "bookmark"
		dump.Url = content.Bookmark.Url
	case *model.BlockContentOfDiv:
		dump.Kind = "div"
	case *model.BlockContentOfSmartblock:
		dump.Kind = "smartblock"
	case *model.BlockContentOfLayout:
		dump.Kind = "layout"
	default:
		dump.Kind = fmt.Sprintf("%T", content)
	}
	return dump
}
