package anyblockjson

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeRun parses the produced raw blocks into comparable maps.
func decodeRun(t *testing.T, run []json.RawMessage) []map[string]any {
	t.Helper()
	out := make([]map[string]any, len(run))
	for i, raw := range run {
		require.NoError(t, json.Unmarshal(raw, &out[i]))
	}
	return out
}

func TestParseMarkdownBlocks(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []map[string]any
	}{
		{
			name: "empty input produces empty run",
			md:   "  \n\n\t\n",
			want: []map[string]any{},
		},
		{
			name: "paragraphs split on blank lines and join soft wraps",
			md:   "first line\nsecond line\n\nnext para",
			want: []map[string]any{
				{"type": "paragraph", "text": "first line\nsecond line"},
				{"type": "paragraph", "text": "next para"},
			},
		},
		{
			name: "atx headings clamp to heading3",
			md:   "# One\n## Two\n### Three\n#### Four\n###### Six",
			want: []map[string]any{
				{"type": "heading1", "text": "One"},
				{"type": "heading2", "text": "Two"},
				{"type": "heading3", "text": "Three"},
				{"type": "heading3", "text": "Four"},
				{"type": "heading3", "text": "Six"},
			},
		},
		{
			name: "closing hashes stripped",
			md:   "## Title ##",
			want: []map[string]any{{"type": "heading2", "text": "Title"}},
		},
		{
			name: "list kinds",
			md:   "- bullet\n* star\n+ plus\n1. first\n2) second\n- [ ] open\n- [x] done",
			want: []map[string]any{
				{"type": "bulletedListItem", "text": "bullet"},
				{"type": "bulletedListItem", "text": "star"},
				{"type": "bulletedListItem", "text": "plus"},
				{"type": "numberedListItem", "text": "first"},
				{"type": "numberedListItem", "text": "second"},
				{"type": "checkbox", "text": "open"},
				{"type": "checkbox", "checked": true, "text": "done"},
			},
		},
		{
			name: "nested lists two spaces per level",
			md:   "- top\n  - child\n    - grandchild\n- top again",
			want: []map[string]any{
				{"type": "bulletedListItem", "text": "top"},
				{"indent": float64(1), "type": "bulletedListItem", "text": "child"},
				{"indent": float64(2), "type": "bulletedListItem", "text": "grandchild"},
				{"type": "bulletedListItem", "text": "top again"},
			},
		},
		{
			name: "three-space numbered nesting clamps to one level",
			md:   "1. item\n   - sub",
			want: []map[string]any{
				{"type": "numberedListItem", "text": "item"},
				{"indent": float64(1), "type": "bulletedListItem", "text": "sub"},
			},
		},
		{
			name: "over-deep jump clamps to previous plus one",
			md:   "- top\n        - way too deep",
			want: []map[string]any{
				{"type": "bulletedListItem", "text": "top"},
				{"indent": float64(1), "type": "bulletedListItem", "text": "way too deep"},
			},
		},
		{
			name: "indented plain line becomes a child paragraph",
			md:   "- item\n  continued note",
			want: []map[string]any{
				{"type": "bulletedListItem", "text": "item"},
				{"indent": float64(1), "type": "paragraph", "text": "continued note"},
			},
		},
		{
			name: "first block indent clamps to zero",
			md:   "    indented start",
			want: []map[string]any{{"type": "paragraph", "text": "indented start"}},
		},
		{
			name: "quote lines join into one quote",
			md:   "> quoted\n> more\n\nafter",
			want: []map[string]any{
				{"type": "quote", "text": "quoted\nmore"},
				{"type": "paragraph", "text": "after"},
			},
		},
		{
			name: "code fence with language keeps literal text",
			md:   "```go\nfunc main() {\n\t# not a heading\n}\n```\nafter",
			want: []map[string]any{
				{"type": "code", "language": "go", "text": "func main() {\n\t# not a heading\n}"},
				{"type": "paragraph", "text": "after"},
			},
		},
		{
			name: "unterminated fence runs to end of input",
			md:   "```\nline one\nline two",
			want: []map[string]any{{"type": "code", "text": "line one\nline two"}},
		},
		{
			name: "dividers",
			md:   "---\n***\n___\n- - -",
			want: []map[string]any{
				{"type": "divider"},
				{"type": "divider"},
				{"type": "divider"},
				{"type": "divider"},
			},
		},
		{
			name: "table with header separator",
			md:   "| Name | Status |\n| --- | --- |\n| Export | done |\n| Import | |",
			want: []map[string]any{{
				"type":    "table",
				"columns": []any{map[string]any{}, map[string]any{}},
				"rows": []any{
					map[string]any{"isHeader": true, "cells": []any{"Name", "Status"}},
					map[string]any{"cells": []any{"Export", "done"}},
					map[string]any{"cells": []any{"Import", nil}},
				},
			}},
		},
		{
			name: "escaped pipe stays in the cell",
			md:   "| a\\|b |\n| --- |\n| c |",
			want: []map[string]any{{
				"type":    "table",
				"columns": []any{map[string]any{}},
				"rows": []any{
					map[string]any{"isHeader": true, "cells": []any{"a|b"}},
					map[string]any{"cells": []any{"c"}},
				},
			}},
		},
		{
			name: "ragged row widens the column set",
			md:   "| a | b |\n| --- | --- |\n| 1 | 2 | 3 |",
			want: []map[string]any{{
				"type":    "table",
				"columns": []any{map[string]any{}, map[string]any{}, map[string]any{}},
				"rows": []any{
					map[string]any{"isHeader": true, "cells": []any{"a", "b"}},
					map[string]any{"cells": []any{"1", "2", "3"}},
				},
			}},
		},
		{
			name: "pipe lines without a separator degrade to a paragraph",
			md:   "| not | a table |\n| just | pipes |",
			want: []map[string]any{
				{"type": "paragraph", "text": "| not | a table |\n| just | pipes |"},
			},
		},
		{
			name: "inline markup passes through verbatim",
			md:   "text with **bold**, `code` and [link](anytype://object?objectId=bafyx)",
			want: []map[string]any{
				{"type": "paragraph", "text": "text with **bold**, `code` and [link](anytype://object?objectId=bafyx)"},
			},
		},
		{
			name: "crlf input",
			md:   "# Title\r\n\r\nbody\r\n",
			want: []map[string]any{
				{"type": "heading1", "text": "Title"},
				{"type": "paragraph", "text": "body"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeRun(t, ParseMarkdownBlocks(tt.md))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestParseMarkdownBlocksImports proves every parser output is a valid §4
// fragment run: it must import through UnmarshalBlocks without validation
// errors (the convergence contract — the wrapper's markdown channel rides the
// same fragment pipeline as a blocks payload).
func TestParseMarkdownBlocksImports(t *testing.T) {
	samples := []string{
		"# Plan\n\nintro paragraph\n\n- [ ] task one\n  - sub\n- [x] task two\n\n```sh\nmake build\n```\n\n> note\n\n---\n\n| a | b |\n| - | - |\n| 1 | 2 |",
		"        deep start\n- l\n    - jumped",
		"only text",
		"| a |\n| broken",
	}
	for i, md := range samples {
		t.Run(fmt.Sprintf("sample_%d", i), func(t *testing.T) {
			run := ParseMarkdownBlocks(md)
			if len(run) == 0 {
				return
			}
			n := 0
			gen := func() string { n++; return fmt.Sprintf("mdgen%02d", n) }
			blocks, topIds, err := UnmarshalBlocks(run, Options{GenerateId: gen})
			require.NoError(t, err)
			assert.NotEmpty(t, blocks)
			assert.NotEmpty(t, topIds)
		})
	}
}
