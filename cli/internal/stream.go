package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	pb "github.com/anyproto/anytype-heart/pb"
)

type eventReceiver struct {
	lock   *sync.Mutex
	events []*pb.EventMessage
}

// ListenSessionEvents keeps the gRPC stream open and waits for an account ID event.
func ListenSessionEvents(token string) (string, error) {
	client, err := GetGRPCClient()
	if err != nil {
		return "", fmt.Errorf("failed to get gRPC client: %v", err)
	}

	// Start listening to session events
	req := &pb.StreamRequest{
		Token: token,
	}
	stream, err := client.ListenSessionEvents(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to start ListenSessionEvents stream: %v", err)
	}

	er := &eventReceiver{
		lock:   &sync.Mutex{},
		events: []*pb.EventMessage{},
	}

	// Start a goroutine to listen to the stream
	go func() {
		for {
			event, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				fmt.Println("🔄 Stream ended, reconnecting...")
				break
			}
			if err != nil {
				if err.Error() == "rpc error: code = Canceled desc = grpc: the client connection is closing" {
					// closed intentionally
					break
				}
				fmt.Printf("❌ Stream error: %v\n", err)
				break
			}

			// Store received messages
			er.lock.Lock()
			for _, m := range event.Messages {
				er.events = append(er.events, m)
			}
			er.lock.Unlock()
		}
	}()

	// Wait for an `accountShow` event
	accountID, err := waitAccountID(er)
	if err != nil {
		return "", fmt.Errorf("❌ Failed to get account ID: %v", err)
	}
	return accountID, nil
}

// waitAccountID continuously checks stored events and returns when an account ID is found.
func waitAccountID(er *eventReceiver) (string, error) {
	for {
		er.lock.Lock()
		for i := len(er.events) - 1; i >= 0; i-- { // Process recent events first
			m := er.events[i]
			if m == nil {
				continue
			}
			if v := m.GetAccountShow(); v != nil && v.GetAccount() != nil {
				accountID := v.GetAccount().Id
				er.events[i] = nil // Mark event as processed
				er.lock.Unlock()
				return accountID, nil
			}
		}
		er.lock.Unlock()
	}
}
