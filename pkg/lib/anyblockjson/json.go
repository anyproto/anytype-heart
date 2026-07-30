package anyblockjson

// json.go holds the shared serialization infrastructure: the ordered-JSON
// writer that produces the §4 canonical byte form, the enum name tables, the
// proto value bridges, date formatting, and id helpers.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// omap is a JSON object with explicit key order — the canonical form fixes
// key order (§4), which encoding/json maps cannot express.
type omap struct {
	keys []string
	vals []any
}

func (m *omap) set(k string, v any) {
	m.keys = append(m.keys, k)
	m.vals = append(m.vals, v)
}

// setNonEmpty appends k only when v is not an empty/default value (§4:
// canonical form omits empty strings, arrays, objects and default scalars).
func (m *omap) setNonEmpty(k string, v any) {
	switch x := v.(type) {
	case string:
		if x == "" {
			return
		}
	case bool:
		if !x {
			return
		}
	case int:
		if x == 0 {
			return
		}
	case int32:
		if x == 0 {
			return
		}
	case int64:
		if x == 0 {
			return
		}
	case float64:
		if x == 0 {
			return
		}
	case []any:
		if len(x) == 0 {
			return
		}
	case *omap:
		if x == nil || len(x.keys) == 0 {
			return
		}
	case nil:
		return
	}
	m.set(k, v)
}

func (m *omap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeJSONValue(&buf, m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeJSONValue(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case *omap:
		buf.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			if err := encodeJSONValue(buf, x.vals[i]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeJSONValue(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeJSONString(buf, x)
	default:
		// numbers and booleans contain no HTML-escapable characters
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode value: %w", err)
		}
		buf.Write(b)
	}
	return nil
}

// writeJSONString escapes like encoding/json but without HTML escaping, so
// inline markup tags stay readable.
func writeJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\u2028':
			buf.WriteString(`\u2028`)
		case '\u2029':
			buf.WriteString(`\u2029`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// marshalCanonical renders the document omap in the canonical byte form:
// UTF-8, LF, two-space indent, trailing newline (§4).
func marshalCanonical(doc *omap) ([]byte, error) {
	compact, err := doc.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return nil, fmt.Errorf("indent document: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

//
// ---- enum name tables ----
//

type enumNames[T comparable] struct {
	toName map[T]string
	toVal  map[string]T
}

func newEnumNames[T comparable](pairs map[T]string) enumNames[T] {
	e := enumNames[T]{toName: pairs, toVal: make(map[string]T, len(pairs))}
	for v, n := range pairs {
		e.toVal[n] = v
	}
	return e
}

func (e enumNames[T]) name(v T) string   { return e.toName[v] }
func (e enumNames[T]) value(n string) T  { return e.toVal[n] }
func (e enumNames[T]) has(n string) bool { _, ok := e.toVal[n]; return ok }

var kindNames = newEnumNames(map[model.SmartBlockType]string{
	model.SmartBlockType_AccountOld:             "accountOld",
	model.SmartBlockType_Page:                   "page",
	model.SmartBlockType_ProfilePage:            "profilePage",
	model.SmartBlockType_Home:                   "home",
	model.SmartBlockType_Archive:                "archive",
	model.SmartBlockType_Widget:                 "widget",
	model.SmartBlockType_File:                   "file",
	model.SmartBlockType_Template:               "template",
	model.SmartBlockType_BundledTemplate:        "bundledTemplate",
	model.SmartBlockType_BundledRelation:        "bundledRelation",
	model.SmartBlockType_SubObject:              "subObject",
	model.SmartBlockType_BundledObjectType:      "bundledObjectType",
	model.SmartBlockType_AnytypeProfile:         "anytypeProfile",
	model.SmartBlockType_Date:                   "date",
	model.SmartBlockType_Workspace:              "workspace",
	model.SmartBlockType_STRelation:             "relation",
	model.SmartBlockType_STType:                 "objectType",
	model.SmartBlockType_STRelationOption:       "relationOption",
	model.SmartBlockType_SpaceView:              "spaceView",
	model.SmartBlockType_Identity:               "identity",
	model.SmartBlockType_Participant:            "participant",
	model.SmartBlockType_MissingObject:          "missingObject",
	model.SmartBlockType_FileObject:             "fileObject",
	model.SmartBlockType_NotificationObject:     "notification",
	model.SmartBlockType_DevicesObject:          "devices",
	model.SmartBlockType_ChatObjectDeprecated:   "chatObject",
	model.SmartBlockType_ChatDerivedObject:      "chat",
	model.SmartBlockType_AccountObject:          "account",
	model.SmartBlockType_DiscussionObject:       "discussion",
	model.SmartBlockType_TechSpaceObject:        "techSpace",
	model.SmartBlockType_TechSpaceVirtualObject: "techSpaceVirtual",
})

// textStyleNames maps text styles to JSON block types. Header4 is absent on
// purpose: deprecated Header4 blocks export as heading3 (§5).
var textStyleNames = newEnumNames(map[model.BlockContentTextStyle]string{
	model.BlockContentText_Paragraph:     "paragraph",
	model.BlockContentText_Header1:       "heading1",
	model.BlockContentText_Header2:       "heading2",
	model.BlockContentText_Header3:       "heading3",
	model.BlockContentText_Quote:         "quote",
	model.BlockContentText_Code:          "code",
	model.BlockContentText_Title:         "title",
	model.BlockContentText_Checkbox:      "checkbox",
	model.BlockContentText_Marked:        "bulletedListItem",
	model.BlockContentText_Numbered:      "numberedListItem",
	model.BlockContentText_Toggle:        "toggle",
	model.BlockContentText_Description:   "description",
	model.BlockContentText_Callout:       "callout",
	model.BlockContentText_ToggleHeader1: "toggleHeading1",
	model.BlockContentText_ToggleHeader2: "toggleHeading2",
	model.BlockContentText_ToggleHeader3: "toggleHeading3",
})

var fileTypeNames = newEnumNames(map[model.BlockContentFileType]string{
	model.BlockContentFile_File:  "file",
	model.BlockContentFile_Image: "image",
	model.BlockContentFile_Video: "video",
	model.BlockContentFile_Audio: "audio",
	model.BlockContentFile_PDF:   "pdf",
})

var alignNames = newEnumNames(map[model.BlockAlign]string{
	model.Block_AlignLeft:    "left",
	model.Block_AlignCenter:  "center",
	model.Block_AlignRight:   "right",
	model.Block_AlignJustify: "justify",
})

var verticalAlignNames = newEnumNames(map[model.BlockVerticalAlign]string{
	model.Block_VerticalAlignTop:    "top",
	model.Block_VerticalAlignMiddle: "middle",
	model.Block_VerticalAlignBottom: "bottom",
})

var fileStyleNames = newEnumNames(map[model.BlockContentFileStyle]string{
	model.BlockContentFile_Auto:  "auto",
	model.BlockContentFile_Link:  "link",
	model.BlockContentFile_Embed: "embed",
})

var cardStyleNames = newEnumNames(map[model.BlockContentLinkCardStyle]string{
	model.BlockContentLink_Text:   "text",
	model.BlockContentLink_Card:   "card",
	model.BlockContentLink_Inline: "inline",
})

var iconSizeNames = newEnumNames(map[model.BlockContentLinkIconSize]string{
	model.BlockContentLink_SizeNone:   "none",
	model.BlockContentLink_SizeSmall:  "small",
	model.BlockContentLink_SizeMedium: "medium",
})

// linkDescriptionNames: proto "Added" is the manually-set description (§5).
var linkDescriptionNames = newEnumNames(map[model.BlockContentLinkDescription]string{
	model.BlockContentLink_None:    "none",
	model.BlockContentLink_Added:   "manual",
	model.BlockContentLink_Content: "content",
})

var divStyleNames = newEnumNames(map[model.BlockContentDivStyle]string{
	model.BlockContentDiv_Line: "line",
	model.BlockContentDiv_Dots: "dots",
})

var processorNames = newEnumNames(map[model.BlockContentLatexProcessor]string{
	model.BlockContentLatex_Latex:         "latex",
	model.BlockContentLatex_Mermaid:       "mermaid",
	model.BlockContentLatex_Chart:         "chart",
	model.BlockContentLatex_Youtube:       "youtube",
	model.BlockContentLatex_Vimeo:         "vimeo",
	model.BlockContentLatex_Soundcloud:    "soundcloud",
	model.BlockContentLatex_GoogleMaps:    "googleMaps",
	model.BlockContentLatex_Miro:          "miro",
	model.BlockContentLatex_Figma:         "figma",
	model.BlockContentLatex_Twitter:       "twitter",
	model.BlockContentLatex_OpenStreetMap: "openStreetMap",
	model.BlockContentLatex_Reddit:        "reddit",
	model.BlockContentLatex_Facebook:      "facebook",
	model.BlockContentLatex_Instagram:     "instagram",
	model.BlockContentLatex_Telegram:      "telegram",
	model.BlockContentLatex_GithubGist:    "githubGist",
	model.BlockContentLatex_Codepen:       "codepen",
	model.BlockContentLatex_Bilibili:      "bilibili",
	model.BlockContentLatex_Excalidraw:    "excalidraw",
	model.BlockContentLatex_Kroki:         "kroki",
	model.BlockContentLatex_Graphviz:      "graphviz",
	model.BlockContentLatex_Sketchfab:     "sketchfab",
	model.BlockContentLatex_Image:         "image",
	model.BlockContentLatex_Drawio:        "drawio",
	model.BlockContentLatex_Spotify:       "spotify",
})

// sourceProcessors carry source code in text; all others carry a URL (§5.2).
var sourceProcessors = map[model.BlockContentLatexProcessor]bool{
	model.BlockContentLatex_Latex:      true,
	model.BlockContentLatex_Mermaid:    true,
	model.BlockContentLatex_Chart:      true,
	model.BlockContentLatex_Graphviz:   true,
	model.BlockContentLatex_Kroki:      true,
	model.BlockContentLatex_Excalidraw: true,
	model.BlockContentLatex_Drawio:     true,
}

var widgetLayoutNames = newEnumNames(map[model.BlockContentWidgetLayout]string{
	model.BlockContentWidget_Link:        "link",
	model.BlockContentWidget_Tree:        "tree",
	model.BlockContentWidget_List:        "list",
	model.BlockContentWidget_CompactList: "compactList",
	model.BlockContentWidget_View:        "view",
})

var viewTypeNames = newEnumNames(map[model.BlockContentDataviewViewType]string{
	model.BlockContentDataviewView_Table:    "table",
	model.BlockContentDataviewView_List:     "list",
	model.BlockContentDataviewView_Gallery:  "gallery",
	model.BlockContentDataviewView_Kanban:   "kanban",
	model.BlockContentDataviewView_Calendar: "calendar",
	model.BlockContentDataviewView_Graph:    "graph",
})

var cardSizeNames = newEnumNames(map[model.BlockContentDataviewViewSize]string{
	model.BlockContentDataviewView_Small:  "small",
	model.BlockContentDataviewView_Medium: "medium",
	model.BlockContentDataviewView_Large:  "large",
})

var listSizeNames = newEnumNames(map[model.BlockContentDataviewViewListSize]string{
	model.BlockContentDataviewView_Compact: "compact",
	model.BlockContentDataviewView_Regular: "regular",
})

var sortDirectionNames = newEnumNames(map[model.BlockContentDataviewSortType]string{
	model.BlockContentDataviewSort_Asc:    "asc",
	model.BlockContentDataviewSort_Desc:   "desc",
	model.BlockContentDataviewSort_Custom: "custom",
})

var emptyPlacementNames = newEnumNames(map[model.BlockContentDataviewSortEmptyType]string{
	model.BlockContentDataviewSort_Start: "start",
	model.BlockContentDataviewSort_End:   "end",
})

var conditionNames = newEnumNames(map[model.BlockContentDataviewFilterCondition]string{
	model.BlockContentDataviewFilter_Equal:          "equal",
	model.BlockContentDataviewFilter_NotEqual:       "notEqual",
	model.BlockContentDataviewFilter_Greater:        "greater",
	model.BlockContentDataviewFilter_Less:           "less",
	model.BlockContentDataviewFilter_GreaterOrEqual: "greaterOrEqual",
	model.BlockContentDataviewFilter_LessOrEqual:    "lessOrEqual",
	model.BlockContentDataviewFilter_Like:           "contains",
	model.BlockContentDataviewFilter_NotLike:        "notContains",
	model.BlockContentDataviewFilter_In:             "in",
	model.BlockContentDataviewFilter_NotIn:          "notIn",
	model.BlockContentDataviewFilter_Empty:          "empty",
	model.BlockContentDataviewFilter_NotEmpty:       "notEmpty",
	model.BlockContentDataviewFilter_AllIn:          "allIn",
	model.BlockContentDataviewFilter_NotAllIn:       "notAllIn",
	model.BlockContentDataviewFilter_ExactIn:        "exactIn",
	model.BlockContentDataviewFilter_NotExactIn:     "notExactIn",
	model.BlockContentDataviewFilter_Exists:         "exists",
})

// countingPresets take a day count from the filter's `value` rather than
// naming a fixed period: getDateRange reads f.Value.Int64() for these two and
// for no others (pkg/lib/database/quickoptions.go). Without a value the count
// is 0, which silently means "today".
var countingPresets = map[model.BlockContentDataviewFilterQuickOption]struct{}{
	model.BlockContentDataviewFilter_NumberOfDaysAgo: {},
	model.BlockContentDataviewFilter_NumberOfDaysNow: {},
}

func countingPreset(q model.BlockContentDataviewFilterQuickOption) bool {
	_, ok := countingPresets[q]
	return ok
}

// countingPresetNames is the same set by name, for validation.
var countingPresetNames = map[string]struct{}{
	"numberOfDaysAgo": {},
	"numberOfDaysNow": {},
}

var datePresetNames = newEnumNames(map[model.BlockContentDataviewFilterQuickOption]string{
	model.BlockContentDataviewFilter_Yesterday:       "yesterday",
	model.BlockContentDataviewFilter_Today:           "today",
	model.BlockContentDataviewFilter_Tomorrow:        "tomorrow",
	model.BlockContentDataviewFilter_LastWeek:        "lastWeek",
	model.BlockContentDataviewFilter_CurrentWeek:     "currentWeek",
	model.BlockContentDataviewFilter_NextWeek:        "nextWeek",
	model.BlockContentDataviewFilter_LastMonth:       "lastMonth",
	model.BlockContentDataviewFilter_CurrentMonth:    "currentMonth",
	model.BlockContentDataviewFilter_NextMonth:       "nextMonth",
	model.BlockContentDataviewFilter_NumberOfDaysAgo: "numberOfDaysAgo",
	model.BlockContentDataviewFilter_NumberOfDaysNow: "numberOfDaysNow",
	model.BlockContentDataviewFilter_LastYear:        "lastYear",
	model.BlockContentDataviewFilter_CurrentYear:     "currentYear",
	model.BlockContentDataviewFilter_NextYear:        "nextYear",
})

var aggregationNames = newEnumNames(map[model.BlockContentDataviewRelationFormulaType]string{
	model.BlockContentDataviewRelation_Count:           "count",
	model.BlockContentDataviewRelation_CountValue:      "countValue",
	model.BlockContentDataviewRelation_CountDistinct:   "countDistinct",
	model.BlockContentDataviewRelation_CountEmpty:      "countEmpty",
	model.BlockContentDataviewRelation_CountNotEmpty:   "countNotEmpty",
	model.BlockContentDataviewRelation_PercentEmpty:    "percentEmpty",
	model.BlockContentDataviewRelation_PercentNotEmpty: "percentNotEmpty",
	model.BlockContentDataviewRelation_MathSum:         "sum",
	model.BlockContentDataviewRelation_MathAverage:     "average",
	model.BlockContentDataviewRelation_MathMedian:      "median",
	model.BlockContentDataviewRelation_MathMin:         "min",
	model.BlockContentDataviewRelation_MathMax:         "max",
	model.BlockContentDataviewRelation_Range:           "range",
})

// FormatName returns the format's canonical JSON name for a property format
// ("text", "select", "objects", …) — the one vocabulary shared by documents
// and API surfaces (APIV2.md C2). It is the exported form of formatName, so
// it applies the same shorttext→"text" fold. Unknown formats return "".
func FormatName(f model.RelationFormat) string {
	return formatName(f)
}

// FormatByName is FormatName's inverse: it maps a §3 format name back to the
// internal relation format. ok is false for names outside the vocabulary.
// "text" maps to longtext (the map's side of the fold); where an existing
// property's stored format matters, the import path resolves it instead.
func FormatByName(name string) (model.RelationFormat, bool) {
	if !formatNames.has(name) {
		return 0, false
	}
	return formatNames.value(name), true
}

// formatNames follows the public REST API vocabulary (§3). Text has exactly
// one name: the editor offers a single Text format, so the stored
// longtext/shorttext split stays out of this serialization — shorttext has
// no name of its own and folds into "text" via formatName. The map must
// remain a bijection (newEnumNames inverts it, and a duplicated name would
// invert nondeterministically), which is why the fold lives outside it.
var formatNames = newEnumNames(map[model.RelationFormat]string{
	model.RelationFormat_longtext:  "text",
	model.RelationFormat_number:    "number",
	model.RelationFormat_status:    "select",
	model.RelationFormat_tag:       "multiSelect",
	model.RelationFormat_date:      "date",
	model.RelationFormat_file:      "files",
	model.RelationFormat_checkbox:  "checkbox",
	model.RelationFormat_url:       "url",
	model.RelationFormat_email:     "email",
	model.RelationFormat_phone:     "phone",
	model.RelationFormat_emoji:     "emoji",
	model.RelationFormat_object:    "objects",
	model.RelationFormat_relations: "properties",
})

// filterTemplatePrefix marks a dynamic filter value: a placeholder the
// client substitutes for a real object id before it issues the query
// (anytype-ts Dataview.valueTemplateMapper). The tokens are built as
// sprintf("_filter_template_%d_", FilterValueTemplate) — _filter_template_2_
// is the current user, resolving to _participant_<space>_<account>, and
// _filter_template_1_ is the object hosting an inline dataview, resolving to
// its id.
//
// They are stored verbatim in the filter's value and are opaque to the
// middleware: nothing in Go resolves them, so a query evaluated server-side
// compares against the literal string and matches nothing. They are not
// object ids and must never be remapped as such.
const filterTemplatePrefix = "_filter_template_"

func isFilterTemplate(v string) bool {
	return strings.HasPrefix(v, filterTemplatePrefix)
}

// layoutNames maps the object layout enum to the names this format uses.
// Layout is *stored* as a number (its bundled relation's format is `number`),
// but a bare integer would be the one opaque enum in an otherwise
// self-describing format — every other enum here is a name (§3).
var layoutNames = newEnumNames(map[model.ObjectTypeLayout]string{
	model.ObjectType_basic:               "basic",
	model.ObjectType_profile:             "profile",
	model.ObjectType_todo:                "todo",
	model.ObjectType_set:                 "set",
	model.ObjectType_objectType:          "objectType",
	model.ObjectType_relation:            "relation",
	model.ObjectType_file:                "file",
	model.ObjectType_dashboard:           "dashboard",
	model.ObjectType_image:               "image",
	model.ObjectType_note:                "note",
	model.ObjectType_space:               "space",
	model.ObjectType_bookmark:            "bookmark",
	model.ObjectType_relationOptionsList: "relationOptionsList",
	model.ObjectType_relationOption:      "relationOption",
	model.ObjectType_collection:          "collection",
	model.ObjectType_audio:               "audio",
	model.ObjectType_video:               "video",
	model.ObjectType_date:                "date",
	model.ObjectType_spaceView:           "spaceView",
	model.ObjectType_participant:         "participant",
	model.ObjectType_pdf:                 "pdf",
	model.ObjectType_chatDeprecated:      "chatDeprecated",
	model.ObjectType_chatDerived:         "chatDerived",
	model.ObjectType_tag:                 "tag",
	model.ObjectType_notification:        "notification",
	model.ObjectType_missingObject:       "missingObject",
	model.ObjectType_devices:             "devices",
	model.ObjectType_discussion:          "discussion",
})

// layoutValuedKeys are the properties whose stored number is an
// ObjectTypeLayout. The other layout-ish bundled keys hold *different* enums
// — layoutAlign is a block align, layoutWidth a fraction, widgetLayout a
// widget layout, headerRelationsLayout its own enum — so they are left alone.
var layoutValuedKeys = map[string]struct{}{
	"recommendedLayout": {},
	"layout":            {},
	"resolvedLayout":    {},
}

func isLayoutKey(key string) bool {
	_, ok := layoutValuedKeys[key]
	return ok
}

// formatName is the export-side name of a stored format: the canonical name
// from formatNames, with legacy shorttext folded into "text" (§3).
func formatName(f model.RelationFormat) string {
	if f == model.RelationFormat_shorttext {
		f = model.RelationFormat_longtext
	}
	return formatNames.name(f)
}

//
// ---- proto value bridges ----
//

// protoValueToJSON converts a types.Value tree into JSON values, with omaps
// (alphabetical keys) for structs so the output stays canonical.
func protoValueToJSON(v *types.Value) any {
	switch k := v.GetKind().(type) {
	case *types.Value_NullValue:
		return nil
	case *types.Value_NumberValue:
		return k.NumberValue
	case *types.Value_StringValue:
		return k.StringValue
	case *types.Value_BoolValue:
		return k.BoolValue
	case *types.Value_ListValue:
		out := make([]any, 0, len(k.ListValue.Values))
		for _, e := range k.ListValue.Values {
			out = append(out, protoValueToJSON(e))
		}
		return out
	case *types.Value_StructValue:
		return protoStructToJSON(k.StructValue)
	}
	return nil
}

func protoStructToJSON(s *types.Struct) *omap {
	m := &omap{}
	if s == nil {
		return m
	}
	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m.set(k, protoValueToJSON(s.Fields[k]))
	}
	return m
}

// jsonToProtoValue converts decoded JSON (float64 numbers or json.Number)
// into a types.Value tree.
func jsonToProtoValue(v any) *types.Value {
	switch x := v.(type) {
	case nil:
		return &types.Value{Kind: &types.Value_NullValue{}}
	case bool:
		return &types.Value{Kind: &types.Value_BoolValue{BoolValue: x}}
	case float64:
		return &types.Value{Kind: &types.Value_NumberValue{NumberValue: x}}
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return &types.Value{Kind: &types.Value_NullValue{}}
		}
		return &types.Value{Kind: &types.Value_NumberValue{NumberValue: f}}
	case string:
		return &types.Value{Kind: &types.Value_StringValue{StringValue: x}}
	case []any:
		vals := make([]*types.Value, 0, len(x))
		for _, e := range x {
			vals = append(vals, jsonToProtoValue(e))
		}
		return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
	case map[string]any:
		return &types.Value{Kind: &types.Value_StructValue{StructValue: jsonMapToProtoStruct(x)}}
	}
	return &types.Value{Kind: &types.Value_NullValue{}}
}

func jsonMapToProtoStruct(m map[string]any) *types.Struct {
	s := &types.Struct{Fields: make(map[string]*types.Value, len(m))}
	for k, v := range m {
		s.Fields[k] = jsonToProtoValue(v)
	}
	return s
}

//
// ---- dates ----
//

// formatDate renders unix seconds in the full UTC RFC 3339 form (§3).
func formatDate(sec int64) string {
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

// parseDate accepts RFC 3339 (with offsets and fractional seconds truncated)
// and date-only strings (UTC midnight), per §3.
func parseDate(s string) (int64, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix(), true
		}
	}
	return 0, false
}

//
// ---- ids ----
//

// defaultGenerateId mints ids shaped like the editor's (24 hex chars).
func defaultGenerateId() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("random id: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// suffixLabels labels each id with its last size characters (§9a). An id
// whose suffix collides with another id's or is rejected by disallow gets no
// label and stays uncompacted — with 5 characters over CID/hex alphabets
// collisions are birthday-rare, and falling back to the full id is always
// correct under the total resolution rule. Ids no longer than size label as
// themselves.
func suffixLabels(ids []string, size int, disallow func(candidate string) bool) map[string]string {
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		if r := []rune(id); len(r) > size {
			counts[string(r[len(r)-size:])]++
		}
	}
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		r := []rune(id)
		if len(r) <= size {
			out[id] = id
			continue
		}
		suffix := string(r[len(r)-size:])
		if counts[suffix] == 1 && (disallow == nil || !disallow(suffix)) {
			out[id] = suffix
		}
	}
	return out
}
