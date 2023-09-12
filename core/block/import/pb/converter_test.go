package pb

import (
	"archive/zip"
	"bufio"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_GetSnapshotsSuccess(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open("testdata/bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb")
	reader := bufio.NewReader(f)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb", reader))
	assert.NoError(t, wr.Close())

	p := &Pb{}

	zipPath := wr.Path()
	res, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{zipPath},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.Nil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)

	assert.Contains(t, res.Snapshots[1].FileName, rootCollectionName)
	assert.NotEmpty(t, res.Snapshots[1].Snapshot.Data.ObjectTypes)
	assert.Equal(t, res.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())
}

func Test_GetSnapshotsFailedReadZip(t *testing.T) {
	p := &Pb{}

	_, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{"not exist.zip"},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.NotNil(t, ce)
}

func Test_GetSnapshotsFailedToGetSnapshot(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open("testdata/test.pb")
	reader := bufio.NewReader(f)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("test.pb", reader))
	assert.NoError(t, wr.Close())

	p := &Pb{}

	_, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{wr.Path()},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.NotNil(t, ce)
	assert.False(t, ce.IsEmpty())
}

func Test_GetSnapshotsFailedToGetSnapshotForTwoFiles(t *testing.T) {
	p := &Pb{}

	paths := []string{"testdata/bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb", "testdata/test.pb"}
	// ALL_OR_NOTHING mode
	res, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: paths,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.NotNil(t, ce)
	assert.Nil(t, res)

	// IGNORE_ERRORS mode
	res, ce = p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: paths,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.NotNil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)
	assert.False(t, ce.IsEmpty())
}

func Test_GetSnapshotsWithoutRootCollection(t *testing.T) {
	p := &Pb{}

	path := "testdata/bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb"
	res, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path:         []string{path},
			NoCollection: true,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.Nil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 1)
}

func Test_GetSnapshotsSkipFileWithoutExtension(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)

	f, err := os.Open("testdata/bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb")
	assert.NoError(t, err)
	reader := bufio.NewReader(f)

	assert.NoError(t, wr.WriteFile("bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb", reader))

	f, err = os.Open("testdata/test")
	assert.NoError(t, err)
	reader = bufio.NewReader(f)

	assert.NoError(t, wr.WriteFile("test", reader))
	assert.NoError(t, wr.Close())
	p := &Pb{}

	zipPath := wr.Path()
	res, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{zipPath},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import), 0)

	assert.Nil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)

	assert.Equal(t, res.Snapshots[0].FileName, "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb")
	assert.Equal(t, res.Snapshots[1].FileName, rootCollectionName)
}

func newZipWriter(path string) (*zipWriter, error) {
	filename := filepath.Join(path, "Anytype"+strconv.FormatInt(rand.Int63(), 10)+".zip")
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	return &zipWriter{
		path: filename,
		zw:   zip.NewWriter(f),
		f:    f,
	}, nil
}

type zipWriter struct {
	path string
	zw   *zip.Writer
	f    io.Closer
	m    sync.Mutex
}

func (d *zipWriter) WriteFile(filename string, r io.Reader) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	zf, err := d.zw.Create(filename)
	if err != nil {
		return
	}
	_, err = io.Copy(zf, r)
	return
}

func (d *zipWriter) Path() string {
	return d.path
}

func (d *zipWriter) Close() (err error) {
	if err = d.zw.Close(); err != nil {
		return
	}
	return d.f.Close()
}

func TestPb_provideRootCollection(t *testing.T) {
	t.Run("no snapshots - root collection without objects", func(t *testing.T) {
		// given
		p := Pb{}

		// when
		collection, err := p.provideRootCollection(nil, nil, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 0)
	})
	t.Run("no widget object - add all objects (except template and subobjects) in Protobuf Import collection", func(t *testing.T) {
		// given
		p := Pb{}
		allSnapshot := []*converter.Snapshot{
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
			},
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
			},
		}

		// when
		collection, err := p.provideRootCollection(allSnapshot, nil, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 2)
		assert.Equal(t, objectsInCollection[0], "id1")
		assert.Equal(t, objectsInCollection[1], "id4")
	})
	t.Run("widget with sets - add only sets in Protobuf Import collection", func(t *testing.T) {
		// given
		p := Pb{}
		allSnapshot := []*converter.Snapshot{
			// skip objects
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			// page
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id:     "id5",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}
		// set widget
		widgetSnapshot := &converter.Snapshot{
			Id:     "widgetID",
			SbType: smartblock2.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: widget.DefaultWidgetSet,
								},
							},
						},
					},
				},
			},
		}

		// when
		collection, err := p.provideRootCollection(allSnapshot, widgetSnapshot, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "id5")
	})
	t.Run("widget with collection - add collection in Protobuf Import collection", func(t *testing.T) {
		// given
		p := Pb{}
		allSnapshot := []*converter.Snapshot{
			// skip objects
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			// page
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id:     "id5",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}

		// collection widget
		widgetSnapshot := &converter.Snapshot{
			Id:     "widgetID",
			SbType: smartblock2.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: widget.DefaultWidgetCollection,
								},
							},
						},
					},
				},
			},
		}

		// when
		collection, err := p.provideRootCollection(allSnapshot, widgetSnapshot, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "id4")
	})
	t.Run("there are favorites objects, dashboard and objects in widget - favorites, objects in widget, dashboard in Protobuf Import", func(t *testing.T) {
		// given
		p := Pb{}
		allSnapshot := []*converter.Snapshot{
			// skip object
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			// favorite page
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{
							bundle.RelationKeyIsFavorite.String(): pbtypes.Bool(true),
						}},
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id:     "id5",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
			// dashboard
			{
				Id:     "id6",
				SbType: smartblock2.SmartBlockTypeWorkspace,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{
							bundle.RelationKeySpaceDashboardId.String(): pbtypes.String("spaceDashboardId"),
						}},
						ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
					},
				},
			},
		}

		// object with widget
		widgetSnapshot := &converter.Snapshot{
			Id:     "widgetID",
			SbType: smartblock2.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: "oldObjectInWidget",
								},
							},
						},
					},
				},
			},
		}

		// when
		collection, err := p.provideRootCollection(allSnapshot, widgetSnapshot, map[string]string{"oldObjectInWidget": "newObjectInWidget"})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 3)
		assert.Equal(t, objectsInCollection[0], "newObjectInWidget")
		assert.Equal(t, objectsInCollection[1], "id1")
		assert.Equal(t, objectsInCollection[2], "spaceDashboardId")
	})
}
