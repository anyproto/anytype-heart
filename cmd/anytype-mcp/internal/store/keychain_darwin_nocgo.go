//go:build darwin && !cgo

package store

import "context"

// A darwin build without cgo has no keychain access. The shipped darwin
// binaries are built with cgo on the macOS job; this keeps cross-vetting
// and CGO_ENABLED=0 builds compiling.
type noStore struct{}

func openPlatform(string) Store { return noStore{} }

func newPlatformStore(string) Store { return noStore{} }

func (noStore) Kind() string                { return "none" }
func (noStore) Probe(context.Context) error { return ErrNoStore }
func (noStore) Read(context.Context, string, bool) ([]byte, error) {
	return nil, ErrNoStore
}
func (noStore) Write(context.Context, string, []byte, bool) error { return ErrNoStore }
func (noStore) Delete(context.Context, string) error              { return ErrNoStore }
