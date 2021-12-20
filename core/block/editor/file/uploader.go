package file

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
	"github.com/h2non/filetype"
)

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

func NewUploader(s BlockService) Uploader {
	return &uploader{
		service: s,
		anytype: s.Anytype(),
	}
}

type Uploader interface {
	SetBlock(block file.Block) Uploader
	SetName(name string) Uploader
	SetType(tp model.BlockContentFileType) Uploader
	SetStyle(tp model.BlockContentFileStyle) Uploader
	SetBytes(b []byte) Uploader
	SetUrl(url string) Uploader
	SetFile(path string) Uploader
	SetGroupId(groupId string) Uploader
	AddOptions(options ...files.AddOption) Uploader
	AutoType(enable bool) Uploader
	AsyncUpdates(smartBlockId string) Uploader

	Upload(ctx context.Context) (result UploadResult)
	UploadAsync(todo context.Context) (ch chan UploadResult)
}

type UploadResult struct {
	Name string
	Type model.BlockContentFileType
	Hash string
	MIME string
	Size int64
	Err  error
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
				Hash:    ur.Hash,
				Name:    ur.Name,
				Type:    ur.Type,
				Mime:    ur.MIME,
				Size_:   ur.Size,
				AddedAt: time.Now().Unix(),
				State:   state,
			},
		},
	}).(file.Block)
}

type uploader struct {
	service      BlockService
	anytype      core.Service
	block        file.Block
	getReader    func(ctx context.Context) (*bufioSeekClose, error)
	name         string
	typeDetect   bool
	forceType    bool
	smartBlockId string
	fileType     model.BlockContentFileType
	fileStyle    model.BlockContentFileStyle
	opts         []files.AddOption
	groupId      string
}

type bufioSeekClose struct {
	*bufio.Reader
	close func() error
	seek  func(offset int64, whence int) (int64, error)
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

func (u *uploader) SetBlock(block file.Block) Uploader {
	u.block = block
	return u
}

func (u *uploader) SetGroupId(groupId string) Uploader {
	u.groupId = groupId
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

func (u *uploader) SetStyle(tp model.BlockContentFileStyle) Uploader {
	u.fileStyle = tp
	return u
}

func (u *uploader) SetBytes(b []byte) Uploader {
	u.getReader = func(_ context.Context) (*bufioSeekClose, error) {
		return &bufioSeekClose{
			Reader: bufio.NewReaderSize(bytes.NewReader(b), bufSize),
		}, nil
	}
	return u
}

func (u *uploader) AddOptions(options ...files.AddOption) Uploader {
	u.opts = append(u.opts, options...)
	return u
}

func (u *uploader) SetUrl(url string) Uploader {
	url, _ = uri.ProcessURI(url)
	u.name = filepath.Base(url)
	u.getReader = func(ctx context.Context) (*bufioSeekClose, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		tmpFile, err := ioutil.TempFile(u.anytype.TempDir(), "anytype_downloaded_file_*")
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
		return &bufioSeekClose{
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
		}, nil
	}
	return u
}

func (u *uploader) SetFile(path string) Uploader {
	u.name = filepath.Base(path)
	u.getReader = func(ctx context.Context) (*bufioSeekClose, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}

		buf := bufio.NewReaderSize(f, bufSize)
		return &bufioSeekClose{
			Reader: buf,
			seek: func(offset int64, whence int) (int64, error) {
				buf.Reset(f)
				return f.Seek(offset, whence)
			},
			close: f.Close,
		}, nil
	}
	return u
}

func (u *uploader) AutoType(enable bool) Uploader {
	u.typeDetect = enable
	return u
}

func (u *uploader) AsyncUpdates(smartBlockId string) Uploader {
	u.smartBlockId = smartBlockId
	return u
}

func (u *uploader) UploadAsync(ctx context.Context) (result chan UploadResult) {
	result = make(chan UploadResult, 1)
	if u.block != nil {
		u.block.SetState(model.BlockContentFile_Uploading)
		u.block = u.block.Copy().(file.Block)
	}
	go func() {
		result <- u.Upload(ctx)
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
		if u.fileType == model.BlockContentFile_File || u.fileType == model.BlockContentFile_None {
			u.fileType = u.detectType(buf)
		}
	}
	if u.block != nil {
		u.fileStyle = u.block.Model().GetFile().GetStyle()
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
		files.WithReader(buf),
	}
	if len(u.opts) > 0 {
		opts = append(opts, u.opts...)
	}

	if u.fileType == model.BlockContentFile_Image {
		im, e := u.anytype.ImageAdd(ctx, opts...)
		if e == image.ErrFormat {
			log.Infof("can't add file '%s' as image: add as file", u.name)
			e = nil
			return u.SetType(model.BlockContentFile_File).Upload(ctx)
		}
		if e != nil {
			err = e
			return
		}
		result.Hash = im.Hash()
	} else {
		fl, e := u.anytype.FileAdd(ctx, opts...)
		if e != nil {
			err = e
			return
		}
		result.Hash = fl.Hash()
		if meta := fl.Meta(); meta != nil {
			result.MIME = meta.Media
			result.Size = meta.Size
		}
	}
	result.Type = u.fileType
	result.Name = u.name
	if u.block != nil {
		u.block.SetName(u.name).
			SetState(model.BlockContentFile_Done).
			SetType(u.fileType).SetHash(result.Hash).
			SetSize(result.Size).
			SetStyle(u.fileStyle).
			SetMIME(result.MIME)
		u.updateBlock()
	}
	return
}

func (u *uploader) detectType(buf *bufioSeekClose) model.BlockContentFileType {
	b, err := buf.Peek(8192)
	if err != nil && err != io.EOF {
		return model.BlockContentFile_File
	}
	tp, _ := filetype.Match(b)
	return u.detectTypeByMIME(tp.MIME.Value)
}

func (u *uploader) detectTypeByMIME(mime string) model.BlockContentFileType {
	if strings.HasPrefix(mime, "image") {
		return model.BlockContentFile_Image
	}
	if strings.HasPrefix(mime, "video") {
		return model.BlockContentFile_Video
	}
	if strings.HasPrefix(mime, "audio") {
		return model.BlockContentFile_Audio
	}
	if strings.HasPrefix(mime, "application/pdf") {
		return model.BlockContentFile_PDF
	}

	return model.BlockContentFile_File
}

func (u *uploader) updateBlock() {
	if u.smartBlockId != "" && u.block != nil {
		err := u.service.DoFile(u.smartBlockId, func(f File) error {
			return f.UpdateFile(u.block.Model().Id, u.groupId, func(b file.Block) error {
				b.SetModel(u.block.Copy().Model().GetFile())
				return nil
			})
		})
		if err != nil {
			log.Warnf("upload file: can't update info: %v", err)
		}
	}
}
