package api

// fulltable.go — the full tool table's inputs, wired once per process. The
// table is derived from two constants of the binary: the embedded v2
// OpenAPI document and the op schemas the v2 service serves from code. Both
// are immutable, so the derivation runs at most once, lazily, and only the
// table and its error live behind the Once — clients, credentials and the
// engine stay per request (spec §2.4).

import (
	"fmt"
	"sync"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/api/wrapper/full"
)

var (
	fullTableOnce sync.Once
	fullTable     *full.Table
	fullTableErr  error
)

// FullTable returns the full tool table, derived on first use. A failure
// is retained: the inputs are constants, so a second attempt would fail
// the same way, and the tests over the embedded document are what keep
// this from ever failing in a shipped build.
func FullTable() (*full.Table, error) {
	fullTableOnce.Do(func() {
		fullTable, fullTableErr = deriveFullTable()
	})
	return fullTable, fullTableErr
}

func deriveFullTable() (*full.Table, error) {
	served, err := v2service.ServedOpSchemas()
	if err != nil {
		return nil, fmt.Errorf("served op schemas: %w", err)
	}
	ops := make(map[string]full.OpSchema, len(served))
	for op, s := range served {
		ops[op] = full.OpSchema{Schema: s.Schema, Example: s.Example, Channels: s.Channels}
	}
	table, err := full.Derive(full.Inputs{OpenAPI: openapiV2JSON, Ops: ops})
	if err != nil {
		return nil, fmt.Errorf("derive full tool table: %w", err)
	}
	return table, nil
}
