package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/cli/internal"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func AutoapproveTask(ctx context.Context, spaceID, role string) error {
	var permissions model.ParticipantPermissions
	switch role {
	case "Editor":
		permissions = model.ParticipantPermissions_Writer
	case "Viewer":
		fallthrough
	default:
		permissions = model.ParticipantPermissions_Reader
	}

	token, err := internal.GetStoredToken()
	if err != nil || token == "" {
		return fmt.Errorf("failed to get stored token; are you logged in?")
	}

	er, err := internal.ListenForEvents(token)
	if err != nil {
		return fmt.Errorf("failed to start event listener: %w", err)
	}

	// Optionally, monitor the server status.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				status, err := internal.IsGRPCServerRunning()
				if err != nil || !status {
					return
				}
			}
		}
	}()

	// Main loop: poll for join request events and approve them.
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			joinReq, err := internal.WaitForJoinRequestEvent(er, spaceID)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			if err := internal.ApproveJoinRequest(token, joinReq.SpaceId, joinReq.Identity, permissions); err != nil {
				fmt.Println("Failed to approve join request: %v", err)
			} else {
				fmt.Println("Successfully approved join request for identity %s", joinReq.Identity)
			}
		}
	}
}
