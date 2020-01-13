package core

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/golang/protobuf/jsonpb"
	structpb "github.com/golang/protobuf/ptypes/struct"
	ipld "github.com/ipfs/go-ipld-format"
	ipfspath "github.com/ipfs/go-path"
	"github.com/mr-tron/base58"
	"github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/crypto"
	"github.com/textileio/go-textile/ipfs"
	ipfsutil "github.com/textileio/go-textile/ipfs"
	"github.com/textileio/go-textile/mill"
	tpb "github.com/textileio/go-textile/pb"
	"github.com/textileio/go-textile/repo/db"
	tschema "github.com/textileio/go-textile/schema"
	tutil "github.com/textileio/go-textile/util"
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
	fileConfig, err := a.getFileConfig(content, filename, "", false)
	if err != nil {
		return nil, err
	}

	// todo: PR textile to be able to use reader instead of bytes
	fileIndex, err := a.Textile.Node().AddFileIndex(&mill.Blob{}, *fileConfig)
	if err != nil {
		return nil, err
	}

	node, keys, err := a.Textile.Node().AddNodeFromFiles([]*tpb.FileIndex{fileIndex})
	if err != nil {
		return nil, err
	}

	nodeHash := node.Cid().Hash().B58String()

	err = a.indexFileData(node, nodeHash)
	if err != nil {
		return nil, err
	}

	filesKeysCacheMutex.Lock()
	defer filesKeysCacheMutex.Unlock()
	filesKeysCache[nodeHash] = keys.Files

	return &file{
		hash:  nodeHash,
		index: fileIndex,
		node:  a,
	}, nil
}

func (a *Anytype) getFileIndexByTarget(target string) ([]tpb.FileIndex, error) {
	var list []tpb.FileIndex
	rows, err := a.Textile.Node().Datastore().Files().PrepareAndExecuteQuery("SELECT * FROM files WHERE targets=?", target)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var mill_, checksum, source, opts, hash, key, media, name string
		var size int64
		var addedInt int64
		var metab []byte
		var targets *string

		if err := rows.Scan(&mill_, &checksum, &source, &opts, &hash, &key, &media, &name, &size, &addedInt, &metab, &targets); err != nil {
			log.Errorf("error in db scan: %s", err)
			continue
		}

		meta := &structpb.Struct{}
		if metab != nil {
			if err := jsonpb.Unmarshal(bytes.NewReader(metab), meta); err != nil {
				log.Errorf("failed to unmarshal file meta: %s", err)
				continue
			}
		}

		tlist := make([]string, 0)
		if targets != nil {
			tlist = tutil.SplitString(*targets, ",")
		}

		list = append(list, tpb.FileIndex{
			Mill:     mill_,
			Checksum: checksum,
			Source:   source,
			Opts:     opts,
			Hash:     hash,
			Key:      key,
			Media:    media,
			Name:     name,
			Size:     size,
			Added:    tutil.ProtoTs(addedInt),
			Meta:     meta,
			Targets:  tlist,
		})
	}

	return list, nil
}

func (a *Anytype) getFileConfig(reader io.Reader, filename string, use string, plaintext bool) (*core.AddFileConfig, error) {
	conf := &core.AddFileConfig{}

	if use == "" {
		conf.Name = filename
	} else {
		ref, err := ipfspath.ParsePath(use)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(ref.String(), "/")
		hash := parts[len(parts)-1]
		var file *tpb.FileIndex
		reader, file, err = a.textile().FileContent(hash)
		if err != nil {
			if err == core.ErrFileNotFound {
				// just cat the data from ipfs
				b, err := ipfsutil.DataAtPath(a.ipfs(), ref.String())
				if err != nil {
					return nil, err
				}
				reader = bytes.NewReader(b)
				conf.Use = ref.String()
			} else {
				return nil, err
			}
		} else {
			conf.Use = file.Checksum
		}
	}

	buf := bufio.NewReader(reader)
	data, err := buf.Peek(512)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to get first 512 bytes to detect content-type: %s", err)
	}
	conf.Media = http.DetectContentType(data)

	data, err = ioutil.ReadAll(buf)
	if err != nil {
		return nil, err
	}
	conf.Input = data
	conf.Plaintext = plaintext

	return conf, nil
}

func (a *Anytype) buildDirectory(reader io.Reader, filename string, sch *tpb.Node) (*tpb.Directory, error) {
	dir := &tpb.Directory{
		Files: make(map[string]*tpb.FileIndex),
	}

	mil, err := schema.GetMill(sch.Mill, sch.Opts)
	if err != nil {
		return nil, err
	}
	if mil != nil {
		conf, err := a.getFileConfig(reader, filename, "", sch.Plaintext)
		if err != nil {
			return nil, err
		}

		added, err := a.textile().AddFileIndex(mil, *conf)
		if err != nil {
			return nil, err
		}
		dir.Files[tschema.SingleFileTag] = added

	} else if len(sch.Links) > 0 {
		// determine order
		steps, err := tschema.Steps(sch.Links)
		if err != nil {
			return nil, err
		}

		// send each link
		for _, step := range steps {
			stepMill, err := schema.GetMill(step.Link.Mill, step.Link.Opts)
			if err != nil {
				return nil, err
			}
			var conf *core.AddFileConfig
			if step.Link.Use == tschema.FileTag {
				conf, err = a.getFileConfig(
					reader,
					filename,
					"",
					step.Link.Plaintext,
				)
				if err != nil {
					return nil, err
				}

			} else {
				if dir.Files[step.Link.Use] == nil {
					return nil, fmt.Errorf(step.Link.Use + " not found")
				}

				conf, err = a.getFileConfig(nil,
					filename,
					dir.Files[step.Link.Use].Hash,
					step.Link.Plaintext,
				)
				if err != nil {
					return nil, err
				}
			}

			added, err := a.textile().AddFileIndex(stepMill, *conf)
			if err != nil {
				return nil, err
			}
			dir.Files[step.Name] = added
		}
	} else {
		return nil, tschema.ErrEmptySchema
	}

	return dir, nil
}

func (a *Anytype) getFileIndexForPath(pth string) (*tpb.FileIndex, error) {
	plaintext, err := ipfs.DataAtPath(a.Textile.Node().Ipfs(), pth+core.MetaLinkName)
	if err != nil {
		return nil, err
	}

	var file tpb.FileIndex
	err = jsonpb.Unmarshal(bytes.NewReader(plaintext), &file)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

// IndexFileData walks a file data node, indexing file links
func (a *Anytype) indexFileData(inode ipld.Node, data string) error {
	for _, link := range inode.Links() {
		nd, err := ipfs.NodeAtLink(a.ipfs(), link)
		if err != nil {
			return err
		}
		err = a.indexFileNode(nd, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// indexFileNode walks a file node, indexing file links
func (a *Anytype) indexFileNode(inode ipld.Node, data string) error {
	links := inode.Links()

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

	return nil
}

// indexFileLink indexes a file link
func (a *Anytype) indexFileLink(inode ipld.Node, data string) error {
	dlink := tschema.LinkByName(inode.Links(), core.ValidContentLinkNames)
	if dlink == nil {
		return core.ErrMissingContentLink
	}

	return a.Textile.Node().Datastore().Files().AddTarget(dlink.Cid.Hash().B58String(), data)
}

func (a *Anytype) addFileIndexFromPath(target string, path string, key string) (*tpb.FileIndex, error) {
	fd, err := ipfs.DataAtPath(a.ipfs(), path+"/"+core.MetaLinkName)
	if err != nil {
		return nil, err
	}

	var plaintext []byte
	if key != "" {
		keyb, err := base58.Decode(key)
		if err != nil {
			return nil, err
		}
		plaintext, err = crypto.DecryptAES(fd, keyb)
		if err != nil {
			return nil, err
		}
	} else {
		plaintext = fd
	}

	var file tpb.FileIndex
	err = jsonpb.Unmarshal(bytes.NewReader(plaintext), &file)
	if err != nil {
		// todo: get a fixed error if trying to unmarshal an encrypted file
		return nil, err
	}

	log.Debugf("addFileIndexFromPath got file: %s", file.Hash)

	file.Targets = []string{target}
	err = a.textile().Datastore().Files().Add(&file)
	if err != nil {
		if !db.ConflictError(err) {
			return nil, err
		}
		log.Debugf("file exists: %s", file.Hash)
	}
	return &file, nil
}

// looksLikeFileNode returns whether or not a node appears to
// be a textile node. It doesn't inspect the actual data.
func looksLikeFileNode(node ipld.Node) bool {
	links := node.Links()
	if len(links) != 2 {
		return false
	}
	if tschema.LinkByName(links, core.ValidMetaLinkNames) == nil ||
		tschema.LinkByName(links, core.ValidContentLinkNames) == nil {
		return false
	}
	return true
}
