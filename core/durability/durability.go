package durability

import (
	"time"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

const CName = "durability"

var log = logging.LoggerNotSugared(CName)

type Flusher interface {
	Flush(timeout time.Duration, waitPending bool)
}

type durability struct {
	spaceCore        Flusher
	anystoreProvider Flusher
}

func New() app.Component {
	return new(durability)
}

func (c *durability) Name() (name string) {
	return CName
}

func (c *durability) Init(a *app.App) (err error) {
	c.spaceCore = a.MustComponent(spacecore.CName).(Flusher)
	c.anystoreProvider = a.MustComponent(anystoreprovider.CName).(Flusher)
	return nil
}

func (s *durability) StateChange(state int) {
	switch domain.CompState(state) {
	case domain.CompStateAppClosingInitiated:
		// waitPending=false because we need to do best effort without locking the app closing
		// db component will perform final flush on closing after all writes are done
		// flush space stores first, because others we can reindex without data loss
		s.spaceCore.Flush(time.Second*3, false)
		s.anystoreProvider.Flush(time.Second*3, false)
	case domain.CompStateAppWentBackground:
		// we need to wait here because on mobile when app goes to background
		// when app goes to background(or hibernat on desktop) we need to be fast, but make sure we wait and have extended timeout in case of slow device and a huge WAL
		start := time.Now()
		s.spaceCore.Flush(time.Second*10, true)
		spaceCoreSpent := time.Since(start)
		start = time.Now()
		s.anystoreProvider.Flush(time.Second*10, true)
		anystoreSpent := time.Since(start)
		if spaceCoreSpent+anystoreSpent > time.Second {
			log.With(zap.Int64("spaceCoreSpentMs", spaceCoreSpent.Milliseconds()), zap.Int64("anystoreSpentMs", anystoreSpent.Milliseconds())).Warn("flushing took too long")
		}

	}
}
