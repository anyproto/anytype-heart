package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anyproto/anytype-heart/cli/tasks"
)

// TaskRequest is used by the HTTP API.
type TaskRequest struct {
	Task   string            `json:"task"`
	Params map[string]string `json:"params,omitempty"`
}

// TaskResponse is returned by the HTTP API.
type TaskResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Manager wraps the in‑memory task manager and exposes HTTP handlers.
type Manager struct {
	taskManager *TaskManager
	mux         *http.ServeMux
}

// NewManager returns a new Manager.
func NewManager() *Manager {
	m := &Manager{
		taskManager: GetTaskManager(),
		mux:         http.NewServeMux(),
	}
	m.routes()
	return m
}

// routes sets up the HTTP endpoints.
func (m *Manager) routes() {
	m.mux.HandleFunc("/task/start", m.handleStartTask)
	m.mux.HandleFunc("/task/stop", m.handleStopTask)
	m.mux.HandleFunc("/task/status", m.handleStatusTask)
}

// ServeHTTP satisfies the http.Handler interface.
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mux.ServeHTTP(w, r)
}

// handleStartTask processes a POST request to start a task.
func (m *Manager) handleStartTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	switch req.Task {
	case "server":
		err = m.taskManager.StartTask("server", tasks.ServerTask)
	case "autoapprove":
		spaceID, ok := req.Params["space"]
		if !ok || spaceID == "" {
			http.Error(w, "missing 'space' param", http.StatusBadRequest)
			return
		}
		role := req.Params["role"]
		taskID := "autoapprove-" + spaceID
		err = m.taskManager.StartTask(taskID, func(ctx context.Context) error {
			return tasks.AutoapproveTask(ctx, spaceID, role)
		})
	default:
		http.Error(w, "unknown task", http.StatusBadRequest)
		return
	}

	resp := TaskResponse{}
	if err != nil {
		resp.Status = "error"
		resp.Error = err.Error()
	} else {
		resp.Status = "started"
	}
	json.NewEncoder(w).Encode(resp)
}

// handleStopTask processes a POST request to stop a task.
func (m *Manager) handleStopTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	switch req.Task {
	case "server":
		err = m.taskManager.StopTask("server")
	case "autoapprove":
		spaceID, ok := req.Params["space"]
		if !ok || spaceID == "" {
			http.Error(w, "missing 'space' param", http.StatusBadRequest)
			return
		}
		taskID := "autoapprove-" + spaceID
		err = m.taskManager.StopTask(taskID)
	default:
		http.Error(w, "unknown task", http.StatusBadRequest)
		return
	}

	resp := TaskResponse{}
	if err != nil {
		resp.Status = "error"
		resp.Error = err.Error()
	} else {
		resp.Status = "stopped"
	}
	json.NewEncoder(w).Encode(resp)
}

// handleStatusTask processes a GET request to check a task’s status.
func (m *Manager) handleStatusTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	// Expect a query parameter like ?task=server or ?task=autoapprove-<spaceID>
	taskID := r.URL.Query().Get("task")
	if taskID == "" {
		http.Error(w, "missing task parameter", http.StatusBadRequest)
		return
	}

	m.taskManager.mu.Lock()
	_, exists := m.taskManager.tasks[taskID]
	m.taskManager.mu.Unlock()

	resp := TaskResponse{}
	if exists {
		resp.Status = "running"
	} else {
		resp.Status = "stopped"
	}

	json.NewEncoder(w).Encode(resp)
}

// StartManager launches the daemon's HTTP server.
func StartManager(addr string) error {
	manager := NewManager()
	srv := &http.Server{
		Addr:              addr,
		Handler:           manager,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Channel to signal when the server is done.
	done := make(chan struct{})
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Errorf("daemon ListenAndServe: %v", err)
		}
		close(done)
	}()

	// Set up channel on which to send signal notifications.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-quit
	fmt.Println("Daemon is shutting down...")

	// First, tell the task manager to stop all tasks.
	GetTaskManager().StopAll()
	fmt.Println("All managed tasks have been stopped.")

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Errorf("daemon forced to shutdown: %v", err)
	}

	<-done
	fmt.Println("Daemon exiting")
	return nil
}
