package autoapprove

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/anyproto/anytype-heart/cli/internal"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func NewAutoapproveCmd() *cobra.Command {
	var spaceID string
	var role string

	autoapproveCmd := &cobra.Command{
		Use:   "autoapprove",
		Short: "Automatically approve join requests for a space",
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("space id is required (use --space)")
			}

			if role == "" {
				return fmt.Errorf("role is required (use --role)")
			}

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
			fmt.Printf("Listening for join requests for space %s...\n", spaceID)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create a channel that will be closed when the background listener exits.
			doneCh := make(chan struct{})

			// Launch the join-approval listener in a background goroutine.
			go func() {
				defer close(doneCh)
				for {
					select {
					case <-ctx.Done():
						fmt.Println("Autoapprove listener shutting down.")
						return
					default:
						// Wait for a join request event for the given space.
						joinReq, err := internal.WaitForJoinRequestEvent(er, spaceID)
						if err != nil {
							fmt.Printf("Error waiting for join request: %v\n", err)
							time.Sleep(time.Second)
							continue
						}
						// Process the join request.
						fmt.Printf("Approving join request from identity %s (%s)...\n", joinReq.Identity, joinReq.IdentityName)
						if err := internal.ApproveJoinRequest(token, joinReq.SpaceId, joinReq.Identity, permissions); err != nil {
							fmt.Printf("Failed to approve join request: %v\n", err)
						} else {
							fmt.Printf("Successfully approved join request for identity %s\n", joinReq.Identity)
						}
					}
				}
			}()

			// Set up signal handling to cancel the background listener on termination.
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigs
				fmt.Println("Shutdown signal received.")
				cancel()
			}()

			// Also, monitor the server status periodically. If the server stops,
			// cancel the background listener.
			go func() {
				for {
					time.Sleep(5 * time.Second)
					status, err := internal.IsGRPCServerRunning()
					if err != nil || !status {
						fmt.Println("Server appears to have stopped, canceling autoapprove listener.")
						cancel()
						return
					}
				}
			}()

			// Wait until the background listener finishes.
			<-doneCh
			fmt.Println("Autoapprove process terminated.")
			return nil
		},
	}

	autoapproveCmd.Flags().StringVar(&spaceID, "space", "", "ID of the space to monitor")
	autoapproveCmd.Flags().StringVar(&role, "role", "", "Role to assign to approved join requests (e.g., Editor, Viewer)")

	return autoapproveCmd
}
