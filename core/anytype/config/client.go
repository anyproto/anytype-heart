package config

import "github.com/anytypeio/go-anytype-middleware/pb"

type ClientConfig struct {
	EnableDataview      bool
	EnableChannelSwitch bool
	EnableDebug         bool
}

func (cc *ClientConfig) ToPB() *pb.RpcAccountConfig {
	return &pb.RpcAccountConfig{
		EnableDataview:             cc.EnableDataview,
		EnableDebug:                cc.EnableDebug,
		EnableReleaseChannelSwitch: cc.EnableChannelSwitch,
		Extra:                      nil,
	}
}
