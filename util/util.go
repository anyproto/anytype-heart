package util

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/timestamp"
	tpb "github.com/textileio/go-textile/pb"
)

func DiffStringSlice(old, new []string) (removed []string, added []string) {
	// create a map of string -> int
	diff := make(map[string]int, len(old))
	for _, x := range old {
		diff[x]++
	}

	for _, y := range new {
		if _, exists := diff[y]; !exists {
			added = append(added, y)
			continue
		}

		diff[y] -= 1
		if diff[y] < 0 {
			added = append(added, y)
		}
	}

	for x, i := range diff {
		if i > 0 {
			removed = append(removed, x)
		}
	}

	return
}

func GzipCompress(b []byte) []byte {
	var buf bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	gw.Write(b)
	gw.Close()
	return buf.Bytes()
}

func GzipUncompress(b []byte) ([]byte, error) {
	// shortcut to save unnecessary initialisations
	if b[0] != 0x1f || b[1] != 0x8b {
		return nil, fmt.Errorf("not a gzip")
	}

	var r io.Reader
	r, err := gzip.NewReader(bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	return resB.Bytes(), nil
}

func CastTimestampFromGogo(tsP *types.Timestamp) *timestamp.Timestamp {
	if tsP == nil {
		return nil
	}

	ts := timestamp.Timestamp(*tsP)
	return &ts
}

func CastTimestampToGogo(tsP *timestamp.Timestamp) *types.Timestamp {
	if tsP == nil {
		return nil
	}

	ts := types.Timestamp(*tsP)
	return &ts
}

func CastFileIndexToStorage(file *tpb.FileIndex) *storage.FileIndex {
	return &storage.FileIndex{
		Mill:     file.Mill,
		Checksum: file.Checksum,
		Source:   file.Source,
		Opts:     file.Opts,
		Hash:     file.Hash,
		Key:      file.Key,
		Media:    file.Media,
		Name:     file.Name,
		Size_:    file.Size,
		Added:    CastTimestampToGogo(file.Added),
		//Meta:     &types.Struct{types.field(file.Meta.Fields},
		Targets: file.Targets,
	}
}

func CastFileIndexToTextile(file *storage.FileIndex) *tpb.FileIndex {
	return &tpb.FileIndex{
		Mill:     file.Mill,
		Checksum: file.Checksum,
		Source:   file.Source,
		Opts:     file.Opts,
		Hash:     file.Hash,
		Key:      file.Key,
		Media:    file.Media,
		Name:     file.Name,
		Size:     file.Size_,
		Added:    CastTimestampFromGogo(file.Added),
		//Meta:     &types.Struct{types.field(file.Meta.Fields},
		Targets: file.Targets,
	}
}
