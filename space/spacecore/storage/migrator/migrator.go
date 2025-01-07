package migrator

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

const CName = "client.storage.migrator"

type migrator struct {
	processService process.Service
}

func New() app.ComponentRunnable {
	return &migrator{}
}

func (m *migrator) Init(a *app.App) (err error) {
	m.processService = app.MustComponent[process.Service](a)
	return nil
}

func (m *migrator) Name() (name string) {
	return CName
}

func (m *migrator) Run(ctx context.Context) (err error) {
	progress := process.NewProgress(&pb.ModelProcessMessageOfMigration{Migration: &pb.ModelProcessMigration{}})
	err = m.processService.Add(progress)
	if err != nil {
		return err
	}
	progress.SetProgressMessage("Migration started")
	progress.SetTotal(1000)
	for i := 0; i < 1000; i++ {
		progress.AddDone(1)
		time.Sleep(10 * time.Millisecond)
	}
	progress.Finish(nil)
	return nil
}

func (m *migrator) Close(ctx context.Context) (err error) {
	return nil
}
