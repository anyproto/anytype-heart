package main

import (
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/textileio/go-textile/keypair"
	nativeconfig "github.com/ipfs/go-ipfs-config"
	"github.com/textileio/go-textile/mobile"
	tconfig "github.com/textileio/go-textile/repo/config"
	"github.com/textileio/go-textile/wallet"
)

const wordCount int = 12

func walletCreate(b []byte) []byte {
	callback := func(code pb.WalletCreateCallback_Error_Code, err error) []byte{
		m := &pb.WalletCreateCallback{Error: &pb.WalletCreateCallback_Error{Code: code}}
		if err != nil {
			m.Error.Desc = err.Error()
		}

		return Marshal(m)
	}

	var msg pb.WalletCreate
	err := proto.Unmarshal(b, &msg)
	if err != nil {
		return callback(pb.WalletCreateCallback_Error_BAD_INPUT, err)
	}

	wallet, err := wallet.WalletFromWordCount(wordCount)
	if err != nil {
		return callback(pb.WalletCreateCallback_Error_UNKNOWN_ERROR, err)
	}

	account, err := wallet.AccountAt(0, msg.Pin)
	if err != nil {
		return callback(pb.WalletCreateCallback_Error_UNKNOWN_ERROR, err)
	}

	kp, err := keypair.Parse(account.Seed())
	if err != nil {
		return callback(pb.WalletCreateCallback_Error_UNKNOWN_ERROR, err)
	}

	nativeconfig.DefaultBootstrapAddresses = []string{}
	tconfig.DefaultBootstrapAddresses = core.BootstrapNodes

	err = mobile.InitRepo(&mobile.InitConfig{Seed: account.Seed(), RepoPath: filepath.Join(os.TempDir(), kp.Address()), Debug: true})
	if err != nil {
		return callback(pb.WalletCreateCallback_Error_UNKNOWN_ERROR, err)
	}

	return callback(pb.WalletCreateCallback_Error_NULL, nil)
}
