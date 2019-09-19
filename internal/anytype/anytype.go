package anytype

import (
	"context"

	"github.com/anytypeio/go-anytype-library/anytypepb"
	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/textileio/go-textile/wallet"
)

type Server struct {

}

func NewServer() (pb.AnytypeServer, error) {
	return &Server{}, nil
}

func (s *Server) NewWallet(context.Context, *pb.Empty) (*pb.NewWalletResponse, error) {
	w, err := wallet.WalletFromWordCount(12)
	if err != nil {
		return nil, err
	}

	return &pb.NewWalletResponse{Mnemonic: w.RecoveryPhrase}, nil
}
