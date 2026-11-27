//go:build !darwin && !windows && !linux

package store

import "context"

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
