package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/anytype-publish-server/publishclient"

	//nolint:misspell
	"github.com/anyproto/any-sync/commonspace/config"
	"github.com/anyproto/any-sync/metric"
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
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

var log = logging.Logger("anytype-config")

const (
	CName = "config"
)

const (
	SpaceStoreBadgerPath = "spacestore"
	SpaceStoreSqlitePath = "spaceStore.db"
	SpaceStoreNewPath    = "spaceStoreNew"
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
	HostAddr               string `json:",omitempty"`
	CustomFileStorePath    string `json:",omitempty"`
	LegacyFileStorePath    string `json:",omitempty"`
	NetworkId              string `json:""` // in case this account was at least once connected to the network on this device, this field will be set to the network id
	AutoDownloadFiles      bool   `json:",omitempty"`
	AutoDownloadOnWifiOnly bool   `json:",omitempty"`
}

// Use separate structure as trick for legacy config management
type ConfigAutoDownloadFiles struct {
	AutoDownloadFiles      bool
	AutoDownloadOnWifiOnly bool
}

type Config struct {
	ConfigRequired `json:",inline"`

	NewAccount     bool   `ignored:"true"` // set to true if a new account is creating. This option controls whether mw should wait for the existing data to arrive before creating the new log
	AutoJoinStream string `ignored:"true"` // contains the invite of the stream space to automatically join

	DisableThreadsSyncEvents               bool
	DontStartLocalNetworkSyncAutomatically bool
	PeferYamuxTransport                    bool
	DisableNetworkIdCheck                  bool
	SpaceStorageMode                       storage.SpaceStorageMode
	NetworkMode                            pb.RpcAccountNetworkMode
	NetworkCustomConfigFilePath            string           `json:",omitempty"` // not saved to config
	SqliteTempPath                         string           `json:",omitempty"` // not saved to config
	AnyStoreConfig                         *anystore.Config `json:",omitempty"` // not saved to config
	JsonApiListenAddr                      string           `json:",omitempty"` // empty means disabled

	RepoPath    string
	AnalyticsId string

	DebugAddr       string
	LocalServerAddr string

	DS                clientds.Config
	FS                FSConfig
	DisableFileConfig bool `ignored:"true"` // set in order to skip reading/writing config from/to file

	nodeConf nodeconf.Configuration
}

func (c *Config) IsLocalOnlyMode() bool {
	return c.NetworkMode == pb.RpcAccount_LocalOnly
}

type FSConfig struct {
	IPFSStorageAddr string
}

type DebugAPIConfig struct {
	debugserver.Config
	IsEnabled bool
}

type PushConfig struct {
	PeerId string
	Addr   []string
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

func WithAutoJoinStream(inviteUrl string) func(*Config) {
	return func(c *Config) {
		c.AutoJoinStream = inviteUrl
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
	repoPath := app.MustComponent[wallet.Wallet](a).RepoPath()
	if err = c.initFromFileAndEnv(repoPath); err != nil {
		return
	}
	if !c.PeferYamuxTransport {
		// PeferYamuxTransport is false by default and used only in case client has some problems with QUIC
		app.MustComponent[quicPreferenceSetter](a).PreferQuic(true)
	}
	// check if sqlite db exists
	if _, err2 := os.Stat(filepath.Join(repoPath, SpaceStoreSqlitePath)); err2 == nil {
		// already have sqlite db
		c.SpaceStorageMode = storage.SpaceStorageModeSqlite
	} else if _, err2 = os.Stat(filepath.Join(repoPath, SpaceStoreBadgerPath)); err2 == nil {
		// old account repos
		c.SpaceStorageMode = storage.SpaceStorageModeBadger
	} else {
		// new account repos
		// todo: remove temporary log
		log.Warn("using sqlite storage")
		c.SpaceStorageMode = storage.SpaceStorageModeSqlite
	}
	return
}

func (c *Config) initFromFileAndEnv(repoPath string) error {
	if repoPath == "" {
		return fmt.Errorf("repo is missing")
	}
	c.RepoPath = repoPath
	c.AnyStoreConfig = &anystore.Config{}
	if runtime.GOOS == "android" {
		split := strings.Split(repoPath, "/files/")
		if len(split) == 1 {
			return fmt.Errorf("failed to split repo path: %s", repoPath)
		}
		c.SqliteTempPath = filepath.Join(split[0], "files")
		c.AnyStoreConfig.SQLiteConnectionOptions = make(map[string]string)
		c.AnyStoreConfig.SQLiteConnectionOptions["temp_store_directory"] = "'" + c.SqliteTempPath + "'"
	}

	if !c.DisableFileConfig {
		var confRequired ConfigRequired
		err := GetFileConfig(c.GetConfigPath(), &confRequired)
		if err != nil && errors.Is(err, ErrInvalidConfigFormat) {
			log.Errorf("config file init: %v", err)
		} else if err != nil {
			return err
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

func (c *Config) GetRepoPath() string {
	return c.RepoPath
}

func (c *Config) GetConfigPath() string {
	return filepath.Join(c.RepoPath, ConfigFileName)
}

func (c *Config) GetSqliteStorePath() string {
	return filepath.Join(c.RepoPath, SpaceStoreSqlitePath)
}

func (c *Config) GetOldSpaceStorePath() string {
	if c.GetSpaceStorageMode() == storage.SpaceStorageModeBadger {
		return filepath.Join(c.RepoPath, SpaceStoreBadgerPath)
	}
	return c.GetSqliteStorePath()
}

func (c *Config) GetNewSpaceStorePath() string {
	return filepath.Join(c.RepoPath, SpaceStoreNewPath)
}

func (c *Config) GetTempDirPath() string {
	return c.SqliteTempPath
}

func (c *Config) GetAnyStoreConfig() *anystore.Config {
	return c.AnyStoreConfig
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
		if !c.DisableNetworkIdCheck && c.NetworkId != "" && c.NetworkId != conf.NetworkId {
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

func (c *Config) GetStreamConfig() streampool.StreamConfig {
	return streampool.StreamConfig{
		SendQueueSize:    300,
		DialQueueWorkers: 4,
		DialQueueSize:    300,
	}
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
		ListenAddrs:       []string{},
		WriteTimeoutSec:   10,
		InitialPacketSize: 1200,
		DialTimeoutSec:    10,
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

func (c *Config) GetSpaceStorageMode() storage.SpaceStorageMode {
	return c.SpaceStorageMode
}

func (c *Config) GetNetworkMode() pb.RpcAccountNetworkMode {
	return c.NetworkMode
}

func (c *Config) GetPublishServer() publishclient.Config {
	publishPeerId := "12D3KooWEQPgbxGPvkny8kikS3zqfziM7JsQBnJHXHL9ByCcATs7"
	publishAddr := "anytype-publish-server.anytype.io:443"

	if peerId := os.Getenv("ANYTYPE_PUBLISH_PEERID"); peerId != "" {
		if addr := os.Getenv("ANYTYPE_PUBLISH_ADDRESS"); addr != "" {
			publishPeerId = peerId
			publishAddr = addr
		}
	}

	return publishclient.Config{
		Addrs: []publishclient.PublishServerAddr{
			{
				PeerId: publishPeerId,
				Addrs:  []string{"yamux://" + publishAddr},
			},
		},
	}
}

type PublishLimitsConfig struct {
	MembershipLimit       int64
	DefaultLimit          int64
	InviteLinkUrlTemplate string
	MemberUrlTemplate     string
	DefaultUrlTemplate    string
	IndexFileName         string
}

func (c *Config) GetPublishLimits() PublishLimitsConfig {

	return PublishLimitsConfig{
		MembershipLimit:       6000 << 20,
		DefaultLimit:          10 << 20,
		InviteLinkUrlTemplate: "https://invite.any.coop/%s#%s",
		MemberUrlTemplate:     "https://%s.org",
		DefaultUrlTemplate:    "https://any.coop/%s",
		IndexFileName:         "index.json.gz",
	}
}

func (c *Config) GetPushConfig() PushConfig {
	pushPeerId := "12D3KooWMATrdteJNq2YvYhtq3RDeWxq6RVXDAr36MsGd5RJzXDn"
	pushAddr := "anytype-push-server.anytype.io:443"

	if peerId := os.Getenv("ANYTYPE_PUSH_PEERID"); peerId != "" {
		if addr := os.Getenv("ANYTYPE_PUSH_ADDRESS"); addr != "" {
			pushPeerId = peerId
			pushAddr = addr
		}
	}

	return PushConfig{
		PeerId: pushPeerId,
		Addr:   []string{"yamux://" + pushAddr},
	}
}
