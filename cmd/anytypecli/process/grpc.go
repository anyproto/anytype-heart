package process

import (
	"context"
	"strings"

	pb "github.com/anyproto/anytype-heart/pb"
)

// CheckGRPCServer checks if the gRPC server is reachable
func CheckGRPCServer() (bool, error) {
	client, err := GetGRPCClient()
	if err != nil {
		return false, err
	}

	req := &pb.RpcAppGetVersionRequest{}
	_, err = client.AppGetVersion(context.Background(), req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
