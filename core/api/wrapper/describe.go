package wrapper

// describe.go assembles the prompt-ready property table from
// GET /types/{type} plus the space's property index and live option lists;
// the server-side types/{type}/schema endpoint is currently a 501 stub. The
// result is composed wrapper-side.
//
// What describe answers is "what may I set on an object of this type"
// (§8.33). It used to answer a different question — "what does this type
// RECOMMEND", the curated lists a client renders in its properties panel —
// and served that under a footer telling the caller to use those keys in
// create and set_properties. The two sets are not the same in either
// direction: `page` recommends `createdDate`, `creator` and `lastModifiedBy`
// (settable: no, no and pointlessly) and recommends neither `name` nor
// `description`, both of which every object takes. Asked to set a
// description, a small model read the recommended list, did not find one,
// and answered that the type has no such property — a task the API performs
// in one call. So describe now reports the settable set, and the type's own
// list survives as a SECTION of it: which properties a type cares about is
// real signal, it was just never the whole answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// describeProperty is one row of describe's machine shape. Key is the
// DOCUMENT spelling — the display name, since reads serve ?keys=name (D5) —
// because it is the spelling the model will use on every other surface.
type describeProperty struct {
	Key     string   `json:"key"`
	Name    string   `json:"name,omitempty"`
	Format  string   `json:"format"`
	Options []string `json:"options,omitempty"`
	// MoreOptions marks a truncated option list (C10 — tag-like properties
	// can hold thousands).
	MoreOptions bool `json:"moreOptions,omitempty"`
	// OptionsUnavailable marks a select whose option listing FAILED — which
	// must render differently from a select that genuinely has no options,
	// or the model is invited to invent a name the A2 guard then rejects.
	OptionsUnavailable bool `json:"optionsUnavailable,omitempty"`
	// OnType marks a property the TYPE names (its recommended/featured
	// lists) — a subset of the settable set, not a bound on it.
	OnType bool `json:"onType,omitempty"`
	// ReadOnly marks an output-only property (v2model, SPEC §4a): a read
	// serves it, set_properties refuses it.
	ReadOnly bool `json:"readOnly,omitempty"`
	// restKey is the REST api key backing Key (the row's Key, or a
	// definition's internal_key) — unexported because it exists only to build
	// route paths (/properties/{key}/options), which take the api spelling,
	// never the display name Key now carries.
	restKey string
}

// describeResult is describe's machine shape. Properties carries the type's
// own rows first, then the rest of the space's, then the read-only ones —
// one array, with the section carried by the row flags.
type describeResult struct {
	Type       string             `json:"type"`
	Name       string             `json:"name,omitempty"`
	Properties []describeProperty `json:"properties"`
}

// describeOptionsLimit bounds the per-property option listing.
const describeOptionsLimit = 25

// describeSettableLimit bounds the off-type settable listing. A space's
// property index is a few dozen rows in practice; the bound exists so a
// pathological space degrades into a count rather than into a prompt.
const describeSettableLimit = 120

// alwaysSettableProperties are settable on every object but appear in
// NEITHER source describe reads: `name` is a hidden bundled relation, so
// GET /properties (which excludes hidden ones) does not list it, and no
// type recommends it because clients render it as the title rather than as
// a property row. It is nonetheless the single most-set property on the
// surface — create takes it as its own argument — so describe must show it.
// Key is the display spelling (the vocabulary this surface teaches);
// restKey the stored key a route would take.
var alwaysSettableProperties = []describeProperty{
	{Key: "Name", Name: "Name", Format: "text", restKey: "name"},
}

func (r *Runner) runDescribe(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := spaceArg(args)
	typeKey := strArg(args, "type")
	getType := func() ([]byte, error) {
		return r.client.raw(ctx, apiRequest{
			method: "GET",
			path:   "/v2/spaces/" + seg(space) + "/types/" + seg(typeKey),
			// the name vocabulary (D5): the definitions this document states
			// are the spellings describe serves onward
			query: url.Values{"keys": []string{"name"}},
		})
	}
	doc, err := getType()
	if err != nil {
		// the §8.21 case fold: retry once with the unique case variant
		folded, ok, foldErr := r.foldTypeArg(ctx, space, typeKey, err)
		if foldErr != nil {
			return nil, foldErr
		}
		if ok {
			typeKey = folded
			doc, err = getType()
		}
	}
	if err != nil {
		// the repair hints naming a REST route no tool backs are re-spelled
		// on Run's error path now (steer.go), for every tool and not just
		// this one — the leak was never describe-specific (§8.34)
		return nil, err
	}
	// §2a/§2e: a type document states its definitions in type_settings, and
	// each entry names its property under `property`. Decoding the old shape
	// failed SILENTLY — json.Unmarshal leaves absent members zero — so
	// describe answered "this type has no properties", which is the one
	// answer an agent acts on without questioning.
	var typeDoc struct {
		Properties   map[string]any `json:"properties"`
		TypeSettings struct {
			ApiKey              string `json:"api_key"`
			PropertyDefinitions []struct {
				Property    string `json:"property"`
				InternalKey string `json:"internal_key"`
				Name        string `json:"name"`
				Format      string `json:"format"`
			} `json:"property_definitions"`
		} `json:"type_settings"`
	}
	if err := json.Unmarshal(doc, &typeDoc); err != nil {
		return nil, fmt.Errorf("decode type document: %w", err)
	}

	result := describeResult{Type: typeKey}
	// the type's own name: under ?keys=name the properties map spells the
	// name property as its display name ("Name"); "name" first keeps an
	// older slug-speaking server readable
	for _, spelling := range []string{"name", "Name"} {
		if name, ok := typeDoc.Properties[spelling].(string); ok && name != "" {
			result.Name = name
			break
		}
	}

	// the property rows load FIRST: the two sources spell one property
	// differently now — the definition carries the display name, the row the
	// api key plus the name — so the exact-string dedup the old code used
	// would list the same property twice. Identity is resolved through the
	// FoldKeyTerm classes both sources share (values.go propertyIndex).
	rows, err := r.propertyRows(ctx, space)
	if err != nil {
		return nil, err
	}
	idx := newPropertyIndex(rows)
	claimed := map[string]bool{}    // row.Key → already served through a definition
	servedFold := map[string]bool{} // fold classes of everything served (cross-source dedup)
	serve := func(spellings ...string) bool {
		classes := foldClasses(spellings...)
		for _, class := range classes {
			if servedFold[class] {
				return false
			}
		}
		for _, class := range classes {
			servedFold[class] = true
		}
		return true
	}

	// the type's own rows first, in the type's order — the curation is the
	// A1 signal and the order carries it
	for _, tp := range typeDoc.TypeSettings.PropertyDefinitions {
		// the entry's authored identity, in the format's own rank (§2e
		// authoredKey): the document-facing spelling, else the stored id
		key := tp.Property
		if key == "" {
			key = tp.InternalKey
		}
		if key == "" {
			key = tp.Name
		}
		if key == "" {
			continue
		}
		row, matched := matchDefinitionRow(idx, tp.Property, tp.Name, tp.InternalKey)
		if matched && claimed[row.Key] {
			continue // two definitions naming one property
		}
		spellings := []string{key, tp.Name, tp.InternalKey}
		if matched {
			spellings = append(spellings, row.Key, row.Name)
		}
		if !serve(spellings...) {
			continue
		}
		// the api key behind the served spelling: the matched row's, or the
		// stated internal_key — a stored key resolves at chain step 1 on any
		// server, where the display name in Key may not
		restKey := key
		if tp.InternalKey != "" {
			restKey = tp.InternalKey
		}
		if matched {
			claimed[row.Key] = true
			restKey = row.Key
		}
		prop := describeProperty{
			Key: key, Name: tp.Name, Format: tp.Format,
			OnType: true,
			// the predicate understands stored keys and api slugs, never
			// display names — ask it about the resolved key (§4a)
			ReadOnly: v2model.IsOutputOnlyProperty(restKey),
			restKey:  restKey,
		}
		if !prop.ReadOnly {
			r.fillOptions(ctx, space, &prop)
		}
		result.Properties = append(result.Properties, prop)
	}

	// then everything else the space can set. The settable universe is the
	// space's property index, NOT the type's list: an object of any type
	// takes any of the space's properties, which is exactly the fact the old
	// output hid. Options are listed only for the type's own selects — one
	// HTTP call per select, and a space's whole index would turn describe
	// into dozens of round trips for values the A2 guard already names on
	// refusal.
	var others []describeProperty
	for _, row := range rows {
		if claimed[row.Key] {
			continue
		}
		spelling := row.Name
		if spelling == "" {
			spelling = row.Key
		}
		if !serve(row.Key, row.Name) {
			continue
		}
		others = append(others, describeProperty{
			Key: spelling, Name: row.Name, Format: row.Format,
			ReadOnly: v2model.IsOutputOnlyProperty(row.Key),
			restKey:  row.Key,
		})
	}
	for _, always := range alwaysSettableProperties {
		if serve(always.Key, always.restKey) {
			others = append(others, always)
		}
	}
	sort.Slice(others, func(i, j int) bool { return others[i].Key < others[j].Key })
	result.Properties = append(result.Properties, others...)

	return &Result{Text: describeText(result), JSON: result}, nil
}

// matchDefinitionRow resolves a type-document property definition to its
// /properties row: an exact internal_key match against the row's api key
// wins (the legacy readable keys, where the two coincide); otherwise the
// row whose FoldKeyTerm classes meet the definition's — through any of its
// spellings. Several candidate rows is an ambiguity this pairing refuses to
// guess at: the definition then lists standalone and the rows list too,
// which is loud where a guess would be silent.
func matchDefinitionRow(idx *propertyIndex, spellings ...string) (v2model.PropertyRow, bool) {
	for _, s := range spellings {
		if s == "" {
			continue
		}
		if _, ok := idx.formats[s]; ok {
			for _, class := range foldClasses(s) {
				for _, row := range idx.byFold[class] {
					if row.Key == s {
						return row, true
					}
				}
			}
		}
	}
	var candidate v2model.PropertyRow
	found := false
	for _, class := range foldClasses(spellings...) {
		for _, row := range idx.byFold[class] {
			if found && row.Key != candidate.Key {
				return v2model.PropertyRow{}, false // ambiguous — no claim
			}
			candidate, found = row, true
		}
	}
	return candidate, found
}

// fillOptions loads a select property's live option names (the A1 lever).
// The route takes the REST api key (restKey), never the display name the
// served Key carries.
func (r *Runner) fillOptions(ctx context.Context, space string, prop *describeProperty) {
	if !selectFormats[prop.Format] {
		return
	}
	key := prop.restKey
	if key == "" {
		key = prop.Key
	}
	var resp v2model.ListResponse[v2model.OptionRow]
	if err := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(space) + "/properties/" + seg(key) + "/options",
		query:  url.Values{"limit": []string{fmt.Sprintf("%d", describeOptionsLimit)}},
	}, &resp); err != nil {
		prop.OptionsUnavailable = true
		return
	}
	for _, o := range resp.Data {
		prop.Options = append(prop.Options, o.Name)
	}
	prop.MoreOptions = resp.HasMore
}

// describeText renders the three sections: what the type names, what else
// the space can set, and what is read-only. The read-only keys are named
// rather than dropped — a caller who saw one in a read and then cannot find
// it in describe would reasonably conclude describe is incomplete, which is
// the defect this rendering exists to close.
func describeText(result describeResult) string {
	var onType, others, readOnly []describeProperty
	for _, p := range result.Properties {
		switch {
		case p.ReadOnly:
			readOnly = append(readOnly, p)
		case p.OnType:
			onType = append(onType, p)
		default:
			others = append(others, p)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "type %s", result.Type)
	if result.Name != "" && result.Name != result.Type {
		fmt.Fprintf(&b, " — %s", result.Name)
	}
	writeRow := func(p describeProperty) {
		fmt.Fprintf(&b, "\n  %s  %s", p.Key, p.Format)
		if len(p.Options) > 0 {
			fmt.Fprintf(&b, "  options: %s", strings.Join(p.Options, ", "))
			if p.MoreOptions {
				b.WriteString(", …")
			}
		}
		if p.OptionsUnavailable {
			b.WriteString("  options: (could not be listed — run describe again before using this property)")
		}
	}

	b.WriteString("\nproperties of this type:")
	if len(onType) == 0 {
		b.WriteString(" (none — it names no settable property of its own)")
	}
	for _, p := range onType {
		writeRow(p)
	}

	b.WriteString("\nalso settable on any object of this type:")
	if len(others) == 0 {
		b.WriteString(" (none)")
	}
	for i, p := range others {
		if i == describeSettableLimit {
			fmt.Fprintf(&b, "\n  … and %d more in this space", len(others)-describeSettableLimit)
			break
		}
		writeRow(p)
	}

	if len(readOnly) > 0 {
		keys := make([]string, 0, len(readOnly))
		for _, p := range readOnly {
			keys = append(keys, p.Key)
		}
		fmt.Fprintf(&b, "\nread-only — read serves these, set_properties refuses them: %s", strings.Join(keys, ", "))
	}
	b.WriteString("\nboth lists are settable: use these exact property names and option names in create and set_properties")
	return b.String()
}
