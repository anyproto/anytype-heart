package export

// anyblockjson_test.go covers the RPC route only: that
// model.Export_AnyBlockJSON reaches the native exporter through the export
// service with the right collection behind it, that its emit still reports
// progress and answers cancellation through the export queue, and that the
// single-object door returns one document. The bundle's CONTENT — layout
// rules, blob binding, determinism — belongs to core/block/export/anyblock
// and pkg/lib/anyblockjson and is proved there, over the corpus.

import (
	"archive/zip"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/export/collect"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// anyblockSpace seeds one object of every kind that owns a bundle
// directory, in the store and in the mocks, and returns the ids by the
// directory each must land in. Seven kinds, seven directories: the layout
// is the format's own vocabulary (design §1.2), and a format value that
// collected the wrong closure would lose most of them.
func anyblockSpace(t *testing.T, fx *fixture) map[string]string {
	const (
		pageId     = "pageId"
		typeId     = "customObjectType"
		templateId = "templateId"
		fileId     = "fileObjectId"
		optionId   = "optionId"
	)
	const propertyKey = domain.RelationKey("customProperty")

	_, pub, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	identity := pub.Account()
	participantId := domain.NewParticipantId(spaceId, identity)

	fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
		prepareTestObjectForStore(pageId, typeId),
		prepareTestObjectTypeForStore(t, typeId, nil),
		{
			bundle.RelationKeyId:               domain.String(templateId),
			bundle.RelationKeyName:             domain.String("Template"),
			bundle.RelationKeyTargetObjectType: domain.String(typeId),
			bundle.RelationKeyType:             domain.String(typeId),
			bundle.RelationKeySpaceId:          domain.String(spaceId),
		},
		prepareTestRelationForStore(t, propertyKey, int64(model.RelationFormat_tag)),
		prepareTestOptionForStore(t, propertyKey, optionId),
		{
			bundle.RelationKeyId:           domain.String(fileId),
			bundle.RelationKeyName:         domain.String("notes"),
			bundle.RelationKeyFileExt:      domain.String("txt"),
			bundle.RelationKeyFileMimeType: domain.String("text/plain"),
			bundle.RelationKeyType:         domain.String(typeId),
			bundle.RelationKeySpaceId:      domain.String(spaceId),
		},
		{
			bundle.RelationKeyId:        domain.String(participantId),
			bundle.RelationKeyIdentity:  domain.String(identity),
			bundle.RelationKeyName:      domain.String("Someone"),
			bundle.RelationKeyType:      domain.String(typeId),
			bundle.RelationKeySpaceId:   domain.String(spaceId),
			bundle.RelationKeyUniqueKey: domain.String("participant" + identity),
		},
	})

	typeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, typeId)
	require.NoError(t, err)

	objects := []struct {
		id      string
		sbType  smartblock.SmartBlockType
		details map[domain.RelationKey]domain.Value
	}{
		{id: pageId, sbType: smartblock.SmartBlockTypePage},
		// the unique key is what the manifest's type table is keyed by, so
		// the type document carries its own
		{id: typeId, sbType: smartblock.SmartBlockTypeObjectType, details: map[domain.RelationKey]domain.Value{
			bundle.RelationKeyUniqueKey: domain.String(typeUniqueKey.Marshal()),
		}},
		{id: templateId, sbType: smartblock.SmartBlockTypeTemplate},
		{id: propertyKey.String(), sbType: smartblock.SmartBlockTypeRelation},
		{id: optionId, sbType: smartblock.SmartBlockTypeRelationOption},
		{id: fileId, sbType: smartblock.SmartBlockTypeFileObject},
		{id: participantId, sbType: smartblock.SmartBlockTypeParticipant},
	}
	for _, object := range objects {
		loaded := setupObject(object.id, typeId, object.sbType, object.details)
		fx.picker.EXPECT().GetObject(mock.Anything, object.id).Return(loaded, nil).Maybe()
		fx.sbtProvider.EXPECT().Type(spaceId, object.id).Return(object.sbType, nil).Maybe()
	}

	return map[string]string{
		"objects":    pageId,
		"types":      typeId,
		"templates":  templateId,
		"properties": propertyKey.String(),
		"options":    optionId,
		"files":      fileId,
		// the STORE id, not the bare identity: the participant fold needs a
		// real space id (`<cid>.<key>`) to parse, and a fixture's "space1"
		// does not — so the envelope keeps the composite, and the filename
		// follows the envelope either way (design §1.3). The fold itself is
		// covered where it lives, in the codec's own tests.
		"participants": participantId,
	}
}

// readExportTree reads every file under root into path -> content, paths
// slash-separated and relative to the export root.
func readExportTree(t *testing.T, root string) map[string]string {
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		tree[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	require.NoError(t, err)
	return tree
}

// The whole route, from the RPC request to the bundle on disk: the format
// reaches the native exporter, the collection behind it is the derived one
// (nothing else would carry types, options and templates into a
// whole-space export), every document lands in its kind directory under
// its own id, and the two bundle files read back through the package that
// wrote them.
//
// How this can fail: route the format to ClosureContent and five of the
// seven directories vanish (only pages and file objects survive that
// closure); send it down the legacy writeDoc path and the extension
// becomes .pb.json in relations/ and relationsOptions/; forget the bundle
// files and a reader has no property dictionary to resolve keys against.
func TestExport_AnyBlockJSONWritesABundle(t *testing.T) {
	// given
	fx := newFixture(t)
	byDirectory := anyblockSpace(t, fx)
	fx.picker.EXPECT().TryRemoveFromCache(mock.Anything, mock.Anything).Return(true, nil)

	// when
	exportPath, succeed, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
		SpaceId:         spaceId,
		Path:            t.TempDir(),
		Format:          model.Export_AnyBlockJSON,
		IncludeArchived: true,
		NoProgress:      true,
	})

	// then
	require.NoError(t, err)
	assert.Equal(t, len(byDirectory), succeed)

	tree := readExportTree(t, exportPath)
	for dir, id := range byDirectory {
		assert.Contains(t, tree, path.Join(dir, id+".anyblock.json"), "%s belongs in %s/", id, dir)
	}
	require.Contains(t, tree, anyblockjson.IndexFileName)
	require.Contains(t, tree, anyblockjson.PropertiesFileName)
	for name := range tree {
		if name == anyblockjson.IndexFileName || name == anyblockjson.PropertiesFileName {
			continue
		}
		assert.True(t, strings.HasSuffix(name, ".anyblock.json"), "unexpected file %q in the bundle", name)
	}

	index, err := anyblockjson.UnmarshalIndex([]byte(tree[anyblockjson.IndexFileName]))
	require.NoError(t, err)
	require.NotNil(t, index.Manifest)
	assert.Equal(t, "types/customObjectType.anyblock.json", index.Manifest.Types["customObjectType"])
	_, err = anyblockjson.UnmarshalPropertyDictionary([]byte(tree[anyblockjson.PropertiesFileName]))
	require.NoError(t, err)
}

// The same bundle into a zip archive, which is what a real backup takes:
// the native exporter writes through the export service's own writers, and
// the zip one is the only writer whose paths are not the filesystem's.
//
// How this can fail: build bundle paths with filepath.Join on a platform
// whose separator is not "/" and the archive grows entries no reader can
// resolve; or write the bundle files after Close and lose them silently.
func TestExport_AnyBlockJSONWritesAZipBundle(t *testing.T) {
	// given
	fx := newFixture(t)
	byDirectory := anyblockSpace(t, fx)
	fx.picker.EXPECT().TryRemoveFromCache(mock.Anything, mock.Anything).Return(true, nil)

	// when
	archivePath, succeed, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
		SpaceId:         spaceId,
		Path:            t.TempDir(),
		Format:          model.Export_AnyBlockJSON,
		IncludeArchived: true,
		NoProgress:      true,
		Zip:             true,
	})

	// then
	require.NoError(t, err)
	assert.Equal(t, len(byDirectory), succeed)

	reader, err := zip.OpenReader(archivePath)
	require.NoError(t, err)
	defer reader.Close()
	entries := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		entries[file.Name] = true
	}
	assert.Len(t, entries, len(byDirectory)+2) // the documents, index.json, properties.json
	for dir, id := range byDirectory {
		assert.True(t, entries[dir+"/"+id+".anyblock.json"], "%s belongs in %s/", id, dir)
	}
	assert.True(t, entries[anyblockjson.IndexFileName])
	assert.True(t, entries[anyblockjson.PropertiesFileName])
}

// Emit runs as queue tasks, so the process the client already watches
// counts this format's documents like every other format's — the native
// exporter's own bounded pool would have left the progress bar at 0/0 for
// the whole export.
//
// How this can fail: give the exporter its internal runner here (Total
// stays 0); or bound the queue somewhere other than exportWorkers, and the
// resident content set stops being the width §1.6 measured.
func TestExport_AnyBlockJSONReportsQueueProgress(t *testing.T) {
	// given
	fx := newFixture(t)
	byDirectory := anyblockSpace(t, fx)
	fx.picker.EXPECT().TryRemoveFromCache(mock.Anything, mock.Anything).Return(true, nil)

	queue := fx.processService.NewQueue(pb.ModelProcess{
		Id:      "anyblockjson",
		Message: &pb.ModelProcessMessageOfExport{Export: &pb.ModelProcessExport{}},
	}, exportWorkers(model.Export_AnyBlockJSON), true, fx.notifications)
	require.NoError(t, queue.Start())

	exportCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
		SpaceId:         spaceId,
		Path:            t.TempDir(),
		Format:          model.Export_AnyBlockJSON,
		IncludeArchived: true,
		NoProgress:      true,
	})
	require.NoError(t, exportCtx.docsForExport(context.Background()))
	wr, err := newDirWriter(exportCtx.path, false)
	require.NoError(t, err)

	// when
	succeed, err := exportCtx.exportByFormat(context.Background(), wr, queue)

	// then
	require.NoError(t, err)
	assert.Equal(t, len(byDirectory), succeed)
	require.NoError(t, queue.Finalize()) // waits for the workers, so Done is settled
	progress := queue.Info().Progress
	assert.Equal(t, int64(len(byDirectory)), progress.Total)
	assert.Equal(t, int64(len(byDirectory)), progress.Done)
}

// A cancelled export stops loading objects. The picker mock is the
// assertion: no TryRemoveFromCache expectation is set, and emit closes
// every object it loads — so a single task that ran would fail the test.
//
// How this can fail: drop the ctx check from the emit task and a cancelled
// account-sized export keeps cold-building trees for minutes; write the
// bundle files anyway and index.json claims documents that were never
// emitted.
func TestExport_AnyBlockJSONStopsWhenCancelled(t *testing.T) {
	// given
	fx := newFixture(t)
	anyblockSpace(t, fx)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// when
	exportPath, succeed, err := fx.Export(ctx, pb.RpcObjectListExportRequest{
		SpaceId:         spaceId,
		Path:            t.TempDir(),
		Format:          model.Export_AnyBlockJSON,
		IncludeArchived: true,
		NoProgress:      true,
	})

	// then
	require.NoError(t, err) // the cancel is the user's own request, not a failure
	assert.Equal(t, 0, succeed)
	assert.NoDirExists(t, exportPath, "a cancelled export leaves nothing behind")
}

// ExportSingleInMemory answers with ONE document — no index.json, no
// property dictionary, nothing a bundle would add (design Q7, principle 7:
// a document stands alone).
//
// How this can fail: fall through to the legacy converter switch, where
// this format has no case, and the export panics on a nil converter.
func TestExport_AnyBlockJSONSingleInMemory(t *testing.T) {
	// given
	fx := newFixture(t)
	byDirectory := anyblockSpace(t, fx)

	// when
	result, err := fx.ExportSingleInMemory(context.Background(), spaceId, byDirectory["objects"], model.Export_AnyBlockJSON)

	// then
	require.NoError(t, err)
	sbType, snapshot, err := anyblockjson.Unmarshal([]byte(result), anyblockjson.Options{SpaceId: spaceId})
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_Page, sbType)
	assert.Equal(t, byDirectory["objects"], snapshot.GetDetails().GetFields()[bundle.RelationKeyId.String()].GetStringValue())
}

// The closure every format collects with. Pinned as a table because the
// predicate this replaced was named isAnyblockExport and meant "protobuf
// or pb.json" — the one place where a wrong answer is invisible until a
// bundle silently ships without its types.
func TestClosureForFormat(t *testing.T) {
	for format, want := range map[model.ExportFormat]collect.Closure{
		model.Export_Protobuf:     collect.ClosureDerived,
		model.Export_JSON:         collect.ClosureDerived,
		model.Export_AnyBlockJSON: collect.ClosureDerived,
		model.Export_Markdown:     collect.ClosureContent,
		model.Export_DOT:          collect.ClosureContent,
		model.Export_SVG:          collect.ClosureContent,
		model.Export_GRAPH_JSON:   collect.ClosureContent,
	} {
		assert.Equal(t, want, closureForFormat(format), "closure for %v", format)
	}
}
