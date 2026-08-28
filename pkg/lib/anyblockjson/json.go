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
	"math"
	"sort"
	"strconv"
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

// sortedNestedOmap renders a two-level string map into nested omaps, both
// levels sorted by key (§4 canon). Returns nil for an empty map so
// setNonEmpty omits the slot.
func sortedNestedOmap(m map[string]map[string]string) *omap {
	if len(m) == 0 {
		return nil
	}
	out := &omap{}
	for _, outer := range sortedStringKeys(m) {
		inner := &omap{}
		for _, k := range sortedStringKeys(m[outer]) {
			inner.set(k, m[outer][k])
		}
		out.set(outer, inner)
	}
	return out
}

func sortedStringKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
	model.SmartBlockType_AccountOld:      "account_old",
	model.SmartBlockType_Page:            "page",
	model.SmartBlockType_ProfilePage:     "profile_page",
	model.SmartBlockType_Home:            "home",
	model.SmartBlockType_Archive:         "archive",
	model.SmartBlockType_Widget:          "widget",
	model.SmartBlockType_File:            "file",
	model.SmartBlockType_Template:        "template",
	model.SmartBlockType_BundledTemplate: "bundled_template",
	// the three definition kinds say "property" where the store says
	// "relation": the product calls these things properties, and the format
	// already did everywhere else — the block type is `featured_properties`,
	// the shape is `propertyDefinition`, the legend `property_internal_keys`.
	// One word for one concept (§15 #14); the model constants are the store's
	// own names and stay.
	model.SmartBlockType_BundledRelation:   "bundled_property",
	model.SmartBlockType_SubObject:         "sub_object",
	model.SmartBlockType_BundledObjectType: "bundled_object_type",
	model.SmartBlockType_AnytypeProfile:    "anytype_profile",
	model.SmartBlockType_Date:              "date",
	// the space's own object holds the space's SETTINGS — its name, icon,
	// homepage — not the space itself, and `space_settings` says that where
	// `workspace` said something the product no longer calls anything. One
	// per space (77 in a 77-space corpus), machine-written and never
	// authored, so the rename costs nothing but is a wire value: after the
	// freeze it would cost a version.
	model.SmartBlockType_Workspace:              "space_settings",
	model.SmartBlockType_STRelation:             "property",
	model.SmartBlockType_STType:                 "object_type",
	model.SmartBlockType_STRelationOption:       "property_option",
	model.SmartBlockType_SpaceView:              "space_view",
	model.SmartBlockType_Identity:               "identity",
	model.SmartBlockType_Participant:            "participant",
	model.SmartBlockType_MissingObject:          "missing_object",
	model.SmartBlockType_FileObject:             "file_object",
	model.SmartBlockType_NotificationObject:     "notification",
	model.SmartBlockType_DevicesObject:          "devices",
	model.SmartBlockType_ChatObjectDeprecated:   "chat_object",
	model.SmartBlockType_ChatDerivedObject:      "chat",
	model.SmartBlockType_AccountObject:          "account",
	model.SmartBlockType_DiscussionObject:       "discussion",
	model.SmartBlockType_TechSpaceObject:        "tech_space",
	model.SmartBlockType_TechSpaceVirtualObject: "tech_space_virtual",
})

// textStyleNames maps text styles to JSON block types. Header4 is absent on
// purpose: deprecated Header4 blocks export as heading3 (§5).
var textStyleNames = newEnumNames(map[model.BlockContentTextStyle]string{
	model.BlockContentText_Paragraph:     "paragraph",
	model.BlockContentText_Header1:       "heading_1",
	model.BlockContentText_Header2:       "heading_2",
	model.BlockContentText_Header3:       "heading_3",
	model.BlockContentText_Quote:         "quote",
	model.BlockContentText_Code:          "code",
	model.BlockContentText_Title:         "title",
	model.BlockContentText_Checkbox:      "checkbox",
	model.BlockContentText_Marked:        "bulleted_list_item",
	model.BlockContentText_Numbered:      "numbered_list_item",
	model.BlockContentText_Toggle:        "toggle",
	model.BlockContentText_Description:   "description",
	model.BlockContentText_Callout:       "callout",
	model.BlockContentText_ToggleHeader1: "toggle_heading_1",
	model.BlockContentText_ToggleHeader2: "toggle_heading_2",
	model.BlockContentText_ToggleHeader3: "toggle_heading_3",
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
	model.BlockContentLatex_GoogleMaps:    "google_maps",
	model.BlockContentLatex_Miro:          "miro",
	model.BlockContentLatex_Figma:         "figma",
	model.BlockContentLatex_Twitter:       "twitter",
	model.BlockContentLatex_OpenStreetMap: "open_street_map",
	model.BlockContentLatex_Reddit:        "reddit",
	model.BlockContentLatex_Facebook:      "facebook",
	model.BlockContentLatex_Instagram:     "instagram",
	model.BlockContentLatex_Telegram:      "telegram",
	model.BlockContentLatex_GithubGist:    "github_gist",
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
	model.BlockContentWidget_CompactList: "compact_list",
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
	model.BlockContentDataviewFilter_NotEqual:       "not_equal",
	model.BlockContentDataviewFilter_Greater:        "greater",
	model.BlockContentDataviewFilter_Less:           "less",
	model.BlockContentDataviewFilter_GreaterOrEqual: "greater_or_equal",
	model.BlockContentDataviewFilter_LessOrEqual:    "less_or_equal",
	model.BlockContentDataviewFilter_Like:           "contains",
	model.BlockContentDataviewFilter_NotLike:        "not_contains",
	model.BlockContentDataviewFilter_In:             "in",
	model.BlockContentDataviewFilter_NotIn:          "not_in",
	model.BlockContentDataviewFilter_Empty:          "empty",
	model.BlockContentDataviewFilter_NotEmpty:       "not_empty",
	model.BlockContentDataviewFilter_AllIn:          "all_in",
	model.BlockContentDataviewFilter_NotAllIn:       "not_all_in",
	model.BlockContentDataviewFilter_ExactIn:        "exact_in",
	model.BlockContentDataviewFilter_NotExactIn:     "not_exact_in",
	model.BlockContentDataviewFilter_Exists:         "exists",
})

// countingPresets take a day count from the filter's `value` rather than
// naming a fixed period: getDateRange reads it as a NUMBER OF DAYS for these
// two and for no others (pkg/lib/database/quickoptions.go — the exactDate
// default reads the same field, as the timestamp it is). Without a value the
// count is 0, which silently means "today" — but only where the range reaches
// the query at all, which takes a date property and one of the six conditions
// below (transformDateFilter, datePresetConditions). Everywhere else the
// preset is inert and the count is never read, which is why the validation
// rule this set feeds is scoped and export writes the count regardless.
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
	"number_of_days_ago": {},
	"number_of_days_now": {},
}

// datePresetConditions are the conditions that apply a preset's day range at
// all — the condition half of transformDateFilter's gate. It computes the
// range for every DATE filter (a filter of any other format it returns before
// computing anything, which is the other half), then substitutes the range
// into the filter for these six and no others
// (pkg/lib/database/quickoptions.go): on any other condition — the
// presence-only leaves above all — it returns the filter unchanged, so the
// preset is inert and its day count is never read. That is why a counting
// preset without a count is an error here and nothing at all there.
var datePresetConditions = map[string]struct{}{
	"equal":            {},
	"in":               {},
	"less":             {},
	"greater":          {},
	"less_or_equal":    {},
	"greater_or_equal": {},
}

var datePresetNames = newEnumNames(map[model.BlockContentDataviewFilterQuickOption]string{
	model.BlockContentDataviewFilter_Yesterday:       "yesterday",
	model.BlockContentDataviewFilter_Today:           "today",
	model.BlockContentDataviewFilter_Tomorrow:        "tomorrow",
	model.BlockContentDataviewFilter_LastWeek:        "last_week",
	model.BlockContentDataviewFilter_CurrentWeek:     "current_week",
	model.BlockContentDataviewFilter_NextWeek:        "next_week",
	model.BlockContentDataviewFilter_LastMonth:       "last_month",
	model.BlockContentDataviewFilter_CurrentMonth:    "current_month",
	model.BlockContentDataviewFilter_NextMonth:       "next_month",
	model.BlockContentDataviewFilter_NumberOfDaysAgo: "number_of_days_ago",
	model.BlockContentDataviewFilter_NumberOfDaysNow: "number_of_days_now",
	model.BlockContentDataviewFilter_LastYear:        "last_year",
	model.BlockContentDataviewFilter_CurrentYear:     "current_year",
	model.BlockContentDataviewFilter_NextYear:        "next_year",
})

var aggregationNames = newEnumNames(map[model.BlockContentDataviewRelationFormulaType]string{
	model.BlockContentDataviewRelation_Count:           "count",
	model.BlockContentDataviewRelation_CountValue:      "count_value",
	model.BlockContentDataviewRelation_CountDistinct:   "count_distinct",
	model.BlockContentDataviewRelation_CountEmpty:      "count_empty",
	model.BlockContentDataviewRelation_CountNotEmpty:   "count_not_empty",
	model.BlockContentDataviewRelation_PercentEmpty:    "percent_empty",
	model.BlockContentDataviewRelation_PercentNotEmpty: "percent_not_empty",
	model.BlockContentDataviewRelation_MathSum:         "sum",
	model.BlockContentDataviewRelation_MathAverage:     "average",
	model.BlockContentDataviewRelation_MathMedian:      "median",
	model.BlockContentDataviewRelation_MathMin:         "min",
	model.BlockContentDataviewRelation_MathMax:         "max",
	model.BlockContentDataviewRelation_Range:           "range",
})

// FormatName returns the format's canonical JSON name for a property format
// ("text", "select", "objects", …) — the one vocabulary shared by documents
// and API surfaces. It is the exported form of formatName, so
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
//
// It is TOTAL over model.RelationFormat, shorttext's fold aside, and that is
// a load-bearing property rather than tidiness: a relation document states
// its format on the envelope as a required NAME (§2d), so a stored format
// this map cannot name is a relation object Marshal cannot export. "map"
// (RelationFormat_map) is in the vocabulary for exactly that reason — the
// API does not serve it, but 72 production relation documents carry format
// 102 (every one the bundled templatePlaceholders relation), and the §3 note
// that names exist for internal formats (emoji, objects, properties) already
// covers it. TestFormatNames_TotalOverModelEnum pins the totality, so a
// format added to the model without a name here fails a test instead of an
// export.
var formatNames = newEnumNames(map[model.RelationFormat]string{
	model.RelationFormat_longtext:  "text",
	model.RelationFormat_number:    "number",
	model.RelationFormat_status:    "select",
	model.RelationFormat_tag:       "multi_select",
	model.RelationFormat_date:      "date",
	model.RelationFormat_file:      "files",
	model.RelationFormat_checkbox:  "checkbox",
	model.RelationFormat_url:       "url",
	model.RelationFormat_email:     "email",
	model.RelationFormat_phone:     "phone",
	model.RelationFormat_emoji:     "emoji",
	model.RelationFormat_object:    "objects",
	model.RelationFormat_relations: "properties",
	model.RelationFormat_map:       "map",
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
	model.ObjectType_basic:      "basic",
	model.ObjectType_profile:    "profile",
	model.ObjectType_todo:       "todo",
	model.ObjectType_set:        "set",
	model.ObjectType_objectType: "object_type",
	// the wire names for the three relation-flavored layouts say "property",
	// like the kinds above — only the NAME moves, the model constants they
	// map from are the store's
	model.ObjectType_relation:            "property",
	model.ObjectType_file:                "file",
	model.ObjectType_dashboard:           "dashboard",
	model.ObjectType_image:               "image",
	model.ObjectType_note:                "note",
	model.ObjectType_space:               "space",
	model.ObjectType_bookmark:            "bookmark",
	model.ObjectType_relationOptionsList: "property_options_list",
	model.ObjectType_relationOption:      "property_option",
	model.ObjectType_collection:          "collection",
	model.ObjectType_audio:               "audio",
	model.ObjectType_video:               "video",
	model.ObjectType_date:                "date",
	model.ObjectType_spaceView:           "space_view",
	model.ObjectType_participant:         "participant",
	model.ObjectType_pdf:                 "pdf",
	model.ObjectType_chatDeprecated:      "chat_deprecated",
	model.ObjectType_chatDerived:         "chat_derived",
	model.ObjectType_tag:                 "tag",
	model.ObjectType_notification:        "notification",
	model.ObjectType_missingObject:       "missing_object",
	model.ObjectType_devices:             "devices",
	model.ObjectType_discussion:          "discussion",
})

// propertyVocabulary is one stored property key's name-over-number contract
// (§3): the stored value is a number whose meaning is a proto enum, and the
// format writes the NAME — a bare integer would be an opaque enum in an
// otherwise self-describing format. All four surfaces that touch such a key
// ask this one struct, so they cannot disagree about what a name means:
// export substitutes the name for an in-vocabulary number (and refuses to
// write a stored string the vocabulary does not name — there is no way to
// write it, I1); import maps a known name back to its number; validation
// refuses an unknown name as an ERROR, because the typo would otherwise
// import as a raw string onto a number-format detail, where every consumer
// reads it with an int getter and silently sees the enum's zero; and a raw
// number outside the vocabulary passes every surface unchanged, because a
// stored value round-trips as its number rather than being lost.
type propertyVocabulary struct {
	what  string               // the concept a refusal names: "layout", "align", …
	has   func(string) bool    // is this string a vocabulary name
	value func(string) float64 // name → stored number (only for names has() admits)
	name  func(float64) string // stored number → name; "" outside the vocabulary
	names func() []string      // the vocabulary, sorted, for refusals that state it
}

// vocabularyOf adapts an enumNames table to the property contract. The name
// direction reads the number the way every consumer of these details does —
// int32 of the float — GUARDED the way relationFormatName is: int32(NaN) is 0
// on this machine, and without the guard a NaN stored on a layout key exports
// as the enum's zero's name, a false claim that then imports as a permanent
// silent rewrite. A fraction, an infinity or an out-of-int32 number likewise
// has no name and round-trips as the number it is.
func vocabularyOf[T ~int32](e enumNames[T], what string) propertyVocabulary {
	return propertyVocabulary{
		what:  what,
		has:   e.has,
		value: func(n string) float64 { return float64(e.value(n)) },
		name: func(n float64) string {
			if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) ||
				n < math.MinInt32 || n > math.MaxInt32 {
				return ""
			}
			return e.name(T(int32(n)))
		},
		names: func() []string {
			out := make([]string, 0, len(e.toVal))
			for n := range e.toVal {
				out = append(out, n)
			}
			sort.Strings(out)
			return out
		},
	}
}

// quotedNames renders the vocabulary for a refusal — 'a', 'b', 'c' — in the
// same quoting the schema's own enum errors use, so the two refusal channels
// read as one.
func (v propertyVocabulary) quotedNames() string {
	names := v.names()
	for i, n := range names {
		names[i] = "'" + n + "'"
	}
	return strings.Join(names, ", ")
}

var layoutVocabulary = vocabularyOf(layoutNames, "layout")

// alignVocabulary spells a model.BlockAlign — the enum a block's `align`,
// a view column's `align` and the layoutAlign DETAIL all store. One concept,
// one spelling (§15 #14): the four names were already the format's alignment
// vocabulary twice over before the property joined.
var alignVocabulary = vocabularyOf(alignNames, "align")

// originNames maps model.ObjectOrigin — how an object entered its space —
// to the format's names: the proto's own identifiers, snake_cased where they
// are camelCase, because there is no established public vocabulary to defer
// to (the REST API stores the same number). TOTAL over the proto enum,
// pinned by TestNamedEnum_VocabulariesTotalOverModelEnums: a member added to
// the proto without a name here would export as a bare integer again.
var originNames = newEnumNames(map[model.ObjectOrigin]string{
	model.ObjectOrigin_none:             "none",
	model.ObjectOrigin_clipboard:        "clipboard",
	model.ObjectOrigin_dragAndDrop:      "drag_and_drop",
	model.ObjectOrigin_import:           "import",
	model.ObjectOrigin_webclipper:       "webclipper",
	model.ObjectOrigin_sharingExtension: "sharing_extension",
	model.ObjectOrigin_usecase:          "usecase",
	model.ObjectOrigin_builtin:          "builtin",
	model.ObjectOrigin_bookmark:         "bookmark",
	model.ObjectOrigin_api:              "api",
})

var originVocabulary = vocabularyOf(originNames, "origin")

// importTypeNames maps model.ImportType — which importer brought an
// import/usecase-originated object in. The names are the proto identifiers
// lowercased; `pb` stays `pb` (the protobuf export format, the store's own
// name for it) rather than gaining an invented alias. Note the enum's ZERO
// is notion — the sharpest reason this key had to be named or die: an
// accepted-then-zeroed string here did not read as "unset", it read as a
// false claim that the object came from Notion.
var importTypeNames = newEnumNames(map[model.ImportType]string{
	model.Import_Notion:   "notion",
	model.Import_Markdown: "markdown",
	model.Import_External: "external",
	model.Import_Pb:       "pb",
	model.Import_Html:     "html",
	model.Import_Txt:      "txt",
	model.Import_Csv:      "csv",
	model.Import_Obsidian: "obsidian",
})

var importTypeVocabulary = vocabularyOf(importTypeNames, "import type")

// imageKindNames maps model.ImageKind — what an image was uploaded FOR — to
// the format's names: the proto identifiers snake_cased. TOTAL over the
// proto enum, pinned below.
//
// This is the fourth of the five 2026-08 bare-integer enums to be named, and
// it is named on the same measured ground the others were left as numbers:
// imageKind occurs on 4,079 file objects across the 77-space corpus — 4,053
// automatically_added, 23 icon, 3 basic-or-cover — where widgetLayout is on
// 13 documents and headerRelationsLayout on 51. A reader of an export saw a
// bare 3 and had no way to learn what it meant.
//
// The two small ones were once recorded as 13 and ZERO, and the zero was
// wrong: headerRelationsLayout is on 51 documents and holds two distinct
// values (44 ones, 7 zeros), which is what typesettings.go already says
// about it. The decision to leave it bare therefore rests on VOLUME alone
// now, not on "nothing writes it" — 51 documents against imageKind's 4,079
// — and it is the weakest of the five verdicts on that account.
//
// Note the enum's ZERO is `basic`, and the app never STORES it:
// makeInitialDetails returns early for Basic, so the key is absent rather
// than 0 on an ordinary upload. The name exists anyway because a total
// vocabulary is what keeps a future writer of 0 from exporting a bare
// integer, and because absent and basic must not be forced to differ.
var imageKindNames = newEnumNames(map[model.ImageKind]string{
	model.ImageKind_Basic:              "basic",
	model.ImageKind_Cover:              "cover",
	model.ImageKind_Icon:               "icon",
	model.ImageKind_AutomaticallyAdded: "automatically_added",
})

var imageKindVocabulary = vocabularyOf(imageKindNames, "image kind")

// viewTypeVocabulary is not a property vocabulary — no stored detail key
// maps to it — but §2a's default_view member shares the reading, and the
// guarded adapter is how both enum members stopped naming NaN.
var viewTypeVocabulary = vocabularyOf(viewTypeNames, "view type")

// namedEnumProperties maps each stored property key whose number the format
// names onto its vocabulary (§3). The three layout keys hold an
// ObjectTypeLayout. The remaining layout-ish bundled keys are left as
// numbers deliberately: layoutWidth is a fraction, not an enum, and
// widgetLayout/headerRelationsLayout hold enums almost nothing writes — 13
// and 51 occurrences across 28,831 real exported documents, against
// imageKind's 4,079.
var namedEnumProperties = map[string]propertyVocabulary{
	"recommendedLayout": layoutVocabulary,
	"layout":            layoutVocabulary,
	"resolvedLayout":    layoutVocabulary,
	// layoutAlign is the object's own page alignment — the one key of the
	// five 2026-08 bare-integer enums a user can set (readonly false in the
	// bundled table), which is why it is NAMED rather than deprecated: it
	// survives the §2a admission on type documents as "the type object's own
	// page display, set by a person where non-zero", and the app writes it
	// as a model.BlockAlign (participant/profile editors stamp AlignCenter;
	// the align UI sets the rest). Before this entry, `layout_align:
	// "center"` VALIDATED and stored the string on a number detail — every
	// int getter answered 0, left — while the reader of an export saw a bare
	// 1 beside a named `layout` and had no way to learn what it meant.
	"layoutAlign": alignVocabulary,
	// origin and importType are the object's PROVENANCE — how it entered the
	// space it was exported from — and they are NAMED rather than deprecated
	// on the format's own precedent: the §2a admission dropped `origin` from
	// TYPE documents as install provenance precisely because "on ordinary
	// objects origin is real provenance and stays", and §2f drops both only
	// on bundled-identical property documents. The corpus agrees it is real:
	// all TEN origin values occur across 15,943 documents (import 6,463 ·
	// bookmark 2,444 · api 2,293 · webclipper 2,080 · usecase 1,110 ·
	// clipboard 449 · none 425 · builtin 333 · drag_and_drop 301 ·
	// sharing_extension 45) — a reader can tell an object a person clipped
	// from one a pipeline made, which is not the class of syncStatus but the
	// class of createdDate (which the import pipeline deliberately preserves
	// as OriginalCreatedTimestamp) and creator (written as attribution).
	//
	// Deprecation was weighed: heart's own import pipeline re-stamps both on
	// every snapshot (objectcreator.injectImportDetails), so nothing
	// downstream of an app import acts on the carried value. But the format
	// is a READ surface first, and a transient key must describe a MOMENT
	// rather than the object — origin describes the object's history. The
	// pair travels together: objectorigin.go writes importType only beside
	// an import/usecase origin.
	"origin":     originVocabulary,
	"importType": importTypeVocabulary,
	// imageKind records what an image was uploaded FOR — a cover, an icon,
	// or added automatically by a pipeline. It is NAMED rather than
	// deprecated even though heart has one writer and no reader, because
	// the format is a read surface first: a person looking at a file object
	// wants to know why the image is there.
	//
	// Deprecation was weighed and is still arguable. The behaviour a client
	// actually runs on is `isHiddenDiscovery`, which travels independently
	// and is in perfect lockstep with the automatically_added member — 4,053
	// of 4,053 in the corpus — so the one live consumer (the client's
	// subscription filter, which hides auto-added images) survives without
	// this key. The two anytype-ts filters that DO read imageKind, in the
	// icon and cover pickers, are both commented out. What would be lost is
	// the 26 documents where the key says icon or cover and nothing else
	// does, and even those are recoverable from whichever object references
	// the image through icon_image or cover_id.
	//
	// It stays because naming costs one entry and drops nothing, while
	// dropping 4,079 documents' worth of a stored, user-visible-in-principle
	// fact is a decision the freeze does not need to take.
	"imageKind": imageKindVocabulary,
}

// namedEnumProperty answers whether a stored key is written by name, and
// with which vocabulary.
func namedEnumProperty(key string) (propertyVocabulary, bool) {
	v, ok := namedEnumProperties[key]
	return v, ok
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

// The range of unix seconds RFC 3339 can represent: its year is four digits,
// so a timestamp outside years 0000–9999 has no form in it. The bound is not a
// matter of taste — outside it, Format produces a string (`57482-01-22…`,
// `-0044-03-15…`) that parseDate cannot read back, so a caller that writes one
// anyway has silently changed the value's type on the way home.
//
// It is reachable from ordinary data: a millisecond timestamp stored where
// seconds belong (1751791445000) lands in year 57482, and that mistake is
// common enough to be a corruption class rather than a curiosity.
var (
	minDateSec = time.Date(0, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	maxDateSec = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC).Unix()
)

// formatDate renders unix seconds in the full UTC RFC 3339 form (§3),
// reporting false when the value has no representation there. Callers must
// handle false rather than write the string anyway: parseDate cannot read it
// back, and the round trip would turn a date into a string.
func formatDate(sec int64) (string, bool) {
	if sec < minDateSec || sec > maxDateSec {
		return "", false
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339), true
}

// formatDateValue is formatDate for a stored property value, which is a
// float64. It range-checks before converting: a float too large for an int64
// converts to an implementation-defined value in Go, so checking after would
// be checking the wrong number.
func formatDateValue(f float64) (string, bool) {
	if math.IsNaN(f) || f < float64(minDateSec) || f > float64(maxDateSec) {
		return "", false
	}
	return formatDate(int64(f))
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

// maxIdLen bounds every id this format writes: the block charset (§4) and the
// table inner charset (§6.1) both stop at 64 characters.
const maxIdLen = 64

// sanitizeBlockId maps a stored id onto the schema's block charset
// [A-Za-z0-9_-]{1,64}. Stored ids are not required to match it — legacy
// accounts hold dots and slashes, and a caller's GenerateId may derive ids from
// file paths — and writing one verbatim produces a document Validate rejects.
// Every replacement is ASCII, so the length bound can be applied to bytes.
func sanitizeBlockId(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isLabelRune(r) || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "b"
	}
	if len(out) > maxIdLen {
		out = out[:maxIdLen]
	}
	return out
}

// uniqueLabel returns base, or base with a "_2", "_3", … suffix — whichever is
// the first one taken() rejects. The result stays inside maxIdLen, so
// disambiguating never pushes an id past the charset bound.
func uniqueLabel(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		suffix := "_" + strconv.Itoa(n)
		trimmed := base
		if len(trimmed)+len(suffix) > maxIdLen {
			trimmed = trimmed[:maxIdLen-len(suffix)]
		}
		if candidate := trimmed + suffix; !taken(candidate) {
			return candidate
		}
	}
}

// isValidTableInnerId reports whether s matches the schema's tableInnerId
// pattern ^[A-Za-z0-9_]{1,64}$ — the local-label charset, which excludes '-'
// because that separates a derived cell id (§6.1).
func isValidTableInnerId(s string) bool {
	return !isInvalidLocalLabel(s)
}

// defaultGenerateId mints ids shaped like the editor's (24 hex chars).
func defaultGenerateId() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("random id: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// mintedSuffixLabels is the doc-local relabeler (§9a): it labels an id with
// its last size characters ONLY when the id matches a machine-minted shape
// (isMintedLocalId). The rule is deliberately inverted from "relabel unless
// the label charset is dirty": a non-opaque id usually carries meaning —
// `dataview` is a documented constant, `title`/`header`/`featuredRelations`
// are structural, imported documents carry human-readable ids — and
// relabeling those destroys information for zero benefit, while a false
// negative on a minted id merely costs a few tokens.
//
// The census counts EVERY id — the ids that never relabel included, with an
// id no longer than size counting as itself — so a label can neither equal
// another served id nor be an ambiguous suffix of one. disallow rejects
// candidates the caller reserves on top of that (every id that stays full,
// via the fullIds avoid-set, plus charset rules).
func mintedSuffixLabels(ids []string, size int, disallow func(candidate string) bool) map[string]string {
	counts := make(map[string]int, len(ids))
	for _, id := range ids {
		if r := []rune(id); len(r) > size {
			counts[string(r[len(r)-size:])]++
		} else {
			counts[id]++
		}
	}
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if !isMintedLocalId(id) {
			continue
		}
		suffix := id[len(id)-size:] // minted shapes are ASCII and longer than size
		if counts[suffix] == 1 && (disallow == nil || !disallow(suffix)) {
			out[id] = suffix
		}
	}
	return out
}

// IsCompactLabelShaped reports whether s has the exact shape of a served
// compact label: compactIdMinLen lowercase-hex characters. Every label the
// relabeler mints is the 5-char hex tail of a minted id (isMintedLocalId),
// so this is the serving layer's cheap tell for "this id probably came off a
// default read" where no owned-id baseline exists to check against.
func IsCompactLabelShaped(s string) bool {
	return isHexLower(s, compactIdMinLen)
}

// isMintedLocalId recognises the machine-minted doc-local id shapes — the
// only ids relabeling may touch. Worked out from the actual minting sites:
//
//   - 24-char lowercase hex: bson.NewObjectId().Hex() — every editor-minted
//     block, table-row and table-column id (core/block/simple, the table
//     editor) — and this package's own defaultGenerateId (12 random bytes).
//   - RFC-4122 UUID (8-4-4-4-12 lowercase hex): uuid.New().String() —
//     dataview view ids.
//
// Derived cell ids (`rowId-colId`) are reserved by the census even though a
// cell carries no id in the flat form: a cell's suffix IS its column's, so
// unless both are counted the column wins the bucket alone and compacts to a
// label its own cells share in the live object. Anything unrecognised stays full: a false
// negative costs a few tokens, a false positive destroys a meaningful
// identifier.
func isMintedLocalId(id string) bool {
	return isHexLower(id, 24) || isUuidShaped(id)
}

// isHexLower reports whether s is exactly n lowercase-hex characters.
func isHexLower(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < n; i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// isUuidShaped reports the 8-4-4-4-12 lowercase-hex UUID shape.
func isUuidShaped(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return false
			}
		}
	}
	return true
}
