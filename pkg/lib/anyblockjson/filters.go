package anyblockjson

// filters.go exposes the §6.2 filter/sort codec at fragment granularity:
// a bare filters array or sorts array — the shapes the API v2 query surface
// carries in request bodies — converts to the model tree
// (`model.BlockContentDataviewFilter` / `Sort`) through the same importer
// the whole-document dataview path uses, so the structured `filters` request
// form and the parsed compact filter string land on ONE internal tree.
//
// Validation runs the same checks the document path applies to a view's
// filters: enum vocabulary (conditions, date presets, sort directions),
// the counting-preset operand rule, the dynamic-placeholder format rule, and
// the unguarded-date-comparison warning (SPEC §6.2 — surfaced through
// Options.OnWarning, the C11 channel). Issue paths are fragment-relative:
// `/filters/i/…` and `/sorts/i/…`.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// filterConditionList renders the condition vocabulary for error messages.
var filterConditionList = strings.Join([]string{
	"equal", "not_equal", "greater", "less", "greater_or_equal", "less_or_equal",
	"contains", "not_contains", "in", "not_in", "empty", "not_empty",
	"all_in", "not_all_in", "exact_in", "not_exact_in", "exists",
}, ", ")

// datePresetList renders the date-preset vocabulary for error messages.
var datePresetList = strings.Join([]string{
	"yesterday", "today", "tomorrow", "last_week", "current_week", "next_week",
	"last_month", "current_month", "next_month", "number_of_days_ago",
	"number_of_days_now", "last_year", "current_year", "next_year",
}, ", ")

// UnmarshalFilters converts a §6.2 structured filters array (the top-level
// nodes combine with an implicit AND) into model filter nodes. Select values
// resolve through Options.ResolveOptions exactly as on document import —
// wire a read-only resolver on query paths and a creating resolver on write
// paths. Formats rehydrate through Options.ResolveFormat. Errors wrap
// *ValidationError with `/filters/i/…` paths; warning-grade findings (the
// §6.2 unguarded-date-comparison trap) ride Options.OnWarning.
func UnmarshalFilters(raw json.RawMessage, opts Options) ([]*model.BlockContentDataviewFilter, error) {
	var nodes []jsonFilter
	if err := jsonUnmarshal(raw, &nodes); err != nil {
		// the schema states the shape with a path where it can — a payload
		// that is not an array, a member of the wrong type — and the raw
		// decode error remains only for what never reaches it
		if fragErr := validateQueryFragment("filters", raw); isValidationError(fragErr) {
			return nil, fragErr
		}
		return nil, fmt.Errorf("decode filters: %w", err)
	}
	var generic []any
	if err := jsonUnmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("decode filters: %w", err)
	}

	var issues []Issue
	addIssue := func(path, format string, args ...any) {
		issues = append(issues, Issue{Path: path, Message: fmt.Sprintf(format, args...)})
	}
	warnIssue := func(path, format string, args ...any) {
		if opts.OnWarning != nil {
			opts.OnWarning(Issue{Path: path, Message: fmt.Sprintf(format, args...)})
		}
	}

	validateFilterVocabulary(nodes, "/filters", addIssue)

	// the document path's per-view semantic checks (counting-preset operand,
	// placeholder format rule, the unguarded date-less warning) run on the
	// generic form, with formats resolved from the space instead of the
	// dataview's properties list
	formats := referencedFormats(nodes, opts)
	checkDateFilters(map[string]any{"filters": generic}, formats,
		func(prop string) bool { return formats[prop] == "date" },
		"", addIssue, warnIssue)

	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}
	// §12, the closed shape: the same $defs/filterNode fragment the
	// whole-document path holds a view's filters to — the pass that refuses
	// an unknown member, which the tolerant decode above silently drops. It
	// runs after the vocabulary pass so the enum wording above keeps owning
	// the faults it can name, and only for what that pass had no rule for.
	if len(nodes) > 0 {
		if err := validateQueryFragment("filters", raw); err != nil {
			return nil, err
		}
	}

	imp := &importer{opts: opts, doc: opts.fragmentDoc()}
	dv := &model.BlockContentDataview{}
	out := make([]*model.BlockContentDataviewFilter, 0, len(nodes))
	for _, jf := range nodes {
		out = append(out, imp.filterFromJSON(jf, dv))
	}
	return out, nil
}

// UnmarshalSorts converts a §6.2 sorts array into model sort nodes. Formats
// rehydrate through Options.ResolveFormat; custom-order select values
// resolve through Options.ResolveOptions. Errors wrap *ValidationError with
// `/sorts/i/…` paths.
func UnmarshalSorts(raw json.RawMessage, opts Options) ([]*model.BlockContentDataviewSort, error) {
	var sorts []jsonSort
	if err := jsonUnmarshal(raw, &sorts); err != nil {
		// same split as UnmarshalFilters: the schema's path-addressed
		// verdict where it has one, the raw decode error otherwise
		if fragErr := validateQueryFragment("sorts", raw); isValidationError(fragErr) {
			return nil, fragErr
		}
		return nil, fmt.Errorf("decode sorts: %w", err)
	}
	var issues []Issue
	for i, js := range sorts {
		if js.Property == "" {
			issues = append(issues, Issue{
				Path:    fmt.Sprintf("/sorts/%d/property", i),
				Message: "a sort needs a property key",
			})
		}
		if js.Direction != "" && !sortDirectionNames.has(js.Direction) {
			issues = append(issues, Issue{
				Path:    fmt.Sprintf("/sorts/%d/direction", i),
				Message: fmt.Sprintf("unknown direction %q — allowed: asc, desc, custom", js.Direction),
			})
		}
		if js.EmptyPlacement != "" && !emptyPlacementNames.has(js.EmptyPlacement) {
			issues = append(issues, Issue{
				Path:    fmt.Sprintf("/sorts/%d/empty_placement", i),
				Message: fmt.Sprintf("unknown empty_placement %q — allowed: start, end", js.EmptyPlacement),
			})
		}
	}
	if len(issues) > 0 {
		return nil, &ValidationError{Issues: issues}
	}
	// §12, the closed shape: the same $defs/sort fragment the whole-document
	// path holds a view's sorts to — after the vocabulary pass, for the same
	// wording reason as UnmarshalFilters
	if len(sorts) > 0 {
		if err := validateQueryFragment("sorts", raw); err != nil {
			return nil, err
		}
	}

	imp := &importer{opts: opts, doc: opts.fragmentDoc()}
	dv := &model.BlockContentDataview{}
	out := make([]*model.BlockContentDataviewSort, 0, len(sorts))
	for _, js := range sorts {
		out = append(out, imp.sortFromJSON(js, dv))
	}
	return out, nil
}

// validateFilterVocabulary checks condition/datePreset/operator names
// recursively, with node paths.
func validateFilterVocabulary(nodes []jsonFilter, path string, addIssue func(string, string, ...any)) {
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s/%d", path, i)
		if node.Operator != "" || len(node.Filters) > 0 {
			if node.Operator != "" && node.Operator != "and" && node.Operator != "or" {
				addIssue(nodePath+"/operator", "unknown operator %q — allowed: and, or", node.Operator)
			}
			validateFilterVocabulary(node.Filters, nodePath+"/filters", addIssue)
			continue
		}
		if node.Property == "" {
			addIssue(nodePath+"/property", "a filter leaf needs a property key")
		}
		if node.Condition != "" && !conditionNames.has(node.Condition) {
			addIssue(nodePath+"/condition", "unknown condition %q — allowed: %s", node.Condition, filterConditionList)
		}
		if node.DatePreset != "" && !datePresetNames.has(node.DatePreset) {
			addIssue(nodePath+"/datePreset", "unknown datePreset %q — allowed: %s", node.DatePreset, datePresetList)
		}
	}
}

// referencedFormats resolves the §3 format name of every property key the
// filter tree references — the formats input of checkDateFilters. The term
// travels through the reader's vocabulary before the format is looked up,
// exactly as importer.filterFromJSON resolves it (propertyKey, then
// impDvFormat): a request naming the documented "Due date" spelling would
// otherwise resolve no format at all, and the format is what says whether a
// date preset means anything here.
func referencedFormats(nodes []jsonFilter, opts Options) map[string]string {
	formats := map[string]string{}
	var walk func(nodes []jsonFilter)
	walk = func(nodes []jsonFilter) {
		for _, node := range nodes {
			if len(node.Filters) > 0 {
				walk(node.Filters)
			}
			if node.Property == "" {
				continue
			}
			if _, seen := formats[node.Property]; seen {
				continue
			}
			if f, ok := resolveFormatWith(opts, opts.propertyKey(node.Property)); ok {
				if name := FormatName(f); name != "" {
					formats[node.Property] = name
				}
			}
		}
	}
	walk(nodes)
	return formats
}

// validateQueryFragment holds a bare §6.2 filters or sorts array to the same
// schema fragments the whole-document path holds a view to ($defs/filterNode,
// $defs/sort): the array is wrapped into a minimal synthetic document — the
// validateFragmentRun pattern (fragment.go) — and the document validation
// runs, so the two doors cannot disagree about the shape. Issue paths are
// remapped from the synthetic /blocks/0/views/0/… to the fragment-relative
// /filters/… and /sorts/… the §6.2 fragment entry points promise. Warnings
// are discarded here: the fragment entry points run their own semantic pass
// with the caller's format resolver, which the synthetic document lacks.
func validateQueryFragment(member string, raw json.RawMessage) error {
	payload, err := json.Marshal(map[string]any{
		"version": FormatVersion,
		"type":    "page",
		"blocks": []any{map[string]any{
			"type":  "dataview",
			"views": []any{map[string]json.RawMessage{member: raw}},
		}},
	})
	if err != nil {
		return fmt.Errorf("build synthetic %s document: %w", member, err)
	}
	if _, err := validateToDoc(payload, false, nil); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return &ValidationError{Issues: refragmentIssues(ve.Issues)}
		}
		return err
	}
	return nil
}

// refragmentIssues rewrites synthetic-document paths back to the
// fragment-relative form: /blocks/0/views/0/filters/1/condition names
// /filters/1/condition of the array the caller handed over.
func refragmentIssues(issues []Issue) []Issue {
	const prefix = "/blocks/0/views/0"
	out := make([]Issue, len(issues))
	for i, iss := range issues {
		iss.Path = strings.TrimPrefix(iss.Path, prefix)
		out[i] = iss
	}
	return out
}

// isValidationError reports whether err wraps a *ValidationError — the
// path-addressed kind the fragment entry points prefer over a bare decode
// error.
func isValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
