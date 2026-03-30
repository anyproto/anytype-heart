package file

import (
	"context"
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("anytype-mw-smartfile")

func NewFile(sb smartblock.SmartBlock, picker cache.ObjectGetter, fileUploaderFactory fileuploader.Service) File {
	return &sfile{
		SmartBlock:          sb,
		picker:              picker,
		fileUploaderFactory: fileUploaderFactory,
	}
}

type File interface {
	Upload(ctx session.Context, id string, source FileSource, isSync bool) (fileObjectId string, err error)
	UploadState(ctx session.Context, s *state.State, id string, source FileSource, isSync bool) (err error)
	UpdateFile(id, groupId string, apply func(b file.Block) error) (err error)
	CreateAndUpload(ctx session.Context, req pb.RpcBlockFileCreateAndUploadRequest) (string, error)
	SetFileStyle(ctx session.Context, style model.BlockContentFileStyle, blockIds ...string) (err error)
	SetFileTargetObjectId(ctx session.Context, blockId, targetObjectId string) error
}

type FileSource struct {
	Path                string
	Url                 string // nolint:revive
	Bytes               []byte
	Name                string
	GroupID             string
	Origin              objectorigin.ObjectOrigin
	ImageKind           model.ImageKind
	CreatedInContext    string
	CreatedInContextRef string
}

type sfile struct {
	smartblock.SmartBlock

	picker              cache.ObjectGetter
	fileUploaderFactory fileuploader.Service
}

func (sf *sfile) Upload(ctx session.Context, blockId string, source FileSource, isSync bool) (fileObjectId string, err error) {
	if source.GroupID == "" {
		source.GroupID = bson.NewObjectId().Hex()
	}
	s := sf.NewStateCtx(ctx).SetGroupId(source.GroupID)
	res := sf.upload(s, blockId, source, isSync)
	if res.Err != nil {
		return "", res.Err
	}
	return res.FileObjectId, sf.Apply(s)
}

func (sf *sfile) UploadState(_ session.Context, s *state.State, id string, source FileSource, isSync bool) (err error) {
	if res := sf.upload(s, id, source, isSync); res.Err != nil {
		return res.Err
	}
	return
}

func (sf *sfile) SetFileStyle(ctx session.Context, style model.BlockContentFileStyle, blockIds ...string) (err error) {
	s := sf.NewStateCtx(ctx)
	for _, id := range blockIds {
		b := s.Get(id)
		if b == nil {
			return smartblock.ErrSimpleBlockNotFound
		}

		if rel, ok := b.(file.Block); ok {
			rel.SetStyle(style)
		} else {
			return fmt.Errorf("unexpected block type: %T (want file)", b)
		}

	}

	return sf.Apply(s)
}

func (sf *sfile) SetFileTargetObjectId(ctx session.Context, blockId, targetObjectId string) error {
	sb, err := sf.picker.GetObject(context.TODO(), targetObjectId)
	if err != nil {
		return err
	}
	var blockContentFileType model.BlockContentFileType
	//nolint:gosec
	switch model.ObjectTypeLayout(sb.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout)) {
	case model.ObjectType_image:
		blockContentFileType = model.BlockContentFile_Image
	case model.ObjectType_audio:
		blockContentFileType = model.BlockContentFile_Audio
	case model.ObjectType_video:
		blockContentFileType = model.BlockContentFile_Video
	case model.ObjectType_pdf:
		blockContentFileType = model.BlockContentFile_PDF
	default:
		blockContentFileType = model.BlockContentFile_File
	}

	return sf.updateFile(ctx, blockId, "", func(b file.Block) error {
		b.SetTargetObjectId(targetObjectId)
		b.SetStyle(model.BlockContentFile_Embed)
		b.SetState(model.BlockContentFile_Done)
		b.SetType(blockContentFileType)
		return nil
	})
}

func (sf *sfile) CreateAndUpload(ctx session.Context, req pb.RpcBlockFileCreateAndUploadRequest) (newId string, err error) {
	s := sf.NewStateCtx(ctx)
	nb := simple.New(&model.Block{
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Type: req.FileType,
			},
		},
	})
	s.Add(nb)
	newId = nb.Model().Id
	if err = s.InsertTo(req.TargetId, req.Position, newId); err != nil {
		return
	}
	if err = sf.upload(s, newId, FileSource{
		Path:                req.LocalPath,
		Url:                 req.Url,
		ImageKind:           req.ImageKind,
		CreatedInContext:    req.ContextId,
		CreatedInContextRef: newId,
	}, false).Err; err != nil {
		return
	}
	if err = sf.Apply(s); err != nil {
		return
	}
	return
}

func (sf *sfile) upload(s *state.State, id string, source FileSource, isSync bool) (res fileuploader.UploadResult) {
	ctx := context.Background()
	b := s.Get(id)
	f, ok := b.(file.Block)
	if !ok {
		return fileuploader.UploadResult{Err: fmt.Errorf("not a file block")}
	}
	upl := sf.fileUploaderFactory.NewUploader(sf.SpaceID(), source.Origin).SetBlock(f)
	if source.ImageKind != model.ImageKind_Basic {
		upl.SetImageKind(source.ImageKind)
	}
	if source.Path != "" {
		upl.SetFile(source.Path)
	} else if source.Url != "" {
		upl.SetUrl(source.Url).
			SetLastModifiedDate()
	} else if len(source.Bytes) > 0 {
		upl.SetBytes(source.Bytes).
			SetName(source.Name).
			SetLastModifiedDate()
	}

	// Set creation context if provided
	if source.CreatedInContext != "" {
		upl.SetCreatedInContext(source.CreatedInContext)
	}
	if source.CreatedInContextRef != "" {
		upl.SetCreatedInContextRef(source.CreatedInContextRef)
	}

	if isSync {
		return upl.Upload(ctx)
	} else {
		upl.SetGroupId(s.GroupId()).AsyncUpdates(sf.Id()).UploadAsync(ctx)
	}
	return
}

func (sf *sfile) UpdateFile(id, groupId string, apply func(b file.Block) error) (err error) {
	return sf.updateFile(nil, id, groupId, apply)
}

func (sf *sfile) updateFile(ctx session.Context, id, groupId string, apply func(b file.Block) error) (err error) {
	s := sf.NewStateCtx(ctx).SetGroupId(groupId)
	b := s.Get(id)
	f, ok := b.(file.Block)
	if !ok {
		return fmt.Errorf("not a file block")
	}
	if err = apply(f); err != nil {
		return
	}
	return sf.Apply(s)
}
