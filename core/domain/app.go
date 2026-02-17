package domain

import "github.com/anyproto/anytype-heart/pb"

type CompState int

const (
	CompStateAppWentBackground   CompState = CompState(pb.RpcAppSetDeviceStateRequest_BACKGROUND) // 0
	CompStateAppWentForeground   CompState = CompState(pb.RpcAppSetDeviceStateRequest_FOREGROUND) // 1
	CompStateAppClosingInitiated CompState = 2                                                    // triggered by app
)

// MigrationObjectContextVersion is the version of the object context migration.
// Bump this to force re-migration if issues are found or we started to migrate all objects.
const MigrationObjectContextVersion = 10
