package file

import (
	"context"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/h2non/filetype"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("anytype-mw")

func NewUploader(a anytype.Service, fn func(f func(file Block))) Uploader {
	return &uploader{
		updateFile: fn,
		storage:    a,
	}
}

type Uploader interface {
	DoAuto(localPath string)
}

type uploader struct {
	updateFile func(f func(file Block))
	storage    anytype.Service
	isImage    bool
}

func (u *uploader) DoAuto(localPath string) {
	tp, _ := filetype.MatchFile(localPath)
	if strings.HasPrefix(tp.MIME.Value, "image") {
		u.DoImage(localPath, "")
	} else {
		u.Do(localPath, "")
	}
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
		log.Warningf("upload file error: %v", err)
		u.updateFile(func(file Block) {
			file.SetState(model.BlockContentFile_Error)
		})
	}
}

func (u *uploader) Do(localPath, url string) {
	if err := u.do(localPath, url); err != nil {
		log.Warningf("upload file error: %v", err)
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
	image, err := u.storage.ImageAddWithReader(context.TODO(), rd, name)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		file.SetImage(image.Hash(), name)
	})
	return
}

func (u *uploader) uploadFile(rd io.Reader, name string) (err error) {
	cf, err := u.storage.FileAddWithReader(context.TODO(), rd, name)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		file.SetFileData(cf.Hash(), *cf.Meta())
	})
	return
}
