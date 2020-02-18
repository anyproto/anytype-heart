package file

import (
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("anytype-mw")

type uploader struct {
	updateFile func(f func(file Block))
	storage    anytype.Anytype
	isImage    bool
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
	image, err := u.storage.ImageAddWithReader(rd, name)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		file.SetImage(image.Hash(), name)
	})
	return
}

func (u *uploader) uploadFile(rd io.Reader, name string) (err error) {
	cf, err := u.storage.FileAddWithReader(rd, name)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		file.SetFileData(cf.Hash(), *cf.Meta())
	})
	return
}
