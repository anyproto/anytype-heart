package core

import (
	"context"
	"io"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/core/coreapi"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/textileio/go-textile/ipfs"
)

func (a *Anytype) IpfsPeers() (*ipfs.ConnInfos, error) {
	return ipfs.SwarmPeers(a.ipfs(), true, true, true, true)
}

// IpfsReaderAtPath return reader under an ipfs path
func (a *Anytype) IpfsReaderAtPath(pth string) (io.ReadCloser, error) {
	api, err := coreapi.NewCoreAPI(a.ipfs())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(a.ipfs().Context(), ipfs.CatTimeout)
	defer cancel()

	f, err := api.Unixfs().Get(ctx, path.New(pth))
	if err != nil {
		return nil, err
	}

	var file files.File
	switch f := f.(type) {
	case files.File:
		file = f
	case files.Directory:
		return nil, iface.ErrIsDir
	default:
		return nil, iface.ErrNotSupported
	}

	return file, nil
}
