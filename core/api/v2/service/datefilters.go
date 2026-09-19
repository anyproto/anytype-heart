package v2service

import (
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// filterPathStyle spells the path of the i-th node under base: JSON-pointer
// style on the queries route ("/views/0/filters/1"), op style on PATCH
// ("ops[0].set.filters[1]").
type filterPathStyle func(base string, i int) string

func pointerFilterPath(base string, i int) string { return fmt.Sprintf("%s/%d", base, i) }
func opFilterPath(base string, i int) string      { return fmt.Sprintf("%s[%d]", base, i) }

// convertDateFilterValues makes a STORED filter compare the way the store
// compares: on a leaf whose property is a date, a string value that is a
// date — RFC 3339 or YYYY-MM-DD, the spellings the compact filter string
// takes — becomes unix seconds in place, and a string that is not a date
// is an issue. A view persists its filter, so a date left as a string is
// not a bad query but a view that compares string-against-int64 and
// quietly matches nothing, for good (round-six eval R6-1: an actor built
// "this quarter's deals", read the definition back, saw zero rows, and
// finished). Search still refuses the string on its structured form and
// spells the conversion out; converting there too is a later iteration.
// Numbers pass untouched; nodes are walked recursively.
func convertDateFilterValues(nodes []any, base string, style filterPathStyle, formatName func(string) (string, bool)) []v2model.Issue {
	var issues []v2model.Issue
	for i, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		nodePath := style(base, i)
		if nested, ok := node["filters"].([]any); ok {
			issues = append(issues, convertDateFilterValues(nested, nodePath+filterChildSegment(style), style, formatName)...)
			continue
		}
		property, _ := node["property"].(string)
		if property == "" {
			continue
		}
		if format, known := formatName(property); !known || format != "date" {
			continue
		}
		valuePath := nodePath + filterValueSegment(style)
		switch value := node["value"].(type) {
		case string:
			if sec, ok := filterstring.ParseDate(value); ok {
				node["value"] = sec
			} else {
				issues = append(issues, notADateIssue(property, value, valuePath))
			}
		case []any:
			converted := make([]any, len(value))
			for j, item := range value {
				s, isString := item.(string)
				if !isString {
					converted[j] = item
					continue
				}
				sec, ok := filterstring.ParseDate(s)
				if !ok {
					issues = append(issues, notADateIssue(property, s, valuePath))
					converted[j] = item
					continue
				}
				converted[j] = sec
			}
			node["value"] = converted
		}
	}
	return issues
}

func notADateIssue(property, value, path string) v2model.Issue {
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("property %q is a date, and %q is not one", property, value),
		Hint:    "a date takes unix seconds, an RFC 3339 string or YYYY-MM-DD — or a datePreset",
	}
}

func filterChildSegment(style filterPathStyle) string {
	if style(".", 0) == "./0" {
		return "/filters"
	}
	return ".filters"
}

func filterValueSegment(style filterPathStyle) string {
	if style(".", 0) == "./0" {
		return "/value"
	}
	return ".value"
}
