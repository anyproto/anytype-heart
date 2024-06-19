package fileuploader

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/h2non/filetype"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("file-uploader")

type Service interface {
	app.Component

	NewUploader(spaceId string, origin objectorigin.ObjectOrigin) Uploader
}

type service struct {
	fileService       files.Service
	tempDirProvider   core.TempDirProvider
	picker            cache.ObjectGetter
	fileObjectService fileobject.Service
}

func New() Service {
	return &service{}
}

func (f *service) NewUploader(spaceId string, origin objectorigin.ObjectOrigin) Uploader {
	return &uploader{
		spaceId:           spaceId,
		picker:            f.picker,
		fileService:       f.fileService,
		tempDirProvider:   f.tempDirProvider,
		fileObjectService: f.fileObjectService,
		origin:            origin,
	}
}

const CName = "file-uploader"

func (f *service) Name() string {
	return CName
}

func (f *service) Init(a *app.App) error {
	f.fileService = app.MustComponent[files.Service](a)
	f.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	f.picker = app.MustComponent[cache.ObjectGetter](a)
	f.fileObjectService = app.MustComponent[fileobject.Service](a)
	return nil
}

var (
	// limiting overall file upload goroutines
	uploadFilesLimiter = make(chan struct{}, 8)
	bufSize            = 8192
)

func init() {
	for i := 0; i < cap(uploadFilesLimiter); i++ {
		uploadFilesLimiter <- struct{}{}
	}
}

type Uploader interface {
	SetBlock(block file.Block) Uploader
	SetName(name string) Uploader
	SetType(tp model.BlockContentFileType) Uploader
	SetStyle(tp model.BlockContentFileStyle) Uploader
	SetAdditionalDetails(details *types.Struct) Uploader
	SetBytes(b []byte) Uploader
	SetUrl(url string) Uploader
	SetFile(path string) Uploader
	SetLastModifiedDate() Uploader
	SetGroupId(groupId string) Uploader
	SetCustomEncryptionKeys(keys map[string]string) Uploader
	AddOptions(options ...files.AddOption) Uploader
	AsyncUpdates(smartBlockId string) Uploader

	Upload(ctx context.Context) (result UploadResult)
	UploadAsync(ctx context.Context) (ch chan UploadResult)
}

type UploadResult struct {
	Name              string
	Type              model.BlockContentFileType
	FileObjectId      string
	FileObjectDetails *types.Struct
	MIME              string
	Size              int64
	Err               error
}

func (ur UploadResult) ToBlock() file.Block {
	state := model.BlockContentFile_Done
	if ur.Err != nil {
		state = model.BlockContentFile_Error
		ur.Name = ur.Err.Error()
	}
	return simple.New(&model.Block{
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				TargetObjectId: ur.FileObjectId,
				Name:           ur.Name,
				Type:           ur.Type,
				Mime:           ur.MIME,
				Size_:          ur.Size,
				AddedAt:        time.Now().Unix(),
				State:          state,
			},
		},
	}).(file.Block)
}

type uploader struct {
	spaceId              string
	fileObjectService    fileobject.Service
	picker               cache.ObjectGetter
	block                file.Block
	getReader            func(ctx context.Context) (*fileReader, error)
	name                 string
	lastModifiedDate     int64
	forceType            bool
	forceUploadingAsFile bool
	smartBlockID         string
	fileType             model.BlockContentFileType
	fileStyle            model.BlockContentFileStyle
	opts                 []files.AddOption
	groupID              string

	tempDirProvider      core.TempDirProvider
	fileService          files.Service
	origin               objectorigin.ObjectOrigin
	additionalDetails    *types.Struct
	customEncryptionKeys map[string]string
}

type bufioSeekClose struct {
	*bufio.Reader
	close func() error
	seek  func(offset int64, whence int) (int64, error)
}

type fileReader struct {
	*bufioSeekClose
	fileName string
}

func (bc *bufioSeekClose) Close() error {
	if bc.close != nil {
		return bc.close()
	}
	return nil
}

func (bc *bufioSeekClose) Seek(offset int64, whence int) (int64, error) {
	if bc.seek != nil {
		return bc.seek(offset, whence)
	}
	return 0, fmt.Errorf("seek not supported for this type")
}

func (fr *fileReader) GetFileName() string {
	return fr.fileName
}

func (u *uploader) SetBlock(block file.Block) Uploader {
	u.block = block
	return u
}

func (u *uploader) SetGroupId(groupId string) Uploader {
	u.groupID = groupId
	return u
}

func (u *uploader) SetName(name string) Uploader {
	u.name = name
	return u
}

func (u *uploader) SetType(tp model.BlockContentFileType) Uploader {
	u.fileType = tp
	u.forceType = true
	return u
}

func (u *uploader) ForceUploadingAsFile() Uploader {
	u.forceUploadingAsFile = true
	return u
}

func (u *uploader) SetStyle(tp model.BlockContentFileStyle) Uploader {
	u.fileStyle = tp
	return u
}

func (u *uploader) SetAdditionalDetails(details *types.Struct) Uploader {
	u.additionalDetails = details
	return u
}

func (u *uploader) SetBytes(b []byte) Uploader {
	u.getReader = func(_ context.Context) (*fileReader, error) {
		buf := bytes.NewReader(b)
		bufReaderSize := bufio.NewReaderSize(buf, bufSize)
		return &fileReader{
			bufioSeekClose: &bufioSeekClose{
				Reader: bufReaderSize,
				seek: func(offset int64, whence int) (int64, error) {
					bufReaderSize.Reset(buf)
					return buf.Seek(offset, whence)
				},
			},
		}, nil
	}
	return u
}

func (u *uploader) SetCustomEncryptionKeys(keys map[string]string) Uploader {
	u.customEncryptionKeys = keys
	return u
}

func (u *uploader) AddOptions(options ...files.AddOption) Uploader {
	u.opts = append(u.opts, options...)
	return u
}

func (u *uploader) SetUrl(url string) Uploader {
	url, err := uri.NormalizeURI(url)
	if err != nil {
		// do nothing
	}
	u.SetName(strings.Split(filepath.Base(url), "?")[0])
	u.getReader = func(ctx context.Context) (*fileReader, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		// setting timeout to avoid locking for a long time
		cl := http.DefaultClient
		cl.Timeout = time.Second * 20

		resp, err := cl.Do(req)
		if err != nil {
			return nil, err
		}

		var fileName string
		if content := resp.Header.Get("Content-Disposition"); content != "" {
			contentDisposition := strings.Split(content, "filename=")
			if len(contentDisposition) > 1 {
				fileName = strings.Trim(contentDisposition[1], "\"")
			}
		}

		tmpFile, err := ioutil.TempFile(u.tempDirProvider.TempDir(), "anytype_downloaded_file_*")
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(tmpFile, resp.Body)
		if err != nil {
			return nil, err
		}

		_, err = tmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		buf := bufio.NewReaderSize(tmpFile, bufSize)
		bsc := &bufioSeekClose{
			Reader: buf,
			seek: func(offset int64, whence int) (int64, error) {
				buf.Reset(tmpFile)
				return tmpFile.Seek(offset, whence)
			},
			close: func() error {
				_ = tmpFile.Close()
				os.Remove(tmpFile.Name())
				return resp.Body.Close()
			},
		}
		return &fileReader{
			bufioSeekClose: bsc,
			fileName:       fileName,
		}, nil
	}
	return u
}

func (u *uploader) SetFile(path string) Uploader {
	u.SetName(filepath.Base(path))
	u.setLastModifiedDate(path)

	u.getReader = func(ctx context.Context) (*fileReader, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, oserror.TransformError(err)
		}

		buf := bufio.NewReaderSize(f, bufSize)
		bsc := &bufioSeekClose{
			Reader: buf,
			seek: func(offset int64, whence int) (int64, error) {
				buf.Reset(f)
				return f.Seek(offset, whence)
			},
			close: f.Close,
		}
		return &fileReader{
			bufioSeekClose: bsc,
		}, nil
	}
	return u
}

func (u *uploader) SetLastModifiedDate() Uploader {
	u.lastModifiedDate = time.Now().Unix()
	return u
}

func (u *uploader) setLastModifiedDate(path string) {
	stat, err := os.Stat(path)
	if err == nil {
		u.lastModifiedDate = stat.ModTime().Unix()
	} else {
		u.lastModifiedDate = time.Now().Unix()
	}
}

func (u *uploader) AsyncUpdates(smartBlockId string) Uploader {
	u.smartBlockID = smartBlockId
	return u
}

func (u *uploader) UploadAsync(ctx context.Context) (result chan UploadResult) {
	result = make(chan UploadResult, 1)
	if u.block != nil {
		u.block.SetState(model.BlockContentFile_Uploading)
		u.block = u.block.Copy().(file.Block)
	}
	go func() {
		res := u.Upload(ctx)
		if res.Err != nil {
			log.Errorf("upload async: %v", res.Err)
		}
		result <- res
		close(result)
	}()
	return
}

func (u *uploader) Upload(ctx context.Context) (result UploadResult) {
	var err error
	defer func() {
		if err != nil {
			result.Err = err
			if u.block != nil {
				u.block.SetState(model.BlockContentFile_Error).SetName(err.Error())
				u.updateBlock()
			}
		}
	}()
	if u.getReader == nil {
		err = fmt.Errorf("uploader: empty source for upload")
		return
	}
	buf, err := u.getReader(ctx)
	if err != nil {
		return
	}
	defer buf.Close()
	if !u.forceType {
		if u.block != nil {
			u.fileType = u.block.Model().GetFile().GetType()
		}
	}

	if fileName := buf.GetFileName(); fileName != "" {
		u.SetName(fileName)
	}

	if u.block != nil {
		u.fileStyle = u.block.Model().GetFile().GetStyle()
	}
	if !u.forceType {
		u.fileType = u.detectType(buf)
	}
	if u.fileStyle == model.BlockContentFile_Auto {
		if u.fileType == model.BlockContentFile_File || u.fileType == model.BlockContentFile_None {
			u.fileStyle = model.BlockContentFile_Link
		} else {
			u.fileStyle = model.BlockContentFile_Embed
		}
	}
	var opts = []files.AddOption{
		files.WithName(u.name),
		files.WithLastModifiedDate(u.lastModifiedDate),
		files.WithReader(buf),
	}
	if u.customEncryptionKeys != nil {
		opts = append(opts, files.WithCustomEncryptionKeys(u.customEncryptionKeys))
	}

	if len(u.opts) > 0 {
		opts = append(opts, u.opts...)
	}

	var addResult *files.AddResult
	if !u.forceUploadingAsFile && u.fileType == model.BlockContentFile_Image {
		addResult, err = u.fileService.ImageAdd(ctx, u.spaceId, opts...)
		if errors.Is(err, image.ErrFormat) ||
			errors.Is(err, mill.ErrFormatSupportNotEnabled) ||
			errors.Is(err, mill.ErrProcessing) {
			err = nil
			return u.ForceUploadingAsFile().Upload(ctx)
		}
		if err != nil {
			return UploadResult{Err: fmt.Errorf("add image to storage: %w", err)}
		}
	} else {
		addResult, err = u.fileService.FileAdd(ctx, u.spaceId, opts...)
		if err != nil {
			return UploadResult{Err: fmt.Errorf("add file to storage: %w", err)}
		}
	}
	defer addResult.Commit()

	result.MIME = addResult.MIME
	result.Size = addResult.Size

	fileObjectId, fileObjectDetails, err := u.getOrCreateFileObject(ctx, addResult)
	if err != nil {
		return UploadResult{Err: err}
	}
	result.FileObjectId = fileObjectId
	result.FileObjectDetails = fileObjectDetails

	result.Type = u.fileType
	result.Name = u.name
	if u.block != nil {
		u.block.SetName(u.name).
			SetState(model.BlockContentFile_Done).
			SetType(u.fileType).
			SetTargetObjectId(result.FileObjectId).
			SetSize(result.Size).
			SetStyle(u.fileStyle).
			SetMIME(result.MIME)
		u.updateBlock()
	}
	return
}

func (u *uploader) getOrCreateFileObject(ctx context.Context, addResult *files.AddResult) (string, *types.Struct, error) {
	if addResult.IsExisting {
		id, details, err := u.fileObjectService.GetObjectDetailsByFileId(domain.FullFileId{
			SpaceId: u.spaceId,
			FileId:  addResult.FileId,
		})
		if err == nil {
			return id, details, nil
		}
		if errors.Is(err, fileobject.ErrObjectNotFound) {
			err = nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("get object details by file id: %w", err)
		}
	}

	fileObjectId, fileObjectDetails, err := u.fileObjectService.Create(ctx, u.spaceId, fileobject.CreateRequest{
		FileId:            addResult.FileId,
		EncryptionKeys:    addResult.EncryptionKeys.EncryptionKeys,
		ObjectOrigin:      u.origin,
		AdditionalDetails: u.additionalDetails,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create file object: %w", err)
	}
	return fileObjectId, fileObjectDetails, nil

}

func (u *uploader) detectType(buf *fileReader) model.BlockContentFileType {
	b, err := buf.Peek(8192)
	if err != nil && err != io.EOF {
		return model.BlockContentFile_File
	}
	tp, _ := filetype.Match(b)
	return file.DetectTypeByMIME(tp.MIME.Value)
}

type FileComponent interface {
	UpdateFile(id, groupId string, apply func(b file.Block) error) (err error)
}

func (u *uploader) updateBlock() {
	if u.smartBlockID != "" && u.block != nil {
		err := cache.Do(u.picker, u.smartBlockID, func(f FileComponent) error {
			return f.UpdateFile(u.block.Model().Id, u.groupID, func(b file.Block) error {
				b.SetModel(u.block.Copy().Model().GetFile())
				return nil
			})
		})
		if err != nil {
			log.Warnf("upload file: can't update info: %v", err)
		}
	}
}
