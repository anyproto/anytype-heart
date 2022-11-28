package test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pb/service"
)

type eventReceiver struct {
	lock   *sync.Mutex
	events []*pb.EventMessage
	// events chan<- *pb.EventMessage
}

func startEventReceiver(ctx context.Context, c service.ClientCommands_ListenSessionEventsClient) *eventReceiver {
	er := &eventReceiver{
		lock: &sync.Mutex{},
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				ev, err := c.Recv()
				if err != nil {
					fmt.Println("receive error:", err)
					continue
				}
				er.lock.Lock()
				for _, m := range ev.Messages {
					er.events = append(er.events, m)
				}
				er.lock.Unlock()
			}
		}
	}()
	return er
}

func waitEvent[t pb.IsEventMessageValue](er *eventReceiver, fn func(x t)) {
	for {
		er.lock.Lock()
		for i := len(er.events) - 1; i >= 0; i-- {
			m := er.events[i]
			if m == nil {
				continue
			}
			if v, ok := m.Value.(t); ok {
				fn(v)
				er.events[i] = nil
				er.lock.Unlock()
				return
			}
		}
		er.lock.Unlock()

		time.Sleep(10 * time.Millisecond)
	}
}
