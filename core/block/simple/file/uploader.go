package file

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

type uploader struct {
	updateFile func(f func(file *File))
	storage    anytype.Anytype
}

func (u *uploader) Do(localPath, url string) {
	var err error
	if url != "" {
		err = u.doLocal(url)
	} else {
		err = u.doLocal(localPath)
	}
	if err != nil {
		u.updateFile(func(file *File) {
			file.content.State = model.BlockContentFile_Error
		})
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
	buf := bufio.NewReader(rd)
	data, _ := buf.Peek(512)
	ct := http.DetectContentType(data)
	cf, err := u.storage.FileAddWithReader(rd, ct, name)
	if err != nil {
		return
	}
	u.updateFile(func(file *File) {
		file.setFile(cf)
	})
}
