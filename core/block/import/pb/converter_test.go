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

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
)

func Test_GetSnapshotsSuccess(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open("testdata/bafybb3otqbe6i75sovxnltksacojux24c7hrk2c6cr6pu7ejji2ezvcs.pb")
	reader := bufio.NewReader(f)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("bafybb3otqbe6i75sovxnltksacojux24c7hrk2c6cr6pu7ejji2ezvcs.pb", reader))
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
	}, process.NewProgress(pb.ModelProcess_Import))

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
			Path: []string{"not exist"},
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

	_, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{wr.Path()},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.NotNil(t, ce)
	assert.Len(t, ce, 1)
}

func Test_GetSnapshotsFailedToGetSnapshotForTwoFiles(t *testing.T) {
	p := &Pb{}

	paths := []string{"testdata/bafybb3otqbe6i75sovxnltksacojux24c7hrk2c6cr6pu7ejji2ezvcs.pb", "testdata/test.pb"}
	// ALL_OR_NOTHING mode
	res, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: paths,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.NotNil(t, ce)
	assert.Nil(t, res)
	assert.NotNil(t, ce.Get("testdata/test.pb"))

	// IGNORE_ERRORS mode
	res, ce = p.GetSnapshots(&pb.RpcObjectImportRequest{
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
	assert.Len(t, ce, 1)
	assert.NotEmpty(t, ce.Get("testdata/test.pb"))
}

func Test_GetSnapshotsWithoutRootCollection(t *testing.T) {
	p := &Pb{}

	path := "testdata/bafybb3otqbe6i75sovxnltksacojux24c7hrk2c6cr6pu7ejji2ezvcs.pb"
	res, ce := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path:                       []string{path},
			NotCreateObjectsCollection: true,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(pb.ModelProcess_Import))

	assert.Nil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 1)
}

func newZipWriter(path string) (*zipWriter, error) {
	filename := filepath.Join(path, "Antype"+strconv.FormatInt(rand.Int63(), 10))
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
