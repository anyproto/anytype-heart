package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/gogo/protobuf/proto"
	"github.com/h2non/filetype"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	ipfspath "github.com/ipfs/go-path"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/multiformats/go-base32"
	mh "github.com/multiformats/go-multihash"
	tpb "github.com/textileio/go-textile/pb"
	tschema "github.com/textileio/go-textile/schema"
	"github.com/textileio/go-threads/crypto/symmetric"

	"github.com/anytypeio/go-anytype-library/ipfs"
	m "github.com/anytypeio/go-anytype-library/mill"
	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/anytypeio/go-anytype-library/pb/lsmodel"
	"github.com/anytypeio/go-anytype-library/pb/storage"
)

var ErrFileNotFound = fmt.Errorf("file not found")
var ErrMissingMetaLink = fmt.Errorf("meta link not in node")
var ErrMissingContentLink = fmt.Errorf("content link not in node")

const MetaLinkName = "meta"
const ContentLinkName = "content"

var ValidMetaLinkNames = []string{"meta", "f"}
var ValidContentLinkNames = []string{"content", "d"}

var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

func (a *Anytype) FileByHash(ctx context.Context, hash string) (File, error) {
	files, err := a.localStore.Files.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, ErrFileNotFound
	}

	/*if len(files) == 0 {
		files, err = a.getFileIndexes(ctx, hash)
		if err != nil {
			log.Errorf("FileByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrFileNotFound
		}
	}*/

	fileIndex := files[0]
	return &file{
		hash:  hash,
		index: fileIndex,
		node:  a,
	}, nil
}

func (a *Anytype) FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error) {
	return a.FileAddWithReader(ctx, bytes.NewReader(content), filename)
}

func (a *Anytype) FileAddWithReader(ctx context.Context, content io.Reader, filename string) (File, error) {
	fileConfig, err := a.getFileConfig(ctx, content, filename, "", false)
	if err != nil {
		return nil, err
	}

	// todo: PR textile to be able to use reader instead of bytes
	fileIndex, err := a.addFileIndex(ctx, &m.Blob{}, *fileConfig)
	if err != nil {
		return nil, err
	}

	node, keys, err := a.AddNodeFromFiles(ctx, []*lsmodel.FileInfo{fileIndex})
	if err != nil {
		return nil, err
	}

	nodeHash := node.Cid().String()

	err = a.indexFileData(ctx, node, nodeHash)
	if err != nil {
		return nil, err
	}

	filesKeysCacheMutex.Lock()
	defer filesKeysCacheMutex.Unlock()
	filesKeysCache[nodeHash] = keys.KeysByPath

	return &file{
		hash:  nodeHash,
		index: fileIndex,
		node:  a,
	}, nil
}

func (a *Anytype) getFileIndexForPath(pth string) (*lsmodel.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// IndexFileData walks a file data node, indexing file links
func (a *Anytype) indexFileData(ctx context.Context, inode ipld.Node, data string) error {
	for _, link := range inode.Links() {
		nd, err := ipfs.NodeAtLink(ctx, a.ipfs(), link)
		if err != nil {
			return err
		}
		err = a.indexFileNode(ctx, nd, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// indexFileNode walks a file node, indexing file links
func (a *Anytype) indexFileNode(ctx context.Context, inode ipld.Node, data string) error {
	links := inode.Links()

	if looksLikeFileNode(inode) {
		return a.indexFileLink(ctx, inode, data)
	}

	for _, link := range links {
		n, err := ipfs.NodeAtLink(ctx, a.ipfs(), link)
		if err != nil {
			return err
		}

		err = a.indexFileLink(ctx, n, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// indexFileLink indexes a file link
func (a *Anytype) indexFileLink(ctx context.Context, inode ipld.Node, data string) error {
	dlink := tschema.LinkByName(inode.Links(), ValidContentLinkNames)
	if dlink == nil {
		return ErrMissingContentLink
	}

	return a.localStore.Files.AddTarget(dlink.Cid.String(), data)
}

func (a *Anytype) addFileIndexFromPath(target string, path string, key string) (*lsmodel.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (t *Anytype) fileMeta(hash string) (*lsmodel.FileInfo, error) {
	file, err := t.localStore.Files.GetByHash(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get the file meta content for hash %s with error: %w", hash, err)
	}
	return file, nil
}

func (t *Anytype) fileContent(ctx context.Context, hash string) (io.ReadSeeker, *lsmodel.FileInfo, error) {
	var err error
	var file *lsmodel.FileInfo
	var reader io.ReadSeeker
	file, err = t.fileMeta(hash)
	if err != nil {
		return nil, nil, err
	}
	reader, err = t.fileIndexContent(ctx, file)
	return reader, file, err
}

func (t *Anytype) fileIndexContent(ctx context.Context, file *lsmodel.FileInfo) (io.ReadSeeker, error) {
	fileCid, err := cid.Parse(file.Hash)
	if err != nil {
		return nil, err
	}
	fd, err := t.ts.GetIpfsLite().GetFile(ctx, fileCid)
	if err != nil {
		return nil, err
	}

	var plaintext []byte
	if file.Key != "" {
		key, err := symmetric.FromString(file.Key)
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		b, err := ioutil.ReadAll(fd)
		if err != nil {
			return nil, err
		}
		plaintext, err = key.Decrypt(b)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(plaintext), nil
	}

	return fd, nil
}

type AddFileConfig struct {
	Input     []byte `json:"input"`
	Use       string `json:"use"`
	Media     string `json:"media"`
	Name      string `json:"name"`
	Plaintext bool   `json:"plaintext"`
}

func checksum(plaintext []byte, wontEncrypt bool) string {
	var add int
	if wontEncrypt {
		add = 1
	}
	plaintext = append(plaintext, byte(add))
	sum := sha256.Sum256(plaintext)
	return base32.RawHexEncoding.EncodeToString(sum[:])
}

func (t *Anytype) addFileIndex(ctx context.Context, mill m.Mill, conf AddFileConfig) (*lsmodel.FileInfo, error) {
	var source string
	if conf.Use != "" {
		source = conf.Use
	} else {
		source = checksum(conf.Input, conf.Plaintext)
	}

	opts, err := mill.Options(map[string]interface{}{
		"plaintext": conf.Plaintext,
	})
	if err != nil {
		return nil, err
	}

	if efile, _ := t.localStore.Files.GetBySource(mill.ID(), source, opts); efile != nil {
		return efile, nil
	}

	res, err := mill.Mill(conf.Input, conf.Name)
	if err != nil {
		return nil, err
	}

	check := checksum(res.File, conf.Plaintext)
	if efile, _ := t.localStore.Files.GetByChecksum(mill.ID(), check); efile != nil {
		return efile, nil
	}

	model := &lsmodel.FileInfo{
		Mill:     mill.ID(),
		Checksum: check,
		Source:   source,
		Opts:     opts,
		Media:    conf.Media,
		Name:     conf.Name,
		Size_:    int64(len(res.File)),
		Added:    time.Now().Unix(),
		Meta:     pb.ToStruct(res.Meta),
	}

	var reader *bytes.Reader
	if mill.Encrypt() && !conf.Plaintext {
		key, err := symmetric.NewRandom()
		if err != nil {
			return nil, err
		}
		ciphertext, err := key.Encrypt(res.File)
		if err != nil {
			return nil, err
		}
		model.Key = key.String()
		reader = bytes.NewReader(ciphertext)
	} else {
		reader = bytes.NewReader(res.File)
	}

	node, err := t.ts.GetIpfsLite().AddFile(ctx, reader, &ipfslite.AddParams{})
	if err != nil {
		return nil, err
	}
	model.Hash = node.Cid().String()

	err = t.localStore.Files.Add(model)
	if err != nil {
		return nil, err
	}

	return model, nil
}

func (a *Anytype) getFileConfig(ctx context.Context, reader io.Reader, filename string, use string, plaintext bool) (*AddFileConfig, error) {
	conf := &AddFileConfig{}

	if use == "" {
		conf.Name = filename
	} else {
		ref, err := ipfspath.ParsePath(use)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(ref.String(), "/")
		hash := parts[len(parts)-1]
		var file *lsmodel.FileInfo
		reader, file, err = a.fileContent(ctx, hash)
		if err != nil {
			/*if err == localstore.ErrNotFound{
				// just cat the data from ipfs
				b, err := ipfsutil.DataAtPath(a.ipfs(), ref.String())
				if err != nil {
					return nil, err
				}
				reader = bytes.NewReader(b)
				conf.Use = ref.String()
			} else {*/
			return nil, err
			//}
		} else {
			conf.Use = file.Checksum
		}
	}

	buf := bufio.NewReader(reader)
	data, err := buf.Peek(512)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to get first 512 bytes to detect content-type: %s", err)
	}
	t, err := filetype.Match(data)
	if err != nil {
		log.Warningf("filetype failed to match for %s: %s", filename, err.Error())
		conf.Media = http.DetectContentType(data)
	} else {
		conf.Media = t.MIME.Value
	}

	data, err = ioutil.ReadAll(buf)
	if err != nil {
		return nil, err
	}
	conf.Input = data
	conf.Plaintext = plaintext

	return conf, nil
}

func (t *Anytype) AddNodeFromFiles(ctx context.Context, files []*lsmodel.FileInfo) (ipld.Node, *storage.FileKeys, error) {
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(t.ts.GetIpfsLite().DAGService)
	outer.SetCidBuilder(cidBuilder)

	var err error
	for i, file := range files {
		link := strconv.Itoa(i)
		err = t.fileNode(ctx, file, outer, link)
		if err != nil {
			return nil, nil, err
		}
		keys.KeysByPath["/"+link+"/"] = file.Key
	}

	node, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}

	err = t.ipfs().Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}

	/*err = ipfs.PinNode(t.node, node, false)
	if err != nil {
		return nil, nil, err
	}*/
	return node, keys, nil
}

func (t *Anytype) fileNode(ctx context.Context, file *lsmodel.FileInfo, dir uio.Directory, link string) error {
	file, err := t.localStore.Files.GetByHash(file.Hash)
	if err != nil {
		return err
	}

	// remove locally indexed targets
	file.Targets = nil

	plaintext, err := proto.Marshal(file)
	if err != nil {
		return err
	}

	var reader io.Reader
	if file.Key != "" {
		key, err := symmetric.FromString(file.Key)
		if err != nil {
			return err
		}

		ciphertext, err := key.Encrypt(plaintext)
		if err != nil {
			return err
		}

		reader = bytes.NewReader(ciphertext)
	} else {
		reader = bytes.NewReader(plaintext)
	}

	pair := uio.NewDirectory(t.ts.GetIpfsLite().DAGService)
	pair.SetCidBuilder(cidBuilder)

	_, err = ipfs.AddDataToDirectory(ctx, t.ts.GetIpfsLite(), pair, MetaLinkName, reader)
	if err != nil {
		return err
	}

	err = ipfs.AddLinkToDirectory(ctx, t.ts.GetIpfsLite(), pair, ContentLinkName, file.Hash)
	if err != nil {
		return err
	}

	node, err := pair.GetNode()
	if err != nil {
		return err
	}
	err = t.ipfs().Add(ctx, node)
	if err != nil {
		return err
	}

	/*err = ipfs.PinNode(t.node, node, false)
	if err != nil {
		return err
	}*/

	return ipfs.AddLinkToDirectory(ctx, t.ts.GetIpfsLite(), dir, link, node.Cid().String())
}

// looksLikeFileNode returns whether or not a node appears to
// be a textile node. It doesn't inspect the actual data.
func looksLikeFileNode(node ipld.Node) bool {
	links := node.Links()
	if len(links) != 2 {
		return false
	}
	if tschema.LinkByName(links, ValidMetaLinkNames) == nil ||
		tschema.LinkByName(links, ValidContentLinkNames) == nil {
		return false
	}
	return true
}

func (a *Anytype) buildDirectory(ctx context.Context, content []byte, filename string, sch *tpb.Node) (*lsmodel.Directory, error) {
	dir := &lsmodel.Directory{
		Files: make(map[string]*lsmodel.FileInfo),
	}

	reader := bytes.NewReader(content)
	mil, err := schema.GetMill(sch.Mill, sch.Opts)
	if err != nil {
		return nil, err
	}
	if mil != nil {
		conf, err := a.getFileConfig(ctx, reader, filename, "", sch.Plaintext)
		if err != nil {
			return nil, err
		}

		added, err := a.addFileIndex(ctx, mil, *conf)
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
			var conf *AddFileConfig
			if step.Link.Use == tschema.FileTag {
				conf, err = a.getFileConfig(
					ctx,
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

				conf, err = a.getFileConfig(
					ctx,
					nil,
					filename,
					dir.Files[step.Link.Use].Hash,
					step.Link.Plaintext,
				)
				if err != nil {
					return nil, err
				}
			}

			added, err := a.addFileIndex(ctx, stepMill, *conf)
			if err != nil {
				return nil, err
			}
			dir.Files[step.Name] = added
			reader.Seek(0, 0)
		}
	} else {
		return nil, tschema.ErrEmptySchema
	}

	return dir, nil
}
func (t *Anytype) AddNodeFromDirs(ctx context.Context, dirs *lsmodel.DirectoryList) (ipld.Node, *storage.FileKeys, error) {
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(t.ts)
	outer.SetCidBuilder(cidBuilder)

	for i, dir := range dirs.Items {
		inner := uio.NewDirectory(t.ts)
		inner.SetCidBuilder(cidBuilder)
		olink := strconv.Itoa(i)

		var err error
		for link, file := range dir.Files {
			err = t.fileNode(ctx, file, inner, link)
			if err != nil {
				return nil, nil, err
			}
			keys.KeysByPath["/"+olink+"/"+link+"/"] = file.Key
		}

		node, err := inner.GetNode()
		if err != nil {
			return nil, nil, err
		}
		// todo: pin?
		err = t.ts.Add(ctx, node)
		if err != nil {
			return nil, nil, err
		}

		id := node.Cid().String()
		err = ipfs.AddLinkToDirectory(ctx, t.ts.GetIpfsLite(), outer, olink, id)
		if err != nil {
			return nil, nil, err
		}
	}

	node, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}
	// todo: pin?
	err = t.ts.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	return node, keys, nil
}
