package v2service

// typeshortcut.go is the flat body for the type endpoints: the shape a caller
// reaches for before they have read anything.
//
//	{"name":"Plant","plural_name":"Plants","icon":{"format":"emoji","emoji":"🌱"},
//	 "layout":"basic","property_definitions":[{"name":"Location","format":"select"}]}
//
// POST /objects has had this for its own resource since R7 — {type, name,
// properties, markdown} beside the full AnyBlock document — and the type
// endpoints had only the document. That document is an interchange artifact: it
// puts the display name inside `properties` (a map of values), the type's own
// settings inside `type_settings`, and the field list inside
// `type_settings.property_definitions`. Every part of that is right for a
// backup and wrong for a first request, and callers reliably send the flat
// shape instead and get a refusal.
//
// The field list keeps the name `property_definitions` rather than taking the
// `properties` an author would guess. That guess is what the format renamed
// away from, on purpose: `properties` already means a map of values at a
// document root and at the shortcut root, an array of view-available entries on
// a dataview block, and an array of match targets on a query source. Letting it
// mean "the field list" here would put two opposite meanings in sibling
// endpoints — POST /objects taking a map, POST /types an array — and make the
// array/map distinction load-bearing for parsing. A caller who sends
// `properties` as an array gets rejectMisplacedPropertyArray naming the move.

import (
	"encoding/json"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// v2TypeShortcut is the flat type body. Every member maps to one slot of the
// AnyBlock document this is rewritten into; nothing here is lossy, so a caller
// never has to fall back to the document to finish a type.
type v2TypeShortcut struct {
	Name                string          `json:"name"`
	PluralName          string          `json:"plural_name"`
	Icon                json.RawMessage `json:"icon"`
	Layout              string          `json:"layout"`
	ApiKey              string          `json:"api_key"`
	DefaultView         string          `json:"default_view"`
	DefaultTemplate     string          `json:"default_template"`
	PropertyDefinitions json.RawMessage `json:"property_definitions"`
}

// typeShortcutKeys bounds the flat body. An unknown key is named rather than
// ignored: a caller who meant the document gets told how to say so.
var typeShortcutKeys = map[string]bool{
	"name": true, "plural_name": true, "icon": true, "layout": true,
	"api_key": true, "default_view": true, "default_template": true,
	"property_definitions": true,
}

// isTypeDocument is the discriminator. It answers on any member only the
// interchange document has, NOT on formatVersion alone: this endpoint has
// always accepted a document that omits it, and a body carrying type_settings
// or a `properties` map is unambiguously the document however it is spelled.
//
// `properties` decides by its JSON type. A map is the document's slot of
// property values; an array is the field list in the wrong place, which the
// flat path names rather than silently treating as a document.
func isTypeDocument(fields map[string]json.RawMessage) bool {
	for _, member := range []string{"formatVersion", "kind", "type_settings", "blocks"} {
		if _, ok := fields[member]; ok {
			return true
		}
	}
	if raw, ok := fields["properties"]; ok && len(raw) > 0 && raw[0] == '{' {
		return true
	}
	return false
}

// typeShortcutDocument rewrites the flat body into the AnyBlock document the
// create and update paths already take, so the flat form adds a translation and
// no second implementation of type creation.
func typeShortcutDocument(fields map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	// `properties` is the guess this shape exists to catch, and it means two
	// different things depending on what the caller put in it. Name the move
	// rather than reporting an unknown key: the generic message below lists
	// property_definitions among seven others and leaves them to spot it.
	if raw, sent := fields["properties"]; sent {
		if len(raw) > 0 && raw[0] == '[' {
			return nil, v2model.ValidationFailed("the field list is named property_definitions",
				v2model.Issue{
					Path:    "/properties",
					Message: "an array here is the field list; at a body root `properties` is a map of property values, never a list of fields",
					Hint:    "rename it to `property_definitions`",
				})
		}
		return nil, v2model.ValidationFailed("`properties` here is neither a value map nor a field list",
			v2model.Issue{
				Path:    "/properties",
				Message: "the field list is `property_definitions`, an array; the display name is `name`, a string; a map here means the interchange document, which also takes `type_settings`",
			})
	}

	// `ops` and `op` are the edit channel, which only an existing type has.
	// The generic message below lists eight members and calls this an unknown
	// key, which sends a caller who knows the ops envelope looking for a
	// spelling mistake rather than telling them this verb has no list to edit.
	for _, member := range []string{"ops", "op"} {
		if _, sent := fields[member]; sent {
			return nil, v2model.ValidationFailed("ops edit a type that already exists",
				v2model.Issue{
					Path:    "/" + member,
					Message: "there is no property list to add to, remove from or reorder until the type is created",
				}.Hintf("create the type with property_definitions, then send ops to %s", v2model.NewRef(v2model.OpUpdateType)))
		}
	}

	for key := range fields {
		if !typeShortcutKeys[key] {
			return nil, v2model.ValidationFailed("unknown field in type body",
				v2model.Issue{
					Path:    "/" + key,
					Message: fmt.Sprintf("unknown key %q", key),
					Hint:    "the flat body takes name, plural_name, icon, layout, api_key, default_view, default_template, property_definitions — or send a full AnyBlock document by including \"formatVersion\":\"2.0\"",
				})
		}
	}
	raw, err := encodeEnvelope(fields)
	if err != nil {
		return nil, err
	}
	var flat v2TypeShortcut
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, v2model.ValidationFailed("decode type body: " + err.Error())
	}
	if flat.Name == "" {
		return nil, v2model.ValidationFailed("name is required",
			v2model.Issue{Path: "/name", Message: "a type needs a display name"})
	}

	settings := map[string]json.RawMessage{}
	for member, value := range map[string]string{
		"api_key": flat.ApiKey, "plural_name": flat.PluralName,
		"layout": flat.Layout, "default_view": flat.DefaultView,
		"default_template": flat.DefaultTemplate,
	} {
		if value == "" {
			continue
		}
		if settings[member], err = rawJSON(value); err != nil {
			return nil, err
		}
	}
	if len(flat.PropertyDefinitions) > 0 {
		settings["property_definitions"] = flat.PropertyDefinitions
	}

	doc := map[string]json.RawMessage{}
	if doc["formatVersion"], err = rawJSON(anyblockjson.FormatVersion); err != nil {
		return nil, err
	}
	if doc["kind"], err = rawJSON("object_type"); err != nil {
		return nil, err
	}
	// the display name lives in `properties`, the document's map of VALUES
	if doc["properties"], err = rawJSON(map[string]string{"name": flat.Name}); err != nil {
		return nil, err
	}
	if len(flat.Icon) > 0 {
		doc["icon"] = flat.Icon
	}
	if len(settings) > 0 {
		if doc["type_settings"], err = rawJSON(settings); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

// typeShortcutPatch rewrites the flat body into the PATCH shape. The same body
// serves both verbs, which is the point: create and update taking different
// shapes for one resource cost a real caller three round trips to discover.
func typeShortcutPatch(fields map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	doc, err := typeShortcutDocument(withNamePlaceholder(fields))
	if err != nil {
		return nil, err
	}
	patch := map[string]json.RawMessage{}
	if _, named := fields["name"]; named {
		patch["properties"] = doc["properties"]
	}
	if settings, ok := doc["type_settings"]; ok {
		patch["type_settings"] = settings
	}
	if icon, ok := doc["icon"]; ok {
		patch["icon"] = icon
	}
	if len(patch) == 0 {
		return nil, v2model.ValidationFailed("the patch changes nothing",
			v2model.Issue{Message: "give at least one of name, plural_name, icon, layout, default_view, default_template, property_definitions — only api_key is create-only"})
	}
	return patch, nil
}

// withNamePlaceholder lets a PATCH omit the name that a CREATE requires. The
// placeholder never reaches the patch: typeShortcutPatch copies `properties`
// only when the caller actually named one.
func withNamePlaceholder(fields map[string]json.RawMessage) map[string]json.RawMessage {
	if _, ok := fields["name"]; ok {
		return fields
	}
	out := make(map[string]json.RawMessage, len(fields)+1)
	for k, v := range fields {
		out[k] = v
	}
	out["name"] = json.RawMessage(`"unchanged"`)
	return out
}
