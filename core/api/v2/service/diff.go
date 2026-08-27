package v2service

// diff.go computes the Phase-3 diff_stats (APIV2.md §2 Phase 3): both
// PATCH and PUT diff the canonical before-document (the live state's
// marshal) against the canonical after-document (the applied snapshot's
// marshal), so normalization noise cancels and the numbers reflect real
// content movement. On PUT the stats are the accidental-full-rewrite signal
// (a body that lost its block ids shows as everything removed + added).

import (
	"encoding/json"
	"fmt"
	"reflect"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// diffDocBlock is one block's diff identity.
type diffDocBlock struct {
	id      string
	parent  string
	content string // canonical JSON of the block minus indent and id
}

// diffDocShape is the parsed diff-relevant part of a document.
type diffDocShape struct {
	blocks     []diffDocBlock
	byId       map[string]int
	properties map[string]any
	items      []string
}

func parseDiffDoc(body []byte) (*diffDocShape, error) {
	var doc struct {
		Properties map[string]any   `json:"properties"`
		Blocks     []map[string]any `json:"blocks"`
		Items      []string         `json:"items"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode document for diff: %w", err)
	}
	shape := &diffDocShape{byId: map[string]int{}, properties: doc.Properties, items: doc.Items}
	// parent derivation: the SPEC §4 stack walk over indents
	type frame struct {
		id     string
		indent int
	}
	stack := []frame{{id: "", indent: -1}}
	for _, b := range doc.Blocks {
		indent := blockIndent(b)
		for stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		content := map[string]any{}
		for k, v := range b {
			if k == "indent" || k == "id" {
				continue
			}
			content[k] = v
		}
		contentJSON, err := json.Marshal(content) // map keys sort — deterministic
		if err != nil {
			return nil, fmt.Errorf("encode block for diff: %w", err)
		}
		id := blockId(b)
		shape.byId[id] = len(shape.blocks)
		shape.blocks = append(shape.blocks, diffDocBlock{
			id:      id,
			parent:  stack[len(stack)-1].id,
			content: string(contentJSON),
		})
		stack = append(stack, frame{id: id, indent: indent})
	}
	return shape, nil
}

// prevCommonSibling finds the nearest preceding block with the same parent
// that also exists in the other document — the movement anchor that keeps
// pure insertions from marking their following siblings as moved.
func (s *diffDocShape) prevCommonSibling(i int, other *diffDocShape) string {
	b := s.blocks[i]
	for j := i - 1; j >= 0; j-- {
		if s.blocks[j].parent != b.parent {
			continue
		}
		if _, common := other.byId[s.blocks[j].id]; common {
			return s.blocks[j].id
		}
	}
	return ""
}

// diffEditDocs computes the diff_stats between two canonical documents.
func diffEditDocs(beforeDoc, afterDoc []byte) (v2model.DiffStats, error) {
	var stats v2model.DiffStats
	before, err := parseDiffDoc(beforeDoc)
	if err != nil {
		return stats, fmt.Errorf("before document: %w", err)
	}
	after, err := parseDiffDoc(afterDoc)
	if err != nil {
		return stats, fmt.Errorf("after document: %w", err)
	}

	for _, b := range after.blocks {
		if _, ok := before.byId[b.id]; !ok {
			stats.BlocksAdded++
		}
	}
	for i, b := range before.blocks {
		j, ok := after.byId[b.id]
		if !ok {
			stats.BlocksRemoved++
			continue
		}
		a := after.blocks[j]
		if a.content != b.content {
			stats.BlocksChanged++
		}
		if a.parent != b.parent || before.prevCommonSibling(i, after) != after.prevCommonSibling(j, before) {
			stats.BlocksMoved++
		}
	}

	keys := map[string]bool{}
	for k := range before.properties {
		keys[k] = true
	}
	for k := range after.properties {
		keys[k] = true
	}
	for k := range keys {
		bv, inB := before.properties[k]
		av, inA := after.properties[k]
		if inB != inA || !reflect.DeepEqual(bv, av) {
			stats.PropertiesChanged++
		}
	}
	return stats, nil
}
