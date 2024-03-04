package network

import (
	"context"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/anytype"
)

type Stat struct {
	SuccessQuicProbes  int
	SuccessYamuxProbes int
	SpentYamux         time.Duration
	SpentQuic          time.Duration
}

func PerformDebug(ctx context.Context, requestIterations int) (Stat, error) {
	cfg := anytype.BootstrapConfig(true, false)

	var stat Stat
	nodeCfg, err := cfg.GetNodeConfWithError()
	if err != nil {
		return stat, err
	}
	a := new(app.App)
	bootstrap(cfg, a)
	if err := a.Start(ctx); err != nil {
		panic(err)
	}

	var errs multierror.Error
	for _, node := range nodeCfg.Nodes {
		if !node.HasType(nodeconf.NodeTypeCoordinator) {
			// we can only test coordinator node for now
			continue
		}
		for _, addr := range node.Addresses {
			addr = strings.TrimSpace(addr)
			if strings.HasPrefix(addr, "quic://") {
				start := time.Now()
				err = probeQuic(ctx, a, requestIterations, addr[7:])
				if err != nil {
					errs.Errors = append(errs.Errors, err)
				} else {
					stat.SuccessQuicProbes++
				}
				stat.SpentQuic += time.Since(start)
			} else if !strings.Contains(addr, "://") {
				// yamux by default
				start := time.Now()
				err = probeYamux(ctx, a, requestIterations, addr)
				if err != nil {
					errs.Errors = append(errs.Errors, err)
				} else {
					stat.SuccessYamuxProbes++
				}
				stat.SpentYamux += time.Since(start)
			}
		}
	}

	if errs.Len() == 0 {
		return stat, nil
	}
	return stat, &errs
}
