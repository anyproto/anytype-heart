package fileuploader

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

	"github.com/gabriel-vasile/mimetype"
	"github.com/globalsign/mgo/bson"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

const addFileWorkersCount = 4

// objectCreator creates objects (used for creating collections during drag-n-drop).
type objectCreator interface {
	CreateObject(ctx context.Context, spaceID string, req objectcreator.CreateObjectRequest) (string, *domain.Details, error)
}

// collectionAppender appends objects to a collection.
type collectionAppender interface {
	ListIdsFromCollection() []string
	AddToCollection(ctx session.Context, req *pb.RpcObjectCollectionAddRequest) error
}

// dropFileEntry represents a file or directory entry in a drag-n-drop operation.
type dropFileEntry struct {
	name string
	path string
	// if isDir true, create a collection for children
	isDir    bool
	checksum string
	children []*dropFileEntry
}

// dropFileInfo carries file upload result information.
type dropFileInfo struct {
	objectId  string
	blockId   string
	path      string
	err       error
	name      string
	fileBlock *model.BlockContentFile
	groupId   string
}

// dropFilesProcess orchestrates a drag-n-drop file upload operation.
type dropFilesProcess struct {
	ctx           context.Context
	ctxCancel     context.CancelFunc
	id            string
	spaceId       string
	groupId       string
	contextId     string
	isDropInSpace bool
	fileType      model.BlockContentFileType

	root        *dropFileEntry
	total, done int64
	doneCh      chan struct{}
	uploadCh    chan *dropFileInfo

	// deps
	processService process.Service
	picker         cache.ObjectGetter
	service        Service
	objectCreator  objectCreator
	objectStore    spaceindex.Store
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

func (dp *dropFilesProcess) Init(paths []string) error {
	dp.root = &dropFileEntry{}
	if dp.isDropInSpace {
		dp.collectFlatPaths(paths)
	} else {
		err := dp.collectAllPaths(paths)
		if err != nil {
			return fmt.Errorf("collect all paths: %w", err)
		}
	}
	dp.groupId = bson.NewObjectId().Hex()
	return nil
}

// collectFlatPaths collects files recursively without creating directory entries.
// When type filter is active, only matching files are included.
func (dp *dropFilesProcess) collectFlatPaths(paths []string) {
	for _, path := range paths {
		dp.collectFlatEntry(path)
	}
}

func (dp *dropFilesProcess) collectAllPaths(paths []string) error {
	for _, path := range paths {
		entry := &dropFileEntry{path: path, name: filepath.Base(path)}
		ok, err := dp.readdir(entry, true)
		if err != nil {
			return anyerror.CleanupError(err)
		}
		if ok {
			dp.root.children = append(dp.root.children, entry)
			dp.total++
		}
	}
	return nil
}

// collectFlatEntry recursively collects files without creating directory entries.
// When type filter is active, only files matching the requested type are included.
func (dp *dropFilesProcess) collectFlatEntry(path string) {
	fi, err := os.Lstat(path)
	if err != nil {
		return
	}
	if fi.IsDir() {
		dp.walkDirFlat(path)
		return
	}
	name := filepath.Base(path)
	if isTypeFilterActive(dp.fileType) && detectFileType(path, name) != filterFileType(dp.fileType) {
		return
	}
	dp.root.children = append(dp.root.children, &dropFileEntry{path: path, name: name})
	dp.total++
}

// walkDirFlat recursively walks a directory, delegating to collectFlatEntry for each child.
func (dp *dropFilesProcess) walkDirFlat(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return
	}
	for _, name := range names {
		if strings.HasPrefix(name, ".") {
			continue
		}
		dp.collectFlatEntry(filepath.Join(dir, name))
	}
}

// detectFileType detects file type by reading file content (magic bytes).
func detectFileType(path, name string) model.BlockContentFileType {
	mime, err := mimetype.DetectFile(path)
	if err != nil {
		return model.BlockContentFile_File
	}
	return file.DetectTypeByMIME(name, mime.String())
}

// isTypeFilterActive returns true when type-based filtering should be applied.
// Only None disables filtering.
func isTypeFilterActive(fileType model.BlockContentFileType) bool {
	return fileType != model.BlockContentFile_None
}

// filterFileType returns the type to match against during filtering.
// Image, Audio, and Video match exactly; everything else matches as File.
func filterFileType(fileType model.BlockContentFileType) model.BlockContentFileType {
	switch fileType {
	case model.BlockContentFile_Image, model.BlockContentFile_Audio, model.BlockContentFile_Video:
		return fileType
	default:
		return model.BlockContentFile_File
	}
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
			entry.children = append(entry.children, chEntry)
			dp.total++
		}
	}
	entry.checksum = computeDirectoryChecksum(entry.children)
	return true, nil
}

// computeDirectoryChecksum computes a deterministic checksum for a set of directory children.
func computeDirectoryChecksum(children []*dropFileEntry) string {
	names := make([]string, len(children))
	for i, ch := range children {
		names[i] = ch.name
	}
	sort.Strings(names)
	h := sha256.Sum256([]byte(strings.Join(names, "\x00")))
	return hex.EncodeToString(h[:])
}

// Start begins the drop files process. rootId is the ID of the target smartblock, isCollection indicates
// whether the target is a collection. req contains the original drop request. rootDone is closed/sent
// when the initial block creation is done (or immediately for collections).
func (dp *dropFilesProcess) Start(rootId string, isCollection bool, req pb.RpcFileDropRequest, rootDone chan error) {
	dp.id = uuid.New().String()
	dp.doneCh = make(chan struct{})
	dp.ctx, dp.ctxCancel = context.WithCancel(context.Background())
	defer close(dp.doneCh)
	dp.processService.Add(dp)

	// start upload workers
	workersCount := int(dp.total)
	if workersCount > addFileWorkersCount {
		workersCount = addFileWorkersCount
	}
	dp.uploadCh = make(chan *dropFileInfo, workersCount)

	var wg = &sync.WaitGroup{}
	wg.Add(workersCount)
	for i := 0; i < workersCount; i++ {
		go dp.uploadFilesWorker(wg)
	}

	if dp.isDropInSpace {
		dp.handleDropInSpace(rootDone)
		close(dp.uploadCh)
	} else if isCollection {
		dp.handleDropInCollection(rootId, dp.root.children, rootDone)
		close(dp.uploadCh)
	} else {
		dp.handleDropInObject(rootId, req.DropTargetId, req.Position, req.Style, rootDone)
		close(dp.uploadCh)
	}
	wg.Wait()
}

func (dp *dropFilesProcess) handleDropInCollection(rootId string, droppedFiles []*dropFileEntry, rootDone chan error) {
	close(rootDone)
	dp.processCollectionEntries(rootId, droppedFiles)
}

func (dp *dropFilesProcess) handleDropInSpace(rootDone chan error) {
	close(rootDone)
	for _, entry := range dp.root.children {
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
			atomic.AddInt64(&dp.done, 1)
			dp.processCollectionEntries(collId, entry.children)
		} else {
			select {
			case <-dp.ctx.Done():
				return
			case dp.uploadCh <- &dropFileInfo{
				path: entry.path,
				name: entry.name,
			}:
			}
		}
	}
}

func (dp *dropFilesProcess) handleDropInObject(
	rootId, targetId string,
	pos model.BlockPosition,
	style model.BlockContentFileStyle,
	rootDone chan error,
) {
	// Separate root entries into files and directories
	var fileEntries []*dropFileEntry
	var dirEntries []*dropFileEntry
	for _, entry := range dp.root.children {
		if entry.isDir {
			dirEntries = append(dirEntries, entry)
		} else {
			fileEntries = append(fileEntries, entry)
		}
	}

	// Create file blocks in the document for file entries
	if len(fileEntries) > 0 {
		blockIds, err := dp.createFileBlocks(rootId, dp.groupId, targetId, pos, style, fileEntries)
		if err != nil {
			rootDone <- err
			log.Warnf("can't create file blocks: %v", err)
			return
		}
		rootDone <- nil
		for i, entry := range fileEntries {
			dp.uploadCh <- &dropFileInfo{
				objectId: rootId,
				blockId:  blockIds[i],
				path:     entry.path,
				name:     entry.name,
				groupId:  dp.groupId,
			}
		}
	} else {
		rootDone <- nil
	}

	// For each directory, create a collection linked in the document, then process children
	for _, dirEntry := range dirEntries {
		if dp.ctx.Err() != nil {
			return
		}
		err := dp.createLinkedCollection(rootId, targetId, pos, dirEntry)
		if err != nil {
			log.Warnf("can't create linked collection: %v", err)
			atomic.AddInt64(&dp.done, 1)
			continue
		}
		atomic.AddInt64(&dp.done, 1)
		// After the first directory, insert below it
		targetId = ""
		pos = 0
	}
}

// createFileBlocks creates empty file blocks in the document for each entry.
func (dp *dropFilesProcess) createFileBlocks(rootId, groupId, targetId string, pos model.BlockPosition, style model.BlockContentFileStyle, entries []*dropFileEntry) (blockIds []string, err error) {
	err = cache.Do(dp.picker, rootId, func(sb smartblock.SmartBlock) error {
		s := sb.NewState().SetGroupId(groupId)
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
			if err := s.InsertTo(targetId, pos, blockId); err != nil {
				return err
			}
			targetId = blockId
			pos = model.Block_Bottom
			blockIds = append(blockIds, blockId)
		}
		return sb.Apply(s)
	})
	return
}

// applyFileInfo applies upload results to the smartblock.
func (dp *dropFilesProcess) applyFileInfo(info *dropFileInfo) error {
	if errors.Is(info.err, context.Canceled) {
		if info.blockId != "" {
			return cache.Do(dp.picker, info.objectId, func(sb smartblock.SmartBlock) error {
				s := sb.NewState().SetGroupId(info.groupId)
				s.Unlink(info.blockId)
				return sb.Apply(s)
			})
		}
		return nil
	}

	if info.err != nil {
		return fmt.Errorf("drop file: %w", info.err)
	}

	// For standalone space uploads (no container), file object already exists
	if info.blockId == "" && info.objectId == "" {
		return nil
	}

	// For collection entries (no blockId), just append to collection
	if info.blockId == "" {
		if info.fileBlock == nil {
			return fmt.Errorf("file block is nil")
		}
		return dp.addObjectToCollection(dp.ctx, info.objectId, info.fileBlock.TargetObjectId)
	}

	// For regular entries: update the file block
	return cache.Do(dp.picker, info.objectId, func(sb smartblock.SmartBlock) error {
		// If parent is a collection, also append the file object
		layout, ok := sb.Layout()
		isCol := ok && layout == model.ObjectType_collection
		if isCol && info.fileBlock != nil {
			coll, ok := sb.(collectionAppender)
			if !ok {
				return fmt.Errorf("collection component not found")
			}
			if err := appendToCollection(coll, info.fileBlock.TargetObjectId); err != nil {
				return fmt.Errorf("append to collection: %w", err)
			}
		}

		// Update file block
		s := sb.NewState().SetGroupId(info.groupId)
		b := s.Get(info.blockId)
		f, ok := b.(file.Block)
		if !ok {
			return fmt.Errorf("not a file block")
		}
		if info.err != nil || info.fileBlock == nil || info.fileBlock.State == model.BlockContentFile_Error {
			if info.err != nil {
				log.Warnf("upload file error: %s", info.err)
			}
			f.SetState(model.BlockContentFile_Error)
			return sb.Apply(s)
		}
		existingStyle := f.Model().GetFile().GetStyle()
		f.SetModel(info.fileBlock)
		if existingStyle != model.BlockContentFile_Auto {
			f.SetStyle(existingStyle)
		}
		return sb.Apply(s)
	})
}

// createLinkedCollection creates a collection for a directory and inserts a link block in the document.
func (dp *dropFilesProcess) createLinkedCollection(rootId, targetId string, pos model.BlockPosition, dirEntry *dropFileEntry) error {
	// Check if a collection with the same checksum already exists
	if existingId, ok := dp.findExistingCollectionByChecksum(dirEntry.checksum); ok {
		err := dp.insertCollectionLink(rootId, targetId, pos, existingId)
		if err != nil {
			return fmt.Errorf("insert link to existing collection: %w", err)
		}
		dp.processCollectionEntries(existingId, dirEntry.children)
		return nil
	}

	// Create new collection object
	collId, err := dp.createCollectionForFolder(dp.ctx, dirEntry.name, dirEntry.checksum)
	if err != nil {
		return fmt.Errorf("create collection for folder: %w", err)
	}

	// Insert link block in the document
	err = dp.insertCollectionLink(rootId, targetId, pos, collId)
	if err != nil {
		return fmt.Errorf("insert link to new collection: %w", err)
	}

	// Process children into the new collection
	dp.processCollectionEntries(collId, dirEntry.children)
	return nil
}

// insertCollectionLink inserts a link block pointing to a collection in the given document.
func (dp *dropFilesProcess) insertCollectionLink(rootId, targetId string, pos model.BlockPosition, collectionId string) error {
	return cache.Do(dp.picker, rootId, func(sb smartblock.SmartBlock) error {
		s := sb.NewState()
		linkBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: collectionId,
					Style:         model.BlockContentLink_Page,
					IconSize:      model.BlockContentLink_SizeSmall,
				},
			},
		})
		s.Add(linkBlock)
		if err := s.InsertTo(targetId, pos, linkBlock.Model().Id); err != nil {
			return fmt.Errorf("insert link block: %w", err)
		}
		return sb.Apply(s)
	})
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

	id, _, err := dp.objectCreator.CreateObject(ctx, dp.spaceId, objectcreator.CreateObjectRequest{
		Details:       details,
		ObjectTypeKey: bundle.TypeKeyCollection,
	})
	if err != nil {
		return "", fmt.Errorf("create collection for folder %q: %w", name, err)
	}
	return id, nil
}

func (dp *dropFilesProcess) addObjectToCollection(ctx context.Context, collectionId, objectId string) error {
	return cache.DoContext(dp.picker, ctx, collectionId, func(coll collectionAppender) error {
		return appendToCollection(coll, objectId)
	})
}

func (dp *dropFilesProcess) processCollectionEntries(parentCollectionId string, entries []*dropFileEntry) {
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
				queue = append(queue, level{collectionId: collId, entries: entry.children})
			} else {
				select {
				case <-dp.ctx.Done():
					return
				case dp.uploadCh <- &dropFileInfo{
					objectId: cur.collectionId,
					path:     entry.path,
					name:     entry.name,
				}:
				}
			}
		}
	}
}

func (dp *dropFilesProcess) uploadFilesWorker(wg *sync.WaitGroup) {
	defer wg.Done()
	var canceled bool
	for {
		select {
		case <-dp.ctx.Done():
			canceled = true
		case info, ok := <-dp.uploadCh:
			if !ok {
				return
			}
			if canceled {
				info.err = context.Canceled
			} else {
				dp.uploadFile(info)
			}
			if err := dp.apply(info); err != nil {
				log.Warnf("can't apply file: %v", err)
			}
		}
	}
}

func (dp *dropFilesProcess) uploadFile(f *dropFileInfo) {
	upl := dp.service.NewUploader(dp.spaceId, objectorigin.DragAndDrop())
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
	f.fileBlock = res.ToBlock().Model().GetFile()
}

func (dp *dropFilesProcess) apply(f *dropFileInfo) (err error) {
	defer func() {
		if !errors.Is(f.err, context.Canceled) {
			atomic.AddInt64(&dp.done, 1)
		}
	}()
	return dp.applyFileInfo(f)
}

func appendToCollection(col collectionAppender, id string) error {
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
