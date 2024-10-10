package pb

import (
	"archive/zip"
	"bufio"
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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
	res, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{zipPath},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.Nil(t, ce)
	assert.Len(t, res.Snapshots, 2)

	assert.Contains(t, res.Snapshots[1].FileName, rootCollectionName)
	assert.NotEmpty(t, res.Snapshots[1].Snapshot.Data.ObjectTypes)
	assert.Equal(t, res.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())
}

func Test_GetSnapshotsFailedReadZip(t *testing.T) {
	p := &Pb{}

	_, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{"not exist.zip"},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

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

	_, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{wr.Path()},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.NotNil(t, ce)
	assert.False(t, ce.IsEmpty())
}

func Test_GetSnapshotsFailedToGetSnapshotForTwoFiles(t *testing.T) {
	p := &Pb{}

	paths := []string{"testdata/bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb", "testdata/test.pb"}
	// ALL_OR_NOTHING mode
	res, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: paths,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.NotNil(t, ce)
	assert.Nil(t, res)

	// IGNORE_ERRORS mode
	res, ce = p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: paths,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.NotNil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)
	assert.False(t, ce.IsEmpty())
}

func Test_GetSnapshotsWithoutRootCollection(t *testing.T) {
	p := &Pb{}

	path := "testdata/bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb"
	res, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path:         []string{path},
			NoCollection: true,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

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
	res, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{zipPath},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.Nil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)

	assert.Equal(t, res.Snapshots[0].FileName, "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb")
	assert.Contains(t, res.Snapshots[1].FileName, rootCollectionName)
}

func Test_normalizeSnapshot(t *testing.T) {
	t.Run("normalize relation option", func(t *testing.T) {
		// given
		p := &Pb{}

		key := bson.NewObjectId().Hex()
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, key)
		assert.NoError(t, err)

		snapshot := &pb.SnapshotWithType{
			SbType: model.SmartBlockType_STRelationOption,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():   pbtypes.String(uniqueKey.Marshal()),
						bundle.RelationKeyRelationKey.String(): pbtypes.String(bundle.RelationKeyTag.String()),
					}},
					ObjectTypes: []string{bundle.TypeKeyRelationOption.URL()},
					Key:         "key",
				},
			},
		}
		// when
		_, err = p.normalizeSnapshot(snapshot, uuid.New().String(), "", "path", false, nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, snapshot.SbType)
		assert.Empty(t, snapshot.Snapshot.Data.Details.Fields[bundle.RelationKeyUniqueKey.String()])
		assert.Empty(t, snapshot.Snapshot.Data.Details.Fields[bundle.RelationKeyRelationKey.String()])
		assert.Equal(t, []string{bundle.TypeKeyTag.URL()}, snapshot.Snapshot.Data.ObjectTypes)
		assert.Empty(t, snapshot.Snapshot.Data.Key)
	})
	t.Run("normalize relation option, but it's not tag relation option", func(t *testing.T) {
		// given
		p := &Pb{}

		key := bson.NewObjectId().Hex()
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, key)
		assert.NoError(t, err)

		snapshot := &pb.SnapshotWithType{
			SbType: model.SmartBlockType_STRelationOption,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():   pbtypes.String(uniqueKey.Marshal()),
						bundle.RelationKeyRelationKey.String(): pbtypes.String("test"),
					}},
					ObjectTypes: []string{bundle.TypeKeyRelationOption.URL()},
					Key:         "key",
				},
			},
		}
		// when
		_, err = p.normalizeSnapshot(snapshot, uuid.New().String(), "", "path", false, nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_STRelationOption, snapshot.SbType)
		assert.Equal(t, pbtypes.String(uniqueKey.Marshal()), snapshot.Snapshot.Data.Details.Fields[bundle.RelationKeyUniqueKey.String()])
		assert.Equal(t, pbtypes.String("test"), snapshot.Snapshot.Data.Details.Fields[bundle.RelationKeyRelationKey.String()])
		assert.Equal(t, []string{bundle.TypeKeyRelationOption.URL()}, snapshot.Snapshot.Data.ObjectTypes)
		assert.Equal(t, "key", snapshot.Snapshot.Data.Key)
	})
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
