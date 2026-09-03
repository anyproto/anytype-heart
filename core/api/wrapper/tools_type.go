package wrapper

// tools_type.go — create_type: the one tool that authors a SCHEMA rather
// than content. The wrapper could already use types (describe, create,
// find) and could not make one, so "set up a Recipe type with ingredients,
// cook time and rating" had no answer on this surface at all.
//
// Two facts shape everything here, both measured against a live heart
// (cmd/apiv2eval/heartboot) rather than assumed from the schemas:
//
//  1. A wrong type is PERMANENT on this surface. There is no delete_type
//     tool and the type PATCH REPLACES the property list rather than
//     appending, so there is no edit path either — a duplicate
//     "Recipe"/"Recipie" pair is a mess nothing in the tool set can clean
//     up. Every refusal below therefore fires BEFORE anything is written,
//     and the tool spends an extra round trip (the pre-flight dry run) to
//     keep that true.
//
//  2. Select OPTIONS declared inside a type document are silently DROPPED.
//     The format carries them (anyblockjson.PropertyDefinition.Options, and
//     the document schema validates them — it even refuses options on a
//     non-select), but the v2 write path ignores them: creatingResolvers
//     .PropertyId mints the relation from name+format alone and never looks
//     at def.Options. Measured: a type created with
//     `Spice: select(Mild, Hot)` came back with the property and
//     `GET /properties/spice/options` returned `{"data":[],"total":0}` —
//     with and without ?create_missing_options=true. A select that claims
//     options it does not have is worse than no options, so this tool does
//     NOT declare them in the type document. It creates each option-bearing
//     property through POST /properties instead, which DOES create the
//     options in the same request (verified: the 201 reports every option
//     under `created.options`, and the options route then lists them).
//
// That makes the call 1 + N + 1 requests instead of one, and the ORDER is
// the contract: nothing before the last request can create a type, so no
// failure anywhere in the sequence can leave a half-made type behind. The
// residue of a mid-sequence failure is properties, which a re-run reuses
// (the plan resolves them as existing) rather than duplicates.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// typePropertyFormats is the property-format enum the API PUBLISHES — the
// `property` discovery kind's `format` enum (v2service schemas.go) and the
// same list on the OpenAPI models (apimodel.PropertyFormat*). It is
// deliberately NARROWER than what the document schema accepts: the AnyBlock
// validator also takes `emoji`, `properties` and `map`, which are not
// property formats a caller of this surface can do anything with. A golden
// test pins the list, so a format added to the published enum fails here
// instead of silently staying unreachable.
var typePropertyFormats = []string{
	"text", "number", "select", "multi_select", "date", "files",
	"checkbox", "url", "email", "phone", "objects",
}

// maxTypeProperties bounds one create_type call's property list. A type
// with more than this is not a dictated request, it is a runaway — and
// because the type cannot be edited or deleted afterwards, the cost of
// letting one through is permanent. 32 covers every type a person would
// describe in a sentence.
const maxTypeProperties = 32

// maxTypePropertyOptions mirrors the options maxItems the property kind
// advertises (v2service maxV2PropertyOptions) — enforced wrapper-side so
// the refusal speaks the DDL, not /options.
const maxTypePropertyOptions = 100

// typePropertiesForm is the one statement of the DDL's shape, quoted by
// every refusal that needs to show it. What `describe` PRINTS per property
// row is this same `Name: format(options)` form, so a describe output can
// be transcribed straight back into this argument.
const typePropertiesForm = `Name: format — e.g. "Cook time: number, Rating: select(Low, Medium, High), Source: url"`

// typePropertyDecl is one parsed DDL entry.
type typePropertyDecl struct {
	Name    string
	Format  string
	Options []string
}

// render spells a declaration back in the DDL's own form — the receipt and
// describe both print properties this way, so what the tool prints is what
// the tool takes.
func (d typePropertyDecl) render() string {
	if len(d.Options) == 0 {
		return d.Name + ": " + d.Format
	}
	return fmt.Sprintf("%s: %s(%s)", d.Name, d.Format, strings.Join(d.Options, ", "))
}

//
// ---- the DDL parser ----
//

// splitTopLevel splits on sep at PARENTHESIS DEPTH 0. Both levels of the
// grammar are comma-separated — the property list and each select's option
// list — so a depth-blind split would read "Rating: select(Low, Medium)" as
// three properties, two of them nonsense. Unbalanced parentheses are
// refused rather than guessed at: an unclosed "(" would otherwise swallow
// every property after it.
func splitTopLevel(s string, sep rune) ([]string, error) {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf(`properties has a ")" with no matching "(" — options go in parentheses after the format: %s`, typePropertiesForm)
			}
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + len(string(r))
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf(`properties has an unclosed "(" — close the option list: %s`, typePropertiesForm)
	}
	return append(out, s[start:]), nil
}

// normalizeTypeFormat folds a format spelling the way the rest of the
// wrapper folds keys: case and the word separator only. "Number", "Select"
// and "multi select"/"multi-select" are the same format written by a model
// that read the enum once — a spelling fold, not a guess at intent. Anything
// the fold does not land on the published enum is refused by name; the tool
// never picks a format for the caller.
func normalizeTypeFormat(s string) string {
	f := strings.ToLower(strings.TrimSpace(s))
	f = strings.ReplaceAll(f, " ", "_")
	f = strings.ReplaceAll(f, "-", "_")
	return f
}

// parseTypeProperties parses the flat DDL into declarations. Every refusal
// names the offending entry and shows the form, because the caller cannot
// see which half of a comma-separated string this layer objected to.
func parseTypeProperties(ddl string) ([]typePropertyDecl, error) {
	if strings.TrimSpace(ddl) == "" {
		return nil, nil
	}
	entries, err := splitTopLevel(ddl, ',')
	if err != nil {
		return nil, err
	}
	var decls []typePropertyDecl
	// seen is keyed by fold class, not by literal spelling: "Cook time" and
	// "cook_time" are one property, and creating a type that declares both
	// would permanently carry a duplicate no tool can remove
	seen := map[string]string{}
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue // a doubled or trailing comma — the intent is unambiguous
		}
		name, spec, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("property %q names no format — write each property as %s", entry, typePropertiesForm)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("property %q has no name before the colon — write each property as %s", entry, typePropertiesForm)
		}
		decl, err := parseTypePropertySpec(name, strings.TrimSpace(spec))
		if err != nil {
			return nil, err
		}
		class := anyblockjson.FoldKeyTerm(name)
		if class == "" {
			class = name // a name the fold collapses to nothing is its own class
		}
		if prev, dup := seen[class]; dup {
			return nil, fmt.Errorf("properties declares %q and %q — two spellings of one property; keep one", prev, name)
		}
		seen[class] = name
		decls = append(decls, decl)
	}
	if len(decls) == 0 {
		return nil, fmt.Errorf("properties names no property — write each property as %s", typePropertiesForm)
	}
	if len(decls) > maxTypeProperties {
		return nil, fmt.Errorf("properties declares %d properties — the cap is %d; nothing was created", len(decls), maxTypeProperties)
	}
	return decls, nil
}

// parseTypePropertySpec parses one entry's `format` or `format(options)`
// half.
func parseTypePropertySpec(name, spec string) (typePropertyDecl, error) {
	formatText, optionText := spec, ""
	if open := strings.IndexByte(spec, '('); open >= 0 {
		if !strings.HasSuffix(spec, ")") {
			return typePropertyDecl{}, fmt.Errorf(`property %q: the option list must end with ")" — %s`, name, typePropertiesForm)
		}
		formatText = strings.TrimSpace(spec[:open])
		optionText = spec[open+1 : len(spec)-1]
	}
	format := normalizeTypeFormat(formatText)
	if format == "" {
		return typePropertyDecl{}, fmt.Errorf("property %q names no format — formats: %s", name, strings.Join(typePropertyFormats, ", "))
	}
	if !containsStr(typePropertyFormats, format) {
		// never guessed at: an unknown format that resolved to "text" would
		// bake the wrong shape into a type nothing can edit afterwards
		return typePropertyDecl{}, fmt.Errorf("property %q: unknown format %q — formats: %s",
			name, strings.TrimSpace(formatText), strings.Join(typePropertyFormats, ", "))
	}
	options, err := parseTypePropertyOptions(name, optionText)
	if err != nil {
		return typePropertyDecl{}, err
	}
	if len(options) > 0 && !selectFormats[format] {
		// the server refuses this too (the document schema: "options is only
		// meaningful on select/multi_select"), but the refusal has to happen
		// before the property mints, and it has to name the DDL's own repair
		return typePropertyDecl{}, fmt.Errorf("property %q: options are only meaningful on select and multi_select, not %s — drop the parentheses, or write the format as select",
			name, format)
	}
	return typePropertyDecl{Name: name, Format: format, Options: options}, nil
}

// parseTypePropertyOptions parses one select's parenthesised option list.
func parseTypePropertyOptions(name, text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	parts, err := splitTopLevel(text, ',')
	if err != nil {
		return nil, err
	}
	var options []string
	seen := map[string]bool{}
	for _, part := range parts {
		option := strings.TrimSpace(part)
		if option == "" {
			continue
		}
		if seen[option] {
			// deduplicating silently would create ONE option where the caller
			// named two, and the caller would never learn which
			return nil, fmt.Errorf("property %q lists option %q twice — name each option once", name, option)
		}
		seen[option] = true
		options = append(options, option)
	}
	if len(options) > maxTypePropertyOptions {
		return nil, fmt.Errorf("property %q lists %d options — the cap is %d; nothing was created", name, len(options), maxTypePropertyOptions)
	}
	return options, nil
}

//
// ---- the plan: which properties exist, which must be minted ----
//

// typePropertyPlan is one declaration resolved against the space.
type typePropertyPlan struct {
	decl typePropertyDecl
	// key is the property's exact api key once one is known: the existing
	// row's, or the key the option mint returns. Empty while the property is
	// still only a name the type create itself will mint.
	key string
	// existing marks a property the space already holds — the type reuses it,
	// which is why its options are the space's and not this call's.
	existing bool
	// mint marks a property this tool must create through POST /properties
	// BEFORE the type create, because its options exist only that way.
	mint bool
}

// planTypeProperties resolves every declaration against the space's
// property index and decides how each one reaches the type. Every refusal
// here fires before the first write.
func (r *Runner) planTypeProperties(ctx context.Context, spaceId string, decls []typePropertyDecl) ([]typePropertyPlan, error) {
	if len(decls) == 0 {
		return nil, nil
	}
	idx, err := r.propertyIndexFor(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	plans := make([]typePropertyPlan, 0, len(decls))
	claimed := map[string]string{} // resolved key → the spelling that claimed it
	for _, decl := range decls {
		key, err := idx.resolveKey(decl.Name)
		if err != nil {
			return nil, err // ambiguous spelling: refused with the candidates listed
		}
		format, existing := idx.formats[key]
		if existing {
			// the parser's fold catches two spellings of one WORD; this
			// catches two spellings that only the space's index knows are one
			// property (a display name and its stored key)
			if prev, dup := claimed[key]; dup {
				return nil, fmt.Errorf("properties declares %q and %q — both name the property %q; keep one", prev, decl.Name, idx.displayName(key))
			}
			claimed[key] = decl.Name
			if format != decl.Format {
				return nil, fmt.Errorf("property %q already exists in this space with format %s, not %s — write it as %q here, or give the new property a different name",
					idx.displayName(key), format, decl.Format, decl.Name+": "+format)
			}
			if err := r.checkDeclaredOptions(ctx, spaceId, key, idx.displayName(key), decl.Options); err != nil {
				return nil, err
			}
			plans = append(plans, typePropertyPlan{decl: decl, key: key, existing: true})
			continue
		}
		plans = append(plans, typePropertyPlan{decl: decl, mint: len(decl.Options) > 0})
	}
	return plans, nil
}

// checkDeclaredOptions refuses a declaration whose options the EXISTING
// property does not already hold. The type reuses that property as it is —
// this tool cannot add options to a property it did not create — so
// accepting the declaration would leave the caller believing in options
// that are not there, which is the one failure mode worth an extra round
// trip to prevent. A declaration the property already satisfies passes
// silently: transcribing a describe row back must not be an error.
func (r *Runner) checkDeclaredOptions(ctx context.Context, spaceId, key, label string, want []string) error {
	if len(want) == 0 {
		return nil
	}
	var resp v2model.ListResponse[v2model.OptionRow]
	if err := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/properties/" + seg(key) + "/options",
		query:  url.Values{"limit": []string{"100"}, "keys": []string{"name"}},
	}, &resp); err != nil {
		return prefixToolError(err, "list options of %q", label)
	}
	var missing []string
	for _, name := range want {
		if !optionExists(resp.Data, name) {
			missing = append(missing, fmt.Sprintf("%q", name))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	have := make([]string, 0, len(resp.Data))
	for _, o := range resp.Data {
		have = append(have, o.Name)
	}
	holds := "it holds no options at all"
	if len(have) > 0 {
		holds = "it holds: " + strings.Join(have, ", ")
	}
	return fmt.Errorf("property %q already exists in this space and has no option %s — %s. Drop the parentheses to use it as it is, or give the new property a different name; nothing was created",
		label, strings.Join(missing, ", "), holds)
}

//
// ---- the document ----
//

// typeDocument builds the kind:"object_type" AnyBlock document. The
// endpoint injects `kind` and `formatVersion` itself and REFUSES `blocks`
// on create (the editor generates the type's default views), so the document
// is exactly the name plus the property definitions.
//
// The definitions deliberately carry NO `options` member: the write path
// ignores it (see the file header), and a document that states options the
// server drops is the silent claim this tool exists to avoid. Options reach
// the space through the property mint below, before this document is sent.
func typeDocument(name string, plans []typePropertyPlan) map[string]any {
	doc := map[string]any{"properties": map[string]any{"name": name}}
	if len(plans) == 0 {
		return doc
	}
	defs := make([]any, 0, len(plans))
	for _, plan := range plans {
		// `property` is the entry's document-facing spelling: the exact api
		// key whenever one is known (an existing row's, or the mint's), the
		// caller's name otherwise — a key cannot be ambiguous, and a name
		// resolved server-side can be
		spelling := plan.decl.Name
		if plan.key != "" {
			spelling = plan.key
		}
		defs = append(defs, map[string]any{
			"property": spelling,
			"name":     plan.decl.Name,
			"format":   plan.decl.Format,
		})
	}
	doc["type_settings"] = map[string]any{"property_definitions": defs}
	return doc
}

//
// ---- the tool ----
//

// createTypeResult is create_type's machine shape.
type createTypeResult struct {
	Id         string               `json:"id,omitempty"`
	Key        string               `json:"key,omitempty"`
	Name       string               `json:"name"`
	Properties []createTypeProperty `json:"properties,omitempty"`
	DryRun     bool                 `json:"dry_run,omitempty"`
	Warnings   []v2model.Issue      `json:"warnings,omitempty"`
}

// createTypeProperty is one property of the created type, as the call
// resolved it.
type createTypeProperty struct {
	Name    string   `json:"name"`
	Format  string   `json:"format"`
	Options []string `json:"options,omitempty"`
	// Existing marks a property the space already held, which the type
	// reuses — its options are the space's, not this call's.
	Existing bool `json:"existing,omitempty"`
}

func (r *Runner) runCreateType(ctx context.Context, session *Session, args map[string]any) (*Result, error) {
	space := spaceArg(args)
	name := strings.TrimSpace(strArg(args, "name"))
	if name == "" {
		return nil, fmt.Errorf("create_type: %q must not be empty — the type's name, e.g. Cookbook entry", "name")
	}
	decls, err := parseTypeProperties(strArg(args, "properties"))
	if err != nil {
		return nil, err
	}
	plans, err := r.planTypeProperties(ctx, space, decls)
	if err != nil {
		return nil, err
	}

	// the pre-flight: the server owns the type namespace and is the ONLY
	// party that can answer whether a name is free — its union check spans
	// bundled type keys (a "Recipe" type is reserved by the bundle even
	// though no listing this tool can make would show it), bundled-derived
	// slugs, live keys and live slugs. Asking it with dry_run=true costs one
	// round trip and buys the invariant this whole tool is built around:
	// nothing is created until the name and every format have been accepted.
	preflight, err := r.createTypeRequest(ctx, session, space, typeDocument(name, plans), true)
	if err != nil {
		return nil, createTypeNameRepair(name, err)
	}
	if r.DryRun {
		// the caller asked for a dry run, and the pre-flight IS one — running
		// the mints now would make --dry-run write
		return createTypeReceipt(name, plans, preflight, true), nil
	}

	// the option-bearing properties, each created with its options in one
	// request. This runs AFTER the pre-flight and BEFORE the type create, so
	// a failure here leaves properties (which a re-run reuses) and never a
	// type. The plan is updated in place with the minted key, so the document
	// the type create sends addresses each one exactly.
	for i := range plans {
		if !plans[i].mint {
			continue
		}
		key, err := r.mintTypeProperty(ctx, session, space, plans[i].decl)
		if err != nil {
			return nil, err
		}
		plans[i].key = key
	}

	result, err := r.createTypeRequest(ctx, session, space, typeDocument(name, plans), false)
	if err != nil {
		return nil, createTypeNameRepair(name, err)
	}
	return createTypeReceipt(name, plans, result, false), nil
}

// createTypeRequest sends one POST /types, with the shared mutation
// machinery. dry forces ?dry_run=true for the pre-flight, whatever the
// runner's own dry-run mode is.
func (r *Runner) createTypeRequest(ctx context.Context, session *Session, spaceId string, doc map[string]any, dry bool) (*v2model.CreateResult, error) {
	path := "/v2/spaces/" + seg(spaceId) + "/types"
	query := r.mutationQuery()
	if dry {
		query.Set("dry_run", "true")
	}
	key := r.mutationKey(session, requestHash("POST", path, query, doc), r.now())
	var result v2model.CreateResult
	if err := r.client.decode(ctx, apiRequest{
		method:         "POST",
		path:           path,
		query:          query,
		body:           doc,
		idempotencyKey: key,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// mintTypeProperty creates one select/multi_select property together with
// its options — the only path on this API that creates options and the
// property in a single request (POST /properties honours `options`; a type
// document's property definitions do not).
func (r *Runner) mintTypeProperty(ctx context.Context, session *Session, spaceId string, decl typePropertyDecl) (string, error) {
	options := make([]any, 0, len(decl.Options))
	for _, name := range decl.Options {
		options = append(options, map[string]any{"name": name})
	}
	body := map[string]any{"name": decl.Name, "format": decl.Format, "options": options}
	path := "/v2/spaces/" + seg(spaceId) + "/properties"
	query := r.mutationQuery()
	key := r.mutationKey(session, requestHash("POST", path, query, body), r.now())
	var result v2model.CreateResult
	if err := r.client.decode(ctx, apiRequest{
		method:         "POST",
		path:           path,
		query:          query,
		body:           body,
		idempotencyKey: key,
	}, &result); err != nil {
		// the type is what the caller asked for, so the refusal has to say
		// that it does not exist — the property that DID get created is
		// harmless and a re-run reuses it, but only if the caller knows to
		// re-run
		return "", prefixToolError(err, "create property %q with its options — the type was NOT created", decl.Name)
	}
	return result.Key, nil
}

// prefixToolError puts context in front of a refusal. It deliberately does
// not use fmt.Errorf on a *ToolError: Run's error path unwraps to the
// ToolError and returns THAT (steer.go steerError), so an outer wrapper
// never reaches the caller — the context would be silently dropped between
// the executor and the agent reading the message. It goes into the
// ToolError's own text instead, the channel every refusal on this surface
// speaks through; a non-ToolError still wraps normally, chain intact.
func prefixToolError(err error, format string, a ...any) error {
	prefix := fmt.Sprintf(format, a...)
	var te *ToolError
	if !errors.As(err, &te) {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	te.Text = prefix + ": " + te.Text
	return te
}

// createTypeNameRepair appends this surface's repair to the server's
// name-conflict refusal. The server's fact stays (which key is taken, by
// what), because it is the fact; what it cannot know is that the caller
// holds tools rather than routes — its own hint offers a PATCH route no tool
// backs.
//
// The bundled case is the one worth spelling out, and it is not rare: a
// dozen everyday type names (Recipe, Book, Movie, Project, Contact…) are
// reserved by the bundle and absent from every listing until something uses
// them. The repair is the discovery: `create` with that type name installs
// the built-in type on first use, which is what the caller wanted anyway.
func createTypeNameRepair(name string, err error) error {
	var te *ToolError
	if !errors.As(err, &te) || te.Status != http.StatusBadRequest {
		return err
	}
	named := false
	for _, issue := range te.Issues {
		if issue.Path == "/properties/name" {
			named = true
			break
		}
	}
	if !named {
		return err
	}
	// the bundled wording is the server's; the fallback below is correct for
	// either case, so a reworded server message degrades rather than breaks
	if strings.Contains(te.Text, "bundled type") {
		te.Text += fmt.Sprintf(" — the built-in type %q already exists: use it as it is (create with type %q installs it on first use, and describe then lists its properties). Pick a different name if you need a type of your own.", name, name)
		return te
	}
	te.Text += fmt.Sprintf(" — a type named %q is already in this space: run describe on it to see its properties, or pick a different name. This surface cannot rename or delete a type.", name)
	return te
}

// createTypeReceipt renders the result. Each property prints in the DDL's
// own `Name: format(options)` form — the form describe prints and the form
// this tool takes — so the receipt can be read back as an argument.
func createTypeReceipt(name string, plans []typePropertyPlan, result *v2model.CreateResult, dryRun bool) *Result {
	out := createTypeResult{Name: name, DryRun: dryRun, Warnings: result.Warnings}
	if !dryRun {
		out.Id, out.Key = result.Id, result.Key
	}
	var b strings.Builder
	if dryRun {
		fmt.Fprintf(&b, "dry run — a type %q would be created", name)
	} else {
		fmt.Fprintf(&b, "created type %q (%s)", name, result.Key)
	}
	for _, plan := range plans {
		out.Properties = append(out.Properties, createTypeProperty{
			Name: plan.decl.Name, Format: plan.decl.Format,
			Options: plan.decl.Options, Existing: plan.existing,
		})
		fmt.Fprintf(&b, "\n  %s", plan.decl.render())
		if plan.existing {
			// stated because it explains whose options these are: the type
			// reuses the space's property, so the option list came from the
			// space and not from this call
			b.WriteString("  (this space's existing property)")
		}
	}
	if len(plans) == 0 {
		b.WriteString("\n  (no properties — every object still takes Name and Description)")
	}
	for _, w := range result.Warnings {
		b.WriteString("\nwarning: " + w.Message)
	}
	if !dryRun {
		fmt.Fprintf(&b, "\ncreate objects of it with create: type %q — describe %q lists these properties again", name, name)
	}
	return &Result{Text: b.String(), JSON: out}
}
