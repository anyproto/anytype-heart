package core

import (
	"fmt"
	"os"

	"github.com/anytypeio/go-anytype-library/ipfs"
	"github.com/anytypeio/go-anytype-library/net"
	"github.com/anytypeio/go-anytype-library/wallet"
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
	WebGatewaySnapshotUri string
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

func (opts *ServiceOptions) SetDefaults() {
	if opts.HostAddr == nil {
		addr, err := ma.NewMultiaddr(DefaultHostAddr)
		if err != nil {
			log.Fatal("failed to parse DefaultHostAddr: %s", err.Error())
		}
		opts.HostAddr = addr
	}

	if opts.WebGatewayBaseUrl == "" {
		opts.WebGatewayBaseUrl = DefaultWebGatewayBaseUrl
	}

	if opts.CafeGrpcHost == "" {
		opts.CafeGrpcHost = DefaultCafeNodeGRPC
	}

	if opts.CafeP2PAddr == nil {
		addr, err := ma.NewMultiaddr(DefaultCafeNodeP2P)
		if err != nil {
			log.Fatal("failed to parse DefaultCafeNodeP2P: %s", err.Error())
		}

		opts.CafeP2PAddr = addr
	}
}
