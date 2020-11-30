package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/helpers"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	m "github.com/anytypeio/go-anytype-middleware/pkg/lib/mill"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill/schema"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill/schema/anytype"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/multiformats/go-base32"
	mh "github.com/multiformats/go-multihash"
	"github.com/textileio/go-threads/crypto/symmetric"
)

var log = logging.Logger("anytype-files")

type Service struct {
	store localstore.FileStore
	ipfs  ipfs.IPFS
	pins  pin.FilePinService
}

func New(store localstore.FileStore, ipfs ipfs.IPFS, pins pin.FilePinService) *Service {
	return &Service{
		store: store,
		ipfs:  ipfs,
		pins:  pins,
	}
}

var ErrMissingMetaLink = fmt.Errorf("meta link not in node")
var ErrMissingContentLink = fmt.Errorf("content link not in node")

const MetaLinkName = "meta"
const ContentLinkName = "content"

var ValidMetaLinkNames = []string{"meta"}
var ValidContentLinkNames = []string{"content"}

var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

const maxPinAttempts = 10

func (s *Service) FileAdd(ctx context.Context, opts AddOptions) (string, *storage.FileInfo, error) {
	fileInfo, err := s.FileAddWithConfig(ctx, &m.Blob{}, opts)
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromFiles(ctx, []*storage.FileInfo{fileInfo})
	if err != nil {
		return "", nil, err
	}

	nodeHash := node.Cid().String()

	if err = s.fileIndexData(ctx, node, nodeHash); err != nil {
		return "", nil, err
	}

	if err = s.store.AddFileKeys(localstore.FileKeys{
		Hash: nodeHash,
		Keys: keys.KeysByPath,
	}); err != nil {
		return "", nil, err
	}

	go func() {
		for attempt := 1; attempt <= maxPinAttempts; attempt++ {
			if err := s.pins.FilePin(nodeHash); err != nil {
				if errors.Is(err, pin.ErrNoCafe) {
					return
				}

				log.Errorf("failed to pin file %s on the cafe (attempt %d): %s", nodeHash, attempt, err.Error())
				time.Sleep(time.Minute * time.Duration(attempt))
				continue
			}

			log.Debugf("pinning file %s started on the cafe", nodeHash)
			break
		}
	}()

	return nodeHash, fileInfo, nil
}

// fileRestoreKeys restores file path=>key map from the IPFS DAG using the keys in the localStore
func (s *Service) FileRestoreKeys(ctx context.Context, hash string) (map[string]string, error) {
	links, err := helpers.LinksAtCid(ctx, s.ipfs, hash)
	if err != nil {
		return nil, err
	}

	var fileKeys = make(map[string]string)
	for _, index := range links {
		node, err := helpers.NodeAtLink(ctx, s.ipfs, index)
		if err != nil {
			return nil, err
		}

		if looksLikeFileNode(node) {
			l := schema.LinkByName(node.Links(), ValidContentLinkNames)
			info, err := s.store.GetByHash(l.Cid.String())
			if err == nil {
				fileKeys["/"+index.Name+"/"] = info.Key
			} else {
				log.Warnf("fileRestoreKeys not found in db %s(%s)", node.Cid().String(), hash+"/"+index.Name)
			}
		} else {
			for _, link := range node.Links() {
				innerLinks, err := helpers.LinksAtCid(ctx, s.ipfs, link.Cid.String())
				if err != nil {
					return nil, err
				}

				l := schema.LinkByName(innerLinks, ValidContentLinkNames)
				if l == nil {
					log.Errorf("con")
					continue
				}

				info, err := s.store.GetByHash(l.Cid.String())

				if err == nil {
					fileKeys["/"+index.Name+"/"+link.Name+"/"] = info.Key
				} else {
					log.Warnf("fileRestoreKeys not found in db %s(%s)", node.Cid().String(), "/"+index.Name+"/"+link.Name+"/")
				}
			}
		}
	}

	err = s.store.AddFileKeys(localstore.FileKeys{
		Hash: hash,
		Keys: fileKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	return fileKeys, nil
}

func (s *Service) fileAddNodeFromDirs(ctx context.Context, dirs *storage.DirectoryList) (ipld.Node, *storage.FileKeys, error) {
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(s.ipfs)
	outer.SetCidBuilder(cidBuilder)

	for i, dir := range dirs.Items {
		inner := uio.NewDirectory(s.ipfs)
		inner.SetCidBuilder(cidBuilder)
		olink := strconv.Itoa(i)

		var err error
		for link, file := range dir.Files {
			err = s.fileNode(ctx, file, inner, link)
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
		err = s.ipfs.Add(ctx, node)
		if err != nil {
			return nil, nil, err
		}

		id := node.Cid().String()
		err = helpers.AddLinkToDirectory(ctx, s.ipfs, outer, olink, id)
		if err != nil {
			return nil, nil, err
		}
	}

	node, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}
	// todo: pin?
	err = s.ipfs.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	return node, keys, nil
}

func (s *Service) fileAddNodeFromFiles(ctx context.Context, files []*storage.FileInfo) (ipld.Node, *storage.FileKeys, error) {
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(s.ipfs)
	outer.SetCidBuilder(cidBuilder)

	var err error
	for i, file := range files {
		link := strconv.Itoa(i)
		err = s.fileNode(ctx, file, outer, link)
		if err != nil {
			return nil, nil, err
		}
		keys.KeysByPath["/"+link+"/"] = file.Key
	}

	node, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}

	err = s.ipfs.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}

	/*err = helpers.PinNode(s.node, node, false)
	if err != nil {
		return nil, nil, err
	}*/
	return node, keys, nil
}

func (s *Service) FileGetInfoForPath(pth string) (*storage.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// fileIndexData walks a file data node, indexing file links
func (s *Service) fileIndexData(ctx context.Context, inode ipld.Node, data string) error {
	for _, link := range inode.Links() {
		nd, err := helpers.NodeAtLink(ctx, s.ipfs, link)
		if err != nil {
			return err
		}
		err = s.fileIndexNode(ctx, nd, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// fileIndexNode walks a file node, indexing file links
func (s *Service) fileIndexNode(ctx context.Context, inode ipld.Node, data string) error {
	links := inode.Links()

	if looksLikeFileNode(inode) {
		return s.fileIndexLink(ctx, inode, data)
	}

	for _, link := range links {
		n, err := helpers.NodeAtLink(ctx, s.ipfs, link)
		if err != nil {
			return err
		}

		err = s.fileIndexLink(ctx, n, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// fileIndexLink indexes a file link
func (s *Service) fileIndexLink(ctx context.Context, inode ipld.Node, data string) error {
	dlink := schema.LinkByName(inode.Links(), ValidContentLinkNames)
	if dlink == nil {
		return ErrMissingContentLink
	}

	return s.store.AddTarget(dlink.Cid.String(), data)
}

func (s *Service) fileInfoFromPath(target string, path string, key string) (*storage.FileInfo, error) {
	plaintext, err := helpers.DataAtPath(context.TODO(), s.ipfs, path+"/"+MetaLinkName)
	if err != nil {
		return nil, err
	}

	if key != "" {
		key, err := symmetric.FromString(key)
		if err != nil {
			return nil, err
		}
		plaintext, err = key.Decrypt(plaintext)
		if err != nil {
			return nil, err
		}
	}

	var file storage.FileInfo
	err = proto.Unmarshal(plaintext, &file)
	if err != nil {
		log.Errorf("failed to decode proto: %s", string(plaintext))
		// todo: get s fixed error if trying to unmarshal an encrypted file
		return nil, err
	}

	file.Targets = []string{target}

	return &file, nil
}

func (s *Service) fileContent(ctx context.Context, hash string) (io.ReadSeeker, *storage.FileInfo, error) {
	var err error
	var file *storage.FileInfo
	var reader io.ReadSeeker
	file, err = s.store.GetByHash(hash)
	if err != nil {
		return nil, nil, err
	}
	reader, err = s.FileContentReader(ctx, file)
	return reader, file, err
}

func (s *Service) FileContentReader(ctx context.Context, file *storage.FileInfo) (io.ReadSeeker, error) {
	fileCid, err := cid.Parse(file.Hash)
	if err != nil {
		return nil, err
	}
	fd, err := s.ipfs.GetFile(ctx, fileCid)
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

func (s *Service) FileAddWithConfig(ctx context.Context, mill m.Mill, conf AddOptions) (*storage.FileInfo, error) {
	var source string
	input, err := ioutil.ReadAll(conf.Reader)
	if err != nil {
		return nil, err
	}

	if conf.Use != "" {
		source = conf.Use
	} else {
		source = checksum(input, conf.Plaintext)
	}

	opts, err := mill.Options(map[string]interface{}{
		"plaintext": conf.Plaintext,
	})
	if err != nil {
		return nil, err
	}

	if efile, _ := s.store.GetBySource(mill.ID(), source, opts); efile != nil {
		return efile, nil
	}

	res, err := mill.Mill(input, conf.Name)
	if err != nil {
		return nil, err
	}

	check := checksum(res.File, conf.Plaintext)
	if efile, _ := s.store.GetByChecksum(mill.ID(), check); efile != nil {
		return efile, nil
	}

	model := &storage.FileInfo{
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

	node, err := s.ipfs.AddFile(ctx, reader, nil)
	if err != nil {
		return nil, err
	}
	model.Hash = node.Cid().String()

	err = s.store.Add(model)
	if err != nil {
		return nil, err
	}

	return model, nil
}

func (s *Service) fileNode(ctx context.Context, file *storage.FileInfo, dir uio.Directory, link string) error {
	file, err := s.store.GetByHash(file.Hash)
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

	pair := uio.NewDirectory(s.ipfs)
	pair.SetCidBuilder(cidBuilder)

	_, err = helpers.AddDataToDirectory(ctx, s.ipfs, pair, MetaLinkName, reader)
	if err != nil {
		return err
	}

	err = helpers.AddLinkToDirectory(ctx, s.ipfs, pair, ContentLinkName, file.Hash)
	if err != nil {
		return err
	}

	node, err := pair.GetNode()
	if err != nil {
		return err
	}
	err = s.ipfs.Add(ctx, node)
	if err != nil {
		return err
	}

	/*err = helpers.PinNode(s.node, node, false)
	if err != nil {
		return err
	}*/

	return helpers.AddLinkToDirectory(ctx, s.ipfs, dir, link, node.Cid().String())
}

func (s *Service) fileBuildDirectory(ctx context.Context, content []byte, filename string, plaintext bool, sch *storage.Node) (*storage.Directory, error) {
	dir := &storage.Directory{
		Files: make(map[string]*storage.FileInfo),
	}

	reader := bytes.NewReader(content)
	mil, err := anytype.GetMill(sch.Mill, sch.Opts)
	if err != nil {
		return nil, err
	}
	if mil != nil {
		opts := AddOptions{
			Reader:    reader,
			Use:       "",
			Media:     "",
			Name:      filename,
			Plaintext: sch.Plaintext || plaintext,
		}
		err := s.NormalizeOptions(ctx, &opts)
		if err != nil {
			return nil, err
		}

		added, err := s.FileAddWithConfig(ctx, mil, opts)
		if err != nil {
			return nil, err
		}
		dir.Files[schema.SingleFileTag] = added

	} else if len(sch.Links) > 0 {
		// determine order
		steps, err := schema.Steps(sch.Links)
		if err != nil {
			return nil, err
		}

		// send each link
		for _, step := range steps {
			stepMill, err := anytype.GetMill(step.Link.Mill, step.Link.Opts)
			if err != nil {
				return nil, err
			}
			var opts *AddOptions
			if step.Link.Use == schema.FileTag {
				opts = &AddOptions{
					Reader:    reader,
					Use:       "",
					Media:     "",
					Name:      filename,
					Plaintext: step.Link.Plaintext || plaintext,
				}
				err = s.NormalizeOptions(ctx, opts)
				if err != nil {
					return nil, err
				}

			} else {
				if dir.Files[step.Link.Use] == nil {
					return nil, fmt.Errorf(step.Link.Use + " not found")
				}

				opts = &AddOptions{
					Reader:    nil,
					Use:       dir.Files[step.Link.Use].Hash,
					Media:     "",
					Name:      filename,
					Plaintext: step.Link.Plaintext || plaintext,
				}

				err = s.NormalizeOptions(ctx, opts)
				if err != nil {
					return nil, err
				}
			}

			added, err := s.FileAddWithConfig(ctx, stepMill, *opts)
			if err != nil {
				return nil, err
			}
			dir.Files[step.Name] = added
			reader.Seek(0, 0)
		}
	} else {
		return nil, schema.ErrEmptySchema
	}

	return dir, nil
}

func (s *Service) FileIndexInfo(ctx context.Context, hash string) ([]*storage.FileInfo, error) {
	links, err := helpers.LinksAtCid(ctx, s.ipfs, hash)
	if err != nil {
		return nil, err
	}

	keys, err := s.store.GetFileKeys(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get file keys from cache: %w", err)
	}

	var files []*storage.FileInfo
	for _, index := range links {
		node, err := helpers.NodeAtLink(ctx, s.ipfs, index)
		if err != nil {
			return nil, err
		}

		if looksLikeFileNode(node) {
			var key string
			if keys != nil {
				key = keys["/"+index.Name+"/"]
			}

			fileIndex, err := s.fileInfoFromPath(hash, hash+"/"+index.Name, key)
			if err != nil {
				return nil, fmt.Errorf("fileInfoFromPath error: %s", err.Error())
			}
			files = append(files, fileIndex)
		} else {
			for _, link := range node.Links() {
				var key string
				if keys != nil {
					key = keys["/"+index.Name+"/"+link.Name+"/"]
				}

				fileIndex, err := s.fileInfoFromPath(hash, hash+"/"+index.Name+"/"+link.Name, key)
				if err != nil {
					return nil, fmt.Errorf("fileInfoFromPath error: %s", err.Error())
				}
				files = append(files, fileIndex)
			}
		}
	}

	err = s.store.AddMulti(files...)
	if err != nil {
		return nil, fmt.Errorf("failed to add files to store: %w", err)
	}

	return files, nil
}

// looksLikeFileNode returns whether or not a node appears to
// be a textile node. It doesn't inspect the actual data.
func looksLikeFileNode(node ipld.Node) bool {
	links := node.Links()
	if len(links) != 2 {
		return false
	}
	if schema.LinkByName(links, ValidMetaLinkNames) == nil ||
		schema.LinkByName(links, ValidContentLinkNames) == nil {
		return false
	}
	return true
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
