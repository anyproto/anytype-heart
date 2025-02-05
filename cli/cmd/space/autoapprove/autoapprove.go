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
		Short: "Start autoapproval of join requests for a space (runs in background)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("space id is required (use --space)")
			}
			if role == "" {
				return fmt.Errorf("role is required (use --role)")
			}

			// If not already running in daemon mode, spawn a detached process.
			if os.Getenv("AUTOAPPROVE_DAEMON") != "1" {
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("failed to get executable path: %w", err)
				}

				// Build the arguments for re‑invoking the command.
				// (Note: we prepend the executable path so that the process's argv[0] is the path.)
				newArgs := []string{exe, "space", "autoapprove", "--space", spaceID, "--role", role}

				// Open /dev/null so the detached process does not inherit our terminal.
				devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
				if err != nil {
					return fmt.Errorf("failed to open /dev/null: %w", err)
				}
				defer devNull.Close()

				// Prepare the process attributes.
				procAttr := &os.ProcAttr{
					Dir:   "",
					Env:   append(os.Environ(), "AUTOAPPROVE_DAEMON=1"),
					Files: []*os.File{devNull, devNull, devNull}, // detach standard input/output/error
				}
				process, err := os.StartProcess(exe, newArgs, procAttr)
				if err != nil {
					return fmt.Errorf("failed to start autoapprove process: %w", err)
				}
				fmt.Printf("Started autoapprove background process (PID: %d)\n", process.Pid)
				return nil
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

			fmt.Printf("Autoapprove daemon started for space %s with role %s\n", spaceID, role)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigs
				fmt.Println("Shutdown signal received, stopping autoapprove daemon...")
				cancel()
			}()

			// Also monitor the server status: if the server stops, cancel the loop.
			go func() {
				for {
					time.Sleep(5 * time.Second)
					status, err := internal.IsGRPCServerRunning()
					if err != nil || !status {
						fmt.Println("Server stopped; autoapprove daemon shutting down.")
						cancel()
						return
					}
				}
			}()

			// Main loop: poll for join request events and approve them.
			for {
				select {
				case <-ctx.Done():
					fmt.Println("Autoapprove daemon terminated.")
					return nil
				default:
					joinReq, err := internal.WaitForJoinRequestEvent(er, spaceID)
					if err != nil {
						fmt.Printf("Error waiting for join request: %v\n", err)
						time.Sleep(time.Second)
						continue
					}
					fmt.Printf("Approving join request from identity %s (%s)...\n", joinReq.Identity, joinReq.IdentityName)
					err = internal.ApproveJoinRequest(token, joinReq.SpaceId, joinReq.Identity, permissions)
					if err != nil {
						fmt.Printf("Failed to approve join request: %v\n", err)
					} else {
						fmt.Printf("Successfully approved join request for identity %s\n", joinReq.Identity)
					}
				}
			}
		},
	}

	autoapproveCmd.Flags().StringVar(&spaceID, "space", "", "ID of the space to monitor")
	autoapproveCmd.Flags().StringVar(&role, "role", "", "Role to assign to approved join requests (e.g., Editor, Viewer)")

	return autoapproveCmd
}
