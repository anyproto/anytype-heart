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
	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/cfb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	m "github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CName = "files"
)

var log = logging.Logger("anytype-files")

var _ Service = (*service)(nil)

type Service interface {
	// only in uploader
	FileAdd(ctx context.Context, spaceID string, options ...AddOption) (*AddResult, error)
	// buildDetails (fileindex.go), gateway, export, DownloadFile
	FileByHash(ctx context.Context, id domain.FullFileId) (File, error)
	FileFromInfos(fileId domain.FullFileId, infos []*storage.FileInfo) File
	FileGetKeys(id domain.FileId) (*domain.FileEncryptionKeys, error)
	GetSpaceUsage(ctx context.Context, spaceID string) (*pb.RpcFileSpaceUsageResponseUsage, error)
	GetNodeUsage(ctx context.Context) (*NodeUsageResponse, error)
	// only in uploader
	ImageAdd(ctx context.Context, spaceID string, options ...AddOption) (*AddResult, error)
	// buildDetails (fileindex.go), gateway, export, DownloadFile, html converter (clipboard)
	ImageByHash(ctx context.Context, id domain.FullFileId) (Image, error)
	ImageFromInfos(fileId domain.FullFileId, infos []*storage.FileInfo) Image

	IndexFile(ctx context.Context, fileId domain.FullFileId, details *types.Struct) ([]*storage.FileInfo, error)

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
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)

	s.dagService = s.commonFile.DAGService()
	s.fileStorage = app.MustComponent[filestorage.FileStorage](a)
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

	rootNode, keys, err := s.addFileRootNode(ctx, spaceId, addNodeResult.variant, addNodeResult.filePairNode)
	if err != nil {
		addLock.Unlock()
		return nil, err
	}
	fileId := domain.FileId(rootNode.Cid().String())

	addNodeResult.variant.Targets = []string{fileId.String()}

	fileKeys := domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: keys.KeysByPath,
	}
	err = s.fileStore.AddFileKeys(fileKeys)
	if err != nil {
		addLock.Unlock()
		return nil, fmt.Errorf("failed to save file keys: %w", err)
	}

	return &AddResult{
		FileId:         fileId,
		EncryptionKeys: &fileKeys,
		Size:           addNodeResult.variant.Size_,
		MIME:           opts.Media,
		lock:           addLock,
	}, nil
}

func (s *service) newExistingFileResult(lock *sync.Mutex, fileId domain.FileId, variants []*storage.FileInfo) (*AddResult, error) {
	keys, err := s.FileGetKeys(fileId)
	if err != nil {
		return nil, fmt.Errorf("get keys: %w", err)
	}
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
		IsExisting:     true,
		FileId:         fileId,
		EncryptionKeys: keys,
		MIME:           variant.GetMedia(),
		Size:           variant.GetSize_(),
		lock:           lock,
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
func (s *service) addFileRootNode(ctx context.Context, spaceID string, fileInfo *storage.FileInfo, fileNode ipld.Node) (ipld.Node, *storage.FileKeys, error) {
	dagService := s.dagServiceForSpace(spaceID)
	keys := &storage.FileKeys{KeysByPath: make(map[string]string)}
	outer := uio.NewDirectory(dagService)
	outer.SetCidBuilder(cidBuilder)

	err := helpers.AddLinkToDirectory(ctx, dagService, outer, schema.LinkFile, fileNode.Cid().String())
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

func (s *service) FileGetKeys(id domain.FileId) (*domain.FileEncryptionKeys, error) {
	keys, err := s.fileStore.GetFileKeys(id)
	if err != nil {
		return nil, err
	}
	return &domain.FileEncryptionKeys{
		FileId:         id,
		EncryptionKeys: keys,
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

func (s *service) getContentReader(ctx context.Context, spaceID string, rawCid string, encKey string) (symmetric.ReadSeekCloser, error) {
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

type addFileNodeResult struct {
	isExisting       bool
	existingVariants []*storage.FileInfo

	fileId  domain.FileId
	variant *storage.FileInfo
	// filePairNode is the root node for meta + content file nodes
	filePairNode ipld.Node
}

func newExistingFileResult(fileId domain.FileId, variants []*storage.FileInfo) (*addFileNodeResult, error) {
	return &addFileNodeResult{
		isExisting:       true,
		existingVariants: variants,
		fileId:           fileId,
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

	if existingFileId, variants, err := s.getFileVariantBySourceChecksum(mill.ID(), conf.checksum, opts); err == nil {
		existingRes, err := newExistingFileResult(existingFileId, variants)
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

	if existingFileId, variant, variants, err := s.getFileVariantByChecksum(mill.ID(), variantChecksum); err == nil {
		if variant.Source == conf.checksum {
			// we may have same variant checksum for different files
			// e.g. empty image exif with the same resolution
			// reuse the whole file only in case the checksum of the original file is the same
			existingRes, err := newExistingFileResult(existingFileId, variants)
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
	fileInfo.EncMode = storage.FileInfo_AES_CFB

	contentNode, err := s.addFileData(ctx, spaceID, contentReader)
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

	metaNode, err := s.addFileData(ctx, spaceID, metaReader)
	if err != nil {
		return nil, err
	}
	fileInfo.MetaHash = metaNode.Cid().String()

	pairNode, err := s.addFilePairNode(ctx, spaceID, fileInfo)
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
		path := encryptionKeyPath(schema.LinkFile)
		var key string
		if keys != nil {
			key = keys[path]
		}

		fileIndex, err := s.fileInfoFromPath(ctx, id.SpaceId, id.FileId, id.FileId.String()+"/"+dirLink.Name, key)
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

			fileIndex, err := s.fileInfoFromPath(ctx, id.SpaceId, id.FileId, id.FileId.String()+"/"+dirLink.Name+"/"+link.Name, key)
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

func getEncryptorDecryptor(key symmetric.Key) (symmetric.EncryptorDecryptor, error) {
	return cfb.New(key, [aes.BlockSize]byte{}), nil
}

func (s *service) IndexFile(ctx context.Context, id domain.FullFileId, details *types.Struct) ([]*storage.FileInfo, error) {
	variantsList := pbtypes.GetStringList(details, bundle.RelationKeyFileVariantIds.String())
	if true || len(variantsList) == 0 {
		// info from ipfs
		fileList, err := s.fileIndexInfo(ctx, id, false)
		if err != nil {
			return nil, err
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
		return fileList, nil
	}
	return nil, nil
}

// TODO SHould be accesed via OBJECT
func (s *service) FileByHash(ctx context.Context, id domain.FullFileId) (File, error) {
	recs, err := s.objectStore.SpaceIndex(id.SpaceId).Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(id.FileId.String()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query details: %w", err)
	}

	if len(recs) == 0 {
		return nil, fmt.Errorf("noooooo")
	}
	fileRec := recs[0]

	return s.fileFromDetails(fileRec.Details)
}

func (s *service) fileFromDetails(details *types.Struct) (File, error) {
	variantsList := pbtypes.GetStringList(details, bundle.RelationKeyFileVariantIds.String())
	if len(variantsList) == 0 {
		return nil, fmt.Errorf("not indexed")
	}

	infos := getFileInfosFromDetails(details)
	return &file{
		spaceID: pbtypes.GetString(details, bundle.RelationKeySpaceId.String()),
		fileId:  domain.FileId(pbtypes.GetString(details, bundle.RelationKeyFileId.String())),
		info:    infos[0],
		node:    s,
	}, nil
}

func (s *service) FileFromInfos(id domain.FullFileId, infos []*storage.FileInfo) File {
	return &file{
		spaceID: id.SpaceId,
		fileId:  id.FileId,
		info:    infos[0],
		node:    s,
	}
}

func getFileInfosFromDetails(details *types.Struct) []*storage.FileInfo {
	variantsList := pbtypes.GetStringList(details, bundle.RelationKeyFileVariantIds.String())
	sourceChecksum := pbtypes.GetString(details, bundle.RelationKeyFileSourceChecksum.String())
	infos := make([]*storage.FileInfo, 0, len(variantsList))
	for i, variantId := range variantsList {
		var meta *types.Struct
		widths := pbtypes.GetIntList(details, bundle.RelationKeyFileVariantWidths.String())
		if widths[i] > 0 {
			meta = &types.Struct{
				Fields: map[string]*types.Value{
					"width": pbtypes.Int64(int64(widths[i])),
				},
			}
		}
		info := &storage.FileInfo{
			Name:   pbtypes.GetString(details, bundle.RelationKeyName.String()),
			Size_:  pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()),
			Source: sourceChecksum,
			Media:  pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()),

			Hash:     variantId,
			Checksum: pbtypes.GetStringList(details, bundle.RelationKeyFileVariantChecksums.String())[i],
			Mill:     pbtypes.GetStringList(details, bundle.RelationKeyFileVariantMills.String())[i],
			Meta:     meta,
			Key:      pbtypes.GetStringList(details, bundle.RelationKeyFileVariantKeys.String())[i],
			Opts:     pbtypes.GetStringList(details, bundle.RelationKeyFileVariantOptions.String())[i],
		}
		infos = append(infos, info)
	}
	return infos
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
