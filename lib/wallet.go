package main

import (
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/textileio/go-textile/keypair"
	nativeconfig "github.com/ipfs/go-ipfs-config"
	"github.com/textileio/go-textile/mobile"
	tconfig "github.com/textileio/go-textile/repo/config"
	"github.com/textileio/go-textile/wallet"
)

const wordCount int = 12

func walletCreate(msgId string, msg *pb.WalletCreate) {
	wallet, err := wallet.WalletFromWordCount(wordCount)
	if err != nil {
		CallClientWithStatus(&pb.Status{
			Description: err.Error(),
			Status:      &pb.Status_IntError{pb.Status_INTERNAL_ERROR},
		})
		return
	}
	account, err := wallet.AccountAt(0, msg.Pin)
	if err != nil {
		// todo: correct error
		CallClientWithStatus(&pb.Status{
			Description: err.Error(),
			Status:      &pb.Status_IntError{pb.Status_INTERNAL_ERROR},
		})
		return
	}

	kp, err := keypair.Parse(account.Seed())
	if err != nil {
		CallClientWithStatus(&pb.Status{
			Description: err.Error(),
			Status:      &pb.Status_IntError{pb.Status_INTERNAL_ERROR},
		})
		return
	}

	nativeconfig.DefaultBootstrapAddresses = []string{}
	tconfig.DefaultBootstrapAddresses = core.BootstrapNodes

	err = mobile.InitRepo(&mobile.InitConfig{Seed: account.Seed(), RepoPath: filepath.Join(os.TempDir(), kp.Address()), Debug: true})

	if err != nil {
		// todo: correct error
		CallClientWithStatus(&pb.Status{
			Description: err.Error(),
			Status:      &pb.Status_IntError{pb.Status_INTERNAL_ERROR},
		})
		return
	}

	CallClientWithStatus(&pb.Status{
		ReplyTo: msgId,
		Status: &pb.Status_Success{Success:true},
	})
}
