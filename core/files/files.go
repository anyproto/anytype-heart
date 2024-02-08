package files

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/gogo/protobuf/proto"
	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/miolini/datacounter"
	"github.com/multiformats/go-base32"
	mh "github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pb"
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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName = "files"

	// We have legacy nodes structure that allowed us to add directories and "0" means the first directory
	// Now we have only one directory in which we have either single file or image variants
	fileLinkName = "0"
)

var log = logging.Logger("anytype-files")

var _ Service = (*service)(nil)

type Service interface {
	FileAdd(ctx context.Context, spaceID string, options ...AddOption) (*AddResult, error)
	FileByHash(ctx context.Context, id domain.FullFileId) (File, error)
	FileGetKeys(id domain.FullFileId) (*domain.FileEncryptionKeys, error)
	FileOffload(ctx context.Context, id domain.FullFileId) (totalSize uint64, err error)
	GetSpaceUsage(ctx context.Context, spaceID string) (*pb.RpcFileSpaceUsageResponseUsage, error)
	GetNodeUsage(ctx context.Context) (*NodeUsageResponse, error)
	ImageAdd(ctx context.Context, spaceID string, options ...AddOption) (*AddResult, error)
	ImageByHash(ctx context.Context, id domain.FullFileId) (Image, error)

	app.Component
}

type service struct {
	fileStore   filestore.FileStore
	commonFile  fileservice.FileService
	fileSync    filesync.FileSync
	dagService  ipld.DAGService
	fileStorage filestorage.FileStorage
	objectStore objectstore.ObjectStore

	lock              sync.Mutex
	addOperationLocks map[string]*sync.Mutex
}

func New() Service {
	return &service{
		addOperationLocks: make(map[string]*sync.Mutex),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.fileSync = app.MustComponent[filesync.FileSync](a)

	s.dagService = s.commonFile.DAGService()
	s.fileStorage = app.MustComponent[filestorage.FileStorage](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

var ErrMissingContentLink = fmt.Errorf("content link not in node")

const MetaLinkName = "meta"
const ContentLinkName = "content"

var ValidMetaLinkNames = []string{"meta"}
var ValidContentLinkNames = []string{"content"}

var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

type AddResult struct {
	FileId         domain.FileId
	EncryptionKeys *domain.FileEncryptionKeys
	IsExisting     bool // Is file already added by user?

	MIME string
	Size int64

	lock *sync.Mutex
}

// Commit transaction of adding a file
func (r *AddResult) Commit() {
	r.lock.Unlock()
}

func (s *service) FileAdd(ctx context.Context, spaceId string, options ...AddOption) (*AddResult, error) {
	opts := AddOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	err := s.normalizeOptions(&opts)
	if err != nil {
		return nil, err
	}

	addLock := s.lockAddOperation(opts.checksum)

	fileInfo, fileNode, err := s.addFileNode(ctx, spaceId, &m.Blob{}, opts)
	if errors.Is(err, errFileExists) {
		return s.newExistingFileResult(addLock, fileInfo)
	}
	if err != nil {
		addLock.Unlock()
		return nil, err
	}

	rootNode, keys, err := s.addFileRootNode(ctx, spaceId, fileInfo, fileNode)
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	fileId := domain.FileId(rootNode.Cid().String())
	err = s.fileStore.LinkFileVariantToFile(fileId, domain.FileContentId(fileInfo.Hash))
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("link file variant to file: %w", err)
	}

	fileKeys := domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: keys.KeysByPath,
	}
	err = s.fileStore.AddFileKeys(fileKeys)
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	err = s.storeFileSize(spaceId, fileId)
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("store file size: %w", err)
	}

	return &AddResult{
		FileId:         fileId,
		EncryptionKeys: &fileKeys,
		Size:           fileInfo.Size_,
		MIME:           opts.Media,
		lock:           addLock,
	}, nil
}

func (s *service) newExistingFileResult(lock *sync.Mutex, fileInfo *storage.FileInfo) (*AddResult, error) {
	fileId, keys, err := s.getFileIdAndEncryptionKeysFromInfo(fileInfo)
	if err != nil {
		return nil, err
	}
	return &AddResult{
		IsExisting:     true,
		FileId:         fileId,
		EncryptionKeys: keys,
		Size:           fileInfo.Size_,
		MIME:           fileInfo.Media,

		lock: lock,
	}, nil
}

func (s *service) getFileIdAndEncryptionKeysFromInfo(fileInfo *storage.FileInfo) (domain.FileId, *domain.FileEncryptionKeys, error) {
	if len(fileInfo.Targets) == 0 {
		return "", nil, fmt.Errorf("file exists but has no root")
	}
	fileId := domain.FileId(fileInfo.Targets[0])
	keys, err := s.fileStore.GetFileKeys(fileId)
	if err != nil {
		return "", nil, fmt.Errorf("can't get encryption keys for existing file: %w", err)
	}
	return fileId, &domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: keys,
	}, nil
}

func (s *service) storeFileSize(spaceId string, fileId domain.FileId) error {
	_, err := s.fileSync.CalculateFileSize(context.Background(), spaceId, fileId)
	return err
}

// fileRestoreKeys restores file path=>key map from the IPFS DAG using the keys in the localStore
func (s *service) fileRestoreKeys(ctx context.Context, id domain.FullFileId) (map[string]string, error) {
	dagService := s.dagServiceForSpace(id.SpaceId)
	outerDirLinks, err := helpers.LinksAtCid(ctx, dagService, id.FileId.String())
	if err != nil {
		return nil, fmt.Errorf("get links of outer dir: %w", err)
	}
	dirNode, dirLink, err := s.getInnerDirNode(ctx, dagService, outerDirLinks)
	if err != nil {
		return nil, fmt.Errorf("get inner dir node: %w", err)
	}

	fileKeys := domain.FileEncryptionKeys{
		FileId:         id.FileId,
		EncryptionKeys: make(map[string]string),
	}

	if looksLikeFileNode(dirNode) {
		l := schema.LinkByName(dirNode.Links(), ValidContentLinkNames)
		info, err := s.fileStore.GetFileVariant(domain.FileContentId(l.Cid.String()))
		if err == nil {
			fileKeys.EncryptionKeys[encryptionKeyPath(fileLinkName)] = info.Key
		} else {
			log.Warnf("fileRestoreKeys not found in db %s(%s)", dirNode.Cid().String(), id.FileId.String()+"/"+dirLink.Name)
		}
	} else {
		for _, link := range dirNode.Links() {
			innerLinks, err := helpers.LinksAtCid(ctx, dagService, link.Cid.String())
			if err != nil {
				return nil, err
			}

			l := schema.LinkByName(innerLinks, ValidContentLinkNames)
			if l == nil {
				continue
			}

			info, err := s.fileStore.GetFileVariant(domain.FileContentId(l.Cid.String()))

			if err == nil {
				fileKeys.EncryptionKeys[encryptionKeyPath(link.Name)] = info.Key
			} else {
				log.Warnf("fileRestoreKeys not found in db %s(%s)", dirNode.Cid().String(), "/"+dirLink.Name+"/"+link.Name+"/")
			}
		}
	}

	err = s.fileStore.AddFileKeys(fileKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	return fileKeys.EncryptionKeys, nil
}

// addFileRootNode has structure:
/*
- dir (outer)
	- dir (file)
		- meta
		- content
	...
*/
func (s *service) addFileRootNode(ctx context.Context, spaceID string, fileInfo *storage.FileInfo, fileNode ipld.Node) (ipld.Node, *storage.FileKeys, error) {
	dagService := s.dagServiceForSpace(spaceID)
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(dagService)
	outer.SetCidBuilder(cidBuilder)

	err := helpers.AddLinkToDirectory(ctx, dagService, outer, fileLinkName, fileNode.Cid().String())
	if err != nil {
		return nil, nil, err
	}
	keys.KeysByPath[encryptionKeyPath(fileLinkName)] = fileInfo.Key

	node, err := outer.GetNode()
	if err != nil {
		return nil, nil, err
	}

	err = dagService.Add(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	return node, keys, nil
}

func (s *service) fileGetInfoForPath(ctx context.Context, spaceID string, pth string) (*storage.FileInfo, error) {
	if !strings.HasPrefix(pth, "/ipfs/") {
		return nil, fmt.Errorf("path should starts with '/ipfs/...'")
	}

	// Path example: /ipfs/bafybeig6lm2kfqqbyh7zwpwb4tszv4upq4lok6pdlzfe4w4a44cfbjiwkm/0/original
	// Path parts:  0 1    2                                                           3 4
	pthParts := strings.Split(pth, "/")
	if len(pthParts) < 4 {
		return nil, fmt.Errorf("path is too short: it should match '/ipfs/:hash/...'")
	}

	id := domain.FullFileId{
		SpaceId: spaceID,
		FileId:  domain.FileId(pthParts[2]),
	}
	keys, err := s.FileGetKeys(id)
	if err != nil {
		return nil, fmt.Errorf("failed to retrive file keys: %w", err)
	}

	if key, exists := keys.EncryptionKeys[encryptionKeyPath(path.Base(pth))]; exists {
		return s.fileInfoFromPath(ctx, id.SpaceId, id.FileId, pth, key)
	}

	return nil, fmt.Errorf("key not found")
}

func (s *service) FileGetKeys(id domain.FullFileId) (*domain.FileEncryptionKeys, error) {
	m, err := s.fileStore.GetFileKeys(id.FileId)
	if err != nil {
		if err != localstore.ErrNotFound {
			return nil, err
		}
	} else {
		return &domain.FileEncryptionKeys{
			FileId:         id.FileId,
			EncryptionKeys: m,
		}, nil
	}

	// in case we don't have keys cached fot this file
	// we should have all the CIDs locally, so 5s is more than enough
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fileKeysRestored, err := s.fileRestoreKeys(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to restore file keys: %w", err)
	}

	return &domain.FileEncryptionKeys{
		FileId:         id.FileId,
		EncryptionKeys: fileKeysRestored,
	}, nil
}

func (s *service) fileInfoFromPath(ctx context.Context, spaceId string, fileId domain.FileId, path string, key string) (*storage.FileInfo, error) {
	id, r, err := s.dataAtPath(ctx, spaceId, path+"/"+MetaLinkName)
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
	file.MetaHash = id.String()
	file.Targets = []string{fileId.String()}
	return &file, nil
}

func (s *service) fileContent(ctx context.Context, spaceId string, childId domain.FileContentId) (io.ReadSeeker, *storage.FileInfo, error) {
	var err error
	var file *storage.FileInfo
	var reader io.ReadSeeker
	file, err = s.fileStore.GetFileVariant(childId)
	if err != nil {
		return nil, nil, err
	}
	reader, err = s.getContentReader(ctx, spaceId, file)
	return reader, file, err
}

func (s *service) getContentReader(ctx context.Context, spaceID string, file *storage.FileInfo) (symmetric.ReadSeekCloser, error) {
	fileCid, err := cid.Parse(file.Hash)
	if err != nil {
		return nil, err
	}
	fd, err := s.getFile(ctx, spaceID, fileCid)
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

var errFileExists = errors.New("file exists")

// addFileNode adds a file node to the DAG. This node has structure:
/*
- dir (file pair):
	- meta
	- content
*/
func (s *service) addFileNode(ctx context.Context, spaceID string, mill m.Mill, conf AddOptions) (*storage.FileInfo, ipld.Node, error) {
	opts, err := mill.Options(map[string]interface{}{
		"plaintext": false,
	})
	if err != nil {
		return nil, nil, err
	}

	if efile, _ := s.fileStore.GetFileVariantBySource(mill.ID(), conf.checksum, opts); efile != nil && efile.MetaHash != "" {
		return efile, nil, errFileExists
	}

	res, err := mill.Mill(conf.Reader, conf.Name)
	if err != nil {
		return nil, nil, err
	}

	// count the result size after the applied mill
	readerWithCounter := datacounter.NewReaderCounter(res.File)
	variantChecksum, err := checksum(readerWithCounter, false)
	if err != nil {
		return nil, nil, err
	}

	if efile, _ := s.fileStore.GetFileVariantByChecksum(mill.ID(), variantChecksum); efile != nil && efile.MetaHash != "" {
		return efile, nil, errFileExists
	}

	_, err = conf.Reader.Seek(0, io.SeekStart)
	if err != nil {
		return nil, nil, err
	}

	// because mill result reader doesn't support seek we need to do the mill again
	res, err = mill.Mill(conf.Reader, conf.Name)
	if err != nil {
		return nil, nil, err
	}

	fileInfo := &storage.FileInfo{
		Mill:             mill.ID(),
		Checksum:         variantChecksum,
		Source:           conf.checksum,
		Opts:             opts,
		Media:            conf.Media,
		Name:             conf.Name,
		LastModifiedDate: conf.LastModifiedDate,
		Added:            time.Now().Unix(),
		Meta:             pbtypes.ToStruct(res.Meta),
		Size_:            int64(readerWithCounter.Count()),
	}

	key, err := symmetric.NewRandom()
	if err != nil {
		return nil, nil, err
	}
	encryptor := cfb.New(key, [aes.BlockSize]byte{})

	contentReader, err := encryptor.EncryptReader(res.File)
	if err != nil {
		return nil, nil, err
	}

	fileInfo.Key = key.String()
	fileInfo.EncMode = storage.FileInfo_AES_CFB

	contentNode, err := s.addFileData(ctx, spaceID, contentReader)
	if err != nil {
		return nil, nil, err
	}

	fileInfo.Hash = contentNode.Cid().String()
	rawMeta, err := proto.Marshal(fileInfo)
	if err != nil {
		return nil, nil, err
	}

	metaReader, err := encryptor.EncryptReader(bytes.NewReader(rawMeta))
	if err != nil {
		return nil, nil, err
	}

	metaNode, err := s.addFileData(ctx, spaceID, metaReader)
	if err != nil {
		return nil, nil, err
	}
	fileInfo.MetaHash = metaNode.Cid().String()

	err = s.fileStore.AddFileVariant(fileInfo)
	if err != nil {
		return nil, nil, err
	}

	pairNode, err := s.addFilePairNode(ctx, spaceID, fileInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("add file pair node: %w", err)
	}
	return fileInfo, pairNode, nil
}

// addFilePairNode has structure:
/*
- dir (pair)
	- meta
	- content
*/
func (s *service) addFilePairNode(ctx context.Context, spaceID string, file *storage.FileInfo) (ipld.Node, error) {
	dagService := s.dagServiceForSpace(spaceID)
	pair := uio.NewDirectory(dagService)
	pair.SetCidBuilder(cidBuilder)

	if file.MetaHash == "" {
		return nil, fmt.Errorf("metaHash is empty")
	}

	err := helpers.AddLinkToDirectory(ctx, dagService, pair, MetaLinkName, file.MetaHash)
	if err != nil {
		return nil, fmt.Errorf("add meta link: %w", err)
	}
	err = helpers.AddLinkToDirectory(ctx, dagService, pair, ContentLinkName, file.Hash)
	if err != nil {
		return nil, fmt.Errorf("add content link: %w", err)
	}

	pairNode, err := pair.GetNode()
	if err != nil {
		return nil, err
	}
	err = dagService.Add(ctx, pairNode)
	if err != nil {
		return nil, err
	}
	return pairNode, nil
}

type dirEntry struct {
	name     string
	fileInfo *storage.FileInfo
	fileNode ipld.Node
}

func (s *service) fileIndexInfo(ctx context.Context, id domain.FullFileId, updateIfExists bool) ([]*storage.FileInfo, error) {
	dagService := s.dagServiceForSpace(id.SpaceId)
	dirLinks, err := helpers.LinksAtCid(ctx, dagService, id.FileId.String())
	if err != nil {
		return nil, err
	}
	dirNode, dirLink, err := s.getInnerDirNode(ctx, dagService, dirLinks)
	if err != nil {
		return nil, fmt.Errorf("get inner dir node: %w", err)
	}

	// File keys should be available at this moment
	keys, err := s.fileStore.GetFileKeys(id.FileId)
	if err != nil {
		// no keys means file is not encrypted or keys are missing
		log.Debugf("failed to get file keys from filestore %s: %s", id.FileId.String(), err)
	}

	var files []*storage.FileInfo
	if looksLikeFileNode(dirNode) {
		var key string
		if keys != nil {
			key = keys[encryptionKeyPath(fileLinkName)]
		}

		fileIndex, err := s.fileInfoFromPath(ctx, id.SpaceId, id.FileId, id.FileId.String()+"/"+dirLink.Name, key)
		if err != nil {
			return nil, fmt.Errorf("fileInfoFromPath error: %w", err)
		}
		files = append(files, fileIndex)
	} else {
		for _, link := range dirNode.Links() {
			var key string
			if keys != nil {
				key = keys[encryptionKeyPath(link.Name)]
			}

			fileIndex, err := s.fileInfoFromPath(ctx, id.SpaceId, id.FileId, id.FileId.String()+"/"+dirLink.Name+"/"+link.Name, key)
			if err != nil {
				return nil, fmt.Errorf("fileInfoFromPath error: %w", err)
			}
			files = append(files, fileIndex)
		}
	}

	err = s.fileStore.AddFileVariants(updateIfExists, files...)
	if err != nil {
		return nil, fmt.Errorf("failed to add files to store: %w", err)
	}

	return files, nil
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

func (s *service) FileByHash(ctx context.Context, id domain.FullFileId) (File, error) {
	fileList, err := s.fileStore.ListFileVariants(id.FileId)
	if err != nil {
		return nil, err
	}

	if len(fileList) == 0 || fileList[0].MetaHash == "" {
		// info from ipfs
		fileList, err = s.fileIndexInfo(ctx, id, false)
		if err != nil {
			log.With("fileId", id.FileId.String()).Errorf("FileByHash: failed to retrieve from IPFS: %s", err)
			return nil, domain.ErrFileNotFound
		}
		ok, err := s.fileStore.IsFileImported(id.FileId)
		if err != nil {
			return nil, fmt.Errorf("check if file is imported: %w", err)
		}
		if ok {
			log.With("fileId", id.FileId.String()).Warn("file is imported, push it to uploading queue")
			// If file is imported we have to sync it, so we don't set sync status to synced
			err = s.fileStore.SetIsFileImported(id.FileId, false)
			if err != nil {
				return nil, fmt.Errorf("set is file imported: %w", err)
			}
		}
	}
	fileIndex := fileList[0]
	return &file{
		spaceID: id.SpaceId,
		fileId:  id.FileId,
		info:    fileIndex,
		node:    s,
	}, nil
}

func encryptionKeyPath(linkName string) string {
	if linkName == fileLinkName {
		return "/0/"
	}
	return "/0/" + linkName + "/"
}

func (s *service) getInnerDirNode(ctx context.Context, dagService ipld.DAGService, outerDirLinks []*ipld.Link) (ipld.Node, *ipld.Link, error) {
	if len(outerDirLinks) == 0 {
		return nil, nil, errors.New("no files in directory node")
	}
	dirLink := outerDirLinks[0]
	node, err := helpers.NodeAtLink(ctx, dagService, dirLink)
	return node, dirLink, err
}

func (s *service) lockAddOperation(checksum string) *sync.Mutex {
	s.lock.Lock()
	opLock, ok := s.addOperationLocks[checksum]
	if !ok {
		opLock = &sync.Mutex{}
		s.addOperationLocks[checksum] = opLock
	}
	s.lock.Unlock()

	opLock.Lock()
	return opLock
}
