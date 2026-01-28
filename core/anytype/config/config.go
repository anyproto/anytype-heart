package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

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

type Config struct {
	persisted  PersistedConfig
	configPath string
	mu         sync.RWMutex

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
	EnableMembershipV2                     bool             `json:",omitempty"` // optional, default is false

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

// Getters for persisted config fields (read from memory)

// NetworkId returns the stored network id.
func (c *Config) NetworkId() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persisted.NetworkId
}

// CustomFileStorePath returns the custom file store path.
func (c *Config) CustomFileStorePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persisted.CustomFileStorePath
}

// LegacyFileStorePath returns the legacy file store path.
func (c *Config) LegacyFileStorePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persisted.LegacyFileStorePath
}

// HostAddr returns the host address.
func (c *Config) HostAddr() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persisted.HostAddr
}

// AutoDownloadFiles returns whether auto download is enabled.
func (c *Config) AutoDownloadFiles() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persisted.AutoDownloadFiles
}

// AutoDownloadOnWifiOnly returns whether auto download is restricted to wifi.
func (c *Config) AutoDownloadOnWifiOnly() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.persisted.AutoDownloadOnWifiOnly
}

// Setters for persisted config fields (write to disk on change)

// writeLocked writes the current persisted config to disk.
// Must be called with c.mu held.
func (c *Config) writeLocked() error {
	if c.DisableFileConfig {
		return nil
	}
	return writeConfigSafe(c.configPath, c.persisted)
}

// SetNetworkId sets the network id and writes to disk if changed.
func (c *Config) SetNetworkId(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.persisted.NetworkId == id {
		return nil
	}
	c.persisted.NetworkId = id
	return c.writeLocked()
}

// SetCustomFileStorePath sets the custom file store path and writes to disk if changed.
func (c *Config) SetCustomFileStorePath(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.persisted.CustomFileStorePath == path {
		return nil
	}
	c.persisted.CustomFileStorePath = path
	return c.writeLocked()
}

// SetLegacyFileStorePath sets the legacy file store path and writes to disk if changed.
func (c *Config) SetLegacyFileStorePath(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.persisted.LegacyFileStorePath == path {
		return nil
	}
	c.persisted.LegacyFileStorePath = path
	return c.writeLocked()
}

// SetHostAddr sets the host address and writes to disk if changed.
func (c *Config) SetHostAddr(addr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.persisted.HostAddr == addr {
		return nil
	}
	c.persisted.HostAddr = addr
	return c.writeLocked()
}

// SetAutoDownloadSettings sets auto download settings and writes to disk if changed.
func (c *Config) SetAutoDownloadSettings(enabled, wifiOnly bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.persisted.AutoDownloadFiles == enabled && c.persisted.AutoDownloadOnWifiOnly == wifiOnly {
		return nil
	}
	c.persisted.AutoDownloadFiles = enabled
	c.persisted.AutoDownloadOnWifiOnly = wifiOnly
	return c.writeLocked()
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

// WithLegacyFileStorePath sets the legacy file store path before init.
// This is used during account recovery from export.
func WithLegacyFileStorePath(path string) func(*Config) {
	return func(c *Config) {
		c.persisted.LegacyFileStorePath = path
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
	cfg := &Config{
		LocalServerAddr: DefaultConfig.LocalServerAddr,
		DS:              DefaultConfig.DS,
	}
	for _, opt := range options {
		opt(cfg)
	}
	return cfg
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
	c.configPath = filepath.Join(repoPath, ConfigFileName)
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
		confRequired, err := readPersistedConfig(c.configPath)
		if err != nil && errors.Is(err, ErrInvalidConfigFormat) {
			log.Errorf("config file corrupted, reinitializing: %v", err)
			confRequired = PersistedConfig{}
			// Try to recover NetworkId from YAML if config is corrupted
			if networkId, recoverErr := c.deriveNetworkIdFromYAML(); recoverErr == nil && networkId != "" {
				log.Warnf("recovered network id from YAML: %s", networkId)
				confRequired.NetworkId = networkId
			}
			// Write clean config immediately to replace corrupted file
			if writeErr := writeConfigSafe(c.configPath, confRequired); writeErr != nil {
				log.Errorf("failed to write recovered config: %v", writeErr)
			}
		} else if err != nil {
			return err
		}

		// Get the in-memory legacy file store path before loading from file
		inMemoryLegacyPath := c.persisted.LegacyFileStorePath

		// Load persisted config into memory
		c.mu.Lock()
		c.persisted = confRequired
		c.mu.Unlock()

		// Do not overwrite the legacy file store path from file if it's already set in memory
		if confRequired.LegacyFileStorePath == "" && inMemoryLegacyPath != "" {
			if err := c.SetLegacyFileStorePath(inMemoryLegacyPath); err != nil {
				return err
			}
		}

		saveRandomHostAddr := func() error {
			port, err := getRandomPort()
			if err != nil {
				port = 4006
				log.Errorf("failed to get random port for gateway, go with the default %d: %s", port, err)
			}
			return c.SetHostAddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))
		}

		hostAddr := c.HostAddr()
		if hostAddr == "" {
			err = saveRandomHostAddr()
			if err != nil {
				return err
			}
		} else {
			parts := strings.Split(hostAddr, "/")
			if len(parts) == 0 {
				log.Errorf("failed to parse cfg.HostAddr: %s", hostAddr)
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

// deriveNetworkIdFromYAML attempts to derive the network ID from the network YAML configuration.
// This is used for recovery when the config.json is corrupted.
func (c *Config) deriveNetworkIdFromYAML() (string, error) {
	var confBytes []byte
	networkConfigPath := loadenv.Get("ANY_SYNC_NETWORK")

	if networkConfigPath == "" && c.NetworkMode == pb.RpcAccount_CustomConfig {
		networkConfigPath = c.NetworkCustomConfigFilePath
	}

	if networkConfigPath != "" {
		var err error
		confBytes, err = os.ReadFile(networkConfigPath)
		if err != nil {
			return "", err
		}
	} else {
		confBytes = nodesConfYmlBytes
	}

	var conf nodeconf.Configuration
	if err := yaml.Unmarshal(confBytes, &conf); err != nil {
		return "", err
	}
	return conf.NetworkId, nil
}

func (c *Config) Name() (name string) {
	return CName
}

func (c *Config) DSConfig() clientds.Config {
	return c.DS
}

// FSConfig returns the file storage configuration read from memory.
func (c *Config) FSConfig() FSConfig {
	return FSConfig{IPFSStorageAddr: c.CustomFileStorePath()}
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
		networkId := c.NetworkId()
		if !c.DisableNetworkIdCheck && networkId != "" && networkId != conf.NetworkId {
			log.Warnf("Network id mismatch: %s != %s", networkId, conf.NetworkId)
			return nodeconf.Configuration{}, errors.Join(ErrNetworkIdMismatch, fmt.Errorf("network id mismatch: %s != %s", networkId, conf.NetworkId))
		}
	case pb.RpcAccount_LocalOnly:
		confBytes = []byte{}
	}

	if conf.NetworkId != "" && c.NetworkId() == "" {
		log.Infof("Network id is not set in config; set to %s", conf.NetworkId)
		c.mu.Lock()
		c.persisted.NetworkId = conf.NetworkId
		c.mu.Unlock()
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
	return c.SetNetworkId("")
}

func (c *Config) PersistAccountNetworkId() error {
	// The network id is already in memory, just write the current config to disk
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked()
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
