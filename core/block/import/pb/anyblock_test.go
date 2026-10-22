package pb

import (
	"archive/zip"
	"context"
	"encoding/json"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func authoringBundleFixture() fstest.MapFS {
	documents := map[string]string{
		"index.json": `{"formatVersion":"2.0","name":"Weekends","entrypoint":"home","widgets":[{"target":"home"},{"target":"type-trip","layout":"view","limit":6},{"target":"_recent_open","layout":"compact_list","limit":4}]}`,
		"properties.json": `{"formatVersion":"2.0","properties":[
   {"name":"Budget","format":"number"},
   {"name":"State","format":"select","options":[{"name":"Booked","color":"blue"},{"name":"Dream","color":"yellow"}]},
   {"name":"Cost state","format":"select","options":["Booked"]},
   {"name":"When","format":"date","include_time":false},
   {"name":"Expenses","format":"objects","object_types":["Expense"]},
   {"name":"Trip","format":"objects","object_types":["Trip"]},
   {"name":"Tags","format":"multi_select","options":[{"name":"Art","color":"purple"},"Water"]}
  ]}`,
		"types/trip.json":      `{"formatVersion":"2.0","kind":"object_type","id":"type-trip","internal_key":"trip","properties":{"Name":"Trip"},"type_settings":{"default_template":"template-trip","property_definitions":[{"property":"Name"},{"property":"Budget","section":"featured"},{"property":"State"},{"property":"When"},{"property":"Expenses"},{"property":"Tags"}]},"blocks":[{"type":"paragraph","text":"Plan a weekend."}]}`,
		"types/expense.json":   `{"formatVersion":"2.0","kind":"object_type","id":"type-expense","internal_key":"expense","properties":{"Name":"Expense"},"type_settings":{"property_definitions":[{"property":"Cost state"},{"property":"Trip"}]}}`,
		"templates/trip.json":  `{"formatVersion":"2.0","kind":"template","type":"template","template_for":"Trip","id":"template-trip","properties":{"Name":"New weekend","State":"Booked","When":"2026-09-18T00:00:00Z"},"blocks":[{"type":"paragraph","text":"Pick a destination."}]}`,
		"objects/home.json":    `{"formatVersion":"2.0","id":"home","type":"Query","properties":{"Name":"Weekends"},"query_source":{"types":["Trip"],"properties":["Budget"]},"blocks":[{"type":"dataview","properties":[{"property":"State","format":"select"},{"property":"Budget","format":"number"}],"views":[{"name":"Booked weekends","type":"table","columns":[{"property":"State"},{"property":"Budget"}],"filters":[{"property":"State","condition":"in","value":["Booked"]}]}]}]}`,
		"objects/trip.json":    `{"formatVersion":"2.0","id":"weekend","type":"Trip","properties":{"Name":"Prague","Budget":750.5,"State":"Booked","When":"2026-09-18T00:00:00Z","Expenses":["hotel"],"Tags":["Art","Music"]},"icon":{"format":"emoji","emoji":"🚆"},"cover":{"format":"gradient","gradient":"sky"},"blocks":[{"type":"heading_2","text":"Saturday"},{"type":"paragraph","text":"Walk across Charles Bridge."},{"type":"link","object_id":"hotel"}]}`,
		"objects/expense.json": `{"formatVersion":"2.0","id":"hotel","type":"Expense","properties":{"Name":"Two hotel nights","Cost state":"Booked","Trip":["weekend"]},"blocks":[{"type":"paragraph","text":"A room for two near the old town."}]}`,
	}
	out := fstest.MapFS{}
	for name, data := range documents {
		out[name] = &fstest.MapFile{Data: []byte(data), Mode: 0o644}
	}
	return out
}

func writeAnyBlockFixture(t *testing.T, fixture fstest.MapFS, mode string) string {
	t.Helper()
	root := t.TempDir()
	if mode == "directory" {
		for name, file := range fixture {
			target := filepath.Join(root, name)
			require.NoError(t, os.MkdirAll(filepath.Dir(target), 0700))
			require.NoError(t, os.WriteFile(target, file.Data, 0600))
		}
		return root
	}
	target := filepath.Join(root, "bundle.zip")
	file, err := os.Create(target)
	require.NoError(t, err)
	archive := zip.NewWriter(file)
	for name, entry := range fixture {
		if mode == "wrapped zip" {
			name = "My bundle/" + name
		}
		writer, err := archive.Create(name)
		require.NoError(t, err)
		_, err = writer.Write(entry.Data)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	require.NoError(t, file.Close())
	return target
}

func importAnyBlockFixture(t *testing.T, input string) (*common.Response, *common.ConvertError) {
	t.Helper()
	return (&Pb{}).GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Type:   model.Import_Pb,
		Mode:   pb.RpcObjectImportRequest_ALL_OR_NOTHING,
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{input}, NoCollection: true}},
	}, process.NewNoOp())
}

func TestAnyBlockBundleImport(t *testing.T) {
	for _, mode := range []string{"directory", "zip", "wrapped zip"} {
		t.Run(mode, func(t *testing.T) {
			result, errs := importAnyBlockFixture(t, writeAnyBlockFixture(t, authoringBundleFixture(), mode))
			require.Nil(t, errs)
			require.NotNil(t, result)
			// Six authored documents, seven properties and six select options. The
			// sidebar is consumed by the collection provider, not installed as an object.
			require.Len(t, result.Snapshots, 19)
			byID := map[string]*common.Snapshot{}
			properties := map[string]*common.Snapshot{}
			for _, snapshot := range result.Snapshots {
				byID[snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyId)] = snapshot
				if snapshot.Snapshot.SbType == smartblock.SmartBlockTypeRelation {
					properties[snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)] = snapshot
				}
			}
			require.Len(t, properties, 7)
			key := func(name string) domain.RelationKey {
				require.Contains(t, properties, name)
				return domain.RelationKey(properties[name].Snapshot.Data.Key)
			}
			weekend, hotel := byID["weekend"], byID["hotel"]
			require.NotNil(t, weekend)
			require.NotNil(t, hotel)
			details := weekend.Snapshot.Data.Details
			assert.Equal(t, 750.5, details.GetFloat64(key("Budget")))
			date, err := time.Parse(time.RFC3339, "2026-09-18T00:00:00Z")
			require.NoError(t, err)
			assert.Equal(t, date.Unix(), details.GetInt64(key("When")))
			assert.Equal(t, []string{hotel.Id}, details.GetStringList(key("Expenses")))
			assert.Equal(t, []string{weekend.Id}, hotel.Snapshot.Data.Details.GetStringList(key("Trip")))
			selected := details.GetStringList(key("State"))
			require.Len(t, selected, 1)
			var booked *common.Snapshot
			for _, snapshot := range result.Snapshots {
				if snapshot.Id == selected[0] {
					booked = snapshot
				}
			}
			require.NotNil(t, booked)
			assert.Equal(t, "Booked", booked.Snapshot.Data.Details.GetString(bundle.RelationKeyName))
			assert.Equal(t, "blue", booked.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationOptionColor))
			assert.NotEqual(t, selected, hotel.Snapshot.Data.Details.GetStringList(key("Cost state")))
			assert.Equal(t, "🚆", details.GetString(bundle.RelationKeyIconEmoji))
			assert.Equal(t, []string{"ot-trip"}, weekend.Snapshot.Data.ObjectTypes)
			assert.Equal(t, byID["type-trip"].Id, byID["template-trip"].Snapshot.Data.Details.GetString(bundle.RelationKeyTargetObjectType))
			assert.Equal(t, byID["template-trip"].Id, byID["type-trip"].Snapshot.Data.Details.GetString(bundle.RelationKeyDefaultTemplateId))
			assert.Equal(t, []string{byID["type-expense"].Id}, properties["Expenses"].Snapshot.Data.Details.GetStringList(bundle.RelationKeyRelationFormatObjectTypes))
			var linked bool
			for _, block := range weekend.Snapshot.Data.Blocks {
				if block.GetLink() != nil {
					assert.Equal(t, hotel.Id, block.GetLink().TargetBlockId)
					linked = true
				}
			}
			assert.True(t, linked)
			for _, block := range byID["home"].Snapshot.Data.Blocks {
				if view := block.GetDataview(); view != nil {
					require.NotEmpty(t, view.Views)
					require.NotEmpty(t, view.Views[0].Filters)
					assert.Equal(t, string(key("State")), view.Views[0].Filters[0].RelationKey)
					assert.Equal(t, selected[0], view.Views[0].Filters[0].Value.GetListValue().Values[0].GetStringValue())
				}
			}
		})
	}
}

func TestAnyBlockInvalidBundle(t *testing.T) {
	for _, tc := range []struct{ name, file, data string }{
		{"missing target", "index.json", `{"formatVersion":"2.0","name":"Invalid","entrypoint":"missing"}`},
		{"future version", "index.json", `{"formatVersion":"3.0","name":"Invalid","entrypoint":"home"}`},
		{"malformed document", "objects/trip.json", `{`},
		{"invalid participant identity", "objects/person.json", `{"formatVersion":"2.0","kind":"participant","id":"participant-person","properties":{"Name":"Person"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := authoringBundleFixture()
			fixture[tc.file] = &fstest.MapFile{Data: []byte(tc.data)}
			response, errs := importAnyBlockFixture(t, writeAnyBlockFixture(t, fixture, "zip"))
			require.NotNil(t, errs)
			assert.Nil(t, response)
			assert.ErrorIs(t, errs.GetResultError(model.Import_Pb), common.ErrPbNotAnyBlockFormat)
		})
	}
}

func TestAnyBlockSingleDocument(t *testing.T) {
	for _, version := range []string{"2.0", "3.0"} {
		t.Run(version, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "note.json")
			data := strings.ReplaceAll(`{"formatVersion":"VERSION","id":"note","type":"Page","properties":{"Name":"A note"},"blocks":[{"type":"paragraph","text":"Hello"}]}`, "VERSION", version)
			require.NoError(t, os.WriteFile(filename, []byte(data), 0600))
			response, errs := importAnyBlockFixture(t, filename)
			if version == "3.0" {
				require.NotNil(t, errs)
				assert.Nil(t, response)
				return
			}
			require.Nil(t, errs)
			require.Len(t, response.Snapshots, 1)
			assert.Equal(t, "A note", response.Snapshots[0].Snapshot.Data.Details.GetString(bundle.RelationKeyName))
		})
	}
}

func TestAnyBlockCanceledBundle(t *testing.T) {
	filename := writeAnyBlockFixture(t, authoringBundleFixture(), "zip")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &Pb{progress: process.NewNoOp()}
	_, found, err := p.anyBlockBundle(ctx, filename)
	assert.True(t, found)
	assert.ErrorIs(t, err, common.ErrCancel)
}

func TestAnyBlockHyphenatedTypeImport(t *testing.T) {
	fixture := authoringBundleFixture()
	for _, file := range fixture {
		file.Data = []byte(strings.NewReplacer(`"internal_key":"trip"`, `"internal_key":"bookclub-trip"`, `"type-trip"`, `"type-bookclub-trip"`).Replace(string(file.Data)))
	}
	result, errs := importAnyBlockFixture(t, writeAnyBlockFixture(t, fixture, "zip"))
	require.Nil(t, errs)
	var typeSnapshot, page *common.Snapshot
	for _, snapshot := range result.Snapshots {
		switch snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyId) {
		case "type-bookclub-trip":
			typeSnapshot = snapshot
		case "weekend":
			page = snapshot
		}
	}
	require.NotNil(t, typeSnapshot)
	require.NotNil(t, page)
	assert.Regexp(t, `^[0-9a-f]{24}$`, typeSnapshot.Snapshot.Data.Key)
	assert.Equal(t, []string{"ot-" + typeSnapshot.Snapshot.Data.Key}, page.Snapshot.Data.ObjectTypes)
}

func TestAnyBlockBundleCollectionAndWidgets(t *testing.T) {
	for _, experience := range []bool{false, true} {
		t.Run(map[bool]string{false: "collection", true: "new space experience"}[experience], func(t *testing.T) {
			filename := writeAnyBlockFixture(t, authoringBundleFixture(), "zip")
			params := &pb.RpcObjectImportRequestPbParams{Path: []string{filename}}
			if experience {
				params.ImportType = pb.RpcObjectImportRequestPbParams_EXPERIENCE
			}
			result, errs := (&Pb{}).GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
				Type: model.Import_Pb, Mode: pb.RpcObjectImportRequest_ALL_OR_NOTHING, IsNewSpace: experience,
				Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: params},
			}, process.NewNoOp())
			require.Nil(t, errs)
			byID := map[string]*common.Snapshot{}
			var widgets, root *common.Snapshot
			for _, snapshot := range result.Snapshots {
				byID[snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyId)] = snapshot
				if snapshot.Snapshot.SbType == smartblock.SmartBlockTypeWidget {
					widgets = snapshot
				}
				if snapshot.Id == result.RootObjectID {
					root = snapshot
				}
			}
			require.Contains(t, byID, "home")
			require.Contains(t, byID, "type-trip")
			if experience {
				require.NotNil(t, widgets)
				var targets []string
				for _, block := range widgets.Snapshot.Data.Blocks {
					if block.GetLink() != nil {
						targets = append(targets, block.GetLink().TargetBlockId)
					}
				}
				assert.Contains(t, targets, byID["home"].Id)
				assert.Contains(t, targets, byID["type-trip"].Id)
			} else {
				require.NotNil(t, root)
				values := root.Snapshot.Data.Collections.GetFields()["objects"].GetListValue().GetValues()
				var targets []string
				for _, value := range values {
					targets = append(targets, value.GetStringValue())
				}
				assert.Contains(t, targets, byID["home"].Id)
				assert.Contains(t, targets, byID["type-trip"].Id)
			}
		})
	}
}

type fixtureTempDir string

func (d fixtureTempDir) TempDir() string { return string(d) }

func TestAnyBlockFullExportImport(t *testing.T) {
	// The canonical AnyBlock exported fixture exercises installed
	// built-ins, stored custom keys, an unresolved property and a binary attachment.
	fixture := fstest.MapFS{}
	err := fs.WalkDir(os.DirFS("testdata/anyblock_full"), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(filepath.Join("testdata/anyblock_full", name))
		if err != nil {
			return err
		}
		fixture[name] = &fstest.MapFile{Data: data}
		return nil
	})
	require.NoError(t, err)
	for _, mode := range []string{"directory", "zip", "wrapped zip"} {
		t.Run(mode, func(t *testing.T) {
			importer := &Pb{tempDirProvider: fixtureTempDir(t.TempDir())}
			result, errs := importer.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
				SpaceId: "root.suffix", Type: model.Import_Pb, Mode: pb.RpcObjectImportRequest_ALL_OR_NOTHING, IsNewSpace: true,
				Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{writeAnyBlockFixture(t, fixture, mode)}, NoCollection: true}},
			}, process.NewNoOp())
			require.Nil(t, errs)
			require.NotNil(t, result)
			var fileFound, spaceFound, widgetsFound, propertyFound bool
			for _, snapshot := range result.Snapshots {
				data := snapshot.Snapshot.Data
				switch snapshot.Snapshot.SbType {
				case smartblock.SmartBlockTypeFileObject:
					fileFound = true
					got, err := os.ReadFile(data.Details.GetString(bundle.RelationKeySource))
					require.NoError(t, err)
					assert.Equal(t, fixture["files/ridge.png"].Data, got)
				case smartblock.SmartBlockTypeWorkspace:
					spaceFound = true
					assert.NotEmpty(t, data.Details.GetString(bundle.RelationKeyHomepage))
					assert.NotEmpty(t, data.Details.GetString(bundle.RelationKeyIconEmoji))
				case smartblock.SmartBlockTypeWidget:
					widgetsFound = true
				case smartblock.SmartBlockTypeRelation:
					if data.Key == "0f1e2d3c4b5a69788796a5b4" {
						propertyFound = true
					}
					assert.NotEqual(t, "deadbeefdeadbeefdeadbeef", data.Key, "unknown format must not become a text property")
				}
			}
			assert.True(t, fileFound)
			assert.True(t, spaceFound)
			assert.True(t, widgetsFound)
			assert.True(t, propertyFound)
		})
	}
}

func TestAnyBlockURLOnlyBookmark(t *testing.T) {
	fixture := fstest.MapFS{
		"index.json": {Data: []byte(`{"formatVersion":"2.0"}`)},
		"page.json":  {Data: []byte(`{"formatVersion":"2.0","id":"page","type":"Page","blocks":[{"type":"bookmark","url":"https://example.com"}]}`)},
	}
	result, errs := importAnyBlockFixture(t, writeAnyBlockFixture(t, fixture, "directory"))
	require.Nil(t, errs)
	require.Len(t, result.Snapshots, 1)
	var found bool
	for _, b := range result.Snapshots[0].Snapshot.Data.Blocks {
		if bookmark := b.GetBookmark(); bookmark != nil {
			found = true
			assert.Equal(t, "https://example.com", bookmark.Url)
			assert.Empty(t, bookmark.TargetObjectId)
		}
	}
	require.True(t, found)
}

func anyBlockTestCid(seed string) string {
	sum, err := mh.Sum([]byte(seed), mh.SHA2_256, -1)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(cid.DagCBOR, sum).String()
}

// A bundle declares WHY an index target dangles (SPEC §2c). A target the
// space DELETED is by-design state: the importer keeps the id, so the widget
// survives and the id can be tombstoned at creation. A target the space had
// no row for stays what it always was on import — the sentinel.
func TestAnyBlockDeclaredDeletedTargetsAreKept(t *testing.T) {
	deleted, absent := anyBlockTestCid("deleted"), anyBlockTestCid("absent")
	fixture := fstest.MapFS{}
	require.NoError(t, fs.WalkDir(os.DirFS("testdata/anyblock_full"), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join("testdata/anyblock_full", name))
		if err != nil {
			return err
		}
		fixture[name] = &fstest.MapFile{Data: data}
		return nil
	}))
	var idx map[string]any
	require.NoError(t, json.Unmarshal(fixture["index.json"].Data, &idx))
	idx["widgets"] = append(idx["widgets"].([]any), map[string]any{"target": deleted}, map[string]any{"target": absent})
	unresolved := idx["unresolved"].(map[string]any)
	unresolved["targets"] = []string{absent, deleted}
	unresolved["deleted"] = []string{deleted}
	indexData, err := json.Marshal(idx)
	require.NoError(t, err)
	fixture["index.json"] = &fstest.MapFile{Data: indexData}

	importer := &Pb{tempDirProvider: fixtureTempDir(t.TempDir())}
	result, errs := importer.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		SpaceId: "root.suffix", Type: model.Import_Pb, Mode: pb.RpcObjectImportRequest_ALL_OR_NOTHING, IsNewSpace: true,
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{Path: []string{writeAnyBlockFixture(t, fixture, "directory")}, NoCollection: true}},
	}, process.NewNoOp())
	require.Nil(t, errs)
	require.NotNil(t, result)

	assert.Equal(t, []string{deleted}, result.KeptIDs, "the creation stage must keep the same ids and tombstone them")
	var widgetTargets []string
	for _, snapshot := range result.Snapshots {
		if snapshot.Snapshot.SbType != smartblock.SmartBlockTypeWidget {
			continue
		}
		for _, block := range snapshot.Snapshot.Data.Blocks {
			if link := block.GetLink(); link != nil {
				widgetTargets = append(widgetTargets, link.TargetBlockId)
			}
		}
	}
	assert.Contains(t, widgetTargets, deleted, "a deleted target keeps its id")
	assert.NotContains(t, widgetTargets, absent, "an absent target takes the sentinel as before")
	assert.Contains(t, widgetTargets, addr.MissingObject)
}
