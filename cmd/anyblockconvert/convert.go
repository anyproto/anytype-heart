package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// convertFile parses one AnyBlock JSON document and returns its object id
// (read back off the resulting snapshot, since a document may omit "id" and
// get a generated one) alongside the reconstructed snapshot.
func convertFile(inDir, path string, b *batch, normalizeIndent bool) (string, model.SmartBlockType, *model.SmartBlockSnapshotBase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, nil, fmt.Errorf("read: %w", err)
	}

	genId := genIdFactory(fallbackSeed(inDir, path))
	opts := anyblockjson.Options{
		ResolveFormat:     b.resolveFormat,
		ResolveOptions:    b,
		ResolveProperties: b,
		GenerateId:        genId,
		NormalizeIndent:   normalizeIndent,
	}

	sbType, snap, err := anyblockjson.Unmarshal(data, opts)
	if err != nil {
		return "", 0, nil, fmt.Errorf("unmarshal: %w", err)
	}
	patchObjectTypes(sbType, snap)

	id := snap.Details.GetFields()["id"].GetStringValue()
	if id == "" {
		return "", 0, nil, fmt.Errorf("converted snapshot has no id")
	}
	return id, sbType, snap, nil
}

// patchObjectTypes fills in the one snapshot field pkg/lib/anyblockjson
// leaves for the wiring to set: kind: "objectType" documents carry their
// identity in the envelope's "key" field, not "type" (SPEC.md §2a), so
// Unmarshal never populates ObjectTypes for them. Relation/RelationOption
// documents parsed straight out of the source folder (rather than minted by
// this tool) get the same treatment, as do chat/discussion documents, whose
// bundled type key is fixed by the kind.
func patchObjectTypes(sbType model.SmartBlockType, snap *model.SmartBlockSnapshotBase) {
	if len(snap.ObjectTypes) > 0 {
		return
	}
	switch sbType {
	case model.SmartBlockType_STType:
		snap.ObjectTypes = []string{bundle.TypeKeyObjectType.URL()}
	case model.SmartBlockType_STRelation:
		snap.ObjectTypes = []string{bundle.TypeKeyRelation.URL()}
	case model.SmartBlockType_STRelationOption:
		snap.ObjectTypes = []string{bundle.TypeKeyRelationOption.URL()}
	case model.SmartBlockType_ChatDerivedObject:
		snap.ObjectTypes = []string{bundle.TypeKeyChatDerived.URL()}
	case model.SmartBlockType_DiscussionObject:
		snap.ObjectTypes = []string{bundle.TypeKeyDiscussion.URL()}
	}
}

// genIdFactory returns a deterministic id generator seeded from a document's
// file path: anyblockjson.Options.GenerateId is called once for a missing
// envelope id and again for every block missing its own id (SPEC.md §9), so
// it must produce a fresh value each call, not a fixed one.
func genIdFactory(seed string) func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%s-%d", seed, n)
	}
}

// fallbackSeed turns a file's path relative to the input root into a stable,
// filesystem-and-id-charset-safe slug, so a re-run of this tool over
// unchanged input produces the same generated ids.
func fallbackSeed(inDir, path string) string {
	rel, err := filepath.Rel(inDir, path)
	if err != nil {
		rel = path
	}
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	return sanitizeId(rel)
}

func sanitizeId(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}
