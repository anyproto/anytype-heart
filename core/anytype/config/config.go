package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/any-sync/app"
	//nolint:misspell
	"github.com/anyproto/any-sync/commonspace/config"
	"github.com/anyproto/any-sync/metric"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/rpc"
	"github.com/anyproto/any-sync/net/rpc/debugserver"
	"github.com/anyproto/any-sync/net/transport/quic"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-config")

const (
	CName = "config"
)

var (
	ErrNetworkIdMismatch       = fmt.Errorf("network id mismatch")
	ErrNetworkFileNotFound     = fmt.Errorf("network configuration file not found")
	ErrNetworkFileFailedToRead = fmt.Errorf("failed to read network configuration")
)

type FileConfig interface {
	GetFileConfig() (ConfigRequired, error)
	WriteFileConfig(cfg ConfigRequired) (ConfigRequired, error)
}

type ConfigRequired struct {
	HostAddr            string `json:",omitempty"`
	CustomFileStorePath string `json:",omitempty"`
	LegacyFileStorePath string `json:",omitempty"`
	NetworkId           string `json:""` // in case this account was at least once connected to the network on this device, this field will be set to the network id
}

type Config struct {
	ConfigRequired                         `json:",inline"`
	NewAccount                             bool `ignored:"true"` // set to true if a new account is creating. This option controls whether mw should wait for the existing data to arrive before creating the new log
	DisableThreadsSyncEvents               bool
	DontStartLocalNetworkSyncAutomatically bool
	PeferYamuxTransport                    bool
	NetworkMode                            pb.RpcAccountNetworkMode
	NetworkCustomConfigFilePath            string `json:",omitempty"` // not saved to config

	RepoPath    string
	AnalyticsId string

	DebugAddr       string
	LocalServerAddr string

	DS                clientds.Config
	FS                FSConfig
	DisableFileConfig bool `ignored:"true"` // set in order to skip reading/writing config from/to file

	nodeConf nodeconf.Configuration
}

type FSConfig struct {
	IPFSStorageAddr string
}

type DebugAPIConfig struct {
	debugserver.Config
	IsEnabled bool
}

const (
	ConfigFileName = "config.json"
)

var DefaultConfig = Config{
	LocalServerAddr: ":0",
	DS:              clientds.DefaultConfig,
}

func WithNewAccount(isNewAccount bool) func(*Config) {
	return func(c *Config) {
		c.NewAccount = isNewAccount
		if isNewAccount {
			c.AnalyticsId = metrics.GenerateAnalyticsId()
		}
	}
}

func WithDebugAddr(addr string) func(*Config) {
	return func(c *Config) {
		c.DebugAddr = addr
	}
}

func WithDisabledLocalNetworkSync() func(*Config) {
	return func(c *Config) {
		c.DontStartLocalNetworkSyncAutomatically = true
	}
}

func WithLocalServer(addr string) func(*Config) {
	return func(c *Config) {
		c.LocalServerAddr = addr
	}
}

func DisableFileConfig(disable bool) func(*Config) {
	return func(c *Config) {
		c.DisableFileConfig = disable
	}
}

type quicPreferenceSetter interface {
	PreferQuic(bool)
}

func New(options ...func(*Config)) *Config {
	cfg := DefaultConfig
	for _, opt := range options {
		opt(&cfg)
	}
	return &cfg
}

func (c *Config) Init(a *app.App) (err error) {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	if err = c.initFromFileAndEnv(repoPath); err != nil {
		return
	}
	if !c.PeferYamuxTransport {
		// PeferYamuxTransport is false by default and used only in case client has some problems with QUIC
		a.MustComponent(peerservice.CName).(quicPreferenceSetter).PreferQuic(true)
	}
	return
}

func (c *Config) initFromFileAndEnv(repoPath string) error {
	if repoPath == "" {
		return fmt.Errorf("repo is missing")
	}
	c.RepoPath = repoPath

	if !c.DisableFileConfig {
		var confRequired ConfigRequired
		err := GetFileConfig(c.GetConfigPath(), &confRequired)
		if err != nil {
			return fmt.Errorf("failed to get config from file: %w", err)
		}

		writeConfig := func() error {
			err = WriteJsonConfig(c.GetConfigPath(), c.ConfigRequired)
			if err != nil {
				return fmt.Errorf("failed to save required configuration to the cfg file: %w", err)
			}
			return nil
		}

		// Do not overwrite the legacy file store path from file if it's already set in memory
		if confRequired.LegacyFileStorePath == "" && c.LegacyFileStorePath != "" {
			confRequired.LegacyFileStorePath = c.LegacyFileStorePath
			c.ConfigRequired = confRequired
			if err := writeConfig(); err != nil {
				return err
			}
		}
		c.ConfigRequired = confRequired

		saveRandomHostAddr := func() error {
			port, err := getRandomPort()
			if err != nil {
				port = 4006
				log.Errorf("failed to get random port for gateway, go with the default %d: %s", port, err)
			}

			c.HostAddr = fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)
			return writeConfig()
		}

		if c.HostAddr == "" {
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

	c.nodeConf, err = c.GetNodeConfWithError()
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) Name() (name string) {
	return CName
}

func (c *Config) DSConfig() clientds.Config {
	return c.DS
}

func (c *Config) FSConfig() (FSConfig, error) {
	res := ConfigRequired{}
	err := GetFileConfig(c.GetConfigPath(), &res)
	if err != nil {
		return FSConfig{}, err
	}

	return FSConfig{IPFSStorageAddr: res.CustomFileStorePath}, nil
}

func (c *Config) GetConfigPath() string {
	return filepath.Join(c.RepoPath, ConfigFileName)
}

func (c *Config) IsNewAccount() bool {
	return c.NewAccount
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

func (c *Config) GetSpace() config.Config {
	return config.Config{
		GCTTL:                60,
		SyncPeriod:           20,
		KeepTreeDataInMemory: true,
	}
}

func (c *Config) GetMetric() metric.Config {
	return metric.Config{}
}

func (c *Config) GetDrpc() rpc.Config {
	return rpc.Config{
		Stream: rpc.StreamConfig{
			MaxMsgSizeMb: 256,
		},
	}
}

func (c *Config) GetDebugAPIConfig() DebugAPIConfig {
	return DebugAPIConfig{
		IsEnabled: len(c.DebugAddr) != 0,
	}
}

func (c *Config) GetDebugServer() debugserver.Config {
	return debugserver.Config{ListenAddr: c.DebugAddr}
}

func (c *Config) GetNodeConfWithError() (conf nodeconf.Configuration, err error) {
	// todo: remvoe set via os env
	networkConfigPath := loadenv.Get("ANY_SYNC_NETWORK")
	confBytes := nodesConfYmlBytes

	if networkConfigPath != "" {
		if c.NetworkMode != pb.RpcAccount_CustomConfig && c.NetworkCustomConfigFilePath != "" {
			return nodeconf.Configuration{}, fmt.Errorf("network config path is set in both env ANY_SYNC_NETWORK(%s) and in RPC request(%s)", networkConfigPath, c.NetworkCustomConfigFilePath)
		}
		log.Warnf("Network config set via os env ANY_SYNC_NETWORK is deprecated")
	} else if c.NetworkMode == pb.RpcAccount_CustomConfig {
		if c.NetworkCustomConfigFilePath == "" {
			return nodeconf.Configuration{}, errors.Join(ErrNetworkFileFailedToRead, fmt.Errorf("CustomConfig network mode is set but NetworkCustomConfigFilePath is empty"))
		}
		networkConfigPath = c.NetworkCustomConfigFilePath
	}

	// save the reference to no override the original pointer to the slice
	if networkConfigPath != "" {
		var err error
		if confBytes, err = os.ReadFile(networkConfigPath); err != nil {
			if os.IsNotExist(err) {
				return nodeconf.Configuration{}, errors.Join(ErrNetworkFileNotFound, err)
			}
			return nodeconf.Configuration{}, errors.Join(ErrNetworkFileFailedToRead, err)
		}
	}

	switch c.NetworkMode {
	case pb.RpcAccount_CustomConfig, pb.RpcAccount_DefaultConfig:
		if err := yaml.Unmarshal(confBytes, &conf); err != nil {
			return nodeconf.Configuration{}, errors.Join(ErrNetworkFileFailedToRead, err)
		}
		if c.NetworkId != "" && c.NetworkId != conf.NetworkId {
			log.Warnf("Network id mismatch: %s != %s", c.NetworkId, conf.NetworkId)
			return nodeconf.Configuration{}, errors.Join(ErrNetworkIdMismatch, fmt.Errorf("network id mismatch: %s != %s", c.NetworkId, conf.NetworkId))
		}
	case pb.RpcAccount_LocalOnly:
		confBytes = []byte{}
	}

	if conf.NetworkId != "" && c.NetworkId == "" {
		log.Infof("Network id is not set in config; set to %s", conf.NetworkId)
		c.NetworkId = conf.NetworkId
	}
	return
}

func (c *Config) GetNodeConf() (conf nodeconf.Configuration) {
	return c.nodeConf
}

func (c *Config) GetNodeConfStorePath() string {
	return filepath.Join(c.RepoPath, "nodeconf")
}

func (c *Config) GetYamux() yamux.Config {
	return yamux.Config{
		ListenAddrs:     []string{},
		WriteTimeoutSec: 10,
		DialTimeoutSec:  10,
	}
}

func (c *Config) GetQuic() quic.Config {
	return quic.Config{
		ListenAddrs:     []string{},
		WriteTimeoutSec: 10,
		DialTimeoutSec:  10,
	}
}

func (c *Config) ResetStoredNetworkId() error {
	configCopy := c.ConfigRequired
	configCopy.NetworkId = ""
	return WriteJsonConfig(c.GetConfigPath(), configCopy)
}

func (c *Config) PersistAccountNetworkId() error {
	configCopy := c.ConfigRequired
	configCopy.NetworkId = c.NetworkId
	return WriteJsonConfig(c.GetConfigPath(), configCopy)
}
