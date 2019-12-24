package file

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

type uploader struct {
	updateFile func(f func(file Block))
	storage    anytype.Anytype
}

func (u *uploader) Do(localPath, url string) {
	var err error
	if url != "" {
		err = u.doUrl(url)
	} else {
		err = u.doLocal(localPath)
	}
	if err != nil {
		fmt.Println("upload file error:", err)
		u.updateFile(func(file Block) {
			file.SetState(model.BlockContentFile_Error)
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
	cf, err := u.storage.FileAddWithReader(buf, ct, name)
	if err != nil {
		return
	}
	u.updateFile(func(file Block) {
		var meta core.FileMeta
		if m := cf.Meta(); m != nil {
			meta = *m
		}
		file.SetFileData(cf.Hash(), meta)
	})
	return
}
