// Package mode defines the lifecycle states of a space and the Process
// interface implemented by per-state component bundles (loader, joiner,
// offloader). Orchestration lives in the accountspace reconciler.
package mode

import (
	"context"
)

type Mode int

const (
	ModeUnknown Mode = iota
	// ModeInitial is the dormant state: the controller is registered but no
	// process is running.
	ModeInitial
	ModeLoading
	ModeOffloading
	ModeJoining
)

func (m Mode) String() string {
	switch m {
	case ModeInitial:
		return "initial"
	case ModeLoading:
		return "loading"
	case ModeOffloading:
		return "offloading"
	case ModeJoining:
		return "joining"
	}
	return "unknown"
}

// Process is one running lifecycle phase of a space, implemented as a child
// app bundle. Start and Close are always called from the reconciler goroutine
// of the owning space controller.
type Process interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}
