//go:build !anydebug

package subscription

import "github.com/anyproto/anytype-heart/pb"

type subDebugger struct {
}

func (s *spaceSubscriptions) initDebugger() {
}

func (s *spaceSubscriptions) debugEvents(ev *pb.Event) {
}
