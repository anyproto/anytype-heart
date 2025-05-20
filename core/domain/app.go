package domain

import "github.com/anyproto/anytype-heart/pb"

type CompState int

const (
	CompStateAppWentBackground   CompState = CompState(pb.RpcAppSetDeviceStateRequest_BACKGROUND) // 0
	CompStateAppWentForeground   CompState = CompState(pb.RpcAppSetDeviceStateRequest_FOREGROUND) // 1
	CompStateAppClosingInitiated CompState = 2                                                    // triggered by app
)
