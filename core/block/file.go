package block

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

const (
	addFileWorkersCount = 4
)

func (p *commonSmart) Upload(id string, localPath, url string) (err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	f, err := s.getFile(id)
	if err != nil {
		return
	}
	if err = f.Upload(p.s.anytype, p, localPath, url); err != nil {
		return
	}
	return p.applyAndSendEventHist(s, false, true)
}

func (p *commonSmart) UpdateFileBlock(id string, apply func(f file.Block)) error {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	f, err := s.getFile(id)
	if err != nil {
		return err
	}
	apply(f)
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) DropFiles(req pb.RpcExternalDropFilesRequest) (err error) {
	process := &dropFilesProcess{s: p.s}
	if err = process.Init(req.LocalFilePaths); err != nil {
		return
	}
	var ch = make(chan error)
	go process.Start(p.GetId(), req.DropTargetId, req.Position, ch)
	err = <-ch
	return
}

func (p *commonSmart) dropFilesCreateStructure(targetId string, pos model.BlockPosition, entries []*dropFileEntry) (blockIds []string, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	s := p.newState()
	for _, entry := range entries {
		var blockId string
		if entry.isDir {
			blockId, err = p.create(s, pb.RpcBlockCreateRequest{
				ContextId: p.GetId(),
				TargetId:  targetId,
				Block: &model.Block{
					Fields: &types.Struct{
						Fields: map[string]*types.Value{
							"name": testStringValue(entry.name),
							"icon": testStringValue(":file_folder:"),
						},
					},
					Content: &model.BlockContentOfPage{
						Page: &model.BlockContentPage{
							Style: model.BlockContentPage_Empty,
						},
					},
				},
				Position: pos,
			})
			if err != nil {
				return
			}
			targetId = blockId
			pos = model.Block_Bottom
			blockId = s.get(blockId).Model().GetLink().TargetBlockId
		} else {
			fb, e := s.create(&model.Block{Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name: entry.name,
				},
			}})
			if e != nil {
				return nil, e
			}
			fb.(file.Block).SetState(model.BlockContentFile_Uploading)

			if err = p.insertTo(s, fb, targetId, pos); err != nil {
				return
			}
			blockId = fb.Model().Id
			targetId = blockId
			pos = model.Block_Bottom
		}
		blockIds = append(blockIds, blockId)
	}
	if err = p.applyAndSendEvent(s); err != nil {
		return
	}
	return
}

func (p *commonSmart) dropFilesSetInfo(info dropFileInfo) (err error) {
	return p.UpdateFileBlock(info.blockId, func(f file.Block) {
		if info.err != nil {
			log.Warningf("upload file[%v] error: %v", info.name, info.err)
			f.SetState(model.BlockContentFile_Error)
			return
		}
		fc := f.Model().GetFile()
		fc.Type = info.file.Type
		fc.Mime = info.file.Mime
		fc.Hash = info.file.Hash
		fc.Name = info.file.Name
		fc.State = info.file.State
		fc.Size_ = info.file.Size_
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
}

type dropFilesHandler interface {
	dropFilesCreateStructure(targetId string, pos model.BlockPosition, entries []*dropFileEntry) (blockIds []string, err error)
	dropFilesSetInfo(info dropFileInfo) (err error)
}

type dropFilesProcess struct {
	s           *service
	root        *dropFileEntry
	total, done int64
	cancel      chan struct{}
}

func (dp *dropFilesProcess) Init(paths []string) (err error) {
	dp.root = &dropFileEntry{}
	for _, path := range paths {
		entry := &dropFileEntry{path: path, name: filepath.Base(path)}
		ok, e := dp.readdir(entry, true)
		if e != nil {
			return e
		}
		if ok {
			dp.root.child = append(dp.root.child, entry)
			dp.total++
		}
	}
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

	if ! allowSymlinks && fi.Mode()&os.ModeSymlink == os.ModeSymlink {
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
	dp.cancel = make(chan struct{})
	return true, nil
}

func (dp *dropFilesProcess) Start(rootId, targetId string, pos model.BlockPosition, rootDone chan error) {
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

	var flatEntries = [][]*dropFileEntry{dp.root.child}
	var smartBockIds = []string{rootId}
	var handleLevel = func(idx int) (ok bool, err error) {
		if idx >= len(smartBockIds) {
			return
		}
		sb, release, err := dp.s.pickBlock(smartBockIds[idx])
		if err != nil {
			return
		}
		defer release()

		sbHandler, ok := sb.(dropFilesHandler)
		if !ok {
			return idx != 0, fmt.Errorf("unexpected smartblock interface %T; want dropFilesHandler", sb)
		}
		blockIds, err := sbHandler.dropFilesCreateStructure(targetId, pos, flatEntries[idx])
		if err != nil {
			return idx != 0, err
		}
		for i, entry := range flatEntries[idx] {
			if entry.isDir {
				smartBockIds = append(smartBockIds, blockIds[i])
				flatEntries = append(flatEntries, entry.child)
				atomic.AddInt64(&dp.done, 1)
			} else {
				in <- &dropFileInfo{
					pageId:  smartBockIds[idx],
					blockId: blockIds[i],
					path:    entry.path,
					name:    entry.name,
				}
			}
		}
		return true, nil
	}
	var idx = 0
	for {
		ok, err := handleLevel(idx)
		if idx == 0 {
			rootDone <- err
			if err != nil {
				log.Warningf("can't create files: %v", err)
				close(in)
				return
			}
			targetId = ""
			pos = 0
		}
		if err != nil {
			log.Warningf("can't create files: %v", err)
		}
		if ! ok {
			break
		}
		idx++
	}
	close(in)
	return
}

func (dp *dropFilesProcess) addFilesWorker(wg *sync.WaitGroup, in chan *dropFileInfo) {
	defer wg.Done()
	var canceled bool
	for {
		select {
		case <-dp.cancel:
			canceled = true
		case info, ok := <-in:
			if ! ok {
				return
			}
			log.Infof("received file %v", info.name)
			if canceled {
				info.err = context.Canceled
			} else {
				info.err = dp.addFile(info)
				log.Infof("add file file %v", info.err)
			}
			if err := dp.apply(info); err != nil {
				log.Warningf("can't apply file: %v", err)
			}
		}
	}
}

func (dp *dropFilesProcess) addFile(f *dropFileInfo) (err error) {
	var tempFile = file.NewFile(&model.Block{Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}).(file.Block)
	u := file.NewUploader(dp.s.anytype, func(f func(file file.Block)) {
		f(tempFile)
	})
	u.DoAuto(f.path)
	fc := tempFile.Model().GetFile()
	if fc.State != model.BlockContentFile_Done {
		f.err = fmt.Errorf("upload error")
		return
	}
	f.file = fc
	return
}

func (dp *dropFilesProcess) apply(f *dropFileInfo) (err error) {
	defer atomic.AddInt64(&dp.done, 1)
	log.Infof("apply file: %+v", f)
	sb, release, err := dp.s.pickBlock(f.pageId)
	if err != nil {
		return
	}
	defer release()

	sbHandler, ok := sb.(dropFilesHandler)
	if !ok {
		return fmt.Errorf("(apply) unexpected smartblock interface %T; want dropFilesHandler", sb)
	}

	return sbHandler.dropFilesSetInfo(*f)
}
