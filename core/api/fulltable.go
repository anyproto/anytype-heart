package api

// fulltable.go — the full tool table, derived once per process from a
// constant of the binary, the embedded v2 OpenAPI document. Only the table
// and its error are kept — clients, credentials and the engine stay per
// request (spec §2.4).

import (
	"fmt"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/api/wrapper/full"
)

var fullTable = full.NewLazy(deriveFullTable)

// FullTable returns the full tool table, derived on first use. A failure
// is retained: the inputs are constants, so a second attempt would fail
// the same way, and the tests over the embedded document are what keep
// this from ever failing in a shipped build.
func FullTable() (*full.Table, error) {
	return fullTable.Get()
}

func deriveFullTable() (*full.Table, error) {
	members, err := fullOpMembers()
	if err != nil {
		return nil, fmt.Errorf("derive full tool table: %w", err)
	}
	table, err := full.Derive(full.Inputs{OpenAPI: openapiV2JSON, OpMembers: members})
	if err != nil {
		return nil, fmt.Errorf("derive full tool table: %w", err)
	}
	return table, nil
}

// fullOpMembers is each served op's members, for the op lists the edit
// tools' descriptions carry.
func fullOpMembers() (map[string][]string, error) {
	served, err := v2service.ServedOpSchemas()
	if err != nil {
		return nil, fmt.Errorf("served op schemas: %w", err)
	}
	members := make(map[string][]string, len(served))
	for name, op := range served {
		m, err := full.OpMembers(op.Schema)
		if err != nil {
			return nil, fmt.Errorf("op %q: %w", name, err)
		}
		members[name] = m
	}
	return members, nil
}
