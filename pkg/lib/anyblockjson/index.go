package anyblockjson

// index.go implements §2c: the bundle-level index.json. Every other document
// in this format describes one object; index.json describes the set — the
// space's name, what opens on entry, and what the sidebar shows.
//
// It exists because none of that is expressible per-object. The installer
// reads it from a `profile` file at the archive root (pb.Profile, consumed by
// util/builtinobjects.inject): spaceDashboardId becomes the space's homepage,
// widgets[] become sidebar widgets, and widgets[0] is the object the install
// opens. A bundle without one imports as an undifferentiated object list.

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/index.schema.json
var indexSchemaJSON []byte

// IndexSchemaURL is the published location of the index schema.
const IndexSchemaURL = "https://schemas.anytype.io/anyblock/1.0/index.schema.json"

// IndexFileName is the name a bundle's index must have, at the bundle root.
const IndexFileName = "index.json"

// Reserved homepage values (core/domain/homepage.go). Anything else is an
// object id.
const (
	HomepageWidgets = "widgets"
	HomepageGraph   = "graph"
)

// reservedWidgetTargets are the widget targets that name a built-in listing
// rather than an object in the bundle (core/block/editor/widget).
var reservedWidgetTargets = map[string]struct{}{
	"favorite": {}, "recent": {}, "set": {}, "collection": {},
	"allObjects": {}, "recentOpen": {},
}

// IsReservedWidgetTarget reports whether target names a built-in listing, in
// which case it does not have to resolve to an object in the bundle.
func IsReservedWidgetTarget(target string) bool {
	_, ok := reservedWidgetTargets[target]
	return ok
}

// IsReservedHomepage reports whether homepage names a built-in screen rather
// than an object in the bundle.
func IsReservedHomepage(homepage string) bool {
	return homepage == HomepageWidgets || homepage == HomepageGraph
}

// Widget is one sidebar widget.
type Widget struct {
	Target string `json:"target"`
	Layout string `json:"layout"`
	Limit  int32  `json:"limit"`
}

// Index is a bundle's index.json.
type Index struct {
	Schema      string `json:"$schema"`
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconEmoji   string `json:"iconEmoji"`
	// IconImage is the object id of an image in the bundle — the same thing
	// iconImage means on any object, so an author never has to remember a
	// second convention. The installer resolves the space icon by image *name*
	// (builtinobjects.getNewAvatarId queries name + image layout), so the
	// wiring looks the name up from this id; that asymmetry is the wire
	// format's, not the author's. Needs the image object and its file in the
	// archive, which is why a generated bundle sets IconEmoji instead.
	IconImage string `json:"iconImage"`
	// Entrypoint is the object opened once, right after the space is created
	// — the first thing a user ever sees. Distinct from Homepage, which is
	// what opens on every later entry, and deliberately not the widget order:
	// the wire format carries the entry point as widgets[0]
	// (builtinobjects.inject), but making authors express it by sorting a list
	// means reordering the sidebar silently changes what opens.
	Entrypoint string   `json:"entrypoint"`
	Homepage   string   `json:"homepage"`
	Widgets    []Widget `json:"widgets"`
}

// EntryPoint returns the entry point the bundle *declares*: the entrypoint
// field, or for a bundle written before it existed, the first widget naming an
// object.
//
// TEMPORARY: this is intent, not behaviour. pb.Profile has no field for an
// entry point — builtinobjects.inject opens widgets[0].targetObjectId — so
// until the profile handling grows one, what actually opens is
// EffectiveEntryPoint. The two differ exactly when a bundle declares an
// entrypoint that is not its first widget, which is worth reporting.
func (i *Index) EntryPoint() string {
	if i.Entrypoint != "" {
		return i.Entrypoint
	}
	for _, w := range i.Widgets {
		if !IsReservedWidgetTarget(w.Target) {
			return w.Target
		}
	}
	return ""
}

// EffectiveEntryPoint returns what the installer opens *today*: the first
// widget naming an object, which is all pb.Profile can express. Compare with
// EntryPoint to detect a declared entry point that will not be honoured yet.
func (i *Index) EffectiveEntryPoint() string {
	for _, w := range i.Widgets {
		if !IsReservedWidgetTarget(w.Target) {
			return w.Target
		}
	}
	return ""
}

// SpaceHomepage returns what opens on entering the space: the declared
// homepage, else the entry point. Only an explicit reserved value gives up a
// real page — omitting homepage does not.
func (i *Index) SpaceHomepage() string {
	if i.Homepage != "" {
		return i.Homepage
	}
	return i.EntryPoint()
}

var compileIndexSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(indexSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("decode embedded index schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(IndexSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("add index schema resource: %w", err)
	}
	sch, err := c.Compile(IndexSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile index schema: %w", err)
	}
	return sch, nil
})

// UnmarshalIndex validates data against the index schema and decodes it.
// Errors wrap *ValidationError with path-addressed issues, like
// Unmarshal.
//
// Whether the ids it names exist is a cross-document question the wiring
// answers, not this package: an index is valid on its own terms while
// pointing at an object no document defines.
func UnmarshalIndex(data []byte) (*Index, error) {
	raw, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, &ValidationError{Issues: []Issue{{Message: fmt.Sprintf("invalid JSON: %v", err)}}}
	}
	sch, err := compileIndexSchema()
	if err != nil {
		return nil, fmt.Errorf("embedded index schema: %w", err)
	}
	if err := sch.Validate(raw); err != nil {
		ve := &ValidationError{}
		printer := message.NewPrinter(language.English)
		var flatten func(e *jsonschema.ValidationError)
		flatten = func(e *jsonschema.ValidationError) {
			if len(e.Causes) == 0 {
				ve.Issues = append(ve.Issues, Issue{
					Path:    jsonPath(e.InstanceLocation),
					Message: schemaIssueMessage(e, printer),
				})
				return
			}
			for _, c := range e.Causes {
				flatten(c)
			}
		}
		if verr, ok := err.(*jsonschema.ValidationError); ok {
			flatten(verr)
		} else {
			ve.Issues = append(ve.Issues, Issue{Message: err.Error()})
		}
		return nil, ve
	}

	var idx Index
	if err := jsonUnmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	return &idx, nil
}

// MarshalIndex renders an index in the canonical byte form.
func MarshalIndex(idx *Index) ([]byte, error) {
	if idx == nil {
		return nil, fmt.Errorf("nil index")
	}
	doc := &omap{}
	doc.set("$schema", IndexSchemaURL)
	doc.set("version", FormatVersion)
	doc.setNonEmpty("name", idx.Name)
	doc.setNonEmpty("description", idx.Description)
	doc.setNonEmpty("iconEmoji", idx.IconEmoji)
	doc.setNonEmpty("iconImage", idx.IconImage)
	doc.setNonEmpty("entrypoint", idx.Entrypoint)
	doc.setNonEmpty("homepage", idx.Homepage)

	var widgets []any
	for _, w := range idx.Widgets {
		if w.Target == "" {
			continue
		}
		wm := &omap{}
		wm.set("target", w.Target)
		// link is the default layout and is omitted, like every other
		// default in this format
		if w.Layout != "" && w.Layout != "link" {
			wm.set("layout", w.Layout)
		}
		wm.setNonEmpty("limit", w.Limit)
		widgets = append(widgets, wm)
	}
	doc.setNonEmpty("widgets", widgets)
	return marshalCanonical(doc)
}
