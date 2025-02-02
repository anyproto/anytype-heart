package internal

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// File to store the process ID
const pidFile = "/tmp/anytype_server.pid"
const logFilePath = "/tmp/anytype.log"

// StartServer runs the Anytype server and waits for the confirmation logs
func StartServer() error {
	status, err := IsGRPCServerRunning()
	if err != nil {
		return err
	}
	if _, err := os.Stat(pidFile); err == nil && status {
		return errors.New("server is already running")
	}

	grpcPort := "31007"
	grpcWebPort := "31008"

	cmd := exec.Command("../dist/server")
	cmd.Env = append(os.Environ(),
		"ANYTYPE_GRPC_ADDR=127.0.0.1:"+grpcPort,
		"ANYTYPE_GRPCWEB_ADDR=127.0.0.1:"+grpcWebPort,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the server process
	if err := cmd.Start(); err != nil {
		logError("Failed to start server: %v", err)
		return fmt.Errorf("failed to start server: %v", err)
	}

	go func() {
		cmd.Wait() // Block until process exits
		logInfo("🚪 Server process exited.")
	}()

	pid := cmd.Process.Pid
	pidData := fmt.Sprintf("%d:%s:%s", pid, grpcPort, grpcWebPort)
	err = os.WriteFile(pidFile, []byte(pidData), 0644)
	if err != nil {
		logError("Failed to save PID: %v", err)
		return fmt.Errorf("failed to save PID: %v", err)
	}

	return nil
}

// StopServer stops the running Anytype server and ensures it is fully terminated.
func StopServer() error {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return errors.New("server is not running")
	}

	// Parse "<pid>:<grpc_port>:<grpc_web_port>"
	dataParts := strings.Split(strings.TrimSpace(string(pidData)), ":")
	if len(dataParts) != 3 {
		return errors.New("invalid PID file format")
	}

	pid, err := strconv.Atoi(dataParts[0])
	if err != nil {
		return fmt.Errorf("failed to parse PID: %v", err)
	}

	grpcPort := dataParts[1]
	grpcWebPort := dataParts[2]

	// Kill process group
	err = syscall.Kill(-pid, syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to send SIGTERM: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Force kill if still running
	process, _ := os.FindProcess(pid)
	if err := process.Signal(syscall.Signal(0)); err == nil {
		fmt.Println("Process did not terminate, sending SIGKILL...")
		syscall.Kill(-pid, syscall.SIGKILL)
	}

	if isPortInUse(grpcPort) || isPortInUse(grpcWebPort) {
		return fmt.Errorf("server stopped, but ports %s and %s are still in use", grpcPort, grpcWebPort)
	}

	os.Remove(pidFile)
	return nil
}

// CheckServerStatus verifies if the server is running by checking the process and gRPC connectivity.
func CheckServerStatus() (string, error) {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return "😴 Server is not running", nil
	}

	// Parse "<pid>:<grpc_port>:<grpc_web_port>"
	dataParts := strings.Split(strings.TrimSpace(string(pidData)), ":")
	if len(dataParts) != 3 {
		return "Invalid PID file format", errors.New("invalid PID file format")
	}

	pid, err := strconv.Atoi(dataParts[0])
	if err != nil {
		return "", fmt.Errorf("failed to parse PID: %v", err)
	}

	grpcPort := dataParts[1]
	grpcWebPort := dataParts[2]

	// Check if the process with the PID is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return "😴 Server is not running", nil
	}

	// Validate if the process is really Anytype gRPC (Unix Only)
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		processName, err := getProcessName(pid)
		if processName != "" && (err != nil || !strings.Contains(processName, "/dist/server")) {
			return fmt.Sprintf("Process found (PID: %d) but it's not Anytype gRPC server: %s", pid, processName), nil
		}
	}

	// Check if the gRPC server is responding
	if isPortInUse(grpcPort) && isPortInUse(grpcWebPort) {
		return fmt.Sprintf("✓ Server is running (pid: %d) and grpc is responsive on port %s", process.Pid, grpcPort), nil
	}

	return fmt.Sprintf("⚠️ Process (PID: %d) is running but gRPC is not responding", process.Pid), nil
}

func isPortInUse(port string) bool {
	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		return false // Port is free
	}
	conn.Close()
	return true // Port is still occupied
}

func getProcessName(pid int) (string, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
