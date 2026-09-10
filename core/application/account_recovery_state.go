package application

import (
	"github.com/anyproto/anytype-heart/core/recovery"
	"github.com/anyproto/anytype-heart/pb"
)

// AccountRecoveryState serves the folded account start-up status
// (Event.Account.Recovery.Snapshot). It is total and lock-free with respect
// to AccountSelect, which holds s.lock for its whole duration: a client may
// call it at any moment — before AccountSelect, racing it, or long after — and
// always gets an answer. Before any run, and again after a run that ended
// without a verdict (a cancelled start, or the app closed before Done), it is
// the idle snapshot (empty runId, phase NotStarted); there is no ordering a
// client has to get right, and no error a correctly behaving client can
// provoke.
func (s *Service) AccountRecoveryState() (*pb.EventAccountRecoverySnapshot, error) {
	if s.recovery == nil {
		// only a zero-value Service (tests); New() always sets the tracker
		return recovery.IdleSnapshot(), nil
	}
	return s.recovery.Snapshot(), nil
}
