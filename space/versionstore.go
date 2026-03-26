package space

// WorkspaceLoadVersionStore persists the workspace load version counter.
// When the stored version differs from ForceWorkspaceLoadVersion, the service
// blocks during initAccount until all spaces are fully loaded.
type WorkspaceLoadVersionStore interface {
	GetWorkspaceLoadVersion() (int32, error)
	SaveWorkspaceLoadVersion(version int32) error
}

type stubVersionStore struct{}

func newStubVersionStore() *stubVersionStore {
	return &stubVersionStore{}
}

// GetWorkspaceLoadVersion always returns 0, meaning synchronous load will trigger
// every time until a real implementation is provided.
func (s *stubVersionStore) GetWorkspaceLoadVersion() (int32, error) {
	return 0, nil
}

// SaveWorkspaceLoadVersion is a no-op for the stub.
func (s *stubVersionStore) SaveWorkspaceLoadVersion(_ int32) error {
	return nil
}
