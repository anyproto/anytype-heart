package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/config"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ma "github.com/multiformats/go-multiaddr"
)

type ServiceOption func(*ServiceOptions) error
type ServiceOptions struct {
	Repo                  string
	Device                wallet.Keypair
	Account               wallet.Keypair
	HostAddr              ma.Multiaddr
	CafeGrpcHost          string
	CafeP2PAddr           ma.Multiaddr
	WebGatewayBaseUrl     string
	Offline               bool
	NetBootstraper        net.NetBoostrapper
	IPFS                  ipfs.IPFS
	ReindexFunc           func(smartblockId string) error
	SnapshotMarshalerFunc func(blocks []*model.Block, details *types.Struct, relations []*pbrelation.Relation, objectTypes []string, fileKeys []*FileKeys) proto.Marshaler
	WebGatewaySnapshotUri string
	NewSmartblockChan     chan string
}

func WithRootPathAndAccount(rootPath string, account string) ServiceOption {
	return func(args *ServiceOptions) error {
		repoPath := filepath.Join(rootPath, account)
		args.Repo = repoPath

		b, err := ioutil.ReadFile(filepath.Join(repoPath, keyFileAccount))
		if err != nil {
			return fmt.Errorf("failed to read account keyfile: %w", err)
		}

		accountKp, err := wallet.UnmarshalBinary(b)
		if err != nil {
			return err
		}
		if accountKp.KeypairType() != wallet.KeypairTypeAccount {
			return fmt.Errorf("got %s key type instead of %s", accountKp.KeypairType(), wallet.KeypairTypeAccount)
		}

		b, err = ioutil.ReadFile(filepath.Join(repoPath, keyFileDevice))
		if err != nil {
			return fmt.Errorf("failed to read device keyfile: %w", err)
		}

		deviceKp, err := wallet.UnmarshalBinary(b)
		if err != nil {
			return err
		}

		if deviceKp.KeypairType() != wallet.KeypairTypeDevice {
			return fmt.Errorf("got %s key type instead of %s", deviceKp.KeypairType(), wallet.KeypairTypeDevice)
		}

		cfg, err := config.GetConfig(repoPath)
		if err != nil {
			return err
		}

		opts := []ServiceOption{WithRepo(repoPath), WithDeviceKey(deviceKp), WithAccountKey(accountKp), WithHostMultiaddr(cfg.HostAddr), WithWebGatewayBaseUrl(cfg.WebGatewayBaseUrl)}

		// "-" or any other single char assumes as empty for env var compatability
		if len(cfg.CafeP2PAddr) > 1 {
			opts = append(opts, WithCafeP2PAddr(cfg.CafeP2PAddr))
		}

		if len(cfg.CafeGRPCAddr) > 1 {
			opts = append(opts, WithCafeGRPCHost(cfg.CafeGRPCAddr))
		}

		for _, opt := range opts {
			err = opt(args)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// WithNewSmartblockChan add a chan to subscribe to new smartblocks' threads that just became available to read
// it is ok to pass unbuffered chan, because all msgs send in the goroutine
func WithNewSmartblockChan(ch chan string) ServiceOption {
	return func(args *ServiceOptions) error {
		args.NewSmartblockChan = ch
		return nil
	}
}

func WithRepo(repoPath string) ServiceOption {
	return func(args *ServiceOptions) error {
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			return fmt.Errorf("repo path not exists")
		}

		args.Repo = repoPath
		return nil
	}
}

func WithDeviceKey(kp wallet.Keypair) ServiceOption {
	return func(args *ServiceOptions) error {
		args.Device = kp
		return nil
	}
}

func WithAccountKey(kp wallet.Keypair) ServiceOption {
	return func(args *ServiceOptions) error {
		args.Account = kp
		return nil
	}
}

func WithCafeGRPCHost(hostname string) ServiceOption {
	return func(args *ServiceOptions) error {
		args.CafeGrpcHost = hostname
		return nil
	}
}

func WithWebGatewayBaseUrl(url string) ServiceOption {
	return func(args *ServiceOptions) error {
		args.WebGatewayBaseUrl = url
		return nil
	}
}

func WithCafeP2PAddr(addr string) ServiceOption {
	return func(args *ServiceOptions) error {
		cafeAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return err
		}

		args.CafeP2PAddr = cafeAddr
		return nil
	}
}

func WithoutCafe() ServiceOption {
	return func(args *ServiceOptions) error {
		args.CafeP2PAddr = nil
		args.CafeGrpcHost = ""
		args.WebGatewayBaseUrl = ""
		return nil
	}
}

func WithHostMultiaddr(addr string) ServiceOption {
	return func(args *ServiceOptions) error {
		hostAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return err
		}

		args.HostAddr = hostAddr
		return nil
	}
}

func WithOfflineMode(offline bool) ServiceOption {
	return func(args *ServiceOptions) error {
		args.Offline = offline
		return nil
	}
}

func WithIPFSNode(node ipfs.IPFS) ServiceOption {
	return func(args *ServiceOptions) error {
		args.IPFS = node
		return nil
	}
}

func WithNetBootstrapper(n net.NetBoostrapper) ServiceOption {
	return func(args *ServiceOptions) error {
		args.NetBootstraper = n
		return nil
	}
}

func WithReindexFunc(f func(smartblockId string) error) ServiceOption {
	return func(args *ServiceOptions) error {
		args.ReindexFunc = f
		return nil
	}
}

func WithSnapshotMarshalerFunc(f func(blocks []*model.Block, details *types.Struct, relations []*pbrelation.Relation, objectTypes []string, fileKeys []*FileKeys) proto.Marshaler) ServiceOption {
	return func(args *ServiceOptions) error {
		args.SnapshotMarshalerFunc = f
		return nil
	}
}
