package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	addFileWorkersCount = 4
)

var log = logging.Logger("anytype-mw-smartfile")

func NewFile(sb smartblock.SmartBlock, blockService BlockService, picker cache.ObjectGetter, processService process.Service, fileUploaderFactory fileuploader.Service) File {
	return &sfile{
		SmartBlock:          sb,
		blockService:        blockService,
		picker:              picker,
		processService:      processService,
		fileUploaderFactory: fileUploaderFactory,
	}
}

type BlockService interface {
	CreateLinkToTheNewObject(ctx context.Context, sctx session.Context, req *pb.RpcBlockLinkCreateWithObjectRequest) (linkID string, pageID string, details *domain.Details, err error)
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
	Path             string
	Url              string // nolint:revive
	Bytes            []byte
	Name             string
	GroupID          string
	Origin           objectorigin.ObjectOrigin
	ImageKind        model.ImageKind
	CreatedInContext string
	CreatedInBlockId string
}

type sfile struct {
	smartblock.SmartBlock

	blockService        BlockService
	picker              cache.ObjectGetter
	processService      process.Service
	fileUploaderFactory fileuploader.Service
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
		Path:             req.LocalPath,
		Url:              req.Url,
		ImageKind:        req.ImageKind,
		CreatedInContext: req.ContextId,
		CreatedInBlockId: newId,
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
	if source.CreatedInBlockId != "" {
		upl.SetCreatedInBlockId(source.CreatedInBlockId)
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
		contextId:           req.ContextId,
	}
	if err = proc.Init(req.LocalFilePaths); err != nil {
		return
	}
	var ch = make(chan error)
	go proc.Start(sf, req.DropTargetId, req.Position, ch)
	err = <-ch
	return
}

func (sf *sfile) dropFilesCreateStructure(groupId, targetId string, pos model.BlockPosition, entries []*dropFileEntry) (blockIds []string, err error) {
	s := sf.NewState().SetGroupId(groupId)
	pageTypeId, err := sf.Space().GetTypeIdByKey(context.Background(), bundle.TypeKeyPage)
	if err != nil {
		return
	}
	for _, entry := range entries {
		var blockId, pageId string
		if entry.isDir {
			if err = sf.Apply(s); err != nil {
				return
			}
			sf.Unlock()
			blockId, pageId, _, err = sf.blockService.CreateLinkToTheNewObject(context.Background(), nil, &pb.RpcBlockLinkCreateWithObjectRequest{
				SpaceId:             sf.SpaceID(),
				ContextId:           sf.Id(),
				ObjectTypeUniqueKey: bundle.TypeKeyPage.URL(),
				TargetId:            targetId,
				Position:            pos,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"type":      pbtypes.String(pageTypeId),
						"name":      pbtypes.String(entry.name),
						"iconEmoji": pbtypes.String("📁"),
					},
				},
			})
			sf.Lock()
			if err != nil {
				return
			}
			targetId = blockId
			pos = model.Block_Bottom
			blockId = pageId
			s = sf.NewState().SetGroupId(groupId)
		} else {
			fb := simple.New(&model.Block{Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name: entry.name,
				},
			}})
			blockId = fb.Model().Id
			fb.(file.Block).SetState(model.BlockContentFile_Uploading)
			s.Add(fb)
			if err = s.InsertTo(targetId, pos, blockId); err != nil {
				return
			}
			targetId = blockId
			pos = model.Block_Bottom
		}
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
		s := sf.NewState()
		if !s.HasInStore([]string{info.file.TargetObjectId}) {
			s.UpdateStoreSlice(template.CollectionStoreKey, append(s.GetStoreSlice(template.CollectionStoreKey), info.file.TargetObjectId))
		}
		return sf.Apply(s)
	}
	return sf.UpdateFile(info.blockId, info.groupId, func(f file.Block) error {
		if info.err != nil || info.file == nil || info.file.State == model.BlockContentFile_Error {
			if info.err != nil {
				log.Warnf("upload file error: %s", info.err)
			}
			f.SetState(model.BlockContentFile_Error)
			return nil
		}
		f.SetModel(info.file)
		return nil
	})
}

type dropFileEntry struct {
	name  string
	path  string
	isDir bool
	child []*dropFileEntry
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
	dropFilesCreateStructure(groupId, targetId string, pos model.BlockPosition, entries []*dropFileEntry) (blockIds []string, err error)
	dropFilesSetInfo(info dropFileInfo) (err error)
	newUploader(origin objectorigin.ObjectOrigin) fileuploader.Uploader
}

type dropFilesProcess struct {
	id             string
	spaceID        string
	processService process.Service
	picker         cache.ObjectGetter
	root           *dropFileEntry
	total, done    int64
	cancel         chan struct{}
	doneCh         chan struct{}
	canceling      int32
	groupId        string
	contextId      string

	fileUploaderFactory fileuploader.Service
}

func (dp *dropFilesProcess) Id() string {
	return dp.id
}

func (dp *dropFilesProcess) Cancel() (err error) {
	if atomic.AddInt32(&dp.canceling, 1) == 1 {
		close(dp.cancel)
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
	if atomic.LoadInt32(&dp.canceling) != 0 {
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
	return true, nil
}

func (dp *dropFilesProcess) Start(file smartblock.SmartBlock, targetId string, pos model.BlockPosition, rootDone chan error) {
	dp.id = uuid.New().String()
	dp.doneCh = make(chan struct{})
	dp.cancel = make(chan struct{})
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
		dp.handleDragAndDropInDocument(file.RootId(), targetId, pos, rootDone, in)
	}
	wg.Wait()
}

func (dp *dropFilesProcess) handleDragAndDropInCollection(rootId string, droppedFiles []*dropFileEntry, rootDone chan error, in chan *dropFileInfo) {
	close(rootDone)
	filesToUpload := dp.getFilesToUploadFromDirs(droppedFiles)
	for _, entry := range filesToUpload {
		in <- &dropFileInfo{
			pageId: rootId,
			path:   entry.path,
			name:   entry.name,
		}
	}
	close(in)
}

func (dp *dropFilesProcess) getFilesToUploadFromDirs(droppedFiles []*dropFileEntry) []*dropFileEntry {
	var (
		stack      []*dropFileEntry
		totalFiles []*dropFileEntry
	)
	stack = append(stack, droppedFiles...)
	for len(stack) > 0 {
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if entry.isDir {
			stack = append(stack, entry.child...)
		} else {
			totalFiles = append(totalFiles, entry)
		}
	}
	return totalFiles
}

func (dp *dropFilesProcess) handleDragAndDropInDocument(rootId, targetId string, pos model.BlockPosition, rootDone chan error, in chan *dropFileInfo) {
	var flatEntries = [][]*dropFileEntry{dp.root.child}
	var smartBlockIds = []string{rootId}
	var handleLevel = func(idx int) (isContinue bool, err error) {
		if idx >= len(smartBlockIds) {
			return
		}
		err = cache.Do(dp.picker, smartBlockIds[idx], func(sb File) error {
			sbHandler, ok := sb.(dropFilesHandler)
			if !ok {
				isContinue = idx != 0
				return fmt.Errorf("unexpected smartblock interface %T; want dropFilesHandler", sb)
			}
			blockIds, err := sbHandler.dropFilesCreateStructure(dp.groupId, targetId, pos, flatEntries[idx])
			if err != nil {
				isContinue = idx != 0
				return err
			}
			for i, entry := range flatEntries[idx] {
				if entry.isDir {
					smartBlockIds = append(smartBlockIds, blockIds[i])
					flatEntries = append(flatEntries, entry.child)
					atomic.AddInt64(&dp.done, 1)
				} else {
					in <- &dropFileInfo{
						pageId:  smartBlockIds[idx],
						blockId: blockIds[i],
						path:    entry.path,
						name:    entry.name,
						groupId: dp.groupId,
					}
				}
			}
			return nil
		})
		if err != nil {
			return isContinue, err
		}
		if atomic.LoadInt32(&dp.canceling) != 0 {
			return false, err
		}
		return true, nil
	}
	var idx = 0
	for {
		ok, err := handleLevel(idx)
		if idx == 0 {
			rootDone <- err
			if err != nil {
				log.Warnf("can't create files: %v", err)
				close(in)
				return
			}
			targetId = ""
			pos = 0
		}
		if err != nil {
			log.Warnf("can't create files: %v", err)
		}
		if !ok {
			break
		}
		idx++
	}
	close(in)
}

func (dp *dropFilesProcess) addFilesWorker(wg *sync.WaitGroup, in chan *dropFileInfo) {
	defer wg.Done()
	var canceled bool
	for {
		select {
		case <-dp.cancel:
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
		SetCreatedInBlockId(f.blockId).
		Upload(context.Background())

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
		if f.err != context.Canceled {
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
