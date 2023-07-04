package files

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/miolini/datacounter"
	"github.com/multiformats/go-base32"
	mh "github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/cfb"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/gcm"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	m "github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema/anytype"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName = "files"
)

var log = logging.Logger("anytype-files")

var _ Service = (*service)(nil)

type Service interface {
	FileAdd(ctx session.Context, options ...AddOption) (File, error)
	FileByHash(ctx session.Context, hash string) (File, error)
	FileGetKeys(hash string) (*FileKeys, error)
	FileListOffload(fileIDs []string, includeNotPinned bool) (totalBytesOffloaded uint64, totalFilesOffloaded uint64, err error)
	FileOffload(fileID string, includeNotPinned bool) (totalSize uint64, err error)
	GetSpaceUsage(ctx session.Context) (*pb.RpcFileSpaceUsageResponseUsage, error)
	ImageAdd(ctx session.Context, options ...AddOption) (Image, error)
	ImageByHash(ctx session.Context, hash string) (Image, error)
	StoreFileKeys(fileKeys ...FileKeys) error

	app.Component
}

type SyncStatusWatcher interface {
	Watch(spaceID string, id string, fileFunc func() []string) (new bool, err error)
}

type service struct {
	fileStore         filestore.FileStore
	commonFile        fileservice.FileService
	fileSync          filesync.FileSync
	dagService        ipld.DAGService
	spaceService      space.Service
	fileStorage       filestorage.FileStorage
	syncStatusWatcher SyncStatusWatcher
	objectStore       objectstore.ObjectStore
}

func New(statusWatcher SyncStatusWatcher, objectStore objectstore.ObjectStore) Service {
	return &service{
		syncStatusWatcher: statusWatcher,
		objectStore:       objectStore,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.fileStore = a.MustComponent("filestore").(filestore.FileStore)
	s.commonFile = a.MustComponent(fileservice.CName).(fileservice.FileService)
	s.fileSync = a.MustComponent(filesync.CName).(filesync.FileSync)
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.dagService = s.commonFile.DAGService()
	s.fileStorage = app.MustComponent[filestorage.FileStorage](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

type FileKeys struct {
	Hash string
	Keys map[string]string
}

var ErrMissingContentLink = fmt.Errorf("content link not in node")

const MetaLinkName = "meta"
const ContentLinkName = "content"

var ValidMetaLinkNames = []string{"meta"}
var ValidContentLinkNames = []string{"content"}

var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

func (s *service) fileAdd(ctx session.Context, opts AddOptions) (string, *storage.FileInfo, error) {
	fileInfo, err := s.fileAddWithConfig(ctx.Context(), &m.Blob{}, opts)
	if err != nil {
		return "", nil, err
	}

	node, keys, err := s.fileAddNodeFromFiles(ctx.Context(), []*storage.FileInfo{fileInfo})
	if err != nil {
		return "", nil, err
	}

	nodeHash := node.Cid().String()
	if err = s.fileIndexData(ctx, node, nodeHash); err != nil {
		return "", nil, err
	}

	if err = s.fileStore.AddFileKeys(filestore.FileKeys{
		Hash: nodeHash,
		Keys: keys.KeysByPath,
	}); err != nil {
		return "", nil, err
	}

	return nodeHash, fileInfo, nil
}

// fileRestoreKeys restores file path=>key map from the IPFS DAG using the keys in the localStore
func (s *service) fileRestoreKeys(ctx context.Context, hash string) (map[string]string, error) {
	links, err := helpers.LinksAtCid(ctx, s.dagService, hash)
	if err != nil {
		return nil, err
	}

	var fileKeys = make(map[string]string)
	for _, index := range links {
		node, err := helpers.NodeAtLink(ctx, s.dagService, index)
		if err != nil {
			return nil, err
		}

		if looksLikeFileNode(node) {
			l := schema.LinkByName(node.Links(), ValidContentLinkNames)
			info, err := s.fileStore.GetByHash(l.Cid.String())
			if err == nil {
				fileKeys["/"+index.Name+"/"] = info.Key
			} else {
				log.Warnf("fileRestoreKeys not found in db %s(%s)", node.Cid().String(), hash+"/"+index.Name)
			}
		} else {
			for _, link := range node.Links() {
				innerLinks, err := helpers.LinksAtCid(ctx, s.dagService, link.Cid.String())
				if err != nil {
					return nil, err
				}

				l := schema.LinkByName(innerLinks, ValidContentLinkNames)
				if l == nil {
					log.Errorf("con")
					continue
				}

				info, err := s.fileStore.GetByHash(l.Cid.String())

				if err == nil {
					fileKeys["/"+index.Name+"/"+link.Name+"/"] = info.Key
				} else {
					log.Warnf("fileRestoreKeys not found in db %s(%s)", node.Cid().String(), "/"+index.Name+"/"+link.Name+"/")
				}
			}
		}
	}

	err = s.fileStore.AddFileKeys(filestore.FileKeys{
		Hash: hash,
		Keys: fileKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	return fileKeys, nil
}

func (s *service) fileAddNodeFromDirs(ctx context.Context, dirs *storage.DirectoryList) (ipld.Node, *storage.FileKeys, error) {
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(s.dagService)
	outer.SetCidBuilder(cidBuilder)

	for i, dir := range dirs.Items {
		inner := uio.NewDirectory(s.dagService)
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
		err = s.dagService.Add(ctx, node)
		if err != nil {
			return nil, nil, err
		}

		id := node.Cid().String()
		err = helpers.AddLinkToDirectory(ctx, s.dagService, outer, olink, id)
		if err != nil {
			return nil, nil, err
		}
	}

	node, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}
	// todo: pin?
	err = s.dagService.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	return node, keys, nil
}

func (s *service) fileAddNodeFromFiles(ctx context.Context, files []*storage.FileInfo) (ipld.Node, *storage.FileKeys, error) {
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(s.dagService)
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

	err = s.dagService.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	return node, keys, nil
}

func (s *service) fileGetInfoForPath(ctx context.Context, pth string) (*storage.FileInfo, error) {
	if !strings.HasPrefix(pth, "/ipfs/") {
		return nil, fmt.Errorf("path should starts with '/dagService/...'")
	}

	pthParts := strings.Split(pth, "/")
	if len(pthParts) < 4 {
		return nil, fmt.Errorf("path is too short: it should match '/ipfs/:hash/...'")
	}

	keys, err := s.FileGetKeys(pthParts[2])
	if err != nil {
		return nil, fmt.Errorf("failed to retrive file keys: %w", err)
	}

	if key, exists := keys.Keys["/"+strings.Join(pthParts[3:], "/")+"/"]; exists {
		return s.fileInfoFromPath(ctx, "", pth, key)
	}

	return nil, fmt.Errorf("key not found")
}

func (s *service) FileGetKeys(hash string) (*FileKeys, error) {
	m, err := s.fileStore.GetFileKeys(hash)
	if err != nil {
		if err != localstore.ErrNotFound {
			return nil, err
		}
	} else {
		return &FileKeys{
			Hash: hash,
			Keys: m,
		}, nil
	}

	// in case we don't have keys cached fot this file
	// we should have all the CIDs locally, so 5s is more than enough
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fileKeysRestored, err := s.fileRestoreKeys(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to restore file keys: %w", err)
	}

	return &FileKeys{
		Hash: hash,
		Keys: fileKeysRestored,
	}, nil
}

// fileIndexData walks a file data node, indexing file links
func (s *service) fileIndexData(ctx session.Context, inode ipld.Node, hash string) error {
	for _, link := range inode.Links() {
		nd, err := helpers.NodeAtLink(ctx.Context(), s.dagService, link)
		if err != nil {
			return err
		}
		err = s.fileIndexNode(ctx, nd, hash)
		if err != nil {
			return err
		}
	}

	return nil
}

// fileIndexNode walks a file node, indexing file links
func (s *service) fileIndexNode(ctx session.Context, inode ipld.Node, fileID string) error {
	if looksLikeFileNode(inode) {
		return s.fileIndexLink(ctx, inode, fileID)
	}

	links := inode.Links()
	for _, link := range links {
		n, err := helpers.NodeAtLink(ctx.Context(), s.dagService, link)
		if err != nil {
			return err
		}

		err = s.fileIndexLink(ctx, n, fileID)
		if err != nil {
			return err
		}
	}

	return nil
}

// fileIndexLink indexes a file link
func (s *service) fileIndexLink(ctx session.Context, inode ipld.Node, fileID string) error {
	dlink := schema.LinkByName(inode.Links(), ValidContentLinkNames)
	if dlink == nil {
		return ErrMissingContentLink
	}
	linkID := dlink.Cid.String()
	if err := s.fileStore.AddTarget(linkID, fileID); err != nil {
		return fmt.Errorf("add target to %s: %w", linkID, err)
	}
	if err := s.addToSyncQueue(ctx, fileID, true); err != nil {
		return fmt.Errorf("add file %s to sync queue: %w", fileID, err)
	}
	return nil
}

func (s *service) fileInfoFromPath(ctx context.Context, target string, path string, key string) (*storage.FileInfo, error) {
	cid, r, err := helpers.DataAtPath(ctx, s.commonFile, path+"/"+MetaLinkName)
	if err != nil {
		return nil, err
	}

	var file storage.FileInfo

	if key != "" {
		key, err := symmetric.FromString(key)
		if err != nil {
			return nil, err
		}

		modes := []storage.FileInfoEncryptionMode{storage.FileInfo_AES_CFB, storage.FileInfo_AES_GCM}
		for i, mode := range modes {
			if i > 0 {
				_, err = r.Seek(0, io.SeekStart)
				if err != nil {
					return nil, fmt.Errorf("failed to seek ciphertext after enc mode try")
				}
			}
			ed, err := getEncryptorDecryptor(key, mode)
			if err != nil {
				return nil, err
			}
			decryptedReader, err := ed.DecryptReader(r)
			if err != nil {
				return nil, err
			}
			b, err := ioutil.ReadAll(decryptedReader)
			if err != nil {
				if i == len(modes)-1 {
					return nil, fmt.Errorf("failed to unmarshal file info proto with all encryption modes: %w", err)
				}

				continue
			}
			err = proto.Unmarshal(b, &file)
			if err != nil || file.Hash == "" {
				if i == len(modes)-1 {
					return nil, fmt.Errorf("failed to unmarshal file info proto with all encryption modes: %w", err)
				}
				continue
			}
			// save successful enc mode so it will be cached in the DB
			file.EncMode = mode
			break
		}
	} else {
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		err = proto.Unmarshal(b, &file)
		if err != nil || file.Hash == "" {
			return nil, fmt.Errorf("failed to unmarshal not-encrypted file info: %w", err)
		}
	}

	if file.Hash == "" {
		return nil, fmt.Errorf("failed to read file info proto with all encryption modes")
	}
	file.MetaHash = cid.String()
	file.Targets = []string{target}
	return &file, nil
}

func (s *service) fileContent(ctx context.Context, hash string) (io.ReadSeeker, *storage.FileInfo, error) {
	var err error
	var file *storage.FileInfo
	var reader io.ReadSeeker
	file, err = s.fileStore.GetByHash(hash)
	if err != nil {
		return nil, nil, err
	}
	reader, err = s.getContentReader(ctx, file)
	return reader, file, err
}

func (s *service) getContentReader(ctx context.Context, file *storage.FileInfo) (symmetric.ReadSeekCloser, error) {
	fileCid, err := cid.Parse(file.Hash)
	if err != nil {
		return nil, err
	}
	fd, err := s.commonFile.GetFile(ctx, fileCid)
	if err != nil {
		return nil, err
	}
	if file.Key == "" {
		return fd, nil
	}

	key, err := symmetric.FromString(file.Key)
	if err != nil {
		return nil, err
	}

	dec, err := getEncryptorDecryptor(key, file.EncMode)
	if err != nil {
		return nil, err
	}

	return dec.DecryptReader(fd)
}

func (s *service) fileAddWithConfig(ctx context.Context, mill m.Mill, conf AddOptions) (*storage.FileInfo, error) {
	var source string
	if conf.Use != "" {
		source = conf.Use
	} else {
		var err error
		source, err = checksum(conf.Reader, conf.Plaintext)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate checksum: %w", err)
		}
		_, err = conf.Reader.Seek(0, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("failed to seek reader: %w", err)
		}
	}

	opts, err := mill.Options(map[string]interface{}{
		"plaintext": conf.Plaintext,
	})
	if err != nil {
		return nil, err
	}

	if efile, _ := s.fileStore.GetBySource(mill.ID(), source, opts); efile != nil && efile.MetaHash != "" {
		efile.Targets = nil
		return efile, nil
	}

	res, err := mill.Mill(conf.Reader, conf.Name)
	if err != nil {
		return nil, err
	}

	// count the result size after the applied mill
	readerWithCounter := datacounter.NewReaderCounter(res.File)
	check, err := checksum(readerWithCounter, conf.Plaintext)
	if err != nil {
		return nil, err
	}

	if efile, _ := s.fileStore.GetByChecksum(mill.ID(), check); efile != nil && efile.MetaHash != "" {
		efile.Targets = nil
		return efile, nil
	}

	_, err = conf.Reader.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// because mill result reader doesn't support seek we need to do the mill again
	res, err = mill.Mill(conf.Reader, conf.Name)
	if err != nil {
		return nil, err
	}

	fileInfo := &storage.FileInfo{
		Mill:             mill.ID(),
		Checksum:         check,
		Source:           source,
		Opts:             opts,
		Media:            conf.Media,
		Name:             conf.Name,
		LastModifiedDate: conf.LastModifiedDate,
		Added:            time.Now().Unix(),
		Meta:             pbtypes.ToStruct(res.Meta),
		Size_:            int64(readerWithCounter.Count()),
	}

	var (
		contentReader io.Reader
		encryptor     symmetric.EncryptorDecryptor
	)
	if mill.Encrypt() && !conf.Plaintext {
		key, err := symmetric.NewRandom()
		if err != nil {
			return nil, err
		}
		encryptor = cfb.New(key, [aes.BlockSize]byte{})

		contentReader, err = encryptor.EncryptReader(res.File)
		if err != nil {
			return nil, err
		}

		fileInfo.Key = key.String()
		fileInfo.EncMode = storage.FileInfo_AES_CFB
	} else {
		contentReader = res.File
	}

	contentNode, err := s.commonFile.AddFile(ctx, contentReader)
	if err != nil {
		return nil, err
	}

	fileInfo.Hash = contentNode.Cid().String()
	plaintext, err := proto.Marshal(fileInfo)
	if err != nil {
		return nil, err
	}

	var metaReader io.Reader
	if encryptor != nil {
		metaReader, err = encryptor.EncryptReader(bytes.NewReader(plaintext))
		if err != nil {
			return nil, err
		}
	} else {
		metaReader = bytes.NewReader(plaintext)
	}

	metaNode, err := s.commonFile.AddFile(ctx, metaReader)
	if err != nil {
		return nil, err
	}

	fileInfo.MetaHash = metaNode.Cid().String()

	err = s.fileStore.Add(fileInfo)
	if err != nil {
		return nil, err
	}

	return fileInfo, nil
}

func (s *service) fileNode(ctx context.Context, file *storage.FileInfo, dir uio.Directory, link string) error {
	file, err := s.fileStore.GetByHash(file.Hash)
	if err != nil {
		return err
	}

	pair := uio.NewDirectory(s.dagService)
	pair.SetCidBuilder(cidBuilder)

	if file.MetaHash == "" {
		return fmt.Errorf("metaHash is empty")
	}

	err = helpers.AddLinkToDirectory(ctx, s.dagService, pair, MetaLinkName, file.MetaHash)
	err = helpers.AddLinkToDirectory(ctx, s.dagService, pair, ContentLinkName, file.Hash)
	if err != nil {
		return err
	}

	node, err := pair.GetNode()
	if err != nil {
		return err
	}
	err = s.dagService.Add(ctx, node)
	if err != nil {
		return err
	}

	return helpers.AddLinkToDirectory(ctx, s.dagService, dir, link, node.Cid().String())
}

func (s *service) fileBuildDirectory(ctx context.Context, reader io.ReadSeeker, filename string, plaintext bool, sch *storage.Node) (*storage.Directory, error) {
	dir := &storage.Directory{
		Files: make(map[string]*storage.FileInfo),
	}

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
		err := s.normalizeOptions(ctx, &opts)
		if err != nil {
			return nil, err
		}

		added, err := s.fileAddWithConfig(ctx, mil, opts)
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
				err = s.normalizeOptions(ctx, opts)
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

				err = s.normalizeOptions(ctx, opts)
				if err != nil {
					return nil, err
				}
			}

			added, err := s.fileAddWithConfig(ctx, stepMill, *opts)
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

func (s *service) fileIndexInfo(ctx context.Context, hash string, updateIfExists bool) ([]*storage.FileInfo, error) {
	links, err := helpers.LinksAtCid(ctx, s.dagService, hash)
	if err != nil {
		return nil, err
	}

	keys, err := s.fileStore.GetFileKeys(hash)
	if err != nil {
		// no keys means file is not encrypted or keys are missing
		log.Debugf("failed to get file keys from filestore %s: %s", hash, err.Error())
	}

	var files []*storage.FileInfo
	for _, index := range links {
		node, err := helpers.NodeAtLink(ctx, s.dagService, index)
		if err != nil {
			return nil, err
		}

		if looksLikeFileNode(node) {
			var key string
			if keys != nil {
				key = keys["/"+index.Name+"/"]
			}

			fileIndex, err := s.fileInfoFromPath(ctx, hash, hash+"/"+index.Name, key)
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

				fileIndex, err := s.fileInfoFromPath(ctx, hash, hash+"/"+index.Name+"/"+link.Name, key)
				if err != nil {
					return nil, fmt.Errorf("fileInfoFromPath error: %s", err.Error())
				}
				files = append(files, fileIndex)
			}
		}
	}

	err = s.fileStore.AddMulti(updateIfExists, files...)
	if err != nil {
		return nil, fmt.Errorf("failed to add files to store: %w", err)
	}

	return files, nil
}

func (s *service) addToSyncQueue(ctx session.Context, fileID string, uploadedByUser bool) error {
	if err := s.fileSync.AddFile(ctx.SpaceID(), fileID, uploadedByUser); err != nil {
		return fmt.Errorf("add file to sync queue: %w", err)
	}
	if _, err := s.syncStatusWatcher.Watch(ctx.SpaceID(), fileID, nil); err != nil {
		return fmt.Errorf("watch sync status: %w", err)
	}
	return nil
}

// looksLikeFileNode returns whether a node appears to
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

func checksum(r io.Reader, wontEncrypt bool) (string, error) {
	var add int
	if wontEncrypt {
		add = 1
	}
	h := sha256.New()
	_, err := io.Copy(h, r)
	if err != nil {
		return "", err
	}

	_, err = h.Write([]byte{byte(add)})
	if err != nil {
		return "", err
	}
	checksum := h.Sum(nil)
	return base32.RawHexEncoding.EncodeToString(checksum[:]), nil
}

func getEncryptorDecryptor(key symmetric.Key, mode storage.FileInfoEncryptionMode) (symmetric.EncryptorDecryptor, error) {
	switch mode {
	case storage.FileInfo_AES_GCM:
		return gcm.New(key), nil
	case storage.FileInfo_AES_CFB:
		return cfb.New(key, [aes.BlockSize]byte{}), nil
	default:
		return nil, fmt.Errorf("unsupported encryption mode")
	}
}

func (s *service) StoreFileKeys(fileKeys ...FileKeys) error {
	var fks []filestore.FileKeys

	for _, fk := range fileKeys {
		fks = append(fks, filestore.FileKeys{
			Hash: fk.Hash,
			Keys: fk.Keys,
		})
	}

	return s.fileStore.AddFileKeys(fks...)
}

var ErrFileNotFound = fmt.Errorf("file not found")

func (s *service) FileByHash(ctx session.Context, hash string) (File, error) {
	ok, err := s.isDeleted(hash)
	if err != nil {
		return nil, fmt.Errorf("check if file is deleted: %w", err)
	}
	if ok {
		return nil, ErrFileNotFound
	}

	fileList, err := s.fileStore.ListByTarget(hash)
	if err != nil {
		return nil, err
	}

	if len(fileList) == 0 || fileList[0].MetaHash == "" {
		// info from ipfs
		fileList, err = s.fileIndexInfo(ctx.Context(), hash, false)
		if err != nil {
			log.With("cid", hash).Errorf("FileByHash: failed to retrieve from IPFS: %s", err.Error())
			return nil, ErrFileNotFound
		}
		ok, err := s.fileStore.IsFileImported(hash)
		if err != nil {
			return nil, fmt.Errorf("check if file is imported: %w", err)
		}
		if ok {
			log.With("fileID", hash).Warn("file is imported, push it to uploading queue")
			// If file is imported we have to sync it, so we don't set sync status to synced
			err = s.fileStore.SetIsFileImported(hash, false)
			if err != nil {
				return nil, fmt.Errorf("set is file imported: %w", err)
			}
		} else {
			// If file is not imported then it's definitely synced
			err = s.fileStore.SetSyncStatus(hash, int(syncstatus.StatusSynced))
			if err != nil {
				return nil, fmt.Errorf("set sync status: %w", err)
			}
		}
	}
	if err := s.addToSyncQueue(ctx, hash, false); err != nil {
		return nil, fmt.Errorf("add file %s to sync queue: %w", hash, err)
	}
	fileIndex := fileList[0]
	return &file{
		hash: hash,
		info: fileIndex,
		node: s,
	}, nil
}

func (s *service) isDeleted(fileID string) (bool, error) {
	d, err := s.objectStore.GetDetails(fileID)
	if err != nil {
		return false, err
	}
	return pbtypes.GetBool(d.GetDetails(), bundle.RelationKeyIsDeleted.String()), nil
}

func (s *service) FileAdd(ctx session.Context, options ...AddOption) (File, error) {
	opts := AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := s.normalizeOptions(ctx.Context(), &opts)
	if err != nil {
		return nil, err
	}

	hash, info, err := s.fileAdd(ctx, opts)
	if err != nil {
		return nil, err
	}

	f := &file{
		hash: hash,
		info: info,
		node: s,
	}
	return f, nil
}
