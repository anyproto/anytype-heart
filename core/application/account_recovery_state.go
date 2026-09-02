package application

import (
	"github.com/anyproto/anytype-heart/pb"
)

// AccountRecoveryState serves the folded account start-up status
// (Event.Account.Recovery.Snapshot). It must stay lock-free with respect to
// AccountSelect, which holds s.lock for its whole duration: the snapshot is
// most valuable exactly while that RPC blocks.
func (s *Service) AccountRecoveryState() (*pb.EventAccountRecoverySnapshot, error) {
	if s.recovery == nil {
		return nil, ErrApplicationIsNotRunning
	}
	snapshot := s.recovery.Snapshot()
	if snapshot == nil {
		return nil, ErrApplicationIsNotRunning
	}
	return snapshot, nil
}
