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
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/cfb"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	m "github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName = "files"
)

var (
	log                        = logging.Logger("anytype-files")
	FailedProtoUnmarshallError = errors.New("failed proto unmarshall")
)

var _ Service = (*service)(nil)

type Service interface {
	FileAdd(ctx context.Context, spaceID string, options ...AddOption) (*AddResult, error)
	ImageAdd(ctx context.Context, spaceID string, options ...AddOption) (*AddResult, error)

	// GetFileVariants get file information from DAG. If file is not available locally, it fetches data from remote peer (file node or p2p peer)
	GetFileVariants(ctx context.Context, fileId domain.FullFileId, keys map[string]string) ([]*storage.FileInfo, error)
	GetContentReader(ctx context.Context, spaceID string, rawCid string, encKey string) (symmetric.ReadSeekCloser, error)

	app.Component
}

type service struct {
	commonFile fileservice.FileService

	dagService ipld.DAGService

	objectStore objectstore.ObjectStore

	lock              sync.Mutex
	addOperationLocks map[string]*sync.Mutex

	// Batch registry for preloading
	batchMu sync.RWMutex
}

func New() Service {
	return &service{
		addOperationLocks: make(map[string]*sync.Mutex),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)

	s.dagService = s.commonFile.DAGService()
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

const MetaLinkName = "meta"
const ContentLinkName = "content"

var ValidMetaLinkNames = []string{"meta"}
var ValidContentLinkNames = []string{"content"}

var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

type AddResult struct {
	FileId         domain.FileId
	EncryptionKeys *domain.FileEncryptionKeys
	IsExisting     bool // Is file already added by user?
	Variants       []*storage.FileInfo

	LastModifiedAt int64
	AddedAt        int64
	MIME           string
	Size           int64

	// Batch is set when file was uploaded with a batch (for preloading)
	Batch filestorage.Batch

	lock *sync.Mutex
}

// Commit transaction of adding a file
func (r *AddResult) Commit() {
	if r.Batch != nil {
		r.Batch.Commit()
	}
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

	addNodeResult, err := s.addFileNode(ctx, spaceId, &m.Blob{}, opts, schema.LinkFile)
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	if addNodeResult.isExisting {
		res, err := s.newExistingFileResult(addLock, addNodeResult.fileId, addNodeResult.existingVariants)
		if err != nil {
			addLock.Unlock()
			return nil, err
		}
		return res, nil
	}
	rootNode, keys, err := s.addFileRootNode(ctx, spaceId, addNodeResult.variant, addNodeResult.filePairNode, opts)
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	fileId := domain.FileId(rootNode.Cid().String())

	fileKeys := domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: keys.KeysByPath,
	}
	err = s.objectStore.AddFileKeys(fileKeys)
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	return &AddResult{
		FileId:         fileId,
		EncryptionKeys: &fileKeys,
		Variants:       []*storage.FileInfo{addNodeResult.variant},
		Size:           addNodeResult.variant.Size_,
		MIME:           opts.Media,
		lock:           addLock,
	}, nil
}

func (s *service) newExistingFileResult(lock *sync.Mutex, fileId domain.FileId, variants []*storage.FileInfo) (*AddResult, error) {
	if len(variants) == 0 {
		return nil, fmt.Errorf("variants not found")
	}
	var variant *storage.FileInfo
	// Find largest variant
	for _, v := range variants {
		if variant == nil {
			variant = v
		} else if variant.Size_ < v.Size_ {
			variant = v
		}
	}

	return &AddResult{
		IsExisting: true,
		FileId:     fileId,
		EncryptionKeys: &domain.FileEncryptionKeys{
			FileId:         fileId,
			EncryptionKeys: collectKeysFromVariants(variants),
		},
		Variants: variants,
		MIME:     variant.GetMedia(),
		Size:     variant.GetSize_(),
		lock:     lock,
	}, nil

}

// addFileRootNode has structure:
/*
- dir (outer)
	- dir (file)
		- meta
		- content
	...
*/
func (s *service) addFileRootNode(ctx context.Context, spaceID string, fileInfo *storage.FileInfo, fileNode ipld.Node, opts AddOptions) (ipld.Node, *storage.FileKeys, error) {
	dagService := s.dagServiceForSpace(spaceID, opts.FileHandler)
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer, err := uio.NewDirectory(dagService)
	if err != nil {
		return nil, nil, err
	}
	outer.SetCidBuilder(cidBuilder)

	err = helpers.AddLinkToDirectory(ctx, dagService, outer, schema.LinkFile, fileNode.Cid().String())
	if err != nil {
		return nil, nil, err
	}
	keys.KeysByPath[encryptionKeyPath(schema.LinkFile)] = fileInfo.Key

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

func (s *service) fileInfoFromPath(ctx context.Context, spaceId string, path string, key string) (*storage.FileInfo, error) {
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
		ed, err := getEncryptorDecryptor(key)
		if err != nil {
			return nil, err
		}
		decryptedReader, err := ed.DecryptReader(r)
		if err != nil {
			return nil, err
		}
		b, err := ioutil.ReadAll(decryptedReader)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal file info proto with all encryption modes: %w", err)

		}
		err = proto.Unmarshal(b, &file)
		if err != nil || file.Hash == "" {
			return nil, fmt.Errorf("failed to unmarshal file info proto with all encryption modes: %w", err)
		}
	} else {
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		err = proto.Unmarshal(b, &file)
		if err != nil || file.Hash == "" {
			return nil, fmt.Errorf("failed to unmarshal not-encrypted file info: %w", FailedProtoUnmarshallError)
		}
	}

	if file.Hash == "" {
		return nil, fmt.Errorf("failed to read file info proto with all encryption modes")
	}
	file.MetaHash = id.String()
	return &file, nil
}

func getEncryptorDecryptor(key symmetric.Key) (symmetric.EncryptorDecryptor, error) {
	return cfb.New(key, [aes.BlockSize]byte{}), nil
}

type addFileNodeResult struct {
	isExisting       bool
	existingVariants []*storage.FileInfo

	fileId  domain.FileId
	variant *storage.FileInfo
	// filePairNode is the root node for meta + content file nodes
	filePairNode ipld.Node
}

func newExistingFileResult(file *existingFile) (*addFileNodeResult, error) {
	return &addFileNodeResult{
		isExisting:       true,
		existingVariants: file.variants,
		fileId:           file.fileId,
	}, nil
}

func newAddedFileResult(variant *storage.FileInfo, fileNode ipld.Node) (*addFileNodeResult, error) {
	return &addFileNodeResult{
		variant:      variant,
		filePairNode: fileNode,
	}, nil
}

// addFileNode adds a file node to the DAG. This node has structure:
/*
- dir (file pair):
	- meta
	- content
*/
func (s *service) addFileNode(ctx context.Context, spaceID string, mill m.Mill, conf AddOptions, linkName string) (*addFileNodeResult, error) {
	opts, err := mill.Options(map[string]interface{}{
		"plaintext": false,
	})
	if err != nil {
		return nil, err
	}

	if existingFile, err := s.getFileVariantBySourceChecksum(mill.ID(), conf.checksum, opts); err == nil {
		existingRes, err := newExistingFileResult(existingFile)
		if err == nil {
			return existingRes, nil
		}
	}

	res, err := mill.Mill(conf.Reader, conf.Name)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", m.ErrProcessing, err)
	}

	// count the result size after the applied mill
	readerWithCounter := datacounter.NewReaderCounter(res.File)
	variantChecksum, err := checksum(readerWithCounter, false)
	if err != nil {
		return nil, err
	}

	if existingFile, variant, err := s.getFileVariantByChecksum(mill.ID(), variantChecksum); err == nil {
		if variant.Source == conf.checksum {
			// we may have same variant checksum for different files
			// e.g. empty image exif with the same resolution
			// reuse the whole file only in case the checksum of the original file is the same
			existingRes, err := newExistingFileResult(existingFile)
			if err == nil {
				return existingRes, nil
			}
		}
	}

	_, err = conf.Reader.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	_, err = res.File.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	fileInfo := &storage.FileInfo{
		Path:             encryptionKeyPath(linkName),
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

	key, err := getOrGenerateSymmetricKey(linkName, conf)
	if err != nil {
		return nil, err
	}
	encryptor := cfb.New(key, [aes.BlockSize]byte{})

	contentReader, err := encryptor.EncryptReader(res.File)
	if err != nil {
		return nil, err
	}

	fileInfo.Key = key.String()

	contentNode, err := s.addFileData(ctx, spaceID, contentReader, conf.FileHandler)
	if err != nil {
		return nil, err
	}

	fileInfo.Hash = contentNode.Cid().String()
	rawMeta, err := proto.Marshal(fileInfo)
	if err != nil {
		return nil, err
	}

	metaReader, err := encryptor.EncryptReader(bytes.NewReader(rawMeta))
	if err != nil {
		return nil, err
	}

	metaNode, err := s.addFileData(ctx, spaceID, metaReader, conf.FileHandler)
	if err != nil {
		return nil, err
	}
	fileInfo.MetaHash = metaNode.Cid().String()

	dagService := s.dagServiceForSpace(spaceID, conf.FileHandler)
	pairNode, err := s.addFilePairNode(ctx, dagService, spaceID, fileInfo, conf)
	if err != nil {
		return nil, err
	}
	err = res.File.Close()
	if err != nil {
		log.Warnf("failed to close file: %s", err)
	}
	if err != nil {
		return nil, fmt.Errorf("add file pair node: %w", err)
	}
	return newAddedFileResult(fileInfo, pairNode)
}

func getOrGenerateSymmetricKey(linkName string, opts AddOptions) (symmetric.Key, error) {
	if key, exists := opts.CustomEncryptionKeys[encryptionKeyPath(linkName)]; exists {
		symKey, err := symmetric.FromString(key)
		if err == nil {
			return symKey, nil
		}
		return symmetric.NewRandom()
	}
	return symmetric.NewRandom()
}

// addFilePairNode has structure:
/*
- dir (pair)
	- meta
	- content
*/
func (s *service) addFilePairNode(ctx context.Context, dagService ipld.DAGService, spaceID string, file *storage.FileInfo, opts AddOptions) (ipld.Node, error) {
	pair, err := uio.NewDirectory(dagService)
	if err != nil {
		return nil, err
	}
	pair.SetCidBuilder(cidBuilder)

	if file.MetaHash == "" {
		return nil, fmt.Errorf("metaHash is empty")
	}

	err = helpers.AddLinkToDirectory(ctx, dagService, pair, MetaLinkName, file.MetaHash)
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

func (s *service) GetFileVariants(ctx context.Context, id domain.FullFileId, keys map[string]string) ([]*storage.FileInfo, error) {
	dagService := s.dagServiceForSpace(id.SpaceId, nil)
	dirLinks, err := helpers.LinksAtCid(ctx, dagService, id.FileId.String())
	if err != nil {
		return nil, err
	}
	dirNode, dirLink, err := s.getInnerDirNode(ctx, dagService, dirLinks)
	if err != nil {
		return nil, fmt.Errorf("get inner dir node: %w", err)
	}

	var files []*storage.FileInfo
	if looksLikeFileNode(dirNode) {
		path := encryptionKeyPath(schema.LinkFile)
		var key string
		if keys != nil {
			key = keys[path]
		}

		fileIndex, err := s.fileInfoFromPath(ctx, id.SpaceId, id.FileId.String()+"/"+dirLink.Name, key)
		if err != nil {
			return nil, fmt.Errorf("fileInfoFromPath error: %w", err)
		}
		fileIndex.Path = path
		files = append(files, fileIndex)
	} else {
		for _, link := range dirNode.Links() {
			path := encryptionKeyPath(link.Name)
			var key string
			if keys != nil {
				key = keys[path]
			}

			fileIndex, err := s.fileInfoFromPath(ctx, id.SpaceId, id.FileId.String()+"/"+dirLink.Name+"/"+link.Name, key)
			if err != nil {
				return nil, fmt.Errorf("fileInfoFromPath error: %w", err)
			}
			fileIndex.Path = path
			files = append(files, fileIndex)
		}
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

func encryptionKeyPath(linkName string) string {
	if linkName == schema.LinkFile {
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

func (s *service) GetContentReader(ctx context.Context, spaceID string, rawCid string, encKey string) (symmetric.ReadSeekCloser, error) {
	fileCid, err := cid.Parse(rawCid)
	if err != nil {
		return nil, err
	}
	fd, err := s.getFile(ctx, spaceID, fileCid)
	if err != nil {
		return nil, err
	}
	if encKey == "" {
		return fd, nil
	}

	key, err := symmetric.FromString(encKey)
	if err != nil {
		return nil, err
	}

	dec, err := getEncryptorDecryptor(key)
	if err != nil {
		return nil, err
	}

	return dec.DecryptReader(fd)
}
