package anyblock_test

// anyblock_test.go drives the native exporter end to end over the store
// fixture: the real collection layer (export.New's Collect), the real plan,
// emit and composition, into a real directory tree. The headline test is
// determinism — export the same space twice, compare trees byte for byte —
// which is the property the whole §1.3 naming decision exists to guarantee,
// proved rather than asserted.
//
// It is an EXTERNAL test package because it builds the real export service
// for that collection seam, and package export now routes
// model.Export_AnyBlockJSON back into this package — an in-package test
// would close that import cycle. Nothing here needs unexported access.

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/export/anyblock"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const spaceId = "space1"

// closingPicker wraps the object-getter mock with the TryRemoveFromCache
// the Exporter's typed Picker demands, recording every close so the tests
// can prove close-after-write actually runs — the memory model (design
// §1.5/§1.6) is a claim about this call being made, and a fixture that
// cannot observe it would leave the whole path uncovered.
type closingPicker struct {
	*mock_cache.MockObjectGetterComponent
	mu      sync.Mutex
	removed map[string]int
}

func (p *closingPicker) TryRemoveFromCache(_ context.Context, objectId string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removed == nil {
		p.removed = map[string]int{}
	}
	p.removed[objectId]++
	return true, nil
}

func (p *closingPicker) removedIds() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]int, len(p.removed))
	for k, v := range p.removed {
		out[k] = v
	}
	return out
}

type fixture struct {
	exporter *anyblock.Exporter
	store    *objectstore.StoreFixture
	picker   *closingPicker
	provider *mock_typeprovider.MockSmartBlockTypeProvider
}

// newFixture builds the REAL export service for its Collect seam (the same
// app wiring core/publish's tests use), so this package's tests run the
// production collection rather than a stub of it.
func newFixture(t *testing.T) *fixture {
	storeFixture := objectstore.NewStoreFixture(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)

	fetcher := mock_relationutils.NewMockRelationFormatFetcher(t)
	fetcher.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(
		func(_ string, key domain.RelationKey) (model.RelationFormat, error) {
			rel, err := bundle.GetRelation(key)
			if err != nil {
				return 0, err
			}
			return rel.Format, nil
		}).Maybe()

	picker := &closingPicker{MockObjectGetterComponent: objectGetter}

	a := &app.App{}
	a.Register(storeFixture)
	a.Register(testutil.PrepareMock(context.Background(), a, mock_event.NewMockSender(t)))
	// the CLOSING picker is what the app holds, not the bare getter mock:
	// export.Init resolves cache.CachedObjectGetter (the service's own
	// picker field is typed that way now), which only the wrapper answers
	testutil.PrepareMock(context.Background(), a, objectGetter)
	a.Register(picker)
	a.Register(process.New())
	a.Register(testutil.PrepareMock(context.Background(), a, mock_space.NewMockService(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, provider))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_files.NewMockService(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_account.NewMockService(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_notifications.NewMockNotifications(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, fetcher))

	exp := export.New()
	require.NoError(t, exp.Init(a))

	return &fixture{
		exporter: &anyblock.Exporter{
			Collector:   exp,
			Picker:      picker,
			ObjectStore: storeFixture,
			SbtProvider: provider,
		},
		store:    storeFixture,
		picker:   picker,
		provider: provider,
	}
}

func setupObject(id, typeId string, sbType smartblock.SmartBlockType, details map[domain.RelationKey]domain.Value) *smarttest.SmartTest {
	smartBlockTest := smarttest.New(id)
	if details == nil {
		details = map[domain.RelationKey]domain.Value{}
	}
	details[bundle.RelationKeyId] = domain.String(id)
	details[bundle.RelationKeyType] = domain.String(typeId)
	doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(details))
	doc.AddBundledRelationLinks(maps.Keys(details)...)
	smartBlockTest.Doc = doc
	smartBlockTest.SetType(sbType)
	return smartBlockTest
}

// setupSpace seeds one small space — a named page and its custom type — in
// both the store fixture and the object mocks, and returns the export
// request that covers it.
func setupSpace(t *testing.T, fx *fixture) anyblock.Request {
	const (
		objectId = "objectId"
		typeId   = "customObjectType"
	)
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, typeId)
	require.NoError(t, err)

	fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:      domain.String(objectId),
			bundle.RelationKeyType:    domain.String(typeId),
			bundle.RelationKeyName:    domain.String("Root page"),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		},
		{
			bundle.RelationKeyId:        domain.String(typeId),
			bundle.RelationKeyUniqueKey: domain.String(uk.Marshal()),
			bundle.RelationKeyName:      domain.String("Custom type"),
			bundle.RelationKeyLayout:    domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeySpaceId:   domain.String(spaceId),
			bundle.RelationKeyType:      domain.String(typeId),
		},
	})

	page := setupObject(objectId, typeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String("Root page"),
	})
	objectType := setupObject(typeId, typeId, smartblock.SmartBlockTypeObjectType, map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:      domain.String("Custom type"),
		bundle.RelationKeyUniqueKey: domain.String(uk.Marshal()),
	})
	fx.picker.EXPECT().GetObject(mock.Anything, objectId).Return(page, nil)
	fx.picker.EXPECT().GetObject(mock.Anything, typeId).Return(objectType, nil)

	fx.provider.EXPECT().Type(spaceId, objectId).Return(smartblock.SmartBlockTypePage, nil)
	fx.provider.EXPECT().Type(spaceId, typeId).Return(smartblock.SmartBlockTypeObjectType, nil)

	return anyblock.Request{SpaceId: spaceId, SpaceName: "Fixture space", IncludeArchived: true}
}

// readTree reads every file below root into path → content bytes.
func readTree(t *testing.T, root string) map[string]string {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	require.NoError(t, err)
	return out
}

// The bundle a space exports to: every document at its planned id path,
// index.json carrying the manifest, properties.json beside it — and every
// file the exporter wrote reads back through the package's own Unmarshal
// (I1, at both scopes).
//
// How this can fail: route a kind to the wrong directory (the path
// assertions go red); let the manifest carry an absolute or writer-rooted
// path (the manifest assertion catches the bundle-relative contract
// breaking); or write a document the codec refuses (the Unmarshal loop
// finds it at export time, which is the whole point of I1).
func TestExporter_WritesABundle(t *testing.T) {
	// given
	fx := newFixture(t)
	req := setupSpace(t, fx)
	dir := t.TempDir()
	wr, err := anyblock.NewDirWriter(dir)
	require.NoError(t, err)

	// when
	result, err := fx.exporter.Export(context.Background(), req, wr)

	// then
	require.NoError(t, err)
	assert.Equal(t, anyblock.Result{Succeed: 2}, result)

	// close-after-write ran for every emitted object — the memory model is
	// this call being made (design §1.5), proved rather than assumed
	removed := fx.picker.removedIds()
	assert.Contains(t, removed, "objectId")
	assert.Contains(t, removed, "customObjectType")

	tree := readTree(t, dir)
	require.Contains(t, tree, "objects/objectId.anyblock.json")
	require.Contains(t, tree, "types/customObjectType.anyblock.json")
	require.Contains(t, tree, anyblockjson.IndexFileName)
	require.Contains(t, tree, anyblockjson.PropertiesFileName)

	idx, err := anyblockjson.UnmarshalIndex([]byte(tree[anyblockjson.IndexFileName]))
	require.NoError(t, err)
	assert.Equal(t, "Fixture space", idx.Name, "no space document in this fixture, so the request name is the fallback")
	require.NotNil(t, idx.Manifest)
	assert.Equal(t, map[string]string{"customObjectType": "types/customObjectType.anyblock.json"}, idx.Manifest.Types)
	assert.Equal(t, anyblockjson.PropertiesFileName, idx.Manifest.Properties)

	_, err = anyblockjson.UnmarshalPropertyDictionary([]byte(tree[anyblockjson.PropertiesFileName]))
	require.NoError(t, err)

	opts := storeresolver.New(fx.store.SpaceIndex(spaceId)).Options()
	for path, content := range tree {
		if path == anyblockjson.IndexFileName || path == anyblockjson.PropertiesFileName {
			continue
		}
		_, _, err := anyblockjson.Unmarshal([]byte(content), opts)
		require.NoError(t, err, "document %s must read back through the codec (I1)", path)
	}
}

// Composing the same space twice produces byte-identical trees. This is the
// property the §1.3 naming decision exists to guarantee — every path a pure
// function of the id, no collision machinery, nothing first-writer-wins —
// and the §1.5 concurrency design promises it survives the width-bounded
// emit. Proved here with a test, not asserted in a comment.
//
// How this can fail: reintroduce anything ordering-sensitive — a namer with
// a dedup counter (the legacy namer's rand.Int63n suffix is the exhibit), a
// composer aggregate the finish does not sort, a timestamp in file CONTENT
// — and the two trees diverge.
func TestExporter_SameSpaceTwiceIsByteIdentical(t *testing.T) {
	// given
	fx := newFixture(t)
	req := setupSpace(t, fx)

	runExport := func(t *testing.T) map[string]string {
		dir := t.TempDir()
		wr, err := anyblock.NewDirWriter(dir)
		require.NoError(t, err)
		result, err := fx.exporter.Export(context.Background(), req, wr)
		require.NoError(t, err)
		require.Equal(t, anyblock.Result{Succeed: 2}, result)
		return readTree(t, dir)
	}

	// when
	first := runExport(t)
	second := runExport(t)

	// then
	require.ElementsMatch(t, maps.Keys(first), maps.Keys(second), "same file set")
	for path, content := range first {
		assert.Equal(t, content, second[path], "file %s must be byte-identical across runs", path)
	}
}

// The blob path plan and the writer's containment guard, at the wiring
// level: a path the plan did not mint may not escape the root.
func TestDirWriter_RefusesEscape(t *testing.T) {
	dir := t.TempDir()
	wr, err := anyblock.NewDirWriter(filepath.Join(dir, "bundle"))
	require.NoError(t, err)
	err = wr.WriteFile("../outside.txt", bytesReader("x"), 0)
	require.Error(t, err)
}

func bytesReader(s string) *os.File {
	f, _ := os.CreateTemp("", "anyblocktest")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}

// fileObjectWrapper marries a smarttest object with a mocked file
// component, the same pattern core/publish's tests use.
type fileObjectWrapper struct {
	*smarttest.SmartTest
	editorsb.FileObject
}

// A file object exports as BOTH halves, adjacent in files/ — the document
// at its id path, the bytes beside it under the sanitized extension — and
// the manifest `files` map is what binds them (§2c, v0.47). The stored
// file_ext here is deliberately EMPTY (431 corpus files), so the extension
// falls back to the mime table; and the document must NOT grow a path
// member — the Source clobber does not carry over.
//
// How this can fail: write the blob before it streams cleanly and observe
// it anyway (the manifest points at bytes that are not there); reintroduce
// the Source detail (the marshalled document diff shows an archive path in
// a user relation); or plan the blob from the sanitized extension but bind
// nothing (the blob is orphaned the moment the layout convention changes).
func TestExporter_StreamsBlobsAndBindsThemInTheManifest(t *testing.T) {
	// given
	fx := newFixture(t)
	const fileId = "fileObjectId"
	fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:           domain.String(fileId),
			bundle.RelationKeyName:         domain.String("notes"),
			bundle.RelationKeyFileExt:      domain.String(""), // the corpus's commonest dirt: no extension at all
			bundle.RelationKeyFileMimeType: domain.String("text/plain"),
			bundle.RelationKeySpaceId:      domain.String(spaceId),
		},
	})

	fileSb := setupObject(fileId, "", smartblock.SmartBlockTypeFileObject, map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String("notes"),
	})
	blob, err := os.CreateTemp(t.TempDir(), "blob")
	require.NoError(t, err)
	_, err = blob.WriteString("file bytes travel")
	require.NoError(t, err)
	_, err = blob.Seek(0, 0)
	require.NoError(t, err)

	fileData := mock_files.NewMockFile(t)
	fileData.EXPECT().MimeType().Return("text/plain")
	fileData.EXPECT().Reader(mock.Anything).Return(blob, nil)
	fileData.EXPECT().LastModifiedDate().Return(int64(1700000000))
	fileComponent := mock_fileobject.NewMockFileObject(t)
	fileComponent.EXPECT().GetFile().Return(fileData, nil)

	fx.picker.EXPECT().GetObject(mock.Anything, fileId).
		Return(&fileObjectWrapper{SmartTest: fileSb, FileObject: fileComponent}, nil)
	fx.provider.EXPECT().Type(spaceId, fileId).Return(smartblock.SmartBlockTypeFileObject, nil)

	dir := t.TempDir()
	wr, err := anyblock.NewDirWriter(dir)
	require.NoError(t, err)

	// when
	result, err := fx.exporter.Export(context.Background(),
		anyblock.Request{SpaceId: spaceId, SpaceName: "Fixture space", IncludeArchived: true, IncludeFiles: true}, wr)

	// then
	require.NoError(t, err)
	assert.Equal(t, anyblock.Result{Succeed: 1}, result)

	tree := readTree(t, dir)
	require.Contains(t, tree, "files/fileObjectId.anyblock.json", "the document half")
	require.Contains(t, tree, "files/fileObjectId.txt", "the bytes, same stem, mime-derived extension")
	assert.Equal(t, "file bytes travel", tree["files/fileObjectId.txt"])
	assert.NotContains(t, tree["files/fileObjectId.anyblock.json"], "files/fileObjectId.txt",
		"a document member is not a slot for archive bookkeeping (§1.4)")

	idx, err := anyblockjson.UnmarshalIndex([]byte(tree[anyblockjson.IndexFileName]))
	require.NoError(t, err)
	require.NotNil(t, idx.Manifest)
	assert.Equal(t, map[string]string{fileId: "files/fileObjectId.txt"}, idx.Manifest.Files,
		"the manifest map is the binding a reader may rely on")
}

// A blob the node cannot serve does not fail the document: the document is
// already written and carries strictly more than the nothing a failed doc
// leaves. The failure is COUNTED — Result.BlobErrors — and the manifest
// omits the binding, which is what the bundle tooling's unbound-file
// warning then surfaces (§2c).
//
// How this can fail: return the stream error from the emit task (the doc
// counts as failed while sitting written in the bundle — the old
// behaviour); or bind the blob before the stream succeeds (the manifest
// points at bytes that are not there, the exact promise CheckManifestFiles
// refuses).
func TestExporter_ABlobFailureIsCountedNotFatal(t *testing.T) {
	// given — a file object whose file component cannot serve bytes
	fx := newFixture(t)
	const fileId = "brokenFileId"
	fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:           domain.String(fileId),
			bundle.RelationKeyName:         domain.String("gone"),
			bundle.RelationKeyFileMimeType: domain.String("image/png"),
			bundle.RelationKeySpaceId:      domain.String(spaceId),
		},
	})
	fileSb := setupObject(fileId, "", smartblock.SmartBlockTypeFileObject, map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String("gone"),
	})
	fileComponent := mock_fileobject.NewMockFileObject(t)
	fileComponent.EXPECT().GetFile().Return(nil, os.ErrNotExist)
	fx.picker.EXPECT().GetObject(mock.Anything, fileId).
		Return(&fileObjectWrapper{SmartTest: fileSb, FileObject: fileComponent}, nil)
	fx.provider.EXPECT().Type(spaceId, fileId).Return(smartblock.SmartBlockTypeFileObject, nil)

	dir := t.TempDir()
	wr, err := anyblock.NewDirWriter(dir)
	require.NoError(t, err)

	// when
	result, err := fx.exporter.Export(context.Background(),
		anyblock.Request{SpaceId: spaceId, SpaceName: "Fixture space", IncludeArchived: true, IncludeFiles: true}, wr)

	// then — the document travels, the failure is counted, nothing binds
	require.NoError(t, err)
	assert.Equal(t, anyblock.Result{Succeed: 1, BlobErrors: 1}, result)
	tree := readTree(t, dir)
	require.Contains(t, tree, "files/brokenFileId.anyblock.json", "the document half still travels")
	idx, err := anyblockjson.UnmarshalIndex([]byte(tree[anyblockjson.IndexFileName]))
	require.NoError(t, err)
	require.NotNil(t, idx.Manifest)
	assert.Empty(t, idx.Manifest.Files, "no binding for bytes that did not travel")
}

// failAfterReader serves a few bytes and then errors — the shape of a
// node that stops serving blocks mid-file ("failed to fetch all nodes",
// seen live on the corpus sweep).
type failAfterReader struct{ served bool }

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.served {
		return 0, os.ErrDeadlineExceeded
	}
	r.served = true
	return copy(p, []byte("truncated")), nil
}
func (r *failAfterReader) Seek(offset int64, whence int) (int64, error) { return 0, nil }
func (r *failAfterReader) Close() error                                 { return nil }

// A stream that dies MID-COPY leaves no partial blob behind: truncated
// bytes a reader may trust are worse than absent bytes, so the writer's
// cleanup hook removes what the failed copy wrote. Caught live: the corpus
// files-on sweep left files/…jpg truncated on disk, flagged only by the
// orphan check.
//
// How this can fail: skip the RemoveFile hook (the truncated blob ships,
// unbound, and every downstream reader that ignores the manifest trusts
// it); or bind the blob before the copy finishes (the manifest points at
// truncated bytes, which is strictly worse).
func TestExporter_AMidStreamFailureLeavesNoPartialBlob(t *testing.T) {
	fx := newFixture(t)
	const fileId = "truncatedFileId"
	fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:      domain.String(fileId),
			bundle.RelationKeyName:    domain.String("cut"),
			bundle.RelationKeyFileExt: domain.String("jpg"),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		},
	})
	fileSb := setupObject(fileId, "", smartblock.SmartBlockTypeFileObject, map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String("cut"),
	})
	fileData := mock_files.NewMockFile(t)
	fileData.EXPECT().MimeType().Return("application/octet-stream")
	fileData.EXPECT().Reader(mock.Anything).Return(&failAfterReader{}, nil)
	fileData.EXPECT().LastModifiedDate().Return(int64(1700000000)).Maybe()
	fileComponent := mock_fileobject.NewMockFileObject(t)
	fileComponent.EXPECT().GetFile().Return(fileData, nil)
	fx.picker.EXPECT().GetObject(mock.Anything, fileId).
		Return(&fileObjectWrapper{SmartTest: fileSb, FileObject: fileComponent}, nil)
	fx.provider.EXPECT().Type(spaceId, fileId).Return(smartblock.SmartBlockTypeFileObject, nil)

	dir := t.TempDir()
	wr, err := anyblock.NewDirWriter(dir)
	require.NoError(t, err)

	result, err := fx.exporter.Export(context.Background(),
		anyblock.Request{SpaceId: spaceId, SpaceName: "Fixture space", IncludeArchived: true, IncludeFiles: true}, wr)

	require.NoError(t, err)
	assert.Equal(t, anyblock.Result{Succeed: 1, BlobErrors: 1}, result)
	tree := readTree(t, dir)
	require.Contains(t, tree, "files/truncatedFileId.anyblock.json")
	assert.NotContains(t, tree, "files/truncatedFileId.bin", "the partial blob must be cleaned up")
}
