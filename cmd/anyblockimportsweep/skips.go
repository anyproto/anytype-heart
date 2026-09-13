package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The export sweep writes this beside native/, outside the bundle payloads.
// Explicit evidence is required: an empty folder alone is not a deleted space.
func readSkippedSpaces(root string) (map[string]string, error) {
	path := filepath.Join(root, "skipped-spaces.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Version int `json:"version"`
		Spaces  []struct {
			ID     string `json:"spaceId"`
			Reason string `json:"reason"`
		} `json:"spaces"`
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("unsupported skip manifest version %d in %s", manifest.Version, path)
	}
	out := map[string]string{}
	for _, space := range manifest.Spaces {
		if space.ID == "" || space.Reason == "" {
			return nil, fmt.Errorf("skip manifest %s has an empty space ID or reason", path)
		}
		out[space.ID] = space.Reason
	}
	return out, nil
}
