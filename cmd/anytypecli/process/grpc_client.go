package process

import (
	"fmt"
	"sync"

	"github.com/anyproto/anytype-heart/pb/service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	clientInstance service.ClientCommandsClient
	grpcConn       *grpc.ClientConn
	once           sync.Once
)

// GetGRPCClient initializes (if needed) and returns the shared gRPC client
func GetGRPCClient() (service.ClientCommandsClient, error) {
	var err error

	// Ensure we only initialize once (singleton)
	once.Do(func() {
		grpcConn, err = grpc.NewClient("dns:///127.0.0.1:31007", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Println("Failed to connect to gRPC server:", err)
			return
		}
		clientInstance = service.NewClientCommandsClient(grpcConn)
	})

	if err != nil {
		return nil, err
	}
	return clientInstance, nil
}

// CloseGRPCConnection ensures the connection is properly closed
func CloseGRPCConnection() {
	if grpcConn != nil {
		grpcConn.Close()
	}
}
