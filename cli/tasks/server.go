package tasks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ServerTask is a background task that starts the server process.
// It spawns the server and waits until the given context is canceled.
func ServerTask(ctx context.Context) error {
	grpcPort := "31007"
	grpcWebPort := "31008"

	cmd := exec.Command("../dist/server")
	cmd.Env = append(os.Environ(),
		"ANYTYPE_GRPC_ADDR=127.0.0.1:"+grpcPort,
		"ANYTYPE_GRPCWEB_ADDR=127.0.0.1:"+grpcWebPort,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Run a goroutine to wait for the process to exit.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait until either the task context is canceled or the process exits.
	select {
	case <-ctx.Done():
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		return <-done
	case err := <-done:
		return err
	}
}
