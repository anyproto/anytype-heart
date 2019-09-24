package core

import (
	"fmt"
	mh "github.com/multiformats/go-multihash"
	"github.com/textileio/go-textile/core"
)

func (a *Anytype) AccountSetName(username string) error {
	return a.Textile.SetName(username)
}

func (a *Anytype) AccountSetAvatar(localPath string) (hash mh.Multihash, err error) {
	if !a.Textile.Online() {
		return nil, core.ErrOffline
	}

	thrd := a.Textile.Node().AccountThread()
	if thrd == nil {
		return nil, fmt.Errorf("account thread not found")
	}

	hash, err = a.Textile.AddFilesSync([]string{localPath}, thrd.Id, "")
	if err != nil {
		return nil, err
	}

	err = a.Textile.Node().SetAvatar()
	if err != nil {
		return nil, err
	}

	a.Textile.Node().FlushCafes()

	return hash, nil
}
