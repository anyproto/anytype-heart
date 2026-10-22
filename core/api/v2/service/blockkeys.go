package v2service

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// blockKeys resolves the property key slots blocks carry — a property block's
// `property`, a dataview's `properties` and every view slot — to stored keys
// BEFORE the codec imports them. The codec's vocabulary takes an exact stored
// key verbatim and settles a shared spelling by the document's own type, so
// for a key two properties answer to (a space's own `priority` beside the
// installed built-in) it binds the built-in whatever the type lists, and for
// a dataview it asks the wrong type altogether: a query's views describe the
// type it queries, not the query object's own type.
type blockKeys struct {
	s       *Service
	spaceId string
	kc      *keyCanon
	// objectScope settles property blocks: the object's own type
	objectScope map[string]bool
	// sourceScope settles a dataview that names no source of its own: the
	// document's queried type (`query_source`, a set's setOf)
	sourceScope map[string]bool
	// changed reports whether any slot was respelled
	changed bool
}

func (s *Service) newBlockKeys(spaceId string, objectScope, sourceScope map[string]bool) *blockKeys {
	return &blockKeys{s: s, spaceId: spaceId, objectScope: objectScope, sourceScope: sourceScope}
}

func (b *blockKeys) canon() (*keyCanon, error) {
	if b.kc == nil {
		kc, err := b.s.newKeyCanon(b.spaceId)
		if err != nil {
			return nil, fmt.Errorf("load property keys: %w", err)
		}
		b.kc = kc
	}
	return b.kc, nil
}

// rewrite walks decoded blocks in place; path is the JSON pointer of the list.
func (b *blockKeys) rewrite(blocks []any, path string) error {
	for i, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := b.rewriteBlock(block, fmt.Sprintf("%s/%d", path, i)); err != nil {
			return err
		}
	}
	return nil
}

// rewriteMaps is rewrite over an already-typed block run.
func (b *blockKeys) rewriteMaps(blocks []map[string]any, path string) error {
	for i, block := range blocks {
		if err := b.rewriteBlock(block, fmt.Sprintf("%s/%d", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func (b *blockKeys) rewriteBlock(block map[string]any, path string) error {
	switch block["type"] {
	case "property":
		prop, ok := block["property"].(string)
		if !ok || prop == "" {
			return nil
		}
		kc, err := b.canon()
		if err != nil {
			return err
		}
		kc.scope = b.objectScope
		canonical, err := kc.canonOrErr(prop, path+"/property")
		if err != nil {
			return err
		}
		b.set(block, "property", prop, canonical)
	case "dataview":
		kc, err := b.canon()
		if err != nil {
			return err
		}
		kc.scope = b.dataviewScope(block)
		if props, ok := block["properties"].([]any); ok {
			for j, rawProp := range props {
				prop, ok := rawProp.(map[string]any)
				if !ok {
					continue
				}
				key, ok := prop["property"].(string)
				if !ok || key == "" {
					continue
				}
				canonical, err := kc.canonOrErr(key, fmt.Sprintf("%s/properties/%d/property", path, j))
				if err != nil {
					return err
				}
				b.set(prop, "property", key, canonical)
			}
		}
		if views, ok := block["views"].([]any); ok {
			before, err := json.Marshal(views)
			if err != nil {
				return fmt.Errorf("encode views: %w", err)
			}
			if err := kc.rewriteViews(views, path+"/views"); err != nil {
				return err
			}
			after, err := json.Marshal(views)
			if err != nil {
				return fmt.Errorf("encode views: %w", err)
			}
			b.changed = b.changed || !bytes.Equal(before, after)
		}
	}
	if children, ok := block["children"].([]any); ok {
		if err := b.rewrite(children, path+"/children"); err != nil {
			return err
		}
	}
	return b.rewriteCells(block, path)
}

// rewriteCells walks a table's cells: a cell is a string, null, one block
// object, or a run of flat blocks (§6.1), and any of those blocks can be a
// property block.
func (b *blockKeys) rewriteCells(block map[string]any, path string) error {
	rows, ok := block["rows"].([]any)
	if !ok {
		return nil
	}
	for r, rawRow := range rows {
		row, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		cells, ok := row["cells"].([]any)
		if !ok {
			continue
		}
		for c, rawCell := range cells {
			cellPath := fmt.Sprintf("%s/rows/%d/cells/%d", path, r, c)
			switch cell := rawCell.(type) {
			case map[string]any:
				if err := b.rewriteBlock(cell, cellPath); err != nil {
					return err
				}
			case []any:
				if err := b.rewrite(cell, cellPath); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// rewriteSet resolves the key slots of an update_block merge that the caller
// set, and only those: the block's live members already carry the bindings
// they were stored with, and re-resolving an untouched slot could move it.
// The merged block lends its type and its source members, so a dataview's
// scope is the one it will have after the merge.
func (b *blockKeys) rewriteSet(merged map[string]any, set []string, path string) error {
	part := map[string]any{}
	for _, member := range []string{"type", "source", "object_id"} {
		if v, ok := merged[member]; ok {
			part[member] = v
		}
	}
	for _, member := range set {
		if v, ok := merged[member]; ok {
			part[member] = v
		}
	}
	if err := b.rewriteBlock(part, path); err != nil {
		return err
	}
	for _, member := range set {
		if v, ok := part[member]; ok {
			merged[member] = v
		}
	}
	return nil
}

func (b *blockKeys) set(node map[string]any, field, from, to string) {
	if from != to {
		node[field] = to
		b.changed = true
	}
}

// rewriteRaw is rewrite over an encoded block list, returning raw unchanged
// when no slot was respelled (numbers keep their spelling: json.Number).
func (b *blockKeys) rewriteRaw(raw json.RawMessage, path string) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var blocks []any
	if err := dec.Decode(&blocks); err != nil {
		return raw, nil // shape errors belong to the document validator
	}
	if err := b.rewrite(blocks, path); err != nil {
		return nil, err
	}
	if !b.changed {
		return raw, nil
	}
	out, err := json.Marshal(blocks)
	if err != nil {
		return nil, fmt.Errorf("encode blocks: %w", err)
	}
	return out, nil
}

// dataviewScope is the type a dataview queries: its own single type source,
// else the single type of the query object it shows, else the document's.
func (b *blockKeys) dataviewScope(block map[string]any) map[string]bool {
	if source, ok := block["source"].([]any); ok && len(source) == 1 {
		if term, ok := source[0].(string); ok && term != "" {
			if typeId := b.typeId(term); typeId != "" {
				return b.s.typePropertyScope(b.spaceId, typeId)
			}
		}
		return nil
	}
	if objectId, ok := block["object_id"].(string); ok && objectId != "" {
		details, err := b.s.store.SpaceIndex(b.spaceId).GetDetails(objectId)
		if err != nil || details == nil {
			return nil
		}
		if setOf := details.GetStringList(bundle.RelationKeySetOf); len(setOf) == 1 {
			return b.s.typePropertyScope(b.spaceId, setOf[0])
		}
		return nil
	}
	return b.sourceScope
}

// typeId resolves a type term (id, stored key or slug) to the type's id; ""
// when it names no single live type.
func (b *blockKeys) typeId(term string) string {
	entries, err := b.s.liveTypes(b.spaceId)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.Id == term {
			return entry.Id
		}
	}
	entry, ok, ambiguous, err := b.s.resolveTypeInput(b.spaceId, term, entries)
	if err != nil || !ok || len(ambiguous) > 0 {
		return ""
	}
	return entry.Id
}
