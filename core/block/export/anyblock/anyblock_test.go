package anyblock

// anyblock_test.go drives the native exporter end to end over the store
// fixture: the real collection layer (export.New's Collect), the real plan,
// emit and composition, into a real directory tree. The headline test is
// determinism — export the same space twice, compare trees byte for byte —
// which is the property the whole §1.3 naming decision exists to guarantee,
// proved rather than asserted.

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/export"
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

type fixture struct {
	exporter *Exporter
	store    *objectstore.StoreFixture
	picker   *mock_cache.MockObjectGetterComponent
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

	a := &app.App{}
	a.Register(storeFixture)
	a.Register(testutil.PrepareMock(context.Background(), a, mock_event.NewMockSender(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter))
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
		exporter: &Exporter{
			Collector:   exp,
			Picker:      objectGetter,
			ObjectStore: storeFixture,
			SbtProvider: provider,
		},
		store:    storeFixture,
		picker:   objectGetter,
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
func setupSpace(t *testing.T, fx *fixture) Request {
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

	return Request{SpaceId: spaceId, SpaceName: "Fixture space", IncludeArchived: true}
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
	wr, err := NewDirWriter(dir)
	require.NoError(t, err)

	// when
	succeed, err := fx.exporter.Export(context.Background(), req, wr)

	// then
	require.NoError(t, err)
	assert.Equal(t, 2, succeed)

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
		wr, err := NewDirWriter(dir)
		require.NoError(t, err)
		succeed, err := fx.exporter.Export(context.Background(), req, wr)
		require.NoError(t, err)
		require.Equal(t, 2, succeed)
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
	wr, err := NewDirWriter(filepath.Join(dir, "bundle"))
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
