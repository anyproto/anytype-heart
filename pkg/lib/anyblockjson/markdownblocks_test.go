package anyblockjson

import (
	"encoding/json"
	"fmt"
	"strings"
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
				{"type": "heading_1", "text": "One"},
				{"type": "heading_2", "text": "Two"},
				{"type": "heading_3", "text": "Three"},
				{"type": "heading_3", "text": "Four"},
				{"type": "heading_3", "text": "Six"},
			},
		},
		{
			name: "closing hashes stripped",
			md:   "## Title ##",
			want: []map[string]any{{"type": "heading_2", "text": "Title"}},
		},
		{
			name: "list kinds",
			md:   "- bullet\n* star\n+ plus\n1. first\n2) second\n- [ ] open\n- [x] done",
			want: []map[string]any{
				{"type": "bulleted_list_item", "text": "bullet"},
				{"type": "bulleted_list_item", "text": "star"},
				{"type": "bulleted_list_item", "text": "plus"},
				{"type": "numbered_list_item", "text": "first"},
				{"type": "numbered_list_item", "text": "second"},
				{"type": "checkbox", "text": "open"},
				{"type": "checkbox", "checked": true, "text": "done"},
			},
		},
		{
			name: "nested lists two spaces per level",
			md:   "- top\n  - child\n    - grandchild\n- top again",
			want: []map[string]any{
				{"type": "bulleted_list_item", "text": "top"},
				{"indent": float64(1), "type": "bulleted_list_item", "text": "child"},
				{"indent": float64(2), "type": "bulleted_list_item", "text": "grandchild"},
				{"type": "bulleted_list_item", "text": "top again"},
			},
		},
		{
			name: "three-space numbered nesting clamps to one level",
			md:   "1. item\n   - sub",
			want: []map[string]any{
				{"type": "numbered_list_item", "text": "item"},
				{"indent": float64(1), "type": "bulleted_list_item", "text": "sub"},
			},
		},
		{
			name: "over-deep jump clamps to previous plus one",
			md:   "- top\n        - way too deep",
			want: []map[string]any{
				{"type": "bulleted_list_item", "text": "top"},
				{"indent": float64(1), "type": "bulleted_list_item", "text": "way too deep"},
			},
		},
		{
			name: "indented plain line becomes a child paragraph",
			md:   "- item\n  continued note",
			want: []map[string]any{
				{"type": "bulleted_list_item", "text": "item"},
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
					map[string]any{"is_header": true, "cells": []any{"Name", "Status"}},
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
					map[string]any{"is_header": true, "cells": []any{"a|b"}},
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
					map[string]any{"is_header": true, "cells": []any{"a", "b"}},
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
				{"type": "heading_1", "text": "Title"},
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

// mustImport asserts a parsed run imports through UnmarshalBlocks — the
// "a run always imports" contract.
func mustImport(t *testing.T, run []json.RawMessage) {
	t.Helper()
	if len(run) == 0 {
		return
	}
	n := 0
	gen := func() string { n++; return fmt.Sprintf("mdgen%02d", n) }
	blocks, topIds, err := UnmarshalBlocks(run, Options{GenerateId: gen})
	require.NoError(t, err)
	assert.NotEmpty(t, blocks)
	assert.NotEmpty(t, topIds)
}

// markdownForBlockType opens one block of each type the parser can emit —
// the leaf-clamp regression below drives an indented line after each of
// them, so a new emitted type cannot silently reopen the leaf-parent hole.
var markdownForBlockType = map[string]string{
	"paragraph":          "plain text",
	"heading_1":          "# Title",
	"heading_2":          "## Title",
	"heading_3":          "### Title",
	"quote":              "> quoted",
	"code":               "```go\nx\n```",
	"divider":            "---",
	"bulleted_list_item": "- item",
	"numbered_list_item": "1. item",
	"checkbox":           "- [ ] item",
	"table":              "| a |\n| - |\n| 1 |",
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
		// leaf-parent regressions: an indented line after a divider or table
		// must become a SIBLING, never a child (V2 — leaf blocks cannot parent)
		"---\n  child line",
		"para\n\n---\n\n  indented after divider",
		"| a |\n| - |\n| 1 |\n  under table",
		"---\n  - x",
		"0\n- 8\n  - X\n```1X\n0\n```\n>1\n---\n  00000000000000000000",
	}
	for i, md := range samples {
		t.Run(fmt.Sprintf("sample_%d", i), func(t *testing.T) {
			mustImport(t, ParseMarkdownBlocks(md))
		})
	}

	t.Run("an indented line after every emitted block type imports", func(t *testing.T) {
		for typ, md := range markdownForBlockType {
			t.Run(typ, func(t *testing.T) {
				run := ParseMarkdownBlocks(md + "\n  indented aside")
				mustImport(t, run)
				blocks := decodeRun(t, run)
				require.NotEmpty(t, blocks)
				last := blocks[len(blocks)-1]
				if leafBlockTypes[typ] {
					assert.NotContains(t, last, "indent",
						"a line after a leaf %s block must stay its sibling", typ)
				}
			})
		}
	})

	t.Run("a 40-deep staircase clamps to the 32-level bound and imports", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < 40; i++ {
			b.WriteString(strings.Repeat("  ", i) + "- item\n")
		}
		run := ParseMarkdownBlocks(b.String())
		mustImport(t, run)
		blocks := decodeRun(t, run)
		require.Len(t, blocks, 40)
		maxSeen := 0.0
		for _, blk := range blocks {
			if v, ok := blk["indent"].(float64); ok && v > maxSeen {
				maxSeen = v
			}
		}
		assert.Equal(t, float64(32), maxSeen, "levels past 32 stay siblings at 32")
	})
}

func TestParseMarkdownBlocksLimit(t *testing.T) {
	t.Run("under the cap passes through", func(t *testing.T) {
		run, exceeded := ParseMarkdownBlocksLimit("- a\n- b", 256)
		assert.False(t, exceeded)
		assert.Len(t, run, 2)
	})
	t.Run("over the cap stops early and reports the excess", func(t *testing.T) {
		md := strings.Repeat("- x\n", 300)
		run, exceeded := ParseMarkdownBlocksLimit(md, 256)
		assert.True(t, exceeded)
		assert.Len(t, run, 257, "the run holds cap+1 blocks as proof, not the full parse")
	})
	t.Run("zero means unbounded", func(t *testing.T) {
		md := strings.Repeat("- x\n", 300)
		run, exceeded := ParseMarkdownBlocksLimit(md, 0)
		assert.False(t, exceeded)
		assert.Len(t, run, 300)
	})
}

func TestFenceEdges(t *testing.T) {
	t.Run("a noise info string is dropped, a language token kept", func(t *testing.T) {
		got := decodeRun(t, ParseMarkdownBlocks("``` ```\nx\n```"))
		require.Len(t, got, 1)
		assert.NotContains(t, got[0], "language", "a backtick run is not a language")

		got = decodeRun(t, ParseMarkdownBlocks("```c++\nx\n```"))
		require.Len(t, got, 1)
		assert.Equal(t, "c++", got[0]["language"])
	})
	t.Run("a closing marker indented past 3 spaces is fence content", func(t *testing.T) {
		got := decodeRun(t, ParseMarkdownBlocks("```\n    ```\nstill code\n```"))
		require.Len(t, got, 1)
		assert.Equal(t, "code", got[0]["type"])
		assert.Equal(t, "    ```\nstill code", got[0]["text"])
	})
	t.Run("a closing marker with up to 3 leading spaces closes", func(t *testing.T) {
		got := decodeRun(t, ParseMarkdownBlocks("```\ncode\n   ```\nafter"))
		require.Len(t, got, 2)
		assert.Equal(t, "code", got[0]["type"])
		assert.Equal(t, "code", got[0]["text"])
		assert.Equal(t, "paragraph", got[1]["type"])
	})
}

// FuzzMarkdownImports enforces the "a run always imports" contract under
// fuzzing: whatever the markdown, the parsed run must pass UnmarshalBlocks.
// The seeds include the shapes that broke the pre-fix clamp (a divider/table
// followed by a deeper line, an over-deep staircase).
func FuzzMarkdownImports(f *testing.F) {
	f.Add("# Plan\n\n- [ ] a\n  - b\n\n```sh\nx\n```\n\n| a |\n| - |\n| 1 |")
	f.Add("---\n  child line")
	f.Add("| a |\n| - |\n| 1 |\n  under table")
	f.Add("0\n- 8\n  - X\n```1X\n0\n```\n>1\n---\n  00000000000000000000")
	f.Add(strings.Repeat("  ", 40) + "- deep\n" + strings.Repeat("- x\n", 3))
	f.Fuzz(func(t *testing.T, md string) {
		if len(md) > 1<<16 {
			return
		}
		run, _ := ParseMarkdownBlocksLimit(md, 512)
		if len(run) == 0 {
			return
		}
		n := 0
		gen := func() string { n++; return fmt.Sprintf("fz%04d", n) }
		if _, _, err := UnmarshalBlocks(run, Options{GenerateId: gen}); err != nil {
			t.Fatalf("parsed run fails import for %q: %v", md, err)
		}
	})
}
