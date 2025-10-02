package fileuploader

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("file-uploader")

type Service interface {
	app.Component

	NewUploader(spaceId string, origin objectorigin.ObjectOrigin) Uploader
	GetPreloadResult(preloadId string) (*files.AddResult, bool)
	RemovePreloadResult(preloadId string)
}

// preloadEntry tracks the status of a preload operation and implements Process interface
type preloadEntry struct {
	id     string
	result *files.AddResult
	err    error
	done   chan struct{} // closed when preload is complete
	state  pb.ModelProcessState
	mu     sync.RWMutex
	ctx    context.Context // wraps service ctx with process cancellation
	cancel context.CancelFunc
}

type service struct {
	fileService       files.Service
	fileStorage       filestorage.FileStorage
	tempDirProvider   core.TempDirProvider
	picker            cache.ObjectGetter
	fileObjectService FileObjectService
	processService    process.Service

	// Manage preloaded results
	preloadMu      sync.RWMutex
	preloadEntries map[string]*preloadEntry // preloadId -> preloadEntry

	ctx       context.Context
	ctxCancel context.CancelFunc
}

func New() Service {
	return &service{
		preloadEntries: make(map[string]*preloadEntry),
	}
}

func (f *service) NewUploader(spaceId string, origin objectorigin.ObjectOrigin) Uploader {
	return &uploader{
		spaceId:           spaceId,
		picker:            f.picker,
		fileService:       f.fileService,
		fileStorage:       f.fileStorage,
		tempDirProvider:   f.tempDirProvider,
		fileObjectService: f.fileObjectService,
		origin:            origin,
		preload:           f,
		serviceCtx:        f.ctx,
	}
}

type preload interface {
	StartPreload(preloadId string, fileName string) *preloadEntry
	GetPreloadEntry(preloadId string) (*preloadEntry, bool)
	CompletePreload(preloadId string, result *files.AddResult, err error)
	GetPreloadResult(preloadId string) (*files.AddResult, bool)
}

// StartPreload creates a new preload entry for async processing
func (f *service) StartPreload(preloadId string, fileName string) *preloadEntry {
	f.preloadMu.Lock()
	defer f.preloadMu.Unlock()

	entry := &preloadEntry{
		id:    preloadId,
		done:  make(chan struct{}),
		state: pb.ModelProcess_Running,
	}

	entry.ctx, entry.cancel = context.WithCancel(f.ctx)

	if f.processService != nil {
		if err := f.processService.Add(entry); err != nil {
			log.Errorf("failed to add preload process: %v", err)
		}
	}
	f.preloadEntries[preloadId] = entry
	return entry
}

// GetPreloadEntry returns a preload entry and waits if it's still in progress
func (f *service) GetPreloadEntry(preloadId string) (*preloadEntry, bool) {
	f.preloadMu.RLock()
	entry, ok := f.preloadEntries[preloadId]
	f.preloadMu.RUnlock()

	if ok {
		// Wait for preload to complete
		<-entry.done
	}
	return entry, ok
}

// CompletePreload marks a preload operation as complete
func (f *service) CompletePreload(preloadId string, result *files.AddResult, err error) {
	f.preloadMu.Lock()
	defer f.preloadMu.Unlock()

	if entry, ok := f.preloadEntries[preloadId]; ok {
		entry.complete(result, err)
	}
}

// GetPreloadResult returns the result of a preload operation (waits if in progress)
func (f *service) GetPreloadResult(preloadId string) (*files.AddResult, bool) {
	entry, ok := f.GetPreloadEntry(preloadId)
	if !ok {
		return nil, false
	}
	if entry.err != nil {
		return nil, false
	}
	return entry.result, true
}

// RemovePreloadResult removes a preload result after it's been used
func (f *service) RemovePreloadResult(preloadId string) {
	f.preloadMu.Lock()
	defer f.preloadMu.Unlock()
	delete(f.preloadEntries, preloadId)
}

const CName = "file-uploader"

func (f *service) Name() string {
	return CName
}

func (f *service) Init(a *app.App) error {
	f.fileService = app.MustComponent[files.Service](a)
	f.fileStorage = app.MustComponent[filestorage.FileStorage](a)
	f.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	f.picker = app.MustComponent[cache.ObjectGetter](a)
	f.fileObjectService = app.MustComponent[FileObjectService](a)
	// Process service is optional - tests may not provide it
	if ps := a.Component(process.CName); ps != nil {
		f.processService = ps.(process.Service)
	}
	f.ctx, f.ctxCancel = context.WithCancel(context.Background())
	return nil
}

func (f *service) Run(_ context.Context) (err error) {
	return nil
}

func (f *service) Close(_ context.Context) (err error) {
	f.ctxCancel()
	return nil
}

var (
	// limiting overall file upload goroutines
	uploadFilesLimiter = make(chan struct{}, 8)
	bufSize            = 8192
)

func init() {
	for i := 0; i < cap(uploadFilesLimiter); i++ {
		uploadFilesLimiter <- struct{}{}
	}
}

type Uploader interface {
	SetBlock(block file.Block) Uploader
	SetName(name string) Uploader
	SetType(tp model.BlockContentFileType) Uploader
	SetStyle(tp model.BlockContentFileStyle) Uploader
	SetAdditionalDetails(details *domain.Details) Uploader
	SetBytes(b []byte) Uploader
	SetUrl(url string) Uploader
	SetFile(path string) Uploader
	SetLastModifiedDate() Uploader
	SetGroupId(groupId string) Uploader
	SetCustomEncryptionKeys(keys map[string]string) Uploader
	SetImageKind(imageKind model.ImageKind) Uploader
	SetPreloadId(preloadId string) Uploader

	AddOptions(options ...files.AddOption) Uploader
	AsyncUpdates(smartBlockId string) Uploader

	// Preload uploads file to storage with batch but doesn't commit or create object
	Preload(ctx context.Context) (preloadId string, err error)
	// Upload uploads file and creates object (or uses preloaded file)
	Upload(ctx context.Context) (result UploadResult)
	UploadAsync(ctx context.Context) (ch chan UploadResult)
}

type UploadResult struct {
	Name              string
	Type              model.BlockContentFileType
	FileObjectId      string
	FileObjectDetails *domain.Details
	EncryptionKeys    map[string]string
	MIME              string
	Size              int64
	Err               error
}

func (ur UploadResult) ToBlock() file.Block {
	state := model.BlockContentFile_Done
	if ur.Err != nil {
		state = model.BlockContentFile_Error
		ur.Name = ur.Err.Error()
	}
	return simple.New(&model.Block{
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				TargetObjectId: ur.FileObjectId,
				Name:           ur.Name,
				Type:           ur.Type,
				Mime:           ur.MIME,
				Size_:          ur.Size,
				AddedAt:        time.Now().Unix(),
				State:          state,
			},
		},
	}).(file.Block)
}

type FileObjectService interface {
	GetObjectDetailsByFileId(fileId domain.FullFileId) (string, *domain.Details, error)
	Create(ctx context.Context, spaceId string, req filemodels.CreateRequest) (id string, object *domain.Details, err error)
}

type uploader struct {
	spaceId              string
	fileObjectService    FileObjectService
	picker               cache.ObjectGetter
	block                file.Block
	preload              preload
	getReader            func(ctx context.Context) (*fileReader, error)
	name                 string
	lastModifiedDate     int64
	forceType            bool
	forceUploadingAsFile bool
	smartBlockID         string
	fileType             model.BlockContentFileType
	fileStyle            model.BlockContentFileStyle
	opts                 []files.AddOption
	groupID              string

	tempDirProvider      core.TempDirProvider
	fileService          files.Service
	fileStorage          filestorage.FileStorage
	origin               objectorigin.ObjectOrigin
	imageKind            model.ImageKind
	additionalDetails    *domain.Details
	customEncryptionKeys map[string]string
	preloadId            string

	serviceCtx context.Context // used to cancel async operations
}

type bufioSeekClose struct {
	*bufio.Reader
	close func() error
	seek  func(offset int64, whence int) (int64, error)
}

type fileReader struct {
	*bufioSeekClose
	fileName string
}

func (bc *bufioSeekClose) Close() error {
	if bc.close != nil {
		return bc.close()
	}
	return nil
}

func (bc *bufioSeekClose) Seek(offset int64, whence int) (int64, error) {
	if bc.seek != nil {
		return bc.seek(offset, whence)
	}
	return 0, fmt.Errorf("seek not supported for this type")
}

func (fr *fileReader) GetFileName() string {
	return fr.fileName
}

func (u *uploader) SetBlock(block file.Block) Uploader {
	u.block = block
	return u
}

func (u *uploader) SetGroupId(groupId string) Uploader {
	u.groupID = groupId
	return u
}

func (u *uploader) SetName(name string) Uploader {
	u.name = name
	return u
}

func (u *uploader) SetType(tp model.BlockContentFileType) Uploader {
	u.fileType = tp
	u.forceType = true
	return u
}

func (u *uploader) ForceUploadingAsFile() Uploader {
	u.forceUploadingAsFile = true
	return u
}

func (u *uploader) SetStyle(tp model.BlockContentFileStyle) Uploader {
	u.fileStyle = tp
	return u
}

func (u *uploader) SetAdditionalDetails(details *domain.Details) Uploader {
	u.additionalDetails = details
	return u
}

func (u *uploader) SetPreloadId(preloadId string) Uploader {
	u.preloadId = preloadId
	return u
}

func (u *uploader) SetBytes(b []byte) Uploader {
	u.getReader = func(_ context.Context) (*fileReader, error) {
		buf := bytes.NewReader(b)
		bufReaderSize := bufio.NewReaderSize(buf, bufSize)
		return &fileReader{
			bufioSeekClose: &bufioSeekClose{
				Reader: bufReaderSize,
				seek: func(offset int64, whence int) (int64, error) {
					bufReaderSize.Reset(buf)
					return buf.Seek(offset, whence)
				},
			},
		}, nil
	}
	return u
}

func (u *uploader) SetCustomEncryptionKeys(keys map[string]string) Uploader {
	u.customEncryptionKeys = keys
	return u
}

func (u *uploader) SetImageKind(imageKind model.ImageKind) Uploader {
	u.imageKind = imageKind
	return u
}

func (u *uploader) AddOptions(options ...files.AddOption) Uploader {
	u.opts = append(u.opts, options...)
	return u
}

func (u *uploader) SetUrl(url string) Uploader {
	url, err := uri.NormalizeURI(url)
	if err != nil {
		// do nothing
	}
	u.getReader = func(ctx context.Context) (*fileReader, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		// setting timeout to avoid locking for a long time
		cl := http.DefaultClient

		resp, err := cl.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("failed to download url, status: %d", resp.StatusCode)
		}

		var fileName string
		if content := resp.Header.Get("Content-Disposition"); content != "" {
			contentDisposition := strings.Split(content, "filename=")
			if len(contentDisposition) > 1 {
				fileName = strings.Trim(contentDisposition[1], "\"")
			}
		}
		if fileName == "" {
			fileName = uri.GetFileNameFromURLAndContentType(resp.Request.URL, resp.Header.Get("Content-Type"))
		}

		tmpFile, err := ioutil.TempFile(u.tempDirProvider.TempDir(), "anytype_downloaded_file_*")
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(tmpFile, resp.Body)
		if err != nil {
			return nil, err
		}

		_, err = tmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		buf := bufio.NewReaderSize(tmpFile, bufSize)
		bsc := &bufioSeekClose{
			Reader: buf,
			seek: func(offset int64, whence int) (int64, error) {
				buf.Reset(tmpFile)
				return tmpFile.Seek(offset, whence)
			},
			close: func() error {
				_ = tmpFile.Close()
				os.Remove(tmpFile.Name())
				return resp.Body.Close()
			},
		}
		return &fileReader{
			bufioSeekClose: bsc,
			fileName:       fileName,
		}, nil
	}
	return u
}

func (u *uploader) SetFile(path string) Uploader {
	if u.name == "" {
		// only set name if it wasn't explicitly set before
		u.SetName(filepath.Base(path))
	}
	u.setLastModifiedDate(path)

	u.getReader = func(ctx context.Context) (*fileReader, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, anyerror.CleanupError(err)
		}

		buf := bufio.NewReaderSize(f, bufSize)
		bsc := &bufioSeekClose{
			Reader: buf,
			seek: func(offset int64, whence int) (int64, error) {
				buf.Reset(f)
				return f.Seek(offset, whence)
			},
			close: f.Close,
		}
		return &fileReader{
			bufioSeekClose: bsc,
		}, nil
	}
	return u
}

func (u *uploader) SetLastModifiedDate() Uploader {
	u.lastModifiedDate = time.Now().Unix()
	return u
}

func (u *uploader) setLastModifiedDate(path string) {
	stat, err := os.Stat(path)
	if err == nil {
		u.lastModifiedDate = stat.ModTime().Unix()
	} else {
		u.lastModifiedDate = time.Now().Unix()
	}
}

func (u *uploader) AsyncUpdates(smartBlockId string) Uploader {
	u.smartBlockID = smartBlockId
	return u
}

func (u *uploader) UploadAsync(ctx context.Context) (result chan UploadResult) {
	result = make(chan UploadResult, 1)
	if u.block != nil {
		u.block.SetState(model.BlockContentFile_Uploading)
		u.block = u.block.Copy().(file.Block)
	}
	go func() {
		res := u.Upload(u.serviceCtx)
		if res.Err != nil {
			log.Errorf("upload async: %v", res.Err)
		}
		result <- res
		close(result)
	}()
	return
}

func (u *uploader) addFile(ctx context.Context) (addResult *files.AddResult, err error) {
	if u.preloadId != "" {
		// Wait for preload to complete if it's still in progress
		entry, ok := u.preload.GetPreloadEntry(u.preloadId)
		if !ok {
			return nil, fmt.Errorf("no preload result found for id %s", u.preloadId)
		}
		if entry.err != nil {
			return nil, fmt.Errorf("preload failed: %w", entry.err)
		}
		return entry.result, nil
	}

	if u.getReader == nil {
		err = fmt.Errorf("uploader: empty source for upload")
		return
	}
	buf, err := u.getReader(ctx)
	if err != nil {
		return
	}
	defer buf.Close()
	if !u.forceType {
		if u.block != nil {
			u.fileType = u.block.Model().GetFile().GetType()
		}
	}

	if fileName := buf.GetFileName(); fileName != "" {
		if u.name == "" {
			u.SetName(fileName)
		} else if filepath.Ext(u.name) == "" {
			// enrich current name with extension
			u.name += filepath.Ext(fileName)
		}
	}

	if u.block != nil {
		u.fileStyle = u.block.Model().GetFile().GetStyle()
	}
	if !u.forceType {
		fileType, detectErr := u.detectType(buf)
		if detectErr != nil {
			err = fmt.Errorf("detectType: %w", detectErr)
			return
		}
		u.fileType = fileType
	}
	if u.fileStyle == model.BlockContentFile_Auto {
		if u.fileType == model.BlockContentFile_File || u.fileType == model.BlockContentFile_None {
			u.fileStyle = model.BlockContentFile_Link
		} else {
			u.fileStyle = model.BlockContentFile_Embed
		}
	}
	batch, err := u.fileStorage.Batch(ctx)
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}

	// Create a FileHandler with the batch as its blockstore
	// This will be used for both file content (AddFile) and directory operations (DAGService)
	fileHandler := fileservice.NewFileHandler(batch)

	var opts = []files.AddOption{
		files.WithName(u.name),
		files.WithLastModifiedDate(u.lastModifiedDate),
		files.WithReader(buf),
		files.WithFileHandler(fileHandler),
	}
	if u.customEncryptionKeys != nil {
		opts = append(opts, files.WithCustomEncryptionKeys(u.customEncryptionKeys))
	}

	if len(u.opts) > 0 {
		opts = append(opts, u.opts...)
	}

	if !u.forceUploadingAsFile && u.fileType == model.BlockContentFile_Image && filepath.Ext(u.name) != constant.SvgExt {
		addResult, err = u.fileService.ImageAdd(ctx, u.spaceId, opts...)
		if errors.Is(err, image.ErrFormat) ||
			errors.Is(err, mill.ErrFormatSupportNotEnabled) ||
			errors.Is(err, mill.ErrProcessing) {
			err = nil
			u.forceUploadingAsFile = true
			return u.addFile(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("add image to storage: %w", err)
		}
	} else {
		addResult, err = u.fileService.FileAdd(ctx, u.spaceId, opts...)
		if err != nil {
			return nil, fmt.Errorf("add file to storage: %w", err)
		}
	}

	addResult.Batch = batch
	return addResult, nil
}

func (u *uploader) processAddedFile(ctx context.Context, addResult *files.AddResult) (result UploadResult) {
	var err error
	defer func() {
		if err != nil {
			result.Err = err
			if u.block != nil {
				u.block.SetState(model.BlockContentFile_Error).SetName(err.Error())
				u.updateBlock()
			}
		}
	}()

	result.MIME = addResult.MIME
	result.Size = addResult.Size
	result.EncryptionKeys = addResult.EncryptionKeys.EncryptionKeys
	result.Type = u.fileType
	result.Name = u.name
	// we still can have orphan blocks if app is killed in the middle of commit, but os.Rename syscalls are very fast
	addResult.Commit()

	addResult.Lock()
	// to avoid race cond with multiple calls to Upload leading to multiple objects created
	fileObjectId, fileObjectDetails, err := u.getOrCreateFileObject(ctx, addResult)
	addResult.Unlock()
	if err != nil {
		return UploadResult{Err: err}
	}
	result.FileObjectId = fileObjectId
	result.FileObjectDetails = fileObjectDetails

	if u.block != nil {
		u.block.SetName(u.name).
			SetState(model.BlockContentFile_Done).
			SetType(u.fileType).
			SetTargetObjectId(result.FileObjectId).
			SetSize(result.Size).
			SetStyle(u.fileStyle).
			SetMIME(result.MIME)
		u.updateBlock()
	}
	return result
}

func (u *uploader) Upload(ctx context.Context) (result UploadResult) {
	addFile, err := u.addFile(ctx)
	if err != nil {
		return UploadResult{Err: err}
	}
	return u.processAddedFile(ctx, addFile)
}

func (u *uploader) Preload(ctx context.Context) (preloadId string, err error) {
	if u.preloadId != "" {
		return "", fmt.Errorf("should not run Preload if preloadId is already set")
	}

	// Validate that we can read the file
	if u.getReader == nil {
		return "", fmt.Errorf("uploader: empty source for upload")
	}

	// Quick validation - try to open the file
	buf, err := u.getReader(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	buf.Close()

	// Generate a random preloadId
	preloadId = uuid.New().String()

	// Get filename for process tracking
	fileName := u.name
	if fileName == "" {
		fileName = "unknown"
	}

	// Start async preload
	entry := u.preload.StartPreload(preloadId, fileName)

	// Process file asynchronously
	go func() {
		addFile, err := u.addFile(entry.ctx)
		if err != nil {
			log.Errorf("preload file: %v", err)
		}
		u.preload.CompletePreload(preloadId, addFile, err)
	}()

	return preloadId, nil
}

func (u *uploader) getOrCreateFileObject(ctx context.Context, addResult *files.AddResult) (string, *domain.Details, error) {
	if addResult.IsExisting {
		id, details, err := u.fileObjectService.GetObjectDetailsByFileId(domain.FullFileId{
			SpaceId: u.spaceId,
			FileId:  addResult.FileId,
		})
		if err == nil {
			return id, details, nil
		}
		if errors.Is(err, filemodels.ErrObjectNotFound) {
			err = nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("get object details by file id: %w", err)
		}
	}

	fileObjectId, fileObjectDetails, err := u.fileObjectService.Create(ctx, u.spaceId, filemodels.CreateRequest{
		FileId:            addResult.FileId,
		EncryptionKeys:    addResult.EncryptionKeys.EncryptionKeys,
		ObjectOrigin:      u.origin,
		ImageKind:         u.imageKind,
		AdditionalDetails: u.additionalDetails,
		FileVariants:      addResult.Variants,
	})
	if err != nil {
		return "", nil, fmt.Errorf("create file object: %w", err)
	}
	return fileObjectId, fileObjectDetails, nil

}

func (u *uploader) detectType(buf *fileReader) (model.BlockContentFileType, error) {
	mime, err := mimetype.DetectReader(buf)
	_, seekErr := buf.Seek(0, io.SeekStart)
	if seekErr != nil {
		return 0, fmt.Errorf("seek: %w", err)
	}
	if err != nil {
		log.With("error", err).Error("detect MIME")
		return model.BlockContentFile_File, nil
	}
	return file.DetectTypeByMIME(u.name, mime.String()), nil
}

type FileComponent interface {
	UpdateFile(id, groupId string, apply func(b file.Block) error) (err error)
}

func (u *uploader) updateBlock() {
	if u.smartBlockID != "" && u.block != nil {
		err := cache.Do(u.picker, u.smartBlockID, func(f FileComponent) error {
			return f.UpdateFile(u.block.Model().Id, u.groupID, func(b file.Block) error {
				b.SetModel(u.block.Copy().Model().GetFile())
				return nil
			})
		})
		if err != nil {
			log.Warnf("upload file: can't update info: %v", err)
		}
	}
}

// Process interface implementation for preloadEntry

func (e *preloadEntry) Id() string {
	return e.id
}

func (e *preloadEntry) Cancel() error {
	e.cancel()
	return nil
}

func (e *preloadEntry) Info() pb.ModelProcess {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var done int64
	if e.state == pb.ModelProcess_Done {
		done = 1
	}
	return pb.ModelProcess{
		Id:    e.id,
		State: e.state,
		Progress: &pb.ModelProcessProgress{
			Total: 1,
			Done:  done,
		},
		Message: &pb.ModelProcessMessageOfPreloadFile{
			PreloadFile: &pb.ModelProcessPreloadFile{},
		},
	}
}

func (e *preloadEntry) Done() chan struct{} {
	return e.done
}

func (e *preloadEntry) complete(result *files.AddResult, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.result = result
	e.err = err
	if err != nil {
		e.state = pb.ModelProcess_Error
	} else {
		e.state = pb.ModelProcess_Done
	}
	close(e.done)
}
