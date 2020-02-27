package core

import (
	"bytes"
	"fmt"
	"io"

	ipld "github.com/ipfs/go-ipld-format"
	tpb "github.com/textileio/go-textile/pb"
)

var ErrFileNotFound = fmt.Errorf("file not found")

func (a *Anytype) FileByHash(hash string) (File, error) {
	files, err := a.getFileIndexByTarget(hash)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		files, err = a.getFileIndexes(hash)
		if err != nil {
			log.Errorf("fImageByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrFileNotFound
		}
	}

	fileIndex := files[0]
	return &file{
		hash:  hash,
		index: &fileIndex,
		node:  a,
	}, nil
}

func (a *Anytype) FileAddWithBytes(content []byte, filename string) (File, error) {
	return a.FileAddWithReader(bytes.NewReader(content), filename)
}

func (a *Anytype) FileAddWithReader(content io.Reader, filename string) (File, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *Anytype) getFileIndexByTarget(target string) ([]tpb.FileIndex, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *Anytype) buildDirectory(content []byte, filename string, sch *tpb.Node) (*tpb.Directory, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *Anytype) getFileIndexForPath(pth string) (*tpb.FileIndex, error) {
	return nil, fmt.Errorf("not implemented")
}

// IndexFileData walks a file data node, indexing file links
func (a *Anytype) indexFileData(inode ipld.Node, data string) error {
	return fmt.Errorf("not implemented")
}

// indexFileNode walks a file node, indexing file links
func (a *Anytype) indexFileNode(inode ipld.Node, data string) error {
	return fmt.Errorf("not implemented")

	/*links := inode.Links()

	if looksLikeFileNode(inode) {
		return a.indexFileLink(inode, data)
	}

	for _, link := range links {
		n, err := ipfs.NodeAtLink(a.ipfs(), link)
		if err != nil {
			return err
		}

		err = a.indexFileLink(n, data)
		if err != nil {
			return err
		}
	}

	return nil*/
}

// indexFileLink indexes a file link
func (a *Anytype) indexFileLink(inode ipld.Node, data string) error {
	return fmt.Errorf("not implemented")
}

func (a *Anytype) addFileIndexFromPath(target string, path string, key string) (*tpb.FileIndex, error) {
	return nil, fmt.Errorf("not implemented")
}
