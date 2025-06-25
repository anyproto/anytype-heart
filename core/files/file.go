package files

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type File interface {
	FileId() domain.FileId
	Meta() *FileMeta
	Reader(ctx context.Context) (io.ReadSeeker, error) // getNode(details.FileVariants[idx])
	Details(ctx context.Context) (*domain.Details, domain.TypeKey, error)
	Name() string
	MimeType() string
	LastModifiedDate() int64
	Mill() string
}

var _ File = (*file)(nil)

type file struct {
	spaceID     string
	fileId      domain.FileId
	info        *storage.FileInfo
	fileService Service
}

type FileMeta struct {
	Media            string
	Name             string
	Size             int64
	LastModifiedDate int64
	Added            time.Time
	Width            int64
	Height           int64
}

func (f *file) audioDetails(ctx context.Context) (*domain.Details, error) {
	r, err := f.Reader(ctx)
	if err != nil {
		return nil, err
	}

	t, err := tag.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	d := domain.NewDetails()

	if t.Album() != "" {
		d.SetString(bundle.RelationKeyAudioAlbum, t.Album())
	}
	if t.Artist() != "" {
		d.SetString(bundle.RelationKeyArtist, t.Artist())
	}
	if t.Genre() != "" {
		d.SetString(bundle.RelationKeyAudioGenre, t.Genre())
	}
	if t.Lyrics() != "" {
		d.SetString(bundle.RelationKeyAudioLyrics, t.Lyrics())
	}
	if n, _ := t.Track(); n != 0 {
		d.SetInt64(bundle.RelationKeyAudioAlbumTrackNumber, int64(n))
	}
	if t.Year() != 0 {
		d.SetInt64(bundle.RelationKeyReleasedYear, int64(t.Year()))
	}
	return d, nil
}

func (f *file) Details(ctx context.Context) (*domain.Details, domain.TypeKey, error) {
	meta := f.Meta()

	typeKey := bundle.TypeKeyFile
	details := calculateCommonDetails(f.fileId, model.ObjectType_file, f.info.LastModifiedDate)
	details.SetString(bundle.RelationKeyFileMimeType, meta.Media)
	details.SetString(bundle.RelationKeyName, strings.TrimSuffix(meta.Name, filepath.Ext(meta.Name)))
	details.SetString(bundle.RelationKeyFileExt, strings.TrimPrefix(filepath.Ext(meta.Name), "."))
	details.SetFloat64(bundle.RelationKeySizeInBytes, float64(meta.Size))
	details.SetFloat64(bundle.RelationKeyAddedDate, float64(meta.Added.Unix()))

	if meta.Media == "application/pdf" {
		typeKey = bundle.TypeKeyFile
		details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_pdf))
	}
	if strings.HasPrefix(meta.Media, "video") {
		typeKey = bundle.TypeKeyVideo
		details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_video))
	}

	if strings.HasPrefix(meta.Media, "audio") {
		details.Set(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_audio))
		if audioDetails, err := f.audioDetails(ctx); err == nil {
			details = details.Merge(audioDetails)
		}
		typeKey = bundle.TypeKeyAudio
	}
	if filepath.Ext(meta.Name) == constant.SvgExt {
		typeKey = bundle.TypeKeyImage
		details.Set(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_image))
	}

	return details, typeKey, nil
}

func (f *file) Name() string {
	return f.info.Name
}

func (f *file) MimeType() string {
	return f.info.Media
}

func (f *file) LastModifiedDate() int64 {
	return f.info.LastModifiedDate
}

func (f *file) Mill() string {
	return f.info.Mill
}

func (f *file) Meta() *FileMeta {
	return &FileMeta{
		Media:            f.info.Media,
		Name:             f.info.Name,
		Size:             f.info.Size_,
		LastModifiedDate: f.info.LastModifiedDate,
		Added:            time.Unix(f.info.Added, 0),
		Width:            pbtypes.GetInt64(f.info.Meta, "width"),
		Height:           pbtypes.GetInt64(f.info.Meta, "height"),
	}
}

func (f *file) FileId() domain.FileId {
	return f.fileId
}

func (f *file) Reader(ctx context.Context) (io.ReadSeeker, error) {
	return f.fileService.GetContentReader(ctx, f.spaceID, f.info.Hash, f.info.Key)
}

func calculateCommonDetails(
	fileId domain.FileId,
	layout model.ObjectTypeLayout,
	lastModifiedDate int64,
) *domain.Details {
	det := domain.NewDetails()
	det.SetString(bundle.RelationKeyFileId, fileId.String())
	det.SetBool(bundle.RelationKeyIsReadonly, false)
	det.SetInt64(bundle.RelationKeyLayout, int64(layout))
	det.SetFloat64(bundle.RelationKeyLastModifiedDate, float64(lastModifiedDate))
	return det
}

func NewFile(fileService Service, id domain.FullFileId, infos []*storage.FileInfo) (File, error) {
	if len(infos) == 0 {
		return nil, fmt.Errorf("empty variant infos")
	}
	return &file{
		spaceID:     id.SpaceId,
		fileId:      id.FileId,
		info:        infos[0],
		fileService: fileService,
	}, nil
}
