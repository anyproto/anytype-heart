package files

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

type File interface {
	Meta() *FileMeta
	FileId() domain.FileId
	Reader(ctx context.Context) (io.ReadSeeker, error)
	Details(ctx context.Context) (*domain.Details, domain.TypeKey, error)
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
		d.Set(bundle.RelationKeyAudioAlbum, t.Album())
	}
	if t.Artist() != "" {
		d.Set(bundle.RelationKeyAudioArtist, t.Artist())
	}
	if t.Genre() != "" {
		d.Set(bundle.RelationKeyAudioGenre, t.Genre())
	}
	if t.Lyrics() != "" {
		d.Set(bundle.RelationKeyAudioLyrics, t.Lyrics())
	}
	if n, _ := t.Track(); n != 0 {
		d.Set(bundle.RelationKeyAudioAlbumTrackNumber, int64(n))
	}
	if t.Year() != 0 {
		d.Set(bundle.RelationKeyReleasedYear, int64(t.Year()))
	}
	d.Set(bundle.RelationKeyLayout, float64(model.ObjectType_audio))

	return d, nil
}

func (f *file) Details(ctx context.Context) (*domain.Details, domain.TypeKey, error) {
	meta := f.Meta()

	typeKey := bundle.TypeKeyFile
	details := calculateCommonDetails(f.fileId, model.ObjectType_file, f.info.LastModifiedDate)
	details.Set(bundle.RelationKeyFileMimeType, meta.Media)
	details.Set(bundle.RelationKeyName, strings.TrimSuffix(meta.Name, filepath.Ext(meta.Name)))
	details.Set(bundle.RelationKeyFileExt, strings.TrimPrefix(filepath.Ext(meta.Name), "."))
	details.Set(bundle.RelationKeySizeInBytes, float64(meta.Size))
	details.Set(bundle.RelationKeyAddedDate, float64(meta.Added.Unix()))

	if meta.Media == "application/pdf" {
		typeKey = bundle.TypeKeyFile
		details.Set(bundle.RelationKeyLayout, model.ObjectType_pdf)
	}
	if strings.HasPrefix(meta.Media, "video") {
		typeKey = bundle.TypeKeyVideo
		details.Set(bundle.RelationKeyLayout, model.ObjectType_video)
	}

	if strings.HasPrefix(meta.Media, "audio") {
		if audioDetails, err := f.audioDetails(ctx); err == nil {
			details = details.Merge(audioDetails)
		}
		typeKey = bundle.TypeKeyAudio
	}

	return details, typeKey, nil
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
) *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]any{
		bundle.RelationKeyFileId:           fileId.String(),
		bundle.RelationKeyIsReadonly:       false,
		bundle.RelationKeyLayout:           float64(layout),
		bundle.RelationKeyLastModifiedDate: lastModifiedDate,
	})
}
