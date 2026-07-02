// Package spacev2 is the greenfield reimplementation of the space
// orchestration layer: how spaces are brought up, tracked, paused and torn
// down. See HANDOFF.md for scope and the compatibility boundary, DESIGN.md
// for the architecture (per-space reconciler over live SpaceView status +
// local demand).
package spacev2

import "github.com/anyproto/any-sync/app/logger"

// CName is the registered component name; consumers resolve the space
// service under the same name v1 used.
const CName = "client.space"

var log = logger.NewNamed(CName)
