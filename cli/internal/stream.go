package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	pb "github.com/anyproto/anytype-heart/pb"
)

// EventReceiver is a universal receiver that collects all incoming event messages.
type EventReceiver struct {
	lock   *sync.Mutex
	events []*pb.EventMessage
}

// ListenForEvents starts the gRPC stream for events using the provided token.
// It returns an EventReceiver that will store all incoming events.
func ListenForEvents(token string) (*EventReceiver, error) {
	client, err := GetGRPCClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get gRPC client: %v", err)
	}

	req := &pb.StreamRequest{
		Token: token,
	}
	stream, err := client.ListenSessionEvents(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to start event stream: %v", err)
	}

	er := &EventReceiver{
		lock:   &sync.Mutex{},
		events: make([]*pb.EventMessage, 0),
	}

	// Start a goroutine to continuously receive events from the stream.
	go func() {
		for {
			event, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				fmt.Println("🔄 Event stream ended, reconnecting...")
				break
			}
			if err != nil {
				// Check for intentional close
				if err.Error() == "rpc error: code = Canceled desc = grpc: the client connection is closing" {
					break
				}
				fmt.Printf("X Event stream error: %v\n", err)
				break
			}

			er.lock.Lock()
			for _, m := range event.Messages {
				er.events = append(er.events, m)
			}
			er.lock.Unlock()
		}
	}()

	return er, nil
}

// WaitForAccountID continuously checks the stored events until an accountShow event is found.
// It returns the account ID from that event.
func WaitForAccountID(er *EventReceiver) (string, error) {
	for {
		er.lock.Lock()
		// Process recent events first.
		for i := len(er.events) - 1; i >= 0; i-- {
			m := er.events[i]
			if m == nil {
				continue
			}
			if v := m.GetAccountShow(); v != nil && v.GetAccount() != nil {
				accountID := v.GetAccount().Id
				// Mark event as processed.
				er.events[i] = nil
				er.lock.Unlock()
				return accountID, nil
			}
		}
		er.lock.Unlock()
		time.Sleep(100 * time.Millisecond) // Sleep briefly to avoid busy waiting.
	}
}
