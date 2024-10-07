package files

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type File interface {
	Meta() *FileMeta
	FileId() domain.FileId
	Reader(ctx context.Context) (io.ReadSeeker, error)
	Details(ctx context.Context) (*types.Struct, domain.TypeKey, error)
	Info() *storage.FileInfo
}

var _ File = (*file)(nil)

type file struct {
	spaceID string
	fileId  domain.FileId
	info    *storage.FileInfo
	node    *service
}

type FileMeta struct {
	Media            string
	Name             string
	Size             int64
	LastModifiedDate int64
	Added            time.Time
}

func (f *file) audioDetails(ctx context.Context) (*types.Struct, error) {
	r, err := f.Reader(ctx)
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
		d.Fields[bundle.RelationKeyArtist.String()] = pbtypes.String(t.Artist())
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
	d.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_audio))

	return d, nil
}

func (f *file) Details(ctx context.Context) (*types.Struct, domain.TypeKey, error) {
	meta := f.Meta()

	typeKey := bundle.TypeKeyFile
	commonDetails := calculateCommonDetails(f.fileId, model.ObjectType_file, f.info.LastModifiedDate)
	commonDetails[bundle.RelationKeyFileMimeType.String()] = pbtypes.String(meta.Media)

	commonDetails[bundle.RelationKeyName.String()] = pbtypes.String(strings.TrimSuffix(meta.Name, filepath.Ext(meta.Name)))
	commonDetails[bundle.RelationKeyFileExt.String()] = pbtypes.String(strings.TrimPrefix(filepath.Ext(meta.Name), "."))
	commonDetails[bundle.RelationKeySizeInBytes.String()] = pbtypes.Float64(float64(meta.Size))
	commonDetails[bundle.RelationKeyAddedDate.String()] = pbtypes.Float64(float64(meta.Added.Unix()))

	t := &types.Struct{
		Fields: commonDetails,
	}

	if meta.Media == "application/pdf" {
		typeKey = bundle.TypeKeyFile
		t.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_pdf))
	}
	if strings.HasPrefix(meta.Media, "video") {
		typeKey = bundle.TypeKeyVideo
		t.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_video))
	}

	if strings.HasPrefix(meta.Media, "audio") {
		if audioDetails, err := f.audioDetails(ctx); err == nil {
			t = pbtypes.StructMerge(t, audioDetails, false)
		}
		typeKey = bundle.TypeKeyAudio
	}
	if filepath.Ext(meta.Name) == constant.SvgExt {
		typeKey = bundle.TypeKeyImage
		t.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_image))
	}

	return t, typeKey, nil
}

func (f *file) Info() *storage.FileInfo {
	return f.info
}

func (f *file) Meta() *FileMeta {
	return &FileMeta{
		Media:            f.info.Media,
		Name:             f.info.Name,
		Size:             f.info.Size_,
		LastModifiedDate: f.info.LastModifiedDate,
		Added:            time.Unix(f.info.Added, 0),
	}
}

func (f *file) FileId() domain.FileId {
	return f.fileId
}

func (f *file) Reader(ctx context.Context) (io.ReadSeeker, error) {
	return f.node.getContentReader(ctx, f.spaceID, f.info)
}

func calculateCommonDetails(
	fileId domain.FileId,
	layout model.ObjectTypeLayout,
	lastModifiedDate int64,
) map[string]*types.Value {
	return map[string]*types.Value{
		bundle.RelationKeyFileId.String():           pbtypes.String(fileId.String()),
		bundle.RelationKeyIsReadonly.String():       pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():           pbtypes.Float64(float64(layout)),
		bundle.RelationKeyLastModifiedDate.String(): pbtypes.Int64(lastModifiedDate),
	}
}
