package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
)

var log = logging.Logger("anytype-config")

const (
	CName = "config"
)

type ConfigRequired struct {
	HostAddr string
}

type Config struct {
	ConfigRequired           `json:",inline"`
	NewAccount               bool `ignored:"true"` // set to true if a new account is creating. This option controls whether mw should wait for the existing data to arrive before creating the new log
	Offline                  bool
	DisableThreadsSyncEvents bool

	PrivateNetworkSecret string

	SwarmLowWater  int
	SwarmHighWater int
	BootstrapNodes []string
	RelayNodes     []string

	CafeAddr        string
	CafeGrpcPort    int
	CafeP2PPort     int
	CafePeerId      string
	CafeAPIInsecure bool

	Threads           threads.Config
	DS                clientds.Config
	DisableFileConfig bool `ignored:"true"` // set in order to skip reading/writing config from/to file
}

const (
	configFileName = "config.json"
)

var DefaultConfig = Config{
	Offline:              false,
	SwarmLowWater:        10,
	SwarmHighWater:       50,
	PrivateNetworkSecret: ipfs.IpfsPrivateNetworkKey,
	BootstrapNodes: []string{
		"/ip4/54.93.109.23/tcp/4001/p2p/QmZ4P1Q8HhtKpMshHorM2HDg4iVGZdhZ7YN7WeWDWFH3Hi",           // fra1
		"/dns4/bootstrap2.anytype.io/tcp/4001/p2p/QmSxuiczQTjgj5agSoNtp4esSsj64RisDyKt2MCZQsKZUx", // sfo1
		"/dns4/bootstrap3.anytype.io/tcp/4001/p2p/QmUdDTWzgdcf4cM4aHeihoYSUfQJJbLVLTZFZvm1b46NNT", // sgp1
	},
	RelayNodes: []string{
		"/dns4/relay2.anytype.io/tcp/4101/p2p/12D3KooWMLuW43JqNzUHbXMJH2Ted5Nf26sxv1VMcZAxXV3d3YtB",
		"/dns4/relay1.anytype.io/tcp/4101/p2p/12D3KooWNPqCu4BC5WMBuHmqdiNWwAHGTNKbNy6JP5W1DML2psg1",
	},
	CafeAPIInsecure: false,
	CafeAddr:        "cafe1.anytype.io",
	CafeP2PPort:     4001,
	CafeGrpcPort:    3006,
	CafePeerId:      "12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw",

	DS:      clientds.DefaultConfig,
	Threads: threads.DefaultConfig,
}

func WithNewAccount(isNewAccount bool) func(*Config) {
	return func(c *Config) {
		c.NewAccount = isNewAccount
	}
}

func WithStagingCafe(isStaging bool) func(*Config) {
	return func(c *Config) {
		if isStaging {
			c.CafeAddr = "cafe-staging.anytype.io"
			c.CafePeerId = "12D3KooWPGR6LQyTEtBzFnJ7fGEMe6hKiQKeNof29zLH4bGq2djR"
		}
	}
}

func New(options ...func(*Config)) *Config {
	cfg := DefaultConfig
	for _, opt := range options {
		opt(&cfg)
	}
	cfg.Threads.CafeP2PAddr = cfg.CafeP2PFullAddr()

	return &cfg
}

func (c *Config) CafeNodeGrpcAddr() string {
	return c.CafeAddr + ":" + strconv.Itoa(c.CafeGrpcPort)
}

func (c *Config) CafeUrl() string {
	if net.ParseIP(c.CafeAddr) != nil {
		return c.CafeAddr
	}
	prefix := "https://"
	if c.CafeAPIInsecure {
		prefix = "http://"
	}
	return prefix + c.CafeAddr
}

func (c *Config) CafeP2PFullAddr() string {
	prefix := "dns4"
	if net.ParseIP(c.CafeAddr) != nil {
		prefix = "ip4"
	}
	return fmt.Sprintf("/%s/%s/tcp/%d/p2p/%s", prefix, c.CafeAddr, c.CafeP2PPort, c.CafePeerId)
}

func (c *Config) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	if err = c.initFromFileAndEnv(repoPath); err != nil {
		return
	}

	return
}

func (c *Config) initFromFileAndEnv(repoPath string) error {
	var configFileNotExists bool

	if !c.DisableFileConfig {
		cfgFilePath := filepath.Join(repoPath, configFileName)
		cfgFile, err := os.OpenFile(cfgFilePath, os.O_RDONLY, 0655)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			configFileNotExists = true
		}
		if err == nil {
			defer cfgFile.Close()
			err = json.NewDecoder(cfgFile).Decode(&c)
			if err != nil {
				return fmt.Errorf("invalid format: %w", err)
			}
		}

		saveRandomHostAddr := func() error {
			port, err := getRandomPort()
			if err != nil {
				port = 4006
				log.Errorf("failed to get random port for gateway, go with the default %d: %s", port, err.Error())
			}

			c.HostAddr = fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)

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

			err = json.NewEncoder(cfgFile).Encode(c.ConfigRequired)
			if err != nil {
				return fmt.Errorf("failed to save port to the cfg file: %s", err.Error())
			}
			return nil
		}
		if c.HostAddr == "" && configFileNotExists {
			err = saveRandomHostAddr()
			if err != nil {
				return err
			}
		} else {
			parts := strings.Split(c.HostAddr, "/")
			if len(parts) == 0 {
				log.Errorf("failed to parse cfg.HostAddr: %s", c.HostAddr)
			} else {
				// lets test the existing port in config
				addr, err := net.ResolveTCPAddr("tcp4", "0.0.0.0:"+parts[len(parts)-1])
				if err == nil {
					l, err := net.ListenTCP("tcp4", addr)
					if err != nil {
						// the port from config is no longer available. It may be used by other app or blocked by the OS(e.g. port exclusion range on windows)
						// lets find another available port and save it to config
						err = saveRandomHostAddr()
						if err != nil {
							return err
						}
					} else {
						_ = l.Close()
					}
				}
			}
		}

	}

	err := envconfig.Process("ANYTYPE", c)
	if err != nil {
		log.Errorf("failed to read config from env: %v", err)
	}

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
	addr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:0")
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
