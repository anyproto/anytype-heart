package pb

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/test"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func Test_GetSnapshotsSuccess(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open(filepath.Join("testdata", "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb"))
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
	}, process.NewNoOp())

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
	}, process.NewNoOp())

	assert.NotNil(t, ce)
}

func Test_GetSnapshotsFailedToGetSnapshot(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open(filepath.Join("testdata", "test.pb"))
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
	}, process.NewNoOp())

	assert.NotNil(t, ce)
	assert.False(t, ce.IsEmpty())
	assert.True(t, errors.Is(ce.GetResultError(model.Import_Pb), common.ErrPbNotAnyBlockFormat))
}

func Test_GetSnapshotsEmptySnapshot(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	wr, err := newZipWriter(path)
	assert.NoError(t, err)
	f, err := os.Open(filepath.Join("testdata", "emptysnapshot.pb.json"))
	reader := bufio.NewReader(f)

	assert.NoError(t, err)
	assert.NoError(t, wr.WriteFile("emptysnapshot.pb.json", reader))
	assert.NoError(t, wr.Close())

	p := &Pb{}

	_, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: []string{wr.Path()},
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}))

	assert.NotNil(t, ce)
	assert.False(t, ce.IsEmpty())
	assert.True(t, errors.Is(ce.GetResultError(model.Import_Pb), common.ErrPbNotAnyBlockFormat))
}

func Test_GetSnapshotsFailedToGetSnapshotForTwoFiles(t *testing.T) {
	p := &Pb{}

	paths := []string{filepath.Join("testdata", "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb"), filepath.Join("testdata", "test.pb")}
	// ALL_OR_NOTHING mode
	res, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path: paths,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewNoOp())

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
	}, process.NewNoOp())

	assert.NotNil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)
	assert.False(t, ce.IsEmpty())
}

func Test_GetSnapshotsWithoutRootCollection(t *testing.T) {
	p := &Pb{}

	path := filepath.Join("testdata", "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb")
	res, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
			Path:         []string{path},
			NoCollection: true,
		}},
		UpdateExistingObjects: false,
		Type:                  0,
		Mode:                  0,
	}, process.NewNoOp())

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

	f, err := os.Open(filepath.Join("testdata", "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb"))
	assert.NoError(t, err)
	reader := bufio.NewReader(f)

	assert.NoError(t, wr.WriteFile("bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb", reader))

	f, err = os.Open(filepath.Join("testdata", "test"))
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
	}, process.NewNoOp())

	assert.Nil(t, ce)
	assert.NotNil(t, res.Snapshots)
	assert.Len(t, res.Snapshots, 2)

	assert.Equal(t, res.Snapshots[0].FileName, "bafyreig5sd7mlmhindapjuvzc4gnetdbszztb755sa7nflojkljmu56mmi.pb")
	assert.Contains(t, res.Snapshots[1].FileName, rootCollectionName)
}

func TestPb_GetSnapshots(t *testing.T) {
	t.Run("no objects in dir", func(t *testing.T) {
		// given
		dir := t.TempDir()
		p := &Pb{}

		// when
		_, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
				Path: []string{dir},
			}},
		}, process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}))

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Pb), common.ErrFileImportNoObjectsInDirectory))
		e := ce.GetResultError(model.Import_Pb)
		fmt.Println(e)
	})
	t.Run("no objects in archive", func(t *testing.T) {
		// given
		dir := t.TempDir()
		p := &Pb{}
		zipPath := filepath.Join(dir, "empty.zip")
		err := test.CreateEmptyZip(t, zipPath)
		assert.Nil(t, err)

		// when
		_, ce := p.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{PbParams: &pb.RpcObjectImportRequestPbParams{
				Path: []string{zipPath},
			}},
		}, process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}))

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Pb), common.ErrFileImportNoObjectsInZipArchive))
		e := ce.GetResultError(model.Import_Pb)
		fmt.Println(e)
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
