package core

import (
	"context"
	"github.com/dhowden/tag"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type File interface {
	Meta() *FileMeta
	Hash() string
	Reader() (io.ReadSeeker, error)
	Details() (*types.Struct, error)
	Info() *storage.FileInfo
}

type file struct {
	hash string
	info *storage.FileInfo
	node *files.Service
}

type FileMeta struct {
	Media string
	Name  string
	Size  int64
	Added time.Time
}

func (i *file) audioDetails() (*types.Struct, error) {
	r, err := i.Reader()
	if err != nil {
		return nil, err
	}

	t, err := tag.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	d := &types.Struct{
		Fields: map[string]*types.Value{},
	}

	if t.Album() != "" {
		d.Fields[bundle.RelationKeyAudioAlbum.String()] = pbtypes.String(t.Album())
	}
	if t.Artist() != "" {
		d.Fields[bundle.RelationKeyAudioArtist.String()] = pbtypes.String(t.Artist())
	}
	if t.Genre() != "" {
		d.Fields[bundle.RelationKeyAudioGenre.String()] = pbtypes.String(t.Genre())
	}
	if t.Lyrics() != "" {
		d.Fields[bundle.RelationKeyAudioLyrics.String()] = pbtypes.String(t.Lyrics())
	}
	if n, _ := t.Track(); n != 0 {
		d.Fields[bundle.RelationKeyAudioAlbumTrackNumber.String()] = pbtypes.Int64(int64(n))
	}
	if t.Year() != 0 {
		d.Fields[bundle.RelationKeyReleasedYear.String()] = pbtypes.Int64(int64(t.Year()))
	}

	return d, nil
}

func (i *file) Details() (*types.Struct, error) {
	meta := i.Meta()

	t := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():           pbtypes.String(i.hash),
			bundle.RelationKeyLayout.String():       pbtypes.Float64(float64(model.ObjectType_file)),
			bundle.RelationKeyIsReadonly.String():   pbtypes.Bool(true),
			bundle.RelationKeyType.String():         pbtypes.String(bundle.TypeKeyFile.URL()),
			bundle.RelationKeyFileMimeType.String(): pbtypes.String(meta.Media),
			bundle.RelationKeyName.String():         pbtypes.String(strings.TrimSuffix(meta.Name, filepath.Ext(meta.Name))),
			bundle.RelationKeyFileExt.String():      pbtypes.String(strings.TrimPrefix(filepath.Ext(meta.Name), ".")),
			bundle.RelationKeySizeInBytes.String():  pbtypes.Float64(float64(meta.Size)),
			bundle.RelationKeyAddedDate.String():    pbtypes.Float64(float64(meta.Added.Unix())),
		},
	}

	if strings.HasPrefix(meta.Media, "video") {
		t.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyVideo.URL())
	}

	if strings.HasPrefix(meta.Media, "audio") {
		if audioDetails, err := i.audioDetails(); err == nil {
			t = pbtypes.StructMerge(t, audioDetails)
		}
		t.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyAudio.URL())
	}

	return t, nil
}

func (i *file) Info() *storage.FileInfo {
	return i.info
}

func (file *file) Meta() *FileMeta {
	return &FileMeta{
		Media: file.info.Media,
		Name:  file.info.Name,
		Size:  file.info.Size_,
		Added: time.Unix(file.info.Added, 0),
	}
}

func (file *file) Hash() string {
	return file.hash
}

func (file *file) Reader() (io.ReadSeeker, error) {
	return file.node.FileContentReader(context.Background(), file.info)
}
