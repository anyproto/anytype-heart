package threads

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/thread"
	"sync"
	"time"
)

type ThreadQueueState struct {
	workspaceThreads map[string]map[string]struct{}
	threadWorkspaces map[string]map[string]struct{}
}

type ThreadWorkspaceStore interface {
	AddThreadToWorkspace(threadId, workspaceId string) error
	RemoveThreadForWorkspace(threadId, workspaceId string) error
	GetThreadQueueState() (map[string]map[string]struct{}, map[string]map[string]struct{}, error)
}

type ThreadQueue interface {
	Init() error
	ProcessThreadsAsync(threadsFromState []ThreadInfo, workspaceId string)
	AddThreadSync(info ThreadInfo, workspaceId string) error
	CreateThreadSync(blockType smartblock.SmartBlockType, workspaceId string) (thread.Info, error)
	DeleteThreadSync(id, workspaceId string) error
	GetWorkspacesForThread(threadId string) []string
	GetThreadsForWorkspace(workspaceId string) []string
}

type ThreadOperation struct {
	IsAddOperation bool
	ID             string
	WorkspaceId    string
	info           *ThreadInfo
}

type threadQueue struct {
	sync.Mutex
	workspaceThreads  map[string]map[string]struct{}
	threadWorkspaces  map[string]map[string]struct{}
	threadsService    *service
	threadStore       ThreadWorkspaceStore
	operationsBuffer  []ThreadOperation
	currentOperations map[string]ThreadOperation
	operationsMutex   sync.Mutex
	wakeupChan        chan struct{}
}

func (p *threadQueue) GetWorkspacesForThread(threadId string) []string {
	p.Lock()
	defer p.Unlock()
	var objects []string
	threadsKV, exists := p.threadWorkspaces[threadId]
	if !exists {
		return nil
	}
	for id := range threadsKV {
		objects = append(objects, id)
	}
	return objects
}

func (p *threadQueue) GetThreadsForWorkspace(workspaceId string) []string {
	p.Lock()
	defer p.Unlock()
	var objects []string
	workspaceKV, exists := p.workspaceThreads[workspaceId]
	if !exists {
		return nil
	}
	for id := range workspaceKV {
		objects = append(objects, id)
	}
	return objects
}

func NewThreadQueue(s *service, store ThreadWorkspaceStore) ThreadQueue {
	return &threadQueue{
		threadsService:    s,
		threadStore:       store,
		wakeupChan:        make(chan struct{}, 1),
		currentOperations: map[string]ThreadOperation{},
	}
}

func (p *threadQueue) Init() error {
	workspaceThreads, threadWorkspaces, err := p.threadStore.GetThreadQueueState()
	if err != nil {
		return err
	}
	p.workspaceThreads = workspaceThreads
	p.threadWorkspaces = threadWorkspaces
	return nil
}

func (p *threadQueue) Run() {
	go func() {
		for {
			select {
			case <-p.wakeupChan:
				go p.processBufferedEvents()
			case <-p.threadsService.ctx.Done():
				return
			}
		}
	}()
}

func (p *threadQueue) AddThreadSync(info ThreadInfo, workspaceId string) error {
	err := p.threadsService.AddThread(info.ID, info.Key, info.Addrs)
	if err != nil {
		return err
	}
	p.finishAddOperation(info.ID, workspaceId)
	return err
}

func (p *threadQueue) CreateThreadSync(blockType smartblock.SmartBlockType, workspaceId string) (thread.Info, error) {
	info, err := p.threadsService.CreateThread(blockType)
	if err != nil {
		return thread.Info{}, err
	}
	p.finishAddOperation(info.ID.String(), workspaceId)
	return info, nil
}

func (p *threadQueue) DeleteThreadSync(id, workspaceId string) error {
	err := p.threadsService.DeleteThread(id)
	if err != nil && err != logstore.ErrThreadNotFound {
		return err
	}
	p.finishDeleteOperation(id, workspaceId)
	return nil
}

func (p *threadQueue) ProcessThreadsAsync(threadsFromState []ThreadInfo, workspaceId string) {
	p.Lock()
	workspaceKV, ok := p.workspaceThreads[workspaceId]
	if !ok {
		workspaceKV = make(map[string]struct{}, len(threadsFromState))
		p.workspaceThreads[workspaceId] = workspaceKV
	}
	currentThreads := make(map[string]struct{})
	addedThreads := make(map[string]ThreadInfo)
	var deletedThreads []string

	for _, currentThread := range threadsFromState {
		id := currentThread.ID
		currentThreads[id] = struct{}{}
		if _, exists := workspaceKV[id]; exists {
			continue
		}
		addedThreads[id] = currentThread
	}

	for id := range workspaceKV {
		if _, existsInCurrent := currentThreads[id]; !existsInCurrent {
			if threadWorkspace, existsThreadWorkspace := p.threadWorkspaces[id]; existsThreadWorkspace {
				// if there is only this workspace
				if len(threadWorkspace) <= 1 {
					deletedThreads = append(deletedThreads, id)
				}
			}
		}
	}
	p.Unlock()

	p.operationsMutex.Lock()
	for _, info := range addedThreads {
		if _, exists := p.currentOperations[info.ID]; exists {
			continue
		}
		p.operationsBuffer = append(p.operationsBuffer, ThreadOperation{
			IsAddOperation: true,
			ID:             info.ID,
			info:           &info,
			WorkspaceId:    workspaceId,
		})
	}
	for _, id := range deletedThreads {
		if _, exists := p.currentOperations[id]; exists {
			continue
		}
		p.operationsBuffer = append(p.operationsBuffer, ThreadOperation{
			IsAddOperation: false,
			ID:             id,
			WorkspaceId:    workspaceId,
		})
	}
	p.operationsMutex.Unlock()
	select {
	case p.wakeupChan <- struct{}{}:
	default:
	}
}

func (p *threadQueue) processBufferedEvents() {
	p.operationsMutex.Lock()
	var operationsCopy []ThreadOperation
	for _, op := range p.operationsBuffer {
		operationsCopy = append(operationsCopy, op)
		p.currentOperations[op.ID] = op
	}
	p.operationsMutex.Unlock()

	for _, op := range operationsCopy {
		if op.IsAddOperation {
			p.processAddedThread(op.info, op.WorkspaceId)
		} else {
			p.processDeletedThread(op.ID, op.WorkspaceId)
		}
	}
}

func (p *threadQueue) processAddedThread(ti *ThreadInfo, workspaceId string) {
	id, err := thread.Decode(ti.ID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	info, err := p.threadsService.t.GetThread(ctx, id)
	cancel()
	if err == nil {
		p.removeFromOperations(id.String())
		return
	}

	if err != nil && err != logstore.ErrThreadNotFound {
		log.With("thread", info.ID.String()).
			Errorf("error getting thread while processing: %v", err)
		p.removeFromOperations(id.String())
		return
	}

	metrics.ExternalThreadReceivedCounter.Inc()
	// TODO: check if we still need to have separate goroutine logic
	go func() {
		defer p.removeFromOperations(id.String())
		err := p.threadsService.processNewExternalThreadUntilSuccess(id, *ti)
		if err != nil {
			log.With("thread", info.ID.String()).
				Errorf("error processing thread: %v", err)
			return
		}
		p.finishAddOperation(id.String(), workspaceId)
	}()
}

func (p *threadQueue) processDeletedThread(id, workspaceId string) {
	go func() {
		select {
		case <-p.threadsService.ctx.Done():
			return
		default:
		}
		defer p.removeFromOperations(id)
		err := p.threadsService.DeleteThread(id)
		if err != nil && err != logstore.ErrThreadNotFound {
			log.Errorf("error deleting thread %s %s %v", id, workspaceId, err.Error())
		}
		p.finishDeleteOperation(id, workspaceId)
	}()
}

func (p *threadQueue) finishDeleteOperation(id, workspaceId string) {
	err := p.threadStore.RemoveThreadForWorkspace(id, workspaceId)
	if err != nil {
		log.Errorf("error removing thread from store %s %s %v", id, workspaceId, err.Error())
		return
	}

	err = p.threadsService.objectDeleter.DeleteObject(id)
	if err != nil {
		log.Errorf("error deleting object from store %s %s %v", id, workspaceId, err.Error())
	}

	p.Lock()
	workspaceKV, exists := p.workspaceThreads[workspaceId]
	if exists {
		delete(workspaceKV, id)
	}
	threadsKV, exists := p.threadWorkspaces[id]
	if exists {
		delete(threadsKV, workspaceId)
	}
	p.Unlock()
}

func (p *threadQueue) finishAddOperation(id, workspaceId string) {
	err := p.threadStore.AddThreadToWorkspace(id, workspaceId)
	if err != nil {
		log.Errorf("error adding thread to store %s %s %v", id, workspaceId, err.Error())
		return
	}

	p.Lock()
	workspaceKV, exists := p.workspaceThreads[workspaceId]
	if !exists {
		workspaceKV = make(map[string]struct{})
		p.workspaceThreads[workspaceId] = workspaceKV
	}
	workspaceKV[id] = struct{}{}
	threadsKV, exists := p.threadWorkspaces[id]
	if !exists {
		threadsKV = make(map[string]struct{})
		p.threadWorkspaces[id] = threadsKV
	}
	threadsKV[workspaceId] = struct{}{}
	p.Unlock()
}

func (p *threadQueue) removeFromOperations(id string) {
	p.operationsMutex.Lock()
	delete(p.currentOperations, id)
	p.operationsMutex.Unlock()
}
