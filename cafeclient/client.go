package cafeclient

import (
	"context"

	"github.com/anytypeio/go-anytype-cafe/api/pb"
	"github.com/anytypeio/go-anytype-library/wallet"
	"google.golang.org/grpc"
)

var _ pb.APIClient = (*Online)(nil)

type Client interface {
	pb.APIClient
	Shutdown() error
}

type Token struct {
	Token string
}

type Online struct {
	client pb.APIClient
	token  *Token

	device  wallet.Keypair
	account wallet.Keypair

	conn *grpc.ClientConn
}

func (client *Online) AuthGetToken(ctx context.Context, opts ...grpc.CallOption) (pb.API_AuthGetTokenClient, error) {
	panic("implement me")
}

func (client *Online) ThreadLogFollow(ctx context.Context, in *pb.ThreadLogFollowRequest, opts ...grpc.CallOption) (*pb.ThreadLogFollowResponse, error) {
	panic("implement me")
}

func (client *Online) FilePin(ctx context.Context, in *pb.FilePinRequest, opts ...grpc.CallOption) (*pb.FilePinResponse, error) {
	panic("implement me")
}

func (client *Online) AccountFind(ctx context.Context, in *pb.AccountFindRequest, opts ...grpc.CallOption) (pb.API_AccountFindClient, error) {
	panic("implement me")
}

func NewClient(url string, device wallet.Keypair, account wallet.Keypair) (Client, error) {
	conn, err := grpc.Dial(url, grpc.WithUserAgent("<todo>"), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &Online{
		pb.NewAPIClient(conn),
		nil,
		device,
		account,
		conn,
	}, nil
}

func (client *Online) Shutdown() error {
	return client.conn.Close()
}
