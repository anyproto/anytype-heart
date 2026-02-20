package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

const (
	addFileWorkersCount = 4
)

var log = logging.Logger("anytype-mw-smartfile")

func NewFile(sb smartblock.SmartBlock, blockService BlockService, picker cache.ObjectGetter, processService process.Service, fileUploaderFactory fileuploader.Service, objectCreator ObjectCreator, collection collection.Collection, objectStore spaceindex.Store) File {
	return &sfile{
		SmartBlock:          sb,
		blockService:        blockService,
		picker:              picker,
		processService:      processService,
		fileUploaderFactory: fileUploaderFactory,
		objectCreator:       objectCreator,
		collection:          collection,
		objectStore:         objectStore,
	}
}

type BlockService interface {
	CreateLinkToTheNewObject(ctx context.Context, sctx session.Context, req *pb.RpcBlockLinkCreateWithObjectRequest) (linkID string, pageID string, details *domain.Details, err error)
}

type ObjectCreator interface {
	CreateObject(ctx context.Context, spaceID string, req objectcreator.CreateObjectRequest) (id string, details *domain.Details, err error)
}

type File interface {
	DropFiles(req pb.RpcFileDropRequest) (err error)
	Upload(ctx session.Context, id string, source FileSource, isSync bool) (fileObjectId string, err error)
	UploadState(ctx session.Context, s *state.State, id string, source FileSource, isSync bool) (err error)
	UpdateFile(id, groupId string, apply func(b file.Block) error) (err error)
	CreateAndUpload(ctx session.Context, req pb.RpcBlockFileCreateAndUploadRequest) (string, error)
	SetFileStyle(ctx session.Context, style model.BlockContentFileStyle, blockIds ...string) (err error)
	SetFileTargetObjectId(ctx session.Context, blockId, targetObjectId string) error
	dropFilesHandler // do not remove, used in downcasts
}

type FileSource struct {
	Path                string
	Url                 string // nolint:revive
	Bytes               []byte
	Name                string
	GroupID             string
	Origin              objectorigin.ObjectOrigin
	ImageKind           model.ImageKind
	CreatedInContext    string
	CreatedInContextRef string
}

type sfile struct {
	smartblock.SmartBlock

	// collection could be nil if object doesn't have collection component
	collection collection.Collection

	blockService        BlockService
	picker              cache.ObjectGetter
	processService      process.Service
	fileUploaderFactory fileuploader.Service
	objectCreator       ObjectCreator
	objectStore         spaceindex.Store
}

func (sf *sfile) Upload(ctx session.Context, blockId string, source FileSource, isSync bool) (fileObjectId string, err error) {
	if source.GroupID == "" {
		source.GroupID = bson.NewObjectId().Hex()
	}
	s := sf.NewStateCtx(ctx).SetGroupId(source.GroupID)
	res := sf.upload(s, blockId, source, isSync)
	if res.Err != nil {
		return "", res.Err
	}
	return res.FileObjectId, sf.Apply(s)
}

func (sf *sfile) UploadState(_ session.Context, s *state.State, id string, source FileSource, isSync bool) (err error) {
	if res := sf.upload(s, id, source, isSync); res.Err != nil {
		return res.Err
	}
	return
}

func (sf *sfile) SetFileStyle(ctx session.Context, style model.BlockContentFileStyle, blockIds ...string) (err error) {
	s := sf.NewStateCtx(ctx)
	for _, id := range blockIds {
		b := s.Get(id)
		if b == nil {
			return smartblock.ErrSimpleBlockNotFound
		}

		if rel, ok := b.(file.Block); ok {
			rel.SetStyle(style)
		} else {
			return fmt.Errorf("unexpected block type: %T (want file)", b)
		}

	}

	return sf.Apply(s)
}

func (sf *sfile) SetFileTargetObjectId(ctx session.Context, blockId, targetObjectId string) error {
	sb, err := sf.picker.GetObject(context.TODO(), targetObjectId)
	if err != nil {
		return err
	}
	var blockContentFileType model.BlockContentFileType
	//nolint:gosec
	switch model.ObjectTypeLayout(sb.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout)) {
	case model.ObjectType_image:
		blockContentFileType = model.BlockContentFile_Image
	case model.ObjectType_audio:
		blockContentFileType = model.BlockContentFile_Audio
	case model.ObjectType_video:
		blockContentFileType = model.BlockContentFile_Video
	case model.ObjectType_pdf:
		blockContentFileType = model.BlockContentFile_PDF
	default:
		blockContentFileType = model.BlockContentFile_File
	}

	return sf.updateFile(ctx, blockId, "", func(b file.Block) error {
		b.SetTargetObjectId(targetObjectId)
		b.SetStyle(model.BlockContentFile_Embed)
		b.SetState(model.BlockContentFile_Done)
		b.SetType(blockContentFileType)
		return nil
	})
}

func (sf *sfile) CreateAndUpload(ctx session.Context, req pb.RpcBlockFileCreateAndUploadRequest) (newId string, err error) {
	s := sf.NewStateCtx(ctx)
	nb := simple.New(&model.Block{
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Type: req.FileType,
			},
		},
	})
	s.Add(nb)
	newId = nb.Model().Id
	if err = s.InsertTo(req.TargetId, req.Position, newId); err != nil {
		return
	}
	if err = sf.upload(s, newId, FileSource{
		Path:                req.LocalPath,
		Url:                 req.Url,
		ImageKind:           req.ImageKind,
		CreatedInContext:    req.ContextId,
		CreatedInContextRef: newId,
	}, false).Err; err != nil {
		return
	}
	if err = sf.Apply(s); err != nil {
		return
	}
	return
}

func (sf *sfile) upload(s *state.State, id string, source FileSource, isSync bool) (res fileuploader.UploadResult) {
	ctx := context.Background()
	b := s.Get(id)
	f, ok := b.(file.Block)
	if !ok {
		return fileuploader.UploadResult{Err: fmt.Errorf("not a file block")}
	}
	upl := sf.newUploader(source.Origin).SetBlock(f)
	if source.ImageKind != model.ImageKind_Basic {
		upl.SetImageKind(source.ImageKind)
	}
	if source.Path != "" {
		upl.SetFile(source.Path)
	} else if source.Url != "" {
		upl.SetUrl(source.Url).
			SetLastModifiedDate()
	} else if len(source.Bytes) > 0 {
		upl.SetBytes(source.Bytes).
			SetName(source.Name).
			SetLastModifiedDate()
	}

	// Set creation context if provided
	if source.CreatedInContext != "" {
		upl.SetCreatedInContext(source.CreatedInContext)
	}
	if source.CreatedInContextRef != "" {
		upl.SetCreatedInContextRef(source.CreatedInContextRef)
	}

	if isSync {
		return upl.Upload(ctx)
	} else {
		upl.SetGroupId(s.GroupId()).AsyncUpdates(sf.Id()).UploadAsync(ctx)
	}
	return
}

func (sf *sfile) newUploader(origin objectorigin.ObjectOrigin) fileuploader.Uploader {
	return sf.fileUploaderFactory.NewUploader(sf.SpaceID(), origin)
}

func (sf *sfile) UpdateFile(id, groupId string, apply func(b file.Block) error) (err error) {
	return sf.updateFile(nil, id, groupId, apply)
}

func (sf *sfile) updateFile(ctx session.Context, id, groupId string, apply func(b file.Block) error) (err error) {
	s := sf.NewStateCtx(ctx).SetGroupId(groupId)
	b := s.Get(id)
	f, ok := b.(file.Block)
	if !ok {
		return fmt.Errorf("not a file block")
	}
	if err = apply(f); err != nil {
		return
	}
	return sf.Apply(s)
}

func (sf *sfile) DropFiles(req pb.RpcFileDropRequest) (err error) {
	if !isCollection(sf) {
		if err = sf.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
			return err
		}
	}
	proc := &dropFilesProcess{
		spaceID:             sf.SpaceID(),
		processService:      sf.processService,
		picker:              sf.picker,
		fileUploaderFactory: sf.fileUploaderFactory,
		objectCreator:       sf.objectCreator,
		objectStore:         sf.objectStore,
		contextId:           req.ContextId,
	}
	if err = proc.Init(req.LocalFilePaths); err != nil {
		return
	}
	var ch = make(chan error)
	go proc.Start(sf, req, ch)
	err = <-ch
	return
}

func (sf *sfile) dropFilesCreateStructure(groupId, targetId string, pos model.BlockPosition, style model.BlockContentFileStyle, entries []*dropFileEntry) (blockIds []string, err error) {
	s := sf.NewState().SetGroupId(groupId)
	for _, entry := range entries {
		fb := simple.New(&model.Block{Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:  entry.name,
				Style: style,
			},
		}})
		blockId := fb.Model().Id
		fb.(file.Block).SetState(model.BlockContentFile_Uploading)
		s.Add(fb)
		if err = s.InsertTo(targetId, pos, blockId); err != nil {
			return
		}
		targetId = blockId
		pos = model.Block_Bottom
		blockIds = append(blockIds, blockId)
	}
	if err = sf.Apply(s); err != nil {
		return
	}
	return
}

func (sf *sfile) dropFilesSetInfo(info dropFileInfo) (err error) {
	if errors.Is(info.err, context.Canceled) {
		s := sf.NewState().SetGroupId(info.groupId)
		s.Unlink(info.blockId)
		return sf.Apply(s)
	}
	if info.err != nil {
		return fmt.Errorf("drop file: %w", info.err)
	}
	if isCollection(sf) {
		if info.file == nil {
			return fmt.Errorf("file block is nil")
		}
		if sf.collection != nil {
			err = appendToCollection(sf.collection, info.file.TargetObjectId)
			if err != nil {
				return fmt.Errorf("append to collection: %w", err)
			}
		} else {
			return fmt.Errorf("collection component not found")
		}
	}
	return sf.UpdateFile(info.blockId, info.groupId, func(f file.Block) error {
		if info.err != nil || info.file == nil || info.file.State == model.BlockContentFile_Error {
			if info.err != nil {
				log.Warnf("upload file error: %s", info.err)
			}
			f.SetState(model.BlockContentFile_Error)
			return nil
		}
		existingStyle := f.Model().GetFile().GetStyle()
		f.SetModel(info.file)
		if existingStyle != model.BlockContentFile_Auto {
			f.SetStyle(existingStyle)
		}
		return nil
	})
}

func appendToCollection(col collection.Collection, id string) error {
	existing := col.ListIdsFromCollection()
	var afterId string
	if len(existing) > 0 {
		afterId = existing[len(existing)-1]
	}
	return col.AddToCollection(nil, &pb.RpcObjectCollectionAddRequest{
		AfterId:   afterId,
		ObjectIds: []string{id},
	})
}

func (sf *sfile) dropFilesCreateLinkedCollection(dp *dropFilesProcess, dirEntry *dropFileEntry, targetId string, pos model.BlockPosition, in chan *dropFileInfo) error {
	// Check if a collection with the same checksum already exists
	if existingId, ok := dp.findExistingCollectionByChecksum(dirEntry.checksum); ok {
		// Create a link block to the existing collection
		s := sf.NewState()
		linkBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: existingId,
					Style:         model.BlockContentLink_Page,
					IconSize:      model.BlockContentLink_SizeSmall,
				},
			},
		})
		s.Add(linkBlock)
		if err := s.InsertTo(targetId, pos, linkBlock.Model().Id); err != nil {
			return fmt.Errorf("insert link to existing collection: %w", err)
		}
		if err := sf.Apply(s); err != nil {
			return fmt.Errorf("apply link to existing collection: %w", err)
		}
		atomic.AddInt64(&dp.done, 1)

		// Process children into the existing collection
		dp.processCollectionEntries(existingId, dirEntry.child, in)
		return nil
	}

	// Create a link block to a new collection object in the document
	detailsMap := map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:      domain.String(dirEntry.name),
		bundle.RelationKeyIconEmoji: domain.String("📁"),
		bundle.RelationKeyOrigin:    domain.Int64(objectorigin.DragAndDrop().Origin),
	}
	if dirEntry.checksum != "" {
		detailsMap[bundle.RelationKeyFileSourceChecksum] = domain.String(dirEntry.checksum)
	}

	if err := sf.Apply(sf.NewState()); err != nil {
		return fmt.Errorf("apply state before creating link: %w", err)
	}
	sf.Unlock()
	_, collectionId, _, err := sf.blockService.CreateLinkToTheNewObject(dp.ctx, nil, &pb.RpcBlockLinkCreateWithObjectRequest{
		SpaceId:             sf.SpaceID(),
		ContextId:           sf.Id(),
		ObjectTypeUniqueKey: bundle.TypeKeyCollection.URL(),
		TargetId:            targetId,
		Position:            pos,
		Details:             domain.NewDetailsFromMap(detailsMap).ToProto(),
		Block: &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					Style:    model.BlockContentLink_Page,
					IconSize: model.BlockContentLink_SizeSmall,
				},
			},
		},
	})
	sf.Lock()
	if err != nil {
		return fmt.Errorf("create linked collection: %w", err)
	}
	atomic.AddInt64(&dp.done, 1)

	// Process children into the new collection
	dp.processCollectionEntries(collectionId, dirEntry.child, in)
	return nil
}

type dropFileEntry struct {
	name     string
	path     string
	isDir    bool
	checksum string
	child    []*dropFileEntry
}

type dropFileInfo struct {
	pageId, blockId string
	path            string
	err             error
	name            string
	file            *model.BlockContentFile
	groupId         string
}

type dropFilesHandler interface {
	dropFilesCreateStructure(groupId, targetId string, pos model.BlockPosition, style model.BlockContentFileStyle, entries []*dropFileEntry) (blockIds []string, err error)
	dropFilesSetInfo(info dropFileInfo) (err error)
	dropFilesCreateLinkedCollection(dp *dropFilesProcess, dirEntry *dropFileEntry, targetId string, pos model.BlockPosition, in chan *dropFileInfo) error
	newUploader(origin objectorigin.ObjectOrigin) fileuploader.Uploader
}

type dropFilesProcess struct {
	id             string
	spaceID        string
	processService process.Service
	picker         cache.ObjectGetter
	root           *dropFileEntry
	total, done    int64
	ctx            context.Context
	ctxCancel      context.CancelFunc
	doneCh         chan struct{}
	groupId        string
	contextId      string

	fileUploaderFactory fileuploader.Service
	objectCreator       ObjectCreator
	objectStore         spaceindex.Store
}

func (dp *dropFilesProcess) Id() string {
	return dp.id
}

func (dp *dropFilesProcess) Cancel() (err error) {
	if dp.ctxCancel != nil {
		dp.ctxCancel()
	}
	return
}

func (dp *dropFilesProcess) Info() pb.ModelProcess {
	var state pb.ModelProcessState
	select {
	case <-dp.doneCh:
		state = pb.ModelProcess_Done
	default:
		state = pb.ModelProcess_Running
	}
	if dp.ctx.Err() != nil {
		state = pb.ModelProcess_Canceled
	}
	return pb.ModelProcess{
		Id:    dp.id,
		State: state,
		Progress: &pb.ModelProcessProgress{
			Total: atomic.LoadInt64(&dp.total),
			Done:  atomic.LoadInt64(&dp.done),
		},
		Message: &pb.ModelProcessMessageOfDropFiles{DropFiles: &pb.ModelProcessDropFiles{}},
	}
}

func (dp *dropFilesProcess) Done() chan struct{} {
	return dp.doneCh
}

func (dp *dropFilesProcess) Init(paths []string) (err error) {
	dp.root = &dropFileEntry{}
	for _, path := range paths {
		entry := &dropFileEntry{path: path, name: filepath.Base(path)}
		ok, e := dp.readdir(entry, true)
		if e != nil {
			return anyerror.CleanupError(err)
		}
		if ok {
			dp.root.child = append(dp.root.child, entry)
			dp.total++
		}
	}
	dp.groupId = bson.NewObjectId().Hex()
	return
}

func (dp *dropFilesProcess) readdir(entry *dropFileEntry, allowSymlinks bool) (ok bool, err error) {
	fi, err := os.Lstat(entry.path)
	if err != nil {
		return
	}
	if !fi.IsDir() {
		ok = true
		return
	}

	if !allowSymlinks && fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		return
	}
	f, err := os.Open(entry.path)
	if err != nil {
		return
	}
	entry.isDir = true
	names, err := f.Readdirnames(-1)
	if err != nil {
		f.Close()
		return
	}
	f.Close()

	for _, name := range names {
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(entry.path, name)
		chEntry := &dropFileEntry{path: path, name: name}
		ok, e := dp.readdir(chEntry, false)
		if e != nil {
			return false, e
		}
		if ok {
			entry.child = append(entry.child, chEntry)
			dp.total++
		}
	}
	entry.checksum = computeDirectoryChecksum(entry.child)
	return true, nil
}

func computeDirectoryChecksum(children []*dropFileEntry) string {
	names := make([]string, len(children))
	for i, ch := range children {
		names[i] = ch.name
	}
	sort.Strings(names)
	h := sha256.Sum256([]byte(strings.Join(names, "\x00")))
	return hex.EncodeToString(h[:])
}

func (dp *dropFilesProcess) Start(file smartblock.SmartBlock, req pb.RpcFileDropRequest, rootDone chan error) {
	dp.id = uuid.New().String()
	dp.doneCh = make(chan struct{})
	dp.ctx, dp.ctxCancel = context.WithCancel(context.Background())
	defer close(dp.doneCh)
	dp.processService.Add(dp)

	// start addFiles workers
	var wc = int(dp.total)
	var in = make(chan *dropFileInfo, wc)
	if wc > addFileWorkersCount {
		wc = addFileWorkersCount
	}
	var wg = &sync.WaitGroup{}
	wg.Add(wc)
	for i := 0; i < wc; i++ {
		go dp.addFilesWorker(wg, in)
	}

	if isCollection(file) {
		dp.handleDragAndDropInCollection(file.RootId(), dp.root.child, rootDone, in)
	} else {
		dp.handleDragAndDropInDocument(file.RootId(), req.DropTargetId, req.Position, req.Style, rootDone, in)
	}
	wg.Wait()
}

func (dp *dropFilesProcess) findExistingCollectionByChecksum(checksum string) (string, bool) {
	if checksum == "" {
		return "", false
	}
	existingIds, _, err := dp.objectStore.QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileSourceChecksum,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(checksum),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_collection)),
			},
		},
		Limit: 1,
	})
	if err != nil || len(existingIds) == 0 {
		return "", false
	}
	return existingIds[0], true
}

func (dp *dropFilesProcess) createCollectionForFolder(ctx context.Context, name, checksum string) (string, error) {
	if id, ok := dp.findExistingCollectionByChecksum(checksum); ok {
		return id, nil
	}

	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetString(bundle.RelationKeyIconEmoji, "📁")
	if checksum != "" {
		details.SetString(bundle.RelationKeyFileSourceChecksum, checksum)
	}
	objectorigin.DragAndDrop().AddToDetails(details)

	id, _, err := dp.objectCreator.CreateObject(ctx, dp.spaceID, objectcreator.CreateObjectRequest{
		Details:       details,
		ObjectTypeKey: bundle.TypeKeyCollection,
	})
	if err != nil {
		return "", fmt.Errorf("create collection for folder %q: %w", name, err)
	}
	return id, nil
}

func (dp *dropFilesProcess) addObjectToCollection(ctx context.Context, collectionId, objectId string) error {
	return cache.DoContext(dp.picker, ctx, collectionId, func(coll collection.Collection) error {
		return appendToCollection(coll, objectId)
	})
}

func (dp *dropFilesProcess) processCollectionEntries(parentCollectionId string, entries []*dropFileEntry, in chan *dropFileInfo) {
	type level struct {
		collectionId string
		entries      []*dropFileEntry
	}
	queue := []level{{collectionId: parentCollectionId, entries: entries}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, entry := range cur.entries {
			if dp.ctx.Err() != nil {
				return
			}
			if entry.isDir {
				collId, err := dp.createCollectionForFolder(dp.ctx, entry.name, entry.checksum)
				if err != nil {
					log.Warnf("create collection for folder: %v", err)
					atomic.AddInt64(&dp.done, 1)
					continue
				}
				if err := dp.addObjectToCollection(dp.ctx, cur.collectionId, collId); err != nil {
					log.Warnf("add collection to parent: %v", err)
					atomic.AddInt64(&dp.done, 1)
					continue
				}
				atomic.AddInt64(&dp.done, 1)
				queue = append(queue, level{collectionId: collId, entries: entry.child})
			} else {
				select {
				case <-dp.ctx.Done():
					return
				case in <- &dropFileInfo{
					pageId: cur.collectionId,
					path:   entry.path,
					name:   entry.name,
				}:
				}
			}
		}
	}
}

func (dp *dropFilesProcess) handleDragAndDropInCollection(rootId string, droppedFiles []*dropFileEntry, rootDone chan error, in chan *dropFileInfo) {
	close(rootDone)
	dp.processCollectionEntries(rootId, droppedFiles, in)
	close(in)
}

func (dp *dropFilesProcess) handleDragAndDropInDocument(
	rootId, targetId string,
	pos model.BlockPosition,
	style model.BlockContentFileStyle,
	rootDone chan error,
	in chan *dropFileInfo,
) {
	// Separate root entries into files and directories
	var fileEntries []*dropFileEntry
	var dirEntries []*dropFileEntry
	for _, entry := range dp.root.child {
		if entry.isDir {
			dirEntries = append(dirEntries, entry)
		} else {
			fileEntries = append(fileEntries, entry)
		}
	}

	// Create file blocks in the document for file entries
	if len(fileEntries) > 0 {
		err := cache.Do(dp.picker, rootId, func(sb File) error {
			sbHandler, ok := sb.(dropFilesHandler)
			if !ok {
				return fmt.Errorf("unexpected smartblock interface %T; want dropFilesHandler", sb)
			}
			blockIds, err := sbHandler.dropFilesCreateStructure(dp.groupId, targetId, pos, style, fileEntries)
			if err != nil {
				return err
			}
			for i, entry := range fileEntries {
				in <- &dropFileInfo{
					pageId:  rootId,
					blockId: blockIds[i],
					path:    entry.path,
					name:    entry.name,
					groupId: dp.groupId,
				}
			}
			return nil
		})
		rootDone <- err
		if err != nil {
			log.Warnf("can't create file blocks: %v", err)
			close(in)
			return
		}
	} else {
		rootDone <- nil
	}

	// For each directory, create a collection linked in the document, then process children
	for _, dirEntry := range dirEntries {
		if dp.ctx.Err() != nil {
			return
		}
		err := cache.DoContext(dp.picker, dp.ctx, rootId, func(sb File) error {
			sbHandler, ok := sb.(dropFilesHandler)
			if !ok {
				return fmt.Errorf("unexpected smartblock interface %T; want dropFilesHandler", sb)
			}
			return sbHandler.dropFilesCreateLinkedCollection(dp, dirEntry, targetId, pos, in)
		})
		if err != nil {
			log.Warnf("can't create linked collection: %v", err)
			atomic.AddInt64(&dp.done, 1)
			continue
		}
		// After the first directory, insert below it
		targetId = ""
		pos = 0
	}
	close(in)
}

func (dp *dropFilesProcess) addFilesWorker(wg *sync.WaitGroup, in chan *dropFileInfo) {
	defer wg.Done()
	var canceled bool
	for {
		select {
		case <-dp.ctx.Done():
			canceled = true
		case info, ok := <-in:
			if !ok {
				return
			}
			if canceled {
				info.err = context.Canceled
			} else {
				dp.addFile(info)
			}
			if err := dp.apply(info); err != nil {
				log.Warnf("can't apply file: %v", err)
			}
		}
	}
}

func (dp *dropFilesProcess) addFile(f *dropFileInfo) {
	upl := dp.fileUploaderFactory.NewUploader(dp.spaceID, objectorigin.DragAndDrop())
	res := upl.
		SetName(f.name).
		SetFile(f.path).
		SetCreatedInContext(dp.contextId).
		SetCreatedInContextRef(f.blockId).
		Upload(dp.ctx)

	if res.Err != nil {
		log.Errorf("upload error: %s", res.Err)
		f.err = fmt.Errorf("upload error: %w", res.Err)
		return
	}
	f.file = res.ToBlock().Model().GetFile()
	return
}

func (dp *dropFilesProcess) apply(f *dropFileInfo) (err error) {
	defer func() {
		if !errors.Is(f.err, context.Canceled) {
			atomic.AddInt64(&dp.done, 1)
		}
	}()
	return cache.Do(dp.picker, f.pageId, func(sb File) error {
		sbHandler, ok := sb.(dropFilesHandler)
		if !ok {
			return fmt.Errorf("(apply) unexpected smartblock interface %T; want dropFilesHandler", sb)
		}
		return sbHandler.dropFilesSetInfo(*f)
	})
}

func isCollection(smartBlock smartblock.SmartBlock) bool {
	layout, ok := smartBlock.Layout()
	return ok && layout == model.ObjectType_collection
}
