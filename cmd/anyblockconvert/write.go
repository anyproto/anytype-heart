package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// outputFormat selects how writeSnapshot serializes a snapshot to disk.
type outputFormat int

const (
	formatPb outputFormat = iota
	formatJSON
)

var jsonMarshaler = jsonpb.Marshaler{Indent: "  "}

// subDirFor mirrors the folder layout of the bundled use-case archives
// (util/builtinobjects/data/*.zip: objects/types/relations/relationsOptions/
// templates/profile/files). The pb importer (core/block/import/pb) walks the
// whole tree recursively and doesn't care about folder names — this is
// purely so a human skimming the output can find things.
func subDirFor(sbType model.SmartBlockType) string {
	switch sbType {
	case model.SmartBlockType_STType:
		return "types"
	case model.SmartBlockType_STRelation:
		return "relations"
	case model.SmartBlockType_STRelationOption:
		return "relationsOptions"
	case model.SmartBlockType_Template, model.SmartBlockType_BundledTemplate:
		return "templates"
	default:
		return "objects"
	}
}

// writeSnapshot serializes a snapshot as pb.SnapshotWithType. With formatJSON
// it writes jsonpb text under a ".json" extension instead of raw proto bytes
// under ".pb" — core/block/import/pb accepts either extension on import, so
// this is both human-inspectable and directly importable.
func writeSnapshot(outDir, id string, sbType model.SmartBlockType, snap *model.SmartBlockSnapshotBase, format outputFormat) error {
	sw := &pb.SnapshotWithType{
		SbType:   sbType,
		Snapshot: &pb.ChangeSnapshot{Data: snap},
	}

	ext := ".pb"
	var data []byte
	if format == formatJSON {
		ext = ".json"
		s, err := jsonMarshaler.MarshalToString(sw)
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		data = []byte(s)
	} else {
		var err error
		data, err = proto.Marshal(sw)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
	}

	dir := filepath.Join(outDir, subDirFor(sbType))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, sanitizeId(id)+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
