package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/kelseyhightower/envconfig"
)

var log = logging.Logger("anytype-config")

const configFileName = "config.json"
const (
	defaultCafeNodeP2P       = "/dns4/cafe1.anytype.io/tcp/4001/p2p/12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw"
	defaultCafeNodeGRPC      = "cafe1.anytype.io:3006"
	defaultWebGatewayBaseUrl = "https://anytype.page"
)

var DefaultConfig = Config{
	CafeP2PAddr:       defaultCafeNodeP2P,
	CafeGRPCAddr:      defaultCafeNodeGRPC,
	WebGatewayBaseUrl: defaultWebGatewayBaseUrl,
}

type Config struct {
	HostAddr          string `json:"host_addr,omitempty" envconfig:"host_addr"`
	CafeP2PAddr       string `json:"cafe_p2p_addr,omitempty" envconfig:"cafe_p2p_addr"`
	CafeGRPCAddr      string `json:"cafe_grpc_addr,omitempty" envconfig:"cafe_grpc_addr"`
	WebGatewayBaseUrl string `json:"web_gateway_base_url,omitempty" envconfig:"web_gateway_base_url"`
}

var mu = sync.Mutex{}

func GetConfig(repoPath string) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	cfg := DefaultConfig
	cfgFile, err := os.OpenFile(filepath.Join(repoPath, configFileName), os.O_RDONLY, 0655)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err == nil {
		defer cfgFile.Close()
		err = json.NewDecoder(cfgFile).Decode(&cfg)
		if err != nil {
			return nil, err
		}
	}

	err = envconfig.Process("ANYTYPE", &cfg)
	if cfg.HostAddr == "" {
		port, err := getRandomPort()
		if err != nil {
			port = 4006
			log.Errorf("failed to get random port for gateway, go with the default %d: %s", port, err.Error())
		}

		cfg.HostAddr = fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)
	}

	if cfgFile == nil {
		cfgFile, err = os.OpenFile(filepath.Join(repoPath, configFileName), os.O_RDWR|os.O_CREATE, 0640)
		if err != nil {
			return nil, fmt.Errorf("failed to save port to the cfg file: %s", err.Error())
		}
		defer cfgFile.Close()

		err = json.NewEncoder(cfgFile).Encode(Config{HostAddr: cfg.HostAddr})
		if err != nil {
			return nil, fmt.Errorf("failed to save port to the cfg file: %s", err.Error())
		}
	}

	return &cfg, nil
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
