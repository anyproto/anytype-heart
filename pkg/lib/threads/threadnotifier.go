package threads

import (
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type ThreadDownloadNotifier interface {
	Start(process.Service)
	SetTotalThreads(int)
	AddThread()
	Finish()
}

type noOpThreadNotifier struct{}

func (n *noOpThreadNotifier) Start(p process.Service) {}

func (n *noOpThreadNotifier) SetTotalThreads(i int) {}

func (n *noOpThreadNotifier) AddThread() {}

func (n *noOpThreadNotifier) Finish() {}

func NewNoOpNotifier() ThreadDownloadNotifier {
	return &noOpThreadNotifier{}
}

type accountRecoveryThreadNotifier struct {
	progress             *process.Progress
	startTime            time.Time
	threadsTotal         int
	simultaneousRequests int
}

func NewAccountNotifier(simultaneousRequests int) ThreadDownloadNotifier {
	return &accountRecoveryThreadNotifier{
		simultaneousRequests: simultaneousRequests,
	}
}

func (a *accountRecoveryThreadNotifier) Start(p process.Service) {
	a.progress = process.NewProgress(pb.ModelProcess_RecoverAccount)
	p.Add(a.progress)
	go func() {
		select {
		case <-a.progress.Canceled():
			a.progress.Finish()
		case <-a.progress.Done():
			return
		}
	}()
	a.progress.SetProgressMessage("recovering account")
	a.startTime = time.Now()
}

func (a *accountRecoveryThreadNotifier) SetTotalThreads(total int) {
	a.progress.SetTotal(int64(total))
	a.threadsTotal = total
}

func (a *accountRecoveryThreadNotifier) AddThread() {
	a.progress.AddDone(1)
}

func (a *accountRecoveryThreadNotifier) Finish() {
	log.Info("finished recovering account")
	metrics.SharedClient.RecordEvent(metrics.AccountRecoverEvent{
		SpentMs:              int(time.Now().Sub(a.startTime).Milliseconds()),
		TotalThreads:         a.threadsTotal,
		SimultaneousRequests: a.simultaneousRequests,
	})
	a.progress.Finish()
}
