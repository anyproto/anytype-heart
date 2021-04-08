package config

import (
	"encoding/json"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/kelseyhightower/envconfig"
	"net"
	"os"
	"path/filepath"
	"sync"
)

var log = logging.Logger("anytype-config")

const (
	CName = "config"

	defaultCafeNodeGRPC = "cafe1.anytype.io:3006"
)

type Config struct {
	NewAccount      bool // set to true if a new account is creating. This option controls whether mw should wait for the existing data to arrive before creating the new log
	Offline         bool
	Pubsub          bool
	CafeAPIInsecure bool
	CafeAPIHost     string

	FullTextSearch bool

	HostAddr             string
	PrivateNetworkSecret string

	SwarmLowWater  int
	SwarmHighWater int
	BootstrapNodes []string

	Threads threads.Config
	DS      clientds.Config
}

var mu sync.Mutex

const (
	configFileName = "config.json"
)

var DefaultConfig = Config{
	FullTextSearch:       true,
	Offline:              false,
	Pubsub:               true,
	SwarmLowWater:        10,
	SwarmHighWater:       50,
	PrivateNetworkSecret: ipfs.IpfsPrivateNetworkKey,
	BootstrapNodes: []string{
		"/ip4/54.93.109.23/tcp/4001/p2p/QmZ4P1Q8HhtKpMshHorM2HDg4iVGZdhZ7YN7WeWDWFH3Hi",           // fra1
		"/dns4/bootstrap2.anytype.io/tcp/4001/p2p/QmSxuiczQTjgj5agSoNtp4esSsj64RisDyKt2MCZQsKZUx", // sfo1
		"/dns4/bootstrap3.anytype.io/tcp/4001/p2p/QmUdDTWzgdcf4cM4aHeihoYSUfQJJbLVLTZFZvm1b46NNT", // sgp1
	},
	CafeAPIHost:     defaultCafeNodeGRPC,
	CafeAPIInsecure: false,

	DS:      clientds.DefaultConfig,
	Threads: threads.DefaultConfig,
}

func New(apply func(*Config)) *Config {
	cfg := DefaultConfig
	if apply != nil {
		apply(&cfg)
	}
	return &cfg
}

func (c *Config) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	if err = c.initFromFileAndEnv(repoPath); err != nil {
		return
	}

	return
}

func (cfg *Config) initFromFileAndEnv(repoPath string) error {
	mu.Lock()
	defer mu.Unlock()

	cfgFilePath := filepath.Join(repoPath, configFileName)
	cfgFile, err := os.OpenFile(cfgFilePath, os.O_RDONLY, 0655)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil {
		defer cfgFile.Close()
		err = json.NewDecoder(cfgFile).Decode(&cfg)
		if err != nil {
			return fmt.Errorf("invalid format: %w", err)
		}
	}

	if cfg.HostAddr == "" {
		port, err := getRandomPort()
		if err != nil {
			port = 4006
			log.Errorf("failed to get random port for gateway, go with the default %d: %s", port, err.Error())
		}

		cfg.HostAddr = fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)

		// we need to save selected port in order in order to increase chances of incoming connections
		if cfgFile != nil {
			// close the readonly-mode file first
			_ = cfgFile.Close()
		}

		cfgFile, err = os.OpenFile(cfgFilePath, os.O_RDWR|os.O_CREATE, 0640)
		if err != nil {
			return fmt.Errorf("failed to save port to the cfg file: %s", err.Error())
		}

		defer cfgFile.Close()

		err = json.NewEncoder(cfgFile).Encode(cfg)
		if err != nil {
			return fmt.Errorf("failed to save port to the cfg file: %s", err.Error())
		}
	}

	err = envconfig.Process("ANYTYPE", &cfg)

	return nil
}

func (c *Config) Name() (name string) {
	return CName
}

func (c *Config) DSConfig() clientds.Config {
	return c.DS
}

func (c *Config) ThreadsConfig() threads.Config {
	return c.Threads
}

func getRandomPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
