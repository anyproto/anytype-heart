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

	"github.com/anytypeio/go-anytype-middleware/pb"
)

func Test_GetSnapshotsSuccess(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open("testdata/bafybbyyhrncspdsr3nwoneemm4v7sbjqzl2e2d3egjc5ut7nxnnlk5fh.pb")
	reader := bufio.NewReader(f)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("bafybbyyhrncspdsr3nwoneemm4v7sbjqzl2e2d3egjc5ut7nxnnlk5fh.pb", reader))
	assert.NoError(t, wr.Close())

	p := &Pb{}

	res := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: wr.Path()}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.Nil(t, res.Error)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 1)
}

func Test_GetSnapshotsFailedReadZip(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	f, err := os.Open("testdata/bafybbyyhrncspdsr3nwoneemm4v7sbjqzl2e2d3egjc5ut7nxnnlk5fh.pb")
	reader := bufio.NewReader(f)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("bafybbyyhrncspdsr3nwoneemm4v7sbjqzl2e2d3egjc5ut7nxnnlk5fh.pb", reader))
	assert.NoError(t, wr.Close())

	p := &Pb{}
	res := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "not exists"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.NotNil(t, res.Error)
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

	res := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: "notexist.zip"}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.NotNil(t, res.Error)
	assert.Len(t, res.Error, 1)
	assert.NotEmpty(t, res.Error.Get("notexist.zip"))
}

func Test_GetSnapshotsFailedToGetSnapshotForTwoFiles(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open("testdata/test.pb")
	assert.NoError(t, err)
	reader := bufio.NewReader(f)

	assert.NoError(t, wr.WriteFile("test.pb", reader))

	secondfile, err := os.Open("testdata/bafybbyyhrncspdsr3nwoneemm4v7sbjqzl2e2d3egjc5ut7nxnnlk5fh.pb")
	reader = bufio.NewReader(secondfile)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("bafybbyyhrncspdsr3nwoneemm4v7sbjqzl2e2d3egjc5ut7nxnnlk5fh.pb", reader))
	assert.NoError(t, wr.Close())

	p := &Pb{}

	// ALL_OR_NOTHING mode
	res := p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: wr.Path()}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	})

	assert.NotNil(t, res.Error)
	assert.Nil(t, res.Snapshots)
	assert.NotEmpty(t, res.Error.Get("test.pb"))

	// IGNORE_ERRORS mode
	res = p.GetSnapshots(&pb.RpcObjectImportRequest{
		Params:                &pb.RpcObjectImportRequestParamsOfNotionParams{NotionParams: &pb.RpcObjectImportRequestNotionParams{Path: wr.Path()}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  1,
	})

	assert.NotNil(t, res.Error)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 1)
	assert.Len(t, res.Error, 1)
	assert.NotEmpty(t, res.Error.Get("test.pb"))
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
