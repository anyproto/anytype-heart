// anyblockrecover reconstructs AnyBlock JSON source documents from a
// directory of pb snapshots produced by cmd/anyblockconvert. It is the
// inverse of that tool: anyblockjson.Marshal turns each snapshot back into a
// document, using the batch's own relations/ and relationsOptions/ snapshots
// to resolve property formats, property names and option names — the three
// things the source expressed by name and the snapshots hold as ids.
//
// Recovered documents are not byte-identical to hand-authored ones: key order
// is canonical, and the synthesized relation/option snapshots are skipped
// (they are generated on every convert, not source).
//
// Usage:
//
//	go run ./cmd/anyblockrecover -in ./out -out ~/usecase2/anyblock/01-company-wiki
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type propInfo struct {
	name   string
	format model.RelationFormat
}

type resolver struct {
	props   map[string]propInfo // relation key -> name/format
	byId    map[string]string   // relation object id -> key
	optName map[string]string   // option object id -> name
}

// format resolves a synthesized property; bundled keys are left to
// anyblockjson, which consults the bundle before any resolver.
func (r *resolver) format(key domain.RelationKey) (model.RelationFormat, bool) {
	p, ok := r.props[string(key)]
	return p.format, ok
}

// lookup returns what is known about a property key, preferring the bundle —
// only synthesized relations appear in the batch's relations/ folder, so a
// bundled key like iconImage has no snapshot to read its format from.
func (r *resolver) lookup(key string) (propInfo, bool) {
	if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil && rel != nil {
		return propInfo{name: rel.Name, format: rel.Format}, true
	}
	p, ok := r.props[key]
	return p, ok
}

func (r *resolver) OptionName(_ domain.RelationKey, id string) (string, bool) {
	n, ok := r.optName[id]
	return n, ok
}

func (r *resolver) OptionId(_ domain.RelationKey, name string) (string, bool) {
	return name, false
}

func (r *resolver) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	key, ok := r.byId[id]
	if !ok {
		// bundled properties arrive as _br<key>
		if strings.HasPrefix(id, "_br") {
			key = strings.TrimPrefix(id, "_br")
		} else {
			return anyblockjson.PropertyDefinition{}, false
		}
	}
	p, _ := r.lookup(key)
	return anyblockjson.PropertyDefinition{
		Key:    domain.RelationKey(key),
		Name:   p.name,
		Format: p.format,
	}, true
}

func (r *resolver) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	return string(def.Key), true
}

func readSnapshot(path string) (*pb.SnapshotWithType, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sw := &pb.SnapshotWithType{}
	if err := jsonpb.Unmarshal(f, sw); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return sw, nil
}

func detailString(d *pb.ChangeSnapshot, key string) string {
	if d == nil || d.Data == nil || d.Data.Details == nil {
		return ""
	}
	return d.Data.Details.Fields[key].GetStringValue()
}

func main() {
	inDir := flag.String("in", "", "directory of pb snapshots (cmd/anyblockconvert output)")
	outDir := flag.String("out", "", "directory to write recovered AnyBlock JSON documents into")
	flag.Parse()
	if *inDir == "" || *outDir == "" {
		flag.Usage()
		os.Exit(2)
	}

	res := &resolver{
		props:   map[string]propInfo{},
		byId:    map[string]string{},
		optName: map[string]string{},
	}

	// pass 1: build the resolvers from the synthesized relation/option snapshots
	for _, sub := range []string{"relations", "relationsOptions"} {
		paths, _ := filepath.Glob(filepath.Join(*inDir, sub, "*.json"))
		for _, p := range paths {
			sw, err := readSnapshot(p)
			if err != nil {
				fmt.Fprintln(os.Stderr, "skip:", err)
				continue
			}
			id := detailString(sw.Snapshot, "id")
			key := detailString(sw.Snapshot, "relationKey")
			name := detailString(sw.Snapshot, "name")
			if sub == "relations" {
				f := sw.Snapshot.Data.Details.Fields["relationFormat"].GetNumberValue()
				res.props[key] = propInfo{name: name, format: model.RelationFormat(int32(f))}
				res.byId[id] = key
			} else {
				res.optName[id] = name
			}
		}
	}

	opts := anyblockjson.Options{
		ResolveFormat:     res.format,
		ResolveOptions:    res,
		ResolveProperties: res,
	}

	// pass 2: recover every type/object document
	var recovered, failed int
	for _, sub := range []string{"types", "objects", "templates"} {
		paths, _ := filepath.Glob(filepath.Join(*inDir, sub, "*.json"))
		for _, p := range paths {
			sw, err := readSnapshot(p)
			if err != nil {
				fmt.Fprintln(os.Stderr, "FAIL:", err)
				failed++
				continue
			}
			data, err := anyblockjson.Marshal(sw.SbType, sw.Snapshot.Data, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %s: %v\n", filepath.Base(p), err)
				failed++
				continue
			}
			dir, name := routeFor(sw, filepath.Base(p))
			target := filepath.Join(*outDir, dir)
			if err := os.MkdirAll(target, 0o755); err != nil {
				fmt.Fprintln(os.Stderr, "FAIL:", err)
				failed++
				continue
			}
			if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "FAIL:", err)
				failed++
				continue
			}
			recovered++
		}
	}
	fmt.Printf("%d documents recovered, %d failed\noutput: %s\n", recovered, failed, *outDir)
}

// routeFor mirrors the source layout the bundles use: types/, chats/, pages/,
// objects/. The snapshot only knows its smartblock type, so page-vs-object is
// taken from the id slug the author chose.
func routeFor(sw *pb.SnapshotWithType, base string) (dir, name string) {
	id := detailString(sw.Snapshot, "id")
	switch sw.SbType {
	case model.SmartBlockType_STType:
		return "types", strings.TrimPrefix(strings.TrimSuffix(base, ".json"), "type-") + ".type.json"
	case model.SmartBlockType_ChatDerivedObject, model.SmartBlockType_DiscussionObject:
		return "chats", strings.TrimPrefix(base, "chat-")
	}
	if strings.HasPrefix(id, "page-") {
		return "pages", strings.TrimPrefix(base, "page-")
	}
	return "objects", base
}

var _ = json.Marshal
