package file

import (
	"context"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-library/files"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
	"github.com/h2non/filetype"
)

var log = logging.Logger("anytype-mw-file")

func NewUploader(a anytype.Service, fn func(f func(file Block)), opts ...files.AddOption) Uploader {
	return &uploader{
		updateFile: fn,
		storage:    a,
		options:    opts,
	}
}

type Uploader interface {
	DoAuto(localPath string)
	DoType(localPath, url string, fType model.BlockContentFileType) (err error)
}

type uploader struct {
	updateFile func(f func(file Block))
	storage    anytype.Service
	isImage    bool
	options    []files.AddOption
}

func (u *uploader) DoAuto(localPath string) {
	tp, _ := filetype.MatchFile(localPath)
	if strings.HasPrefix(tp.MIME.Value, "image") {
		u.DoImage(localPath, "")
	} else {
		u.Do(localPath, "")
	}
}

func (u *uploader) DoType(localPath, url string, fType model.BlockContentFileType) (err error) {
	u.isImage = fType == model.BlockContentFile_Image
	url, _ = uri.ProcessURI(url)
	return u.do(localPath, url)
}

func (u *uploader) DoImage(localPath, url string) {
	u.isImage = true
	err := u.do(localPath, url)
	if err == image.ErrFormat {
		log.Infof("can't decode image upload as file: %v", err)
		u.isImage = false
		err = u.do(localPath, url)
	}
	if err != nil {
		log.Warnf("upload file error: %v", err)
		u.updateFile(func(file Block) {
			file.SetState(model.BlockContentFile_Error)
		})
	}
}

func (u *uploader) Do(localPath, url string) {
	if err := u.do(localPath, url); err != nil {
		log.Warnf("upload file error: %v", err)
		u.updateFile(func(file Block) {
			file.SetState(model.BlockContentFile_Error)
		})
	}
}

func (u *uploader) do(localPath, url string) (err error) {
	if url != "" {
		return u.doUrl(url)
	} else {
		return u.doLocal(localPath)
	}
}

func (u *uploader) doLocal(localPath string) (err error) {
	name := filepath.Base(localPath)
	f, err := os.Open(localPath)
	if err != nil {
		return
	}
	return u.upload(f, name)
}

func (u *uploader) doUrl(url string) (err error) {
	name := filepath.Base(url)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	return u.upload(resp.Body, name)
}

func (u *uploader) upload(rd io.ReadCloser, name string) (err error) {
	defer rd.Close()
	if u.isImage {
		return u.uploadImage(rd, name)
	}
	return u.uploadFile(rd, name)
}

func (u *uploader) uploadImage(rd io.Reader, name string) (err error) {
	image, err := u.storage.ImageAdd(context.TODO(), append(u.options, files.WithReader(rd), files.WithName(name))...)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		file.SetImage(image.Hash(), name)
	})
	return
}

func (u *uploader) uploadFile(rd io.Reader, name string) (err error) {
	cf, err := u.storage.FileAdd(context.TODO(), append(u.options, files.WithReader(rd), files.WithName(name))...)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		file.SetFileData(cf.Hash(), *cf.Meta())
	})
	return
}
