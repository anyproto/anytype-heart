package config

import (
	"fmt"
	"github.com/anytypeio/any-sync/commonspace"
	commonnet "github.com/anytypeio/any-sync/net"
	"github.com/anytypeio/any-sync/nodeconf"
	"github.com/anytypeio/go-anytype-middleware/util/files"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"

	"github.com/anytypeio/any-sync/app"
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

type FileConfig interface {
	GetFileConfig() (ConfigRequired, error)
	WriteFileConfig(cfg ConfigRequired) (ConfigRequired, error)
}

type ConfigRequired struct {
	HostAddr        string `json:",omitempty"`
	IPFSStorageAddr string `json:",omitempty"`
	TimeZone        string `json:",omitempty"`
}

type Config struct {
	ConfigRequired           `json:",inline"`
	NewAccount               bool `ignored:"true"` // set to true if a new account is creating. This option controls whether mw should wait for the existing data to arrive before creating the new log
	Offline                  bool
	DisableThreadsSyncEvents bool

	RepoPath string

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
	FS                clientds.FSConfig
	DisableFileConfig bool `ignored:"true"` // set in order to skip reading/writing config from/to file
}

const (
	ConfigFileName = "config.json"
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

func DisableFileConfig(disable bool) func(*Config) {
	return func(c *Config) {
		c.DisableFileConfig = disable
	}
}

func New(options ...func(*Config)) *Config {
	cfg := DefaultConfig
	for _, opt := range options {
		opt(&cfg)
	}
	cfg.Threads.CafeP2PAddr = cfg.CafeP2PFullAddr()
	cfg.Threads.CafePID = cfg.CafePeerId

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
	c.RepoPath = repoPath

	if !c.DisableFileConfig {
		err := files.GetFileConfig(c.GetConfigPath(), &c.ConfigRequired)
		if err != nil {
			return fmt.Errorf("failed to get config from file: %s", err.Error())
		}

		saveRandomHostAddr := func() error {
			port, err := getRandomPort()
			if err != nil {
				port = 4006
				log.Errorf("failed to get random port for gateway, go with the default %d: %s", port, err.Error())
			}

			c.HostAddr = fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)

			err = files.WriteJsonConfig(c.GetConfigPath(), c.ConfigRequired)
			if err != nil {
				return fmt.Errorf("failed to save port to the cfg file: %s", err.Error())
			}
			return nil
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

	return nil
}

func (c *Config) Name() (name string) {
	return CName
}

func (c *Config) DSConfig() clientds.Config {
	return c.DS
}

func (c *Config) FSConfig() (clientds.FSConfig, error) {
	res := ConfigRequired{}
	err := files.GetFileConfig(c.GetConfigPath(), &res)
	if err != nil {
		return clientds.FSConfig{}, err
	}

	return clientds.FSConfig{IPFSStorageAddr: res.IPFSStorageAddr}, nil
}

func (c *Config) ThreadsConfig() threads.Config {
	return c.Threads
}

func (c *Config) GetConfigPath() string {
	return filepath.Join(c.RepoPath, ConfigFileName)
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

func (c *Config) GetSpace() commonspace.Config {
	return commonspace.Config{
		GCTTL:      60,
		SyncPeriod: 20,
	}
}

func (c *Config) GetNet() commonnet.Config {
	return commonnet.Config{
		Stream: commonnet.StreamConfig{
			TimeoutMilliseconds: 1000,
			MaxMsgSizeMb:        256,
		},
	}
}

func (c *Config) GetDebugNet() commonnet.Config {
	return commonnet.Config{
		Server: commonnet.ServerConfig{ListenAddrs: []string{"127.0.0.1:8090"}},
		Stream: commonnet.StreamConfig{
			TimeoutMilliseconds: 1000,
			MaxMsgSizeMb:        256,
		},
	}
}

func (c *Config) GetNodes() []nodeconf.NodeConfig {
	return []nodeconf.NodeConfig{
		{
			PeerId:        "12D3KooWKnXTtbveMDUFfeSqR5dt9a4JW66tZQXG7C7PdDh3vqGu",
			Addresses:     []string{"127.0.0.1:4430"},
			SigningKey:    "/Ou28/uU/z3BbGmkHMV5ev0mwl6lJI/NNniFlMm2gOeUHDfed/zbwYZLbPt1B0sujNx0DGKUgUTUXy/SE7biwg==",
			EncryptionKey: "MIIEpAIBAAKCAQEA23AWlsGaLrUxea+x6rkpy1ByqUJKdb2oS10q0urntUCivsNb7ipo1tvM2rldn6DAIrIC1nQHdlwrakNhl/j9zfX/GdACRDEuy7pVigm78QqYwSoyZn1l717HGjXKpJpa2m9Qyo6ZNGTLcej06zuOK8yUR4gudxSV5BJ2AmMvB9v4jUdCUwROSMy0e/hDa3Cx2xV33iULF/XltoeBVDoW6yYXfsok2yH3jHUV59yqQDbZO+kAj5rO8Fd/DnmGriuQZwAwdqJBH6/NtkhbA6triS/xW7f+RoyjjA3dp7TSUscHTFW0LIUaS0ZKt6amaanT7L/Jwi/0w133wTboYPG7NQIDAQABAoIBAGRYaGv+ElSDlSFRP6nXjI4ltplB8CzRUrFL5HZY5yZSbVmQmmxZxmFiV9Y5Z9/EMbhr8LJCktsEIPJR58IIIX/amxIhtbebSgvtpIogE2qRbvL+KdgGRePlUmTg7k7BKwSyXJ6UAOkdoo7veXhMXqIWxkPDuYPIzRZjfsVut6X+j0e9W2/kK7Tx1cn07GGyEX6T9sYsrDZud9xAgDWeE3ARga+M8l3RpTi3GiM6aaz82byjtn+uROUioUSDRLeOx4lOsmTzwWs4+tcyguqC4h/01Hejs9gDsJTOZ+xsrSc+eTsj1oOmABtHcuQ6NWq4KTqV9gVAB9BEMa39k3SrzaECgYEA9kd71vAwqFp6iu3INXBuwJouV72Egwm936KGcs9/NxfXQ7R6NdKa4LiWxdIn8e7uT6+bGPxCPuUmeJ9pYandb6j6fcBcGAjPOJPg5cTtoaeXSTOBNtzaoDLM191dNkhftXAgVoz6EQXUXaGXqnUEWDCptZ8/VKO4Jbb+Zco3F30CgYEA5BljE1KiWKP3IXwfndnPFKwev8Ds6YMczCFpiCzHB4LVLAWShUQrQIcA12yIRHTvD35Vg7SwH5mM+DFd9AMNxTznz8aC7R+jXEApeaCEQqBEKakoDFMsdblT6WFwerfl5ug32IyeKglfLuOiID5M740t+6TxUR1ZjnhIx3QdMBkCgYBUANn63H6cThBSZvzNTYZZZ72668fRMndzWmplquUHk7g3Pc4ZobZ2REAZRa+eVBMxVwKeKR0net3+ctFtIJWJSqf2ULCY+hhEghxKAzvS0elGbKz8W6Su0+UPFFCA/Xq31oERRJDfUY+4SDAFUlijBCY+7wyZACcFpj9r1OepuQKBgQDAJV6ffWHC98SLkYr0pvFZgbCZaYRpJQiSMKc8jjPO8PhwD/Wgi48/2TntPsD/od1sGMkinIgs5mWpAbUag6QK/cZs8dWCuL8dS/NkQMKJeYZR7ewNjdcLCGZWV72vstBZFk9M/Q+Ta7ehwSHmtXSL60rMC+M4qFezu0HbusWHqQKBgQCNuIsiB1yS1aByY2Hw2NJyAWFIRLuv4VNm+S/+YUJzOb/cuX5rwJ+PydJj0xD5mKKq5zGhCFzGZYdheKCYC4UQKn+Zz6Iv5T/mIJU7ELy9R2/I/Jv/IQMQwDXUokbNU8p25Hxul+rI+INjoHihUFKRNL68iuGVNjQHyKQYjit+mg==",
			Types: []nodeconf.NodeType{
				nodeconf.NodeTypeTree,
			},
		},
		{
			PeerId:        "12D3KooWKgVN2kW8xw5Uvm2sLUnkeUNQYAvcWvF58maTzev7FjPi",
			Addresses:     []string{"127.0.0.1:4431"},
			SigningKey:    "ckI1ThxMQmAWH48so5jT614HNg4VppB/3jTfh1cdrzSSkFuVPiGzSDJcsgFLsRGW2WT0a8ymqtc4hog5Z4mdfQ==",
			EncryptionKey: "MIIEpAIBAAKCAQEAwQcHwXkTynIvU5PfTn9kaQEb5Zzz8eHl/uO1srbWI0hM+sIvhOq/ahYuhDlai0mAhRFQIvb6QRnWzCp3qvMH8+j8ls83BKV4hfxuS+SR/Y/LonW+WeLljiloMM5ju9tjw1iSslUK6z7d0HSLppQCCjMP+v3kKjx24qJ2T+2N1MAMbdt+S5c2P2jBUJVSvQbZ1AVuZTEzDau//+cZzV47lsODrVZ2SxW/KAAKA9xIJSIwBFyIqya54FDlrsy4HMcThxUbmcqy0Ou3bRjPR2f9A6QaNDNlolPhW1Z4gs+MTQOXJyAj2vdKwNKPwr8X6wyUtN4v8MiJt2yl4h3EVSVkGQIDAQABAoIBAQCak4iwdMdWfa5MncRb1kSQmiS+8bug3igSwEOHREejpYiP3uWy6VI0IGNL26oYlNnotf6YoXOyooCSqwd9zHV37AIquvHyNJiZo5aoC/ilN3v5M5Ul4j+2Yo5fV0gi7gOsMcf4K4Y88PEst8gFs80WYeHQO3V2JUmHIFbilvfXgDfQNuKlHXgWq5kNNCxIobGpuP70Vvn1ANkDXLt+bU8RCbmBYFUZ/4HUeawf2NX+Xn5T1NFpheA68hAGprFQu6CuEsM34TtPZwH5SxyvOmA218nQbtq5fNLUgHZdq/uAeQyq7E8kxyOMEbx7m1UYtxDjwowBQgQd1dOtm3wTeQdpAoGBANjs7B0IW/B3TY6mRipcKtTYMVJ+M6AKu7etTkcmM6NpLGaIL9HqNP/a8PoNmHzVOBPJlC96ChQAvcULe+SKBL4L5g1T+k7OQvHEFMwzkFgs/HxjvQE9b1xX1Qyp+e9H7yvrcJhvRPbj3LLp7vunHL9dEm4/PFNKgCFExNsiv8WvAoGBAOPMGXtAethDYZE7vdlD55GdhsgklCmF3YQcraWsZrswj9paIBV8FWtVZIUDdaqOasPwT8L2NdajYWXXcmLlvxkOQQx7+cJG4SdLRwEz531rXvtcYrWPYYwjf37DQMi2qRmvZ0Oc/atitwN7mQ4wCMzZ+CV04HJTjK4eRfeDeyy3AoGAFbQWp4z3jeaR1uzh5kkUa/k5bhR8W83XHDh1tX6n+fiC3btQdYMmTFb+dzY3BH5cvvYTInDmYzvzwKw2eBYrBUyxdrHpQEs4vXGt1wRts7TEijl8ZoxcAPQ0t7Cl9f/PeSh0OnwffUgtA6WOKJV+tdK9DlS7V2YDzdBQldAzObcCgYAbjn0mo844iy4qW4fD2KsSunOrkoE55K+/Y5i+CfUDgARm7bAj6TbOHS5jyN9bGo9f1JpRg2dP58PIhh+YRyKu7UKBOB4mmlxyXHDifFzslyiOT8bBH+80/LZXp1cW8MHUEZv0WfF14iMxkKobRURLQ7L5FQJx0hmalp7wKj+kUQKBgQCEavx+ELQu1M22t4Kk1MOI1zEsrG4D+hjACmoWQsIjMqi5x+9vPJNshrO9vbYljjsIt4EOfqBB88oMepCesVHwbX3+/j7Yb48pmf2vZNcQ+SFozJqnCqYNQiWMtNXWT312NVdzkvpTlA0e1vaauwJjtetRnYDJUOPjc8BrdKsftw==",
			Types: []nodeconf.NodeType{
				nodeconf.NodeTypeTree,
			},
		},
		{
			PeerId:        "12D3KooWCUPYuMnQhu9yREJgQyjcz8zWY83rZGmDLwb9YR6QkbZX",
			Addresses:     []string{"127.0.0.1:4432"},
			SigningKey:    "EqD0bdvO2E9+i29hfJTMtae8Zw8Dgnb/KanLSg59K7YndQHdxxvtVUljy3htlXd7dGKfEuJ7EglVHdNxzg5q7g==",
			EncryptionKey: "MIIEpAIBAAKCAQEAuOwKfzHLrm94jH4PElKfkB9geQ5OhtdbcS4K3TeyZ5xU0hi0TiXxOiHQOCg2uO7B/fnQcAArUBvPNa4QOvNuWRY+246FHYBxCJvJdfOpwirQbQ2l5iqR13rpSAELnF/zB4XMQa6wGVhsgWipvY9PUrLF6RwpeeBu9OtpVaA2oUiUXyD+v1f4dNXjZqrTKS9IycksGEEI4knEfG421K862BRty94pVWfqv/ZgpJUQZiWLX9CSbAdvjoHxOdm96VhMjTBw3oyv7hTr/zQR9y77OMFwOOBtn4QWXSS54r89inoksgGjH5cProhR+V4QZ4962TGsnc92j+IjtlxI/o3CuQIDAQABAoIBACyLl5+6NBFqAsT9HM5SHuPN2yRuINZ0jC+AYteVMiGpU/lkQBLPKwPQ32KXtU7pHMv8YIyKTeS3Pjv1GS3KNBu7sxqag1Bu/0uOk4IZVxxRyfFrJzqBqK4aipVwwwZBSr7WKTTtSrhgR4sI1lK2ceo+7FPSF9+nA7N1/eLFfENvWegG9cM9G1162f1ypTmTKB6zvyEhrMnpw5IWGiyL41Pbn7Q6qMxJCMedJn40UWwZ+K04StxcX0MdPqiRC1mhEECF23y/Yu3QYVlyAR/Ya6POPK+Y/PQhvYuA8gMbdJgM/BpFntqqJx6nf9ojZjmkpFytHuKojDP+SOYSvifGSzECgYEA85YGa9DNHeEGU7e7U6iReHn0Cy1ZlDl4q/E1tyOSY1tpuTdiIYNLZAmx/ZDLj7y8NcNwK44CWhNGUlsTGOL0wrsFefH6tJOLLklTgwo6gnFFeNKUaTrZMQhu1Rf6fruO6yUHe34EgDnMIclx3+bXpZmWGVVOXLx7e/gQfDiRg3UCgYEAwlimHW2pJshQdCCyIFPvf36FK7xNiTIGy4VW8q8ulX4UESioxQtv8Irf5UCo1WpTSNA8RYRPU4gZnp0jkBydNDU2bdNGiN3IySU99/2dtYEHPJAferwXoOVdSCVXw2fGyQZK6GaciWDP3bgZdVKBSGBCN30eJ2PDVKni2NiHbbUCgYAdPHKE2kjkPy/9OF45ik/7f9e0x7qqucMsEAV8d76IQl6MJoOWtiWEWk2Mu6ZTGDoW0eBSufa6TPnxxJCkOglangvoOQz4Q4U/BvoJDl87bNED0XKStsd+xR5YYUplj6l1u7oMLnHn2ggQPhd24kQb0jVb0QtYwh6oIHwKDNgaSQKBgQCuTYeeuS2ORPYzUOexKtaQSE7z7My1kZKakhprSkbDePJSeV70as+Ys1UfbaB+1/+ePHTx/DqRNm2T3md45tDvdBI+6dBHDHL6RFaRxnrdwL1WygQRtgSTH2NMQ4G1Fawpu2UPjogyhguoVWcv3DFrUjnRPnv+4/DaTAvSZFECSQKBgQDh6g0n+MIknDo89ffCmIuD5qkPi7GYYGBfQ7HHHq5dUXazN6+vKazgGEJo72MU4ZlHl/U33MoMuuVP2cBEYJx8iMaP7ze8jUWVRA7rzg4A2ayBCOQxpBehEBhD1yEBPKO5PVTYV0a3BiM4tQLLPuoh5QcB7ANwmZkFdLpizLqn2A==",
			Types: []nodeconf.NodeType{
				nodeconf.NodeTypeTree,
			},
		},
		{
			PeerId:    "12D3KooWQxiZ5a7vcy4DTJa8Gy1eVUmwb5ojN4SrJC9Rjxzigw6C",
			Addresses: []string{"127.0.0.1:4730"},
			Types: []nodeconf.NodeType{
				nodeconf.NodeTypeFile,
			},
		},
	}
}
