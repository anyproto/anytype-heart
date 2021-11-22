package threads

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/thread"
	"strings"
	"sync"
	"time"
)

var queueLog = logging.Logger("anytype-threadqueue")

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
	Run()
	ProcessThreadsAsync(threadsFromState []ThreadInfo, workspaceId string)
	AddThreadSync(info ThreadInfo, workspaceId string) error
	CreateThreadSync(blockType smartblock.SmartBlockType, workspaceId string) (thread.Info, error)
	DeleteThreadSync(id, workspaceId string) error
	GetWorkspacesForThread(threadId string) []string
	GetThreadsForWorkspace(workspaceId string) []string
	UpdatePriority(ids []string, priority int)
}

type ThreadOperation struct {
	IsAddOperation bool
	ID             string
	WorkspaceId    string
	info           ThreadInfo
}

type threadQueue struct {
	sync.Mutex
	workspaceThreads  map[string]map[string]struct{}
	threadWorkspaces  map[string]map[string]struct{}
	threadsService    *service
	threadStore       ThreadWorkspaceStore
	operationsBuffer  []Operation
	wakeupChan        chan struct{}
	l                 *limiterPool
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
		threadsService: s,
		threadStore:    store,
		wakeupChan:     make(chan struct{}, 1),
		l:              newLimiterPool(s.ctx, s.simultaneousRequests),
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
	go p.l.run()
	go func() {
		for {
			select {
			case <-p.wakeupChan:
				p.processBufferedEvents()
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

func (p *threadQueue) UpdatePriority(ids []string, priority int) {
	p.l.UpdatePriorities(ids, priority)
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

	for _, info := range addedThreads {
		p.operationsBuffer = append(p.operationsBuffer, p.NewThreadAddOperation(info.ID, workspaceId, info))
	}
	for _, id := range deletedThreads {
		p.operationsBuffer = append(p.operationsBuffer, p.NewThreadDeleteOperation(id, workspaceId))
	}
	p.Unlock()
	select {
	case p.wakeupChan <- struct{}{}:
	default:
	}
}

func (p *threadQueue) processBufferedEvents() {
	p.Lock()
	var operationsCopy []Operation
	for _, op := range p.operationsBuffer {
		operationsCopy = append(operationsCopy, op)
	}
	p.operationsBuffer = nil
	p.Unlock()

	p.l.AddOperations(operationsCopy, DefaultPriority)
}

func (p *threadQueue) finishDeleteOperation(id, workspaceId string) {
	// we leave it here instead of moving to block service
	// because if this operation fails we would want to retry it
	// and we can do that only if we still have the entry in threadStore not removed
	err := p.threadsService.objectStoreDeleter.DeleteObject(id)
	if err != nil {
		log.Errorf("error deleting object from store %s %s %v", id, workspaceId, err.Error())
	}

	// it is important that we remove thread from workspace only if everything is fine
	err = p.threadStore.RemoveThreadForWorkspace(id, workspaceId)
	if err != nil {
		log.Errorf("error removing thread from store %s %s %v", id, workspaceId, err.Error())
		return
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

func (p *threadQueue) logOperation(op Operation, success bool, workspaceId string) {
	p.Lock()
	defer p.Unlock()
	threadsInWorkspace, exists := p.workspaceThreads[workspaceId]
	if !exists {
		return
	}
	totalThreadsOverall := len(p.threadWorkspaces) + p.l.PendingOperations()
	l := queueLog.With("thread id", op.Id()).With("workspace id", workspaceId)

	if success {
		l.Infof("downloaded new thread to workspace (now %d, including user profiles), %d of %d threads downloaded",
			len(threadsInWorkspace),
			len(p.threadWorkspaces),
			totalThreadsOverall)
	} else {
		l.Errorf("failed to download new thread to workspace (now %d, including user profiles), %d of %d threads downloaded",
			len(threadsInWorkspace),
			len(p.threadWorkspaces),
			totalThreadsOverall)
	}
}

type threadAddOperation struct {
	ID             string
	WorkspaceId    string
	info           ThreadInfo
	threadsService *service
	queue          *threadQueue
}

func (p *threadQueue) NewThreadAddOperation(id string, workspaceId string, info ThreadInfo) Operation {
	return threadAddOperation{
		ID:             id,
		WorkspaceId:    workspaceId,
		info:           info,
		threadsService: p.threadsService,
		queue:          p,
	}
}

func (o threadAddOperation) Id() string {
	return o.ID
}

func (o threadAddOperation) IsRetriable() bool {
	return true
}

func (o threadAddOperation) Run() (err error) {
	id, err := thread.Decode(o.ID)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	_, err = o.threadsService.t.GetThread(ctx, id)
	cancel()
	if err == nil {
		return
	}

	if err != nil && err != logstore.ErrThreadNotFound {
		log.With("thread", o.ID).
			Errorf("error getting thread while processing: %v", err)
		return
	}

	return o.threadsService.processNewExternalThread(id, o.info, false)
}

func (o threadAddOperation) OnFinish(err error) {
	defer o.queue.logOperation(o, err == nil, o.WorkspaceId)
	if err == nil {
		o.queue.finishAddOperation(o.ID, o.WorkspaceId)
		return
	}
	log.Errorf("could not add object with object id %s : %v", o.ID, err)
}

func (o threadAddOperation) Type() string {
	return "add"
}

type threadDeleteOperation struct {
	ID             string
	WorkspaceId    string
	threadsService *service
	queue          *threadQueue
}

func (p *threadQueue) NewThreadDeleteOperation(id string, workspaceId string) Operation {
	return threadDeleteOperation{
		ID:             id,
		WorkspaceId:    workspaceId,
		threadsService: p.threadsService,
		queue:          p,
	}
}

func (o threadDeleteOperation) Id() string {
	return o.ID
}

func (o threadDeleteOperation) IsRetriable() bool {
	return false
}

func (o threadDeleteOperation) Run() (err error) {
	// this is more than just deleting a thread as opposed to DeleteThreadSync
	// because we are calling DeleteThreadSync from blockService :-)
	// and here we are calling blockService so that it will do a bunch of stuff and then call DeleteThreadSync
	// it's confusing I know
	err = o.threadsService.blockServiceObjectDeleter.DeleteObject(o.ID)
	return
}

func (o threadDeleteOperation) OnFinish(err error) {
	if err != nil {
		if strings.Contains(err.Error(), "block not found") {
			return
		} else {
			log.Errorf("could not delete object with object id %s : %v", o.ID, err)
		}
	}
}

func (o threadDeleteOperation) Type() string {
	return "delete"
}