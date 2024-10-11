//go:build !anydebug

package subscription

import "github.com/anyproto/anytype-heart/pb"

type subDebugger struct {
}

func (s *service) initDebugger() {
}

func (s *service) debugEvents(ev *pb.Event) {
}
