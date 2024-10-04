package indexer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/taskmanager"
)

const (
	reindexTimeoutFirstAttempt = time.Second * 10 // next attempts will be increased by 2 times
	taskRetrySeparator         = "#"
)

func (i *indexer) getSpacesPriority() []string {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.spacesPriority
}

// reindexAddTask reindex all objects in the space that have outdated head hashes
func (i *indexer) reindexAddSpaceTask(space clientspace.Space) {
	task := i.reindexNewTask(space, 0)
	i.spaceReindexQueue.AddTask(task)
	go i.reindexWatchTaskAndRetry(task)
}

func (i *indexer) reindexWatchTaskAndRetry(task *reindexTask) {
	for {
		result, err := task.WaitResult(i.componentCtx)
		l := log.With("hadTimeouts", task.hadTimeouts).With("spaceId", task.space.Id()).With("tryNumber", task.tryNumber).With("total", task.total).With("invalidated", task.invalidated).With("succeed", task.success)
		if err != nil {
			l.Error("reindex failed", zap.Error(err))
			break
		} else {
			l = l.With("spentWorkMs", int(result.WorkTime.Milliseconds())).With("spentTotalMs", int(result.FinishTime.Sub(result.StartTime).Milliseconds()))
			if task.invalidated-task.success > 0 {
				l.Warn("reindex finished not fully")
				if task.hadTimeouts {
					// reschedule timeouted space task
					// it will be executed after all tasks with previous tryNumber are finished
					task = i.reindexNewTask(task.space, task.tryNumber+1)
					i.spaceReindexQueue.AddTask(task)
				} else {
					break
				}
			} else {
				if task.total > 0 {
					l.Warn("reindex finished")
				}
				break
			}
		}
	}
}

func (i *indexer) reindexNewTask(space clientspace.Space, tryNumber int) *reindexTask {
	taskId := fmt.Sprintf("%s%s%d", space.Id(), taskRetrySeparator, tryNumber)
	return &reindexTask{
		TaskBase:  taskmanager.NewTaskBase(taskId),
		space:     space,
		store:     i.store,
		indexer:   i,
		tryNumber: tryNumber,
	}
}

// taskPrioritySorter sort taskIds
// - first by the number of the try (0, 1, 2, ...)
// - then by the space priority. if space priority is not set for the space, it put to the end
func (i *indexer) reindexTasksSorter(taskIds []string) []string {
	priority := i.getSpacesPriority()
	// Sort the filtered task IDs based on retry attempts and space priority
	sort.Slice(taskIds, func(a, b int) bool {
		spaceA, tryA := reindexTaskId(taskIds[a]).Parse()
		spaceB, tryB := reindexTaskId(taskIds[b]).Parse()

		// First, sort by retry attempts (lower retries have higher priority)
		if tryA != tryB {
			return tryA < tryB
		}

		// Then, sort by the index in spacesPriority (earlier spaces have higher priority)
		indexA := slices.Index(priority, spaceA)
		indexB := slices.Index(priority, spaceB)

		if indexA == -1 && indexB == -1 {
			// to make it stable
			return spaceA < spaceB
		}
		if indexA == -1 {
			return false
		}
		if indexB == -1 {
			return true
		}

		return indexA < indexB
	})
	return taskIds
}

type reindexTask struct {
	taskmanager.TaskBase
	space       clientspace.Space
	store       objectstore.ObjectStore
	indexer     *indexer
	total       int
	invalidated int
	success     int
	tryNumber   int
	hadTimeouts bool
}

func (t *reindexTask) Timeout() time.Duration {
	return reindexTimeoutFirstAttempt * time.Duration(1<<t.tryNumber)
}

func (t *reindexTask) Run(ctx context.Context) error {
	objectIds := t.space.StoredIds()
	var err error
	t.total = len(objectIds)
	// priorities indexing of system objects
	priorityIds, err := t.indexer.getIdsForTypes(t.space, coresb.SmartBlockTypeObjectType, coresb.SmartBlockTypeRelation, coresb.SmartBlockTypeParticipant)
	if err != nil {
		log.Errorf("reindexOutdatedObjects failed to get priority ids: %s", err)
	} else {
		objectIds = append(priorityIds, slice.Difference(objectIds, priorityIds)...)
	}

	// todo: query lastIndexedHeadHashes for all tids
	for _, objectId := range objectIds {
		err = t.WaitIfPaused(ctx)
		if err != nil {
			return err
		}
		logErr := func(err error) {
			log.With("objectId", objectId).Errorf("reindexOutdatedObjects failed to get tree to reindex: %s", err)
		}

		lastHash, err := t.store.GetLastIndexedHeadsHash(objectId)
		if err != nil {
			logErr(err)
			continue
		}
		info, err := t.space.Storage().TreeStorage(objectId)
		if err != nil {
			logErr(err)
			continue
		}
		heads, err := info.Heads()
		if err != nil {
			logErr(err)
			continue
		}

		hh := headsHash(heads)
		if lastHash == hh {
			continue
		}

		if lastHash != "" {
			log.With("objectId", objectId).Warnf("not equal indexed heads hash: %s!=%s (%d logs)", lastHash, hh, len(heads))
		}
		t.invalidated++

		indexTimeout, cancel := context.WithTimeout(ctx, t.Timeout())
		err = t.indexer.reindexDoc(indexTimeout, t.space, objectId)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.hadTimeouts = true
			}
			logErr(err)
			continue
		}
		t.success++
	}
	return nil
}

type reindexTaskId string

func (t reindexTaskId) Parse() (spaceId string, try int) {
	s := strings.Split(string(t), taskRetrySeparator)
	if len(s) == 1 {
		return s[0], 0
	}
	retry, _ := strconv.ParseInt(s[1], 10, 64)
	return s[0], int(retry)
}
